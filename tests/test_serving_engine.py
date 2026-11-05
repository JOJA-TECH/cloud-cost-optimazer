# Tests for the inference engine (mocked feature store).

from __future__ import annotations

from datetime import UTC, datetime

import numpy as np
import pandas as pd
import pytest
from conftest import (
    FEATURE_LIST,
    TARGET,
    save_artifact,
    make_observed_frame,
    make_registry,
)

from forecasting.serving.engine import InferenceEngine, InferenceError
from forecasting.serving.model import ModelRegistry


class FakeReader:
    def __init__(self, frame, exists=True):
        self.frame = frame
        self.exists = exists
        self.calls = []

    def resource_exists(self, resource_uid: str) -> bool:
        self.calls.append(("exists", resource_uid))
        return self.exists

    def fetch_hourly_features(self, resource_uid: str) -> pd.DataFrame:
        self.calls.append(("fetch", resource_uid))
        return self.frame.copy()


def make_engine(
    tmp_path,
    frame,
    target=TARGET,
    version="1.0.0",
    kind="level",
    max_stale_minutes=1000000,
    now=None,
    exists=True,
    with_interval=True,
):
    save_artifact(
        tmp_path,
        target=target,
        version=version,
        kind=kind,
        with_interval=with_interval,
    )
    registry = ModelRegistry(
        model_dir=tmp_path,
        by_target={
            target: {version: None},  # placeholder, replaced below
        },
    )
    # Load the artifact properly into the registry.
    from forecasting.serving.model import load_registry

    registry = load_registry(tmp_path)
    reader = FakeReader(frame, exists=exists)
    engine = InferenceEngine(
        registry=registry,
        reader=reader,
        max_stale_minutes=max_stale_minutes,
        supported_horizon_minutes=60,
    )
    return engine, reader


def test_ok_prediction_matches_direct(tmp_path, now=None):
    now = now or datetime(2026, 9, 10, 23, 0, tzinfo=UTC)
    frame = make_observed_frame(20)
    engine, reader = make_engine(tmp_path, frame, now=now)
    payload = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="req-1",
        cutoff_time=None,
        now=now,
    )
    assert payload["status"] == "ok"
    assert payload["experimental"] is True
    assert payload["request_id"] == "req-1"
    assert payload["unit"] == "percent"
    assert payload["horizon_minutes"] == 60
    assert payload["interval"] is not None
    assert (
        payload["interval"]["lower"] < payload["value"] < payload["interval"]["upper"]
    )

    # Must equal direct inference on the same input matrix.
    from forecasting.features import (
        FeatureSpec,
        add_derived_features,
        build_modeling_frame,
    )

    spec = FeatureSpec()
    enriched = add_derived_features(frame.copy(), spec)
    modeling = build_modeling_frame(enriched, TARGET, spec)
    X = modeling[FEATURE_LIST].copy().tail(1)
    artifact = engine.get_artifact(TARGET)
    expected = float(artifact.predict(X).reshape(-1)[0])
    assert abs(payload["value"] - expected) < 1e-9

    # Period is the next hour after the last observed window.
    assert payload["period_start_utc"] == frame["period_start_utc"].iloc[
        -1
    ] + pd.Timedelta(hours=1)
    assert reader.calls[-1][0] == "fetch"


def test_unsupported_target_rejected(tmp_path):
    frame = make_observed_frame(20)
    engine, _ = make_engine(tmp_path, frame)
    with pytest.raises(InferenceError) as exc:
        engine.validate_target_horizon("target_storage_next1h", 60)
    assert "is not loaded" in str(exc.value) or "not available" in str(exc.value)


def test_unsupported_horizon_rejected(tmp_path):
    frame = make_observed_frame(20)
    engine, _ = make_engine(tmp_path, frame)
    with pytest.raises(InferenceError) as exc:
        engine.validate_target_horizon(TARGET, 30)
    assert "unsupported horizon" in str(exc.value)


def test_short_history_insufficient(tmp_path):
    frame = make_observed_frame(3)  # fewer than REQUIRED_HISTORY_HOURS+1
    engine, _ = make_engine(tmp_path, frame)
    payload = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
    )
    assert payload["status"] == "insufficient_data"


def test_gap_in_history_insufficient(tmp_path):
    frame = make_observed_frame(12)
    # Remove one middle window to create a gap.
    frame = frame.drop(index=5).reset_index(drop=True)
    engine, _ = make_engine(tmp_path, frame)
    payload = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
    )
    assert payload["status"] == "insufficient_data"


def test_duplicate_window_insufficient(tmp_path):
    frame = make_observed_frame(12)
    frame = pd.concat([frame, frame.tail(1)], ignore_index=True)
    engine, _ = make_engine(tmp_path, frame)
    payload = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
    )
    assert payload["status"] == "insufficient_data"


def test_out_of_order_insufficient(tmp_path):
    frame = make_observed_frame(12)
    frame = frame.iloc[list(range(11, -1, -1))].reset_index(drop=True)
    engine, _ = make_engine(tmp_path, frame)
    payload = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
    )
    assert payload["status"] == "insufficient_data"


def test_stale_data(tmp_path):
    frame = make_observed_frame(12)
    engine, _ = make_engine(
        tmp_path,
        frame,
        max_stale_minutes=1,
        now=datetime(2026, 9, 20, 0, 0, tzinfo=UTC),  # far after last window
    )
    payload = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
    )
    assert payload["status"] == "stale_data"


def test_missing_required_feature_insufficient(tmp_path):
    frame = make_observed_frame(12).drop(columns=["workload_intensity"])
    engine, _ = make_engine(tmp_path, frame)
    payload = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
    )
    assert payload["status"] == "insufficient_data"


def test_nan_and_inf_handled(tmp_path):
    frame = make_observed_frame(12)
    frame.loc[5, "data_io_avg"] = np.nan
    frame.loc[6, "latency_p95_ms"] = np.inf
    engine, _ = make_engine(tmp_path, frame)
    payload = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
    )
    # Optional NaN/Inf are imputed by the trained pipeline; prediction still ok.
    assert payload["status"] == "ok"


def test_no_future_labels_used(tmp_path):
    frame = make_observed_frame(12)
    # Confirm the observed frame carries no target_* columns.
    assert not any(c.startswith("target_") for c in frame.columns)
    engine, _ = make_engine(tmp_path, frame)
    payload = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
    )
    assert payload["status"] == "ok"
    artifact = engine.get_artifact(TARGET)
    assert "target_" not in str(artifact.feature_list)


def test_differenced_predictor_ok(tmp_path):
    frame = make_observed_frame(20)
    engine, _ = make_engine(tmp_path, frame, kind="diff")
    payload = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
    )
    assert payload["status"] == "ok"


def test_versioning_multiple_models(tmp_path):
    """Several model versions can coexist and be selected by version."""
    frame = make_observed_frame(20)

    # Two targets, each with one version, in the same model_dir.
    registry = make_registry(
        tmp_path,
        [
            (TARGET, "1.0.0", "level", True),
            ("target_workload_next1h", "1.0.0", "level", True),
        ],
    )
    engine = InferenceEngine(
        registry=registry,
        reader=FakeReader(make_observed_frame(20), exists=True),
        max_stale_minutes=1000000,
        supported_horizon_minutes=60,
    )
    assert sorted(engine.targets()) == [
        "target_cpu_avg_next1h",
        "target_workload_next1h",
    ]
    assert engine.versions(TARGET) == ["1.0.0"]
    assert engine.default_version(TARGET) == "1.0.0"

    # Predicting default target without version uses the default artifact.
    payload_default = engine.predict(
        resource_uid="uid-1",
        target=TARGET,
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
        model_version=None,
    )
    assert payload_default["status"] == "ok"
    assert payload_default["model_version"] == "1.0.0"

    # Predicting the other target explicitly.
    payload_wl = engine.predict(
        resource_uid="uid-1",
        target="target_workload_next1h",
        horizon_minutes=60,
        request_id="",
        cutoff_time=None,
        model_version="1.0.0",
    )
    assert payload_wl["status"] == "ok"
    assert payload_wl["model_version"] == "1.0.0"

    # Unknown target is rejected.
    with pytest.raises(InferenceError) as exc:
        engine.validate_target_horizon("target_storage_next1h", 60)
    assert "not loaded" in str(exc.value)

    # Unknown version for a known target is rejected.
    with pytest.raises(InferenceError) as exc:
        engine.validate_target_horizon(TARGET, 60, "9.9.9")
    assert "not loaded" in str(exc.value)
