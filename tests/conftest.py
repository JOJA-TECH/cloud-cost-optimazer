# Pytest fixtures for the forecasting gRPC serving tests.

from __future__ import annotations

import joblib
import numpy as np
import pandas as pd

from forecasting.features import FeatureSpec, feature_definition_hash
from forecasting.models import ModelSpec, fit_model
from forecasting.serving.model import ModelArtifact
from forecasting.transforms import DifferencedPredictor

TARGET = "target_cpu_avg_next1h"

# A stable, small feature list matching the default FeatureSpec derivation.
FEATURE_LIST = [
    "cpu_avg",
    "workload_intensity",
    "cpu_avg_lag_1h",
    "cpu_avg_lag_2h",
    "cpu_avg_lag_3h",
    "cpu_avg_rolling_3h",
    "cpu_avg_rolling_6h",
    "workload_intensity_lag_1h",
    "workload_intensity_rolling_3h",
    "storage_growth_bytes_lag_1h",
    "hour_sin",
    "hour_cos",
    "day_of_week",
    "is_peak_hour",
]


def make_training_frame(n_rows: int = 40, seed: int = 42) -> pd.DataFrame:
    rng = np.random.default_rng(seed)
    start = pd.Timestamp("2026-09-01", tz="UTC")
    periods = pd.date_range(start, periods=n_rows, freq="h", tz="UTC")
    cpu = 30 + 10 * np.sin(np.linspace(0, 3 * np.pi, n_rows)) + rng.normal(0, 2, n_rows)
    workload = (
        100 + 50 * np.cos(np.linspace(0, 2 * np.pi, n_rows)) + rng.normal(0, 5, n_rows)
    )
    df = pd.DataFrame(
        {
            "period_start_utc": periods,
            "cpu_avg": cpu,
            "workload_intensity": workload,
            "data_io_avg": rng.uniform(0, 50, n_rows),
            "log_write_avg": rng.uniform(0, 30, n_rows),
            "sessions_count": rng.uniform(10, 100, n_rows),
            "sessions_percent": rng.uniform(0, 80, n_rows),
            "memory_percent": rng.uniform(20, 90, n_rows),
            "workers_avg": rng.uniform(5, 50, n_rows),
            "storage_used_pct": rng.uniform(40, 90, n_rows),
            "storage_used_bytes": rng.integers(1000, 9000, n_rows),
            "storage_growth_bytes": rng.normal(0, 100, n_rows),
            "deadlock_count": rng.integers(0, 3, n_rows),
            "latency_p95_ms": rng.uniform(5, 50, n_rows),
            "is_peak_hour": ((periods.hour >= 9) & (periods.hour < 18)).astype(float),
            "day_of_week": periods.dayofweek,
            "hour_of_day": periods.hour,
        }
    )
    return df


def make_level_target(df: pd.DataFrame, seed: int = 42) -> pd.Series:
    rng = np.random.default_rng(seed)
    # Deterministic relationship so a fitted model can predict it.
    return (
        0.5 * df["cpu_avg"]
        + 0.05 * df["workload_intensity"]
        + rng.normal(0, 0.5, len(df))
    )


def make_artifact(
    tmp_path, kind: str = "level", with_interval: bool = True
) -> ModelArtifact:
    """Build and persist a small artifact + metadata in tmp_path.

    Returns the loaded ModelArtifact (validated through the public loader).
    """
    df = make_training_frame()
    spec = FeatureSpec()
    ft = __import__("forecasting.features", fromlist=["add_derived_features"])
    enriched = ft.add_derived_features(df.copy(), spec)
    modeling = ft.build_modeling_frame(enriched, TARGET, spec)

    X = modeling[FEATURE_LIST].copy()
    y = make_level_target(modeling)

    if kind == "level":
        model = fit_model(
            ModelSpec(
                "rf",
                "random_forest",
                {"n_estimators": 10, "max_depth": 3, "random_state": 42},
            ),
            X,
            y,
        )
    else:
        level_index = FEATURE_LIST.index("cpu_avg")
        model = DifferencedPredictor(
            ModelSpec(
                "rf-diff",
                "random_forest",
                {"n_estimators": 10, "max_depth": 3, "random_state": 42},
            ),
            level_index,
        ).fit(X, y)

    model_dir = tmp_path / "model"
    model_dir.mkdir()
    joblib.dump(model, model_dir / "model.joblib")

    metadata = {
        "model_name": "test_forecast",
        "model_version": "1.0.0",
        "model_kind": kind,
        "target": TARGET,
        "period_type": "1h",
        "feature_list": FEATURE_LIST,
        "feature_definition_hash": feature_definition_hash(FEATURE_LIST, spec),
        "dataset_hash": "test-dataset-hash",
        "target_transform": "level" if kind == "level" else "diff",
        "prediction_interval": (
            {
                "primary_method": "jackknife_plus",
                "lower_offset": -1.5,
                "upper_offset": 1.5,
                "formal_guarantee_level": 0.80,
            }
            if with_interval
            else None
        ),
    }
    import json

    (model_dir / "model_metadata.json").write_text(
        json.dumps(metadata), encoding="utf-8"
    )

    from forecasting.serving.model import load

    return load(str(model_dir))


def make_versioned_artifact(tmp_path, version: str, kind: str = "level") -> ModelArtifact:
    """Build and persist a versioned artifact (model_<v>.joblib +
    model_metadata_<v>.json) in tmp_path.

    Returns the loaded ModelArtifact validated through load_version.
    """
    df = make_training_frame()
    spec = FeatureSpec()
    ft = __import__("forecasting.features", fromlist=["add_derived_features"])
    enriched = ft.add_derived_features(df.copy(), spec)
    modeling = ft.build_modeling_frame(enriched, TARGET, spec)

    X = modeling[FEATURE_LIST].copy()
    y = make_level_target(modeling)

    if kind == "level":
        model = fit_model(
            ModelSpec(
                "rf",
                "random_forest",
                {"n_estimators": 10, "max_depth": 3, "random_state": 42},
            ),
            X,
            y,
        )
    else:
        level_index = FEATURE_LIST.index("cpu_avg")
        model = DifferencedPredictor(
            ModelSpec(
                "rf-diff",
                "random_forest",
                {"n_estimators": 10, "max_depth": 3, "random_state": 42},
            ),
            level_index,
        ).fit(X, y)

    model_dir = tmp_path
    joblib.dump(model, model_dir / f"model_{version}.joblib")

    metadata = {
        "model_name": "test_forecast_v%s" % version,
        "model_version": "1.0.0",
        "model_kind": kind,
        "target": TARGET,
        "period_type": "1h",
        "feature_list": FEATURE_LIST,
        "feature_definition_hash": feature_definition_hash(FEATURE_LIST, spec),
        "dataset_hash": "test-dataset-hash",
        "target_transform": "level",
        "prediction_interval": {
            "primary_method": "jackknife_plus",
            "lower_offset": -1.5,
            "upper_offset": 1.5,
            "formal_guarantee_level": 0.80,
        },
    }
    import json

    (model_dir / f"model_metadata_{version}.json").write_text(
        json.dumps(metadata), encoding="utf-8"
    )

    from forecasting.serving.model import load_version

    return load_version(str(model_dir), version)


def make_observed_frame(n_rows: int = 20, seed: int = 7) -> pd.DataFrame:
    """Observed feature-store rows (no target_* / future labels)."""
    rng = np.random.default_rng(seed)
    start = pd.Timestamp("2026-09-10 00:00", tz="UTC")
    periods = pd.date_range(start, periods=n_rows, freq="h", tz="UTC")
    cpu = (
        30 + 8 * np.sin(np.linspace(0, 2 * np.pi, n_rows)) + rng.normal(0, 1.5, n_rows)
    )
    workload = (
        100 + 40 * np.cos(np.linspace(0, 2 * np.pi, n_rows)) + rng.normal(0, 3, n_rows)
    )
    return pd.DataFrame(
        {
            "period_start_utc": periods,
            "period_end_utc": periods + pd.Timedelta(hours=1),
            "cpu_avg": cpu,
            "workload_intensity": workload,
            "data_io_avg": rng.uniform(0, 50, n_rows),
            "log_write_avg": rng.uniform(0, 30, n_rows),
            "sessions_count": rng.uniform(10, 100, n_rows),
            "sessions_percent": rng.uniform(0, 80, n_rows),
            "memory_percent": rng.uniform(20, 90, n_rows),
            "workers_avg": rng.uniform(5, 50, n_rows),
            "storage_used_pct": rng.uniform(40, 90, n_rows),
            "storage_used_bytes": rng.integers(1000, 9000, n_rows),
            "storage_growth_bytes": rng.normal(0, 100, n_rows),
            "deadlock_count": rng.integers(0, 3, n_rows),
            "latency_p95_ms": rng.uniform(5, 50, n_rows),
            "is_peak_hour": ((periods.hour >= 9) & (periods.hour < 18)).astype(float),
            "day_of_week": periods.dayofweek,
            "hour_of_day": periods.hour,
        }
    )
