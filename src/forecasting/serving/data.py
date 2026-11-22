"""Access to the cco_data feature store and ML forecast persistence.

The Python service owns a dedicated connection to cco_data (NOT cco_app),
configured by FORECAST_DATA_DSN. It never connects to cco_app and never
receives SQL, DSNs, internal resource ids or table names from the caller.

The public resource_uid is resolved to the internal resource_id through a
join with core.resource; that key never leaves the service.
"""

from __future__ import annotations

from contextlib import contextmanager
from typing import Any
from uuid import UUID, uuid4

import pandas as pd

from forecasting.db import connect, query, read_frame

# Columns read from feat.feature_snapshot. Observed features only: no
# target_* columns (no future labels) are ever selected.
FEATURE_SNAPSHOT_COLUMNS = (
    "period_start_utc",
    "period_end_utc",
    "cpu_avg",
    "cpu_max",
    "data_io_avg",
    "log_write_avg",
    "sessions_count",
    "sessions_percent",
    "memory_percent",
    "workers_avg",
    "storage_used_pct",
    "storage_used_bytes",
    "storage_growth_bytes",
    "deadlock_count",
    "latency_p95_ms",
    "workload_intensity",
    "is_peak_hour",
    "day_of_week",
    "hour_of_day",
)

# Minimum contiguous hourly history required before the last observed window
# (covers lag_3h and rolling_6h built by the feature pipeline).
REQUIRED_HISTORY_HOURS = 6


class FeatureStoreError(RuntimeError):
    """Raised when the feature store cannot be queried."""


class FeatureStoreReader:
    """Read-only reader for observed hourly features of one resource."""

    def __init__(self, dsn: str, timeout_seconds: int = 10) -> None:
        if not dsn:
            raise FeatureStoreError("FORECAST_DATA_DSN is not configured")
        self.dsn = dsn
        self.timeout_seconds = timeout_seconds

    @contextmanager
    def connect(self):
        """Open a read-only connection that is always closed on exit."""
        conn = connect(self.dsn, read_only=True, autocommit=True)
        try:
            with conn.cursor() as cur:
                cur.execute("set default_transaction_read_only = on")
                cur.execute("set statement_timeout = %s", (f"{self.timeout_seconds}s",))
            yield conn
        finally:
            conn.close()

    def resource_exists(self, resource_uid: str) -> bool:
        """Return True when the public uid resolves to a registered resource."""
        with self.connect() as conn:
            rows = read_frame(
                conn,
                """
                select 1 as ok
                from core.resource
                where resource_uid = %s
                limit 1
                """,
                (resource_uid,),
            )
        return not rows.empty

    def fetch_hourly_features(self, resource_uid: str) -> pd.DataFrame:
        """Return the observed hourly feature rows for resource_uid.

        Rows are ordered by period_start_utc ascending. The internal resource_id
        is resolved by joining core.resource; the caller never sees it.
        """
        columns = ", ".join(f"f.{c}" for c in FEATURE_SNAPSHOT_COLUMNS)
        with self.connect() as conn:
            frame = read_frame(
                conn,
                f"""
                select {columns}
                from feat.feature_snapshot f
                join core.resource r on r.resource_id = f.resource_id
                where r.resource_uid = %s
                  and f.period_type = '1h'
                  and f.period_start_utc is not null
                order by f.period_start_utc
                """,
                (resource_uid,),
            )
        if frame.empty:
            return frame
        frame["period_start_utc"] = pd.to_datetime(frame["period_start_utc"], utc=True)
        return frame.reset_index(drop=True)


# Target names served by the Python artifact -> ml.forecast target_metric.
TARGET_TO_METRIC = {
    "target_cpu_avg_next1h": "cpu_avg",
    "target_workload_next1h": "workload_intensity",
    "target_storage_next1h": "storage_used_bytes",
}

# horizon_minutes -> (horizon_steps, step_unit) for ml.forecast.
HORIZON_TO_STEP = {
    60: (1, "1h"),
}


class ForecastWriter:
    """Write forecasts to ml.forecast in cco_data.

    Writes are idempotent: ml.forecast has a unique constraint on
    (model_version_id, resource_id, target_metric, horizon_steps, step_unit,
    period_start_utc). Re-predicting the same window returns the existing
    forecast_uid instead of duplicating rows.
    """

    def __init__(self, dsn: str, timeout_seconds: int = 10) -> None:
        if not dsn:
            raise FeatureStoreError("FORECAST_DATA_DSN is not configured")
        self.dsn = dsn
        self.timeout_seconds = timeout_seconds

    @contextmanager
    def connect(self):
        """Open a read-write connection that is always closed on exit."""
        conn = connect(self.dsn, read_only=False, autocommit=False)
        try:
            with conn.cursor() as cur:
                cur.execute("set statement_timeout = %s", (f"{self.timeout_seconds}s",))
            yield conn
        except Exception:
            conn.rollback()
            raise
        finally:
            conn.close()

    def resolve_resource_id(self, resource_uid: str) -> int:
        """Resolve public resource_uid to internal resource_id."""
        with self.connect() as conn:
            rows = query(
                conn,
                """
                select resource_id
                from core.resource
                where resource_uid = %s
                limit 1
                """,
                (resource_uid,),
            )
        if not rows:
            raise FeatureStoreError(f"resource not found: {resource_uid}")
        return int(rows[0]["resource_id"])

    def resolve_model_version_id(
        self, model_name: str, model_version: str
    ) -> int:
        """Resolve model_name/version to ml.model_version.model_version_id.

        Creates a minimal candidate registry row when the artifact has not
        been registered through training yet (Slice 2 bridge; Slice 3
        replaces this with proper training-run registration).
        """
        with self.connect() as conn:
            rows = query(
                conn,
                """
                select model_version_id
                from ml.model_version
                where model_name = %s and version = %s
                limit 1
                """,
                (model_name, model_version),
            )
            if rows:
                return int(rows[0]["model_version_id"])

            # Minimal registry row so the forecast can reference a model.
            # stage='candidate' signals it has not gone through training yet.
            cur = conn.cursor()
            cur.execute(
                """
                insert into ml.model_version (model_name, version, framework, stage, params, metrics_summary)
                values (%s, %s, 'serving_artifact', 'candidate', '{}'::jsonb, '{}'::jsonb)
                returning model_version_id
                """,
                (model_name, model_version),
            )
            row = cur.fetchone()
            model_version_id = int(row[0])
            conn.commit()
            return model_version_id

    def persist_forecast(
        self,
        *,
        resource_uid: str,
        target: str,
        model_name: str,
        model_version: str,
        horizon_minutes: int,
        period_start_utc,
        y_pred: float,
        y_lower: float | None,
        y_upper: float | None,
        alpha: float,
        dataset_hash: str | None = None,
    ) -> UUID:
        """Insert (or return existing) forecast row and return forecast_uid.

        Idempotent: ml.forecast has a unique constraint on
        (model_version_id, resource_id, target_metric, horizon_steps, step_unit,
        period_start_utc). Re-predicting the same window returns the existing
        forecast_uid instead of duplicating rows.
        """
        resource_id = self.resolve_resource_id(resource_uid)
        model_version_id = self.resolve_model_version_id(model_name, model_version)
        target_metric = TARGET_TO_METRIC.get(target, target)
        horizon_steps, step_unit = HORIZON_TO_STEP.get(horizon_minutes, (horizon_minutes, "1m"))

        forecast_uid = uuid4()

        with self.connect() as conn:
            with conn.cursor() as cur:
                cur.execute(
                    """
                    insert into ml.forecast (
                        forecast_uid, model_version_id, resource_id, target_metric,
                        horizon_steps, step_unit, period_start_utc, y_pred, y_lower,
                        y_upper, alpha, generated_at
                    )
                    values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, now())
                    on conflict (model_version_id, resource_id, target_metric,
                                horizon_steps, step_unit, period_start_utc)
                    do nothing
                    returning forecast_uid
                    """,
                    (
                        str(forecast_uid),
                        model_version_id,
                        resource_id,
                        target_metric,
                        horizon_steps,
                        step_unit,
                        period_start_utc,
                        y_pred,
                        y_lower,
                        y_upper,
                        alpha,
                    ),
                )
                row = cur.fetchone()
                if row is None:
                    # Conflict: an existing row holds the same logical key.
                    # Fetch its forecast_uid instead of inventing a new one.
                    cur.execute(
                        """
                        select forecast_uid
                        from ml.forecast
                        where model_version_id = %s and resource_id = %s
                          and target_metric = %s and horizon_steps = %s
                          and step_unit = %s and period_start_utc = %s
                        limit 1
                        """,
                        (
                            model_version_id,
                            resource_id,
                            target_metric,
                            horizon_steps,
                            step_unit,
                            period_start_utc,
                        ),
                    )
                    existing = cur.fetchone()
                    if existing:
                        forecast_uid = UUID(str(existing[0]))
            conn.commit()

        return forecast_uid
