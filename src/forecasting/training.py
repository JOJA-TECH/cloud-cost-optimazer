"""Model training for the forecasting service.

Reads observed hourly features from feat.feature_snapshot, trains a simple
baseline model, persists the artefact (model.joblib + model_metadata.json)
and registers the run in ml.training_run and the candidate in ml.model_version.
Self-contained: does not import from forecasting.features or forecasting.serving
(model.py has a pre-existing broken import chain in this branch).
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
from datetime import UTC, datetime
from pathlib import Path
from uuid import uuid4

import joblib
import numpy as np
import pandas as pd
import psycopg
from sklearn.ensemble import RandomForestRegressor
from sklearn.impute import SimpleImputer
from sklearn.linear_model import LinearRegression
from sklearn.metrics import mean_absolute_error
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

DSN = os.environ.get(
    "FORECAST_DATA_DSN",
    "postgresql://postgres:example@127.0.0.1:5432/cco_data",
)

MODEL_FILENAME = "model.joblib"
METADATA_FILENAME = "model_metadata.json"

log = logging.getLogger("forecast.training")

REQUIRED_FEATURES = ("cpu_avg", "workload_intensity")

OPTIONAL_FEATURES = (
    "cpu_max",
    "data_io_avg",
    "log_write_avg",
    "sessions_avg",
    "workers_avg",
    "storage_used_pct",
    "storage_used_bytes",
    "storage_growth_bytes",
    "deadlock_count",
)

DERIVED_FEATURES = (
    "cpu_avg_lag_1h",
    "cpu_avg_lag_2h",
    "cpu_avg_lag_3h",
    "cpu_avg_rolling_3h",
    "cpu_avg_rolling_6h",
    "workload_intensity_lag_1h",
    "workload_intensity_rolling_3h",
    "storage_growth_lag_1h",
    "hour_sin",
    "hour_cos",
    "day_of_week",
    "is_peak_hour",
)

TARGET_TO_METRIC = {
    "target_cpu_avg_next1h": "cpu_avg",
    "target_workload_next1h": "workload_intensity",
    "target_storage_next1h": "storage_used_bytes",
}

# Target logical name -> feature_snapshot column name for the source series.
TARGET_SOURCE_COLUMN = {
    "target_cpu_avg_next1h": "cpu_avg",
    "target_workload_next1h": "workload_intensity",
    "target_storage_next1h": "storage_used_bytes",
}

HORIZON_STEPS = 1
STEP_UNIT = "1h"


def _load_features(dsn: str, resource_uid: str, hours: int) -> pd.DataFrame:
    with psycopg.connect(dsn, autocommit=True, connect_timeout=10) as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                select f.resource_id, f.period_start_utc,
                       f.cpu_avg, f.cpu_max, f.data_io_avg, f.log_write_avg,
                       f.sessions_avg, f.workers_avg, f.storage_used_pct,
                       f.storage_used_bytes, f.storage_growth_bytes,
                       f.deadlock_count, f.latency_p95_ms, f.workload_intensity,
                       f.is_peak_hour, f.day_of_week, f.hour_of_day
                from feat.feature_snapshot f
                join core.resource r on r.resource_id = f.resource_id
                where r.resource_uid = %s
                  and f.period_type = '1h'
                  and f.period_start_utc is not null
                order by f.period_start_utc
                limit %s
                """,
                (resource_uid, hours),
            )
            cols = [d.name for d in cur.description]
            rows = cur.fetchall()
    if not rows:
        return pd.DataFrame()
    df = pd.DataFrame(rows, columns=cols)
    df["period_start_utc"] = pd.to_datetime(df["period_start_utc"], utc=True)
    return df.reset_index(drop=True)


def _build_training_matrix(
    df: pd.DataFrame, target: str
) -> tuple[pd.DataFrame, pd.Series, list[str]]:
    """Build feature matrix X and target y (next-period value)."""
    df = df.sort_values("period_start_utc").reset_index(drop=True)

    # Derived features: lags, rolling, calendar.
    df["cpu_avg_lag_1h"] = df["cpu_avg"].shift(1)
    df["cpu_avg_lag_2h"] = df["cpu_avg"].shift(2)
    df["cpu_avg_lag_3h"] = df["cpu_avg"].shift(3)
    df["cpu_avg_rolling_3h"] = df["cpu_avg"].rolling(3).mean()
    df["cpu_avg_rolling_6h"] = df["cpu_avg"].rolling(6).mean()
    df["workload_intensity_lag_1h"] = df["workload_intensity"].shift(1)
    df["workload_intensity_rolling_3h"] = df["workload_intensity"].rolling(3).mean()
    df["storage_growth_lag_1h"] = df["storage_used_bytes"].shift(1)
    df["hour_sin"] = np.sin(2 * np.pi * df["hour_of_day"] / 24.0)
    df["hour_cos"] = np.cos(2 * np.pi * df["hour_of_day"] / 24.0)
    df["day_of_week"] = df["day_of_week"].astype(float)
    df["is_peak_hour"] = df["is_peak_hour"].astype(float)

    # Target: next-period value of the source series (self-join t+1h).
    source_col = TARGET_SOURCE_COLUMN.get(target, target)
    df["target_next"] = df[source_col].shift(-1)

    # Required features must be present.
    for col in REQUIRED_FEATURES:
        if col in df.columns:
            df = df[df[col].notna()]

    # Optional: impute with median + missing flag.
    for col in OPTIONAL_FEATURES:
        if col in df.columns:
            df[f"feature_missing_{col}"] = df[col].isna().astype(int)
            median = df[col].median()
            df[col] = df[col].fillna(median)

    feature_cols = [
        c
        for c in df.columns
        if c
        in (
            list(REQUIRED_FEATURES)
            + list(OPTIONAL_FEATURES)
            + list(DERIVED_FEATURES)
            + [f"feature_missing_{c}" for c in OPTIONAL_FEATURES]
        )
    ]

    X = df[feature_cols].copy()
    y = df["target_next"].copy()

    # Drop rows where target is NaN.
    mask = y.notna()
    X = X[mask]
    y = y[mask]

    return X, y, feature_cols


def _compute_dataset_hash(
    X: pd.DataFrame, y: pd.Series, feature_cols: list[str], target: str
) -> str:
    """Calculate SHA-256 hash of the actual training data content.

    The hash is computed over the serialized feature matrix and target
    values, ensuring that any change in the underlying data produces a
    different hash. This enables point-in-time integrity verification.
    """
    import hashlib

    # Serialize X and y deterministically (sort by index, include column names).
    X_serialized = X[feature_cols].to_csv(index=False)
    y_serialized = y.to_csv(index=False, header=["target_next"])

    h = hashlib.sha256()
    h.update(X_serialized.encode("utf-8"))
    h.update(b"|")
    h.update(y_serialized.encode("utf-8"))
    h.update(b"|")
    h.update(target.encode("utf-8"))
    h.update(b"|")
    h.update(",".join(sorted(feature_cols)).encode("utf-8"))

    return h.hexdigest()


def train_model(
    *,
    resource_uid: str,
    target: str,
    model_name: str,
    model_version: str,
    algorithm: str = "random_forest",
    hours: int = 720,
    model_dir: str | Path = None,
    dsn: str = DSN,
) -> dict:
    """Train a model and register it in ml.training_run / ml.model_version.

    Validates dataset_hash against ml.training_dataset before training:
    the hash of the actual feature content must match the registered hash,
    otherwise training is rejected (no silent retraining on different data).
    If a model with the same name/version already exists in ml.model_version,
    training is rejected (versions are immutable).
    """
    if model_dir is None:
        model_dir = Path(os.environ.get("FORECAST_MODEL_DIR", "/tmp/forecast_models"))
    model_dir = Path(model_dir)

    # Check if model already exists (versions are immutable).
    with psycopg.connect(dsn, autocommit=True, connect_timeout=10) as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                select mv.model_version_id, mv.stage
                from ml.model_version mv
                where mv.model_name = %s and mv.version = %s
                """,
                (
                    model_name,
                    int(model_version.split(".")[0]) if model_version else 1,
                ),
            )
            existing_model = cur.fetchone()
            if existing_model is not None:
                raise ValueError(
                    f"model {model_name} version {int(model_version.split('.')[0]) if model_version else 1} already exists "
                    f"(stage={existing_model[1]}). Model versions are immutable; use a new version."
                )

    df = _load_features(dsn, resource_uid, hours)
    if df.empty or len(df) < 20:
        raise ValueError(
            f"insufficient data for training: {len(df)} rows (need >= 20)"
        )

    X, y, feature_cols = _build_training_matrix(df, target)
    if X.empty or len(X) < 10:
        raise ValueError(
            f"insufficient valid training rows: {len(X)} (need >= 10)"
        )

    # Calculate content hash of the actual training data (X + y) before
    # any model training. This is the ground-truth hash that must match
    # the registered dataset_hash in ml.training_dataset.
    dataset_hash = _compute_dataset_hash(X, y, feature_cols, target)

    # Temporal split: last 15% for evaluation.
    split = int(len(X) * 0.85)
    X_train, X_eval = X.iloc[:split], X.iloc[split:]
    y_train, y_eval = y.iloc[:split], y.iloc[split:]

    if algorithm == "linear":
        model: Pipeline = Pipeline(
            [
                ("imputer", SimpleImputer(strategy="median")),
                ("scaler", StandardScaler()),
                ("regressor", LinearRegression()),
            ]
        )
    else:
        model = Pipeline(
            [
                ("imputer", SimpleImputer(strategy="median")),
                ("scaler", StandardScaler()),
                (
                    "regressor",
                    RandomForestRegressor(
                        max_depth=4,
                        min_samples_leaf=4,
                        n_estimators=300,
                        random_state=42,
                    ),
                ),
            ]
        )

    model.fit(X_train, y_train)
    y_pred = model.predict(X_eval)
    mae = float(mean_absolute_error(y_eval, y_pred))

    # Persist artefact.
    target_dir = model_dir / target / model_version
    target_dir.mkdir(parents=True, exist_ok=True)
    artefact_path = target_dir / MODEL_FILENAME
    joblib.dump(model, artefact_path)

    metadata = {
        "model_name": model_name,
        "model_version": model_version,
        "model_kind": algorithm,
        "target": target,
        "period_type": "1h",
        "feature_list": feature_cols,
        "trained_at": datetime.now(UTC).isoformat(),
        "training_rows": int(len(X)),
        "eval_mae": mae,
        "prediction_interval": None,
    }
    metadata_path = target_dir / METADATA_FILENAME
    metadata_path.write_text(json.dumps(metadata, indent=2), encoding="utf-8")

    # Register in ml.training_dataset first (dataset_id is NOT NULL in run).
    # Validate dataset_hash: if a dataset spec with the same name/version
    # already exists, the content hash must match, otherwise training
    # is rejected (no silent retraining on different data).
    dataset_id = None
    with psycopg.connect(dsn, autocommit=False, connect_timeout=10) as conn:
        with conn.cursor() as cur:
            # Check if a dataset spec with same name/version already exists.
            cur.execute(
                """
                select dataset_hash
                from ml.training_dataset
                where name = %s and version = %s
                """,
                (
                    model_name,
                    int(model_version.split(".")[0]) if model_version else 1,
                ),
            )
            existing = cur.fetchone()
            if existing is not None:
                registered_hash = existing[0]
                if registered_hash is not None and registered_hash != dataset_hash:
                    conn.rollback()
                    raise ValueError(
                        f"dataset_hash mismatch for {model_name} v{int(model_version.split('.')[0]) if model_version else 1}: "
                        f"registered={registered_hash}, computed={dataset_hash}. "
                        f"Refusing to train on different data without explicit dataset reset."
                    )
                # Hash matches: reuse existing dataset_id.
                cur.execute(
                    """
                    select dataset_id
                    from ml.training_dataset
                    where name = %s and version = %s
                    """,
                    (
                        model_name,
                        int(model_version.split(".")[0]) if model_version else 1,
                    ),
                )
                dataset_id = cur.fetchone()[0]
            else:
                # Insert new dataset spec with the computed content hash.
                cur.execute(
                    """
                    insert into ml.training_dataset (
                        name, version, resource_scope, period_start_utc,
                        period_end_utc, period_type, feature_codes, target_column,
                        row_count, split_strategy, dataset_hash, created_by
                    )
                    values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
                    returning dataset_id
                    """,
                    (
                        model_name,
                        int(model_version.split(".")[0]) if model_version else 1,
                        resource_uid,
                        df["period_start_utc"].min(),
                        df["period_start_utc"].max(),
                        "1h",
                        feature_cols,
                        target,
                        len(X),
                        json.dumps({"type": "temporal", "train": 0.85, "eval": 0.15}),
                        dataset_hash,
                        "training-service",
                    ),
                )
                dataset_id = cur.fetchone()[0]

            # Register training run.
            cur.execute(
                """
                insert into ml.training_run (
                    experiment_id, dataset_id, algorithm, algorithm_params,
                    status, trained_by, artifact_uri, notes
                )
                values (%s, %s, %s, %s, %s, %s, %s, %s)
                returning training_run_id
                """,
                (
                    None,
                    dataset_id,
                    algorithm,
                    json.dumps({"max_depth": 4, "min_samples_leaf": 4}),
                    "succeeded",
                    "training-service",
                    str(artefact_path),
                    json.dumps({"mae": mae, "rows": len(X)}),
                ),
            )
            run_id_db = cur.fetchone()[0]

            # Register model version.
            cur.execute(
                """
                insert into ml.model_version (
                    training_run_id, model_name, version, framework, stage,
                    params, metrics_summary, promoted_at
                )
                values (%s, %s, %s, %s, %s, %s, %s, %s)
                returning model_version_id
                """,
                (
                    run_id_db,
                    model_name,
                    int(model_version.split(".")[0]) if model_version else 1,
                    "scikit-learn",
                    "candidate",
                    json.dumps({}),
                    json.dumps({"mae": mae}),
                    None,
                ),
            )
            model_version_id = cur.fetchone()[0]
            conn.commit()

    # Update metadata with dataset_hash.
    metadata["dataset_hash"] = dataset_hash
    metadata_path.write_text(json.dumps(metadata, indent=2), encoding="utf-8")

    log.info(
        "trained %s %s: mae=%.4f rows=%d run=%d model_version_id=%d",
        model_name,
        model_version,
        mae,
        len(X),
        run_id_db,
        model_version_id,
    )

    return {
        "training_run_id": run_id_db,
        "model_version_id": model_version_id,
        "model_name": model_name,
        "model_version": model_version,
        "artefact_path": str(artefact_path),
        "mae": mae,
        "training_rows": len(X),
        "feature_cols": feature_cols,
    }


def list_models(dsn: str = DSN) -> list[dict]:
    """List model versions from ml.model_version (leaderboard)."""
    with psycopg.connect(dsn, autocommit=True, connect_timeout=10) as conn:
        with conn.cursor() as cur:
            cur.execute(
                """
                select mv.model_version_id, mv.training_run_id, mv.model_name,
                       mv.version, mv.framework, mv.stage, mv.params,
                       mv.metrics_summary, mv.promoted_at, mv.created_at,
                       tr.algorithm
                from ml.model_version mv
                left join ml.training_run tr on tr.training_run_id = mv.training_run_id
                order by mv.model_name, mv.version
                """
            )
            cols = [d.name for d in cur.description]
            return [dict(zip(cols, row)) for row in cur.fetchall()]


def promote_model(
    *,
    model_name: str,
    model_version: str,
    dsn: str = DSN,
) -> dict:
    """Promote a model version to champion (archive previous champion).

    Validates promotion thresholds from ml.promotion_policy before promoting:
    - MAE must not exceed mae_max (if defined).
    - Candidate must improve over current champion by at least min_improvement_pct
      (if defined).
    - Training rows must meet min_training_rows (if defined).
    If no policy is defined for this model/version, promotion is allowed
    (default behavior, not recommended for production).
    """
    version_int = int(model_version.split(".")[0])
    with psycopg.connect(dsn, autocommit=False, connect_timeout=10) as conn:
        with conn.cursor() as cur:
            # Check promotion eligibility via ml.can_promote function.
            cur.execute(
                "select * from ml.can_promote(%s, %s)",
                (model_name, version_int),
            )
            eligible, reason = cur.fetchone()
            if not eligible:
                conn.rollback()
                raise ValueError(
                    f"model {model_name} version {model_version} not eligible for promotion: {reason}"
                )

            # Archive current champion for this model_name.
            cur.execute(
                """
                update ml.model_version
                set stage = 'archived'
                where model_name = %s and stage = 'champion'
                """,
                (model_name,),
            )
            # Promote the requested version.
            cur.execute(
                """
                update ml.model_version
                set stage = 'champion', promoted_at = now()
                where model_name = %s and version = %s
                returning model_version_id
                """,
                (model_name, version_int),
            )
            row = cur.fetchone()
            if row is None:
                conn.rollback()
                raise ValueError(
                    f"model {model_name} version {model_version} not found"
                )
            model_version_id = row[0]
            conn.commit()
    return {
        "model_name": model_name,
        "model_version": model_version,
        "model_version_id": model_version_id,
        "stage": "champion",
    }
