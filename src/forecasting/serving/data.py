"""Read-only access to the cco_data feature store.

The Python service owns a dedicated read-only connection to cco_data,
configured by FORECAST_DATA_DSN. It never connects to cco_app and never
receives SQL, DSNs, internal resource ids or table names from the caller.

The public resource_uid is resolved to the internal resource_id through a
join with core.resource; that key never leaves the service.
"""

from __future__ import annotations

from contextlib import contextmanager

import pandas as pd

from forecasting.db import connect, read_frame

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
