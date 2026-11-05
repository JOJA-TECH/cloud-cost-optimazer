# Tests for the model artifact loader (serving).

from __future__ import annotations

import json
from pathlib import Path

import pytest
from conftest import FEATURE_LIST, TARGET, save_artifact, make_level_target

from forecasting.serving.model import (
    ModelArtifact,
    ModelArtifactError,
    load_artifact,
)


def test_load_level_artifact(tmp_path):
    artifact = save_artifact(tmp_path, kind="level", with_interval=True)
    assert isinstance(artifact, ModelArtifact)
    assert artifact.model_name == f"test_forecast_{TARGET}_1.0.0"
    assert artifact.model_version == "1.0.0"
    assert artifact.target == TARGET
    assert artifact.feature_list == FEATURE_LIST
    assert artifact.interval_config is not None
    assert artifact.interval_config["primary_method"] == "jackknife_plus"


def test_load_differenced_artifact(tmp_path):
    artifact = save_artifact(tmp_path, kind="diff", with_interval=False)
    assert artifact.is_differenced()
    assert artifact.target == TARGET
    assert artifact.interval_config is None


def test_predict_matches_direct_estimator(tmp_path):
    artifact = save_artifact(tmp_path, kind="level", with_interval=True)
    import numpy as np
    from conftest import make_training_frame

    from forecasting.features import (
        FeatureSpec,
        add_derived_features,
        build_modeling_frame,
    )

    df = make_training_frame()
    spec = FeatureSpec()
    enriched = add_derived_features(df.copy(), spec)
    modeling = build_modeling_frame(enriched, TARGET, spec)
    X = modeling[FEATURE_LIST].copy()
    expected = artifact.model.predict(X.to_numpy(dtype=float))
    got = artifact.predict(X)
    np.testing.assert_allclose(got, expected, rtol=1e-6, atol=1e-8)


def test_missing_artifact_file(tmp_path):
    missing = tmp_path / "nope"
    missing.mkdir()
    with pytest.raises(ModelArtifactError):
        load_artifact(missing)


def test_missing_metadata(tmp_path):
    model_dir = tmp_path / "m"
    model_dir.mkdir()
    import joblib
    from conftest import make_training_frame

    from forecasting.features import (
        FeatureSpec,
        add_derived_features,
        build_modeling_frame,
    )
    from forecasting.models import ModelSpec, fit_model

    df = make_training_frame()
    spec = FeatureSpec()
    enriched = add_derived_features(df.copy(), spec)
    modeling = build_modeling_frame(enriched, TARGET, spec)
    X = modeling[FEATURE_LIST].copy()
    y = make_level_target(modeling)
    model = fit_model(
        ModelSpec(
            "rf",
            "random_forest",
            {"n_estimators": 5, "max_depth": 2, "random_state": 42},
        ),
        X,
        y,
    )
    joblib.dump(model, model_dir / "model.joblib")
    with pytest.raises(ModelArtifactError):
        load_artifact(model_dir)


def test_metadata_hash_mismatch(tmp_path):
    artifact = save_artifact(tmp_path, kind="level", with_interval=True)
    artifact_dir = tmp_path / TARGET / "1.0.0"
    metadata = json.loads((artifact_dir / "model_metadata.json").read_text())
    metadata["feature_definition_hash"] = "0000-mismatch"
    (artifact_dir / "model_metadata.json").write_text(
        json.dumps(metadata), encoding="utf-8"
    )
    with pytest.raises(ModelArtifactError):
        load_artifact(artifact_dir)


def test_empty_feature_list_rejected(tmp_path):
    artifact = save_artifact(tmp_path, kind="level", with_interval=True)
    artifact_dir = tmp_path / TARGET / "1.0.0"
    metadata = json.loads((artifact_dir / "model_metadata.json").read_text())
    metadata["feature_list"] = []
    (artifact_dir / "model_metadata.json").write_text(
        json.dumps(metadata), encoding="utf-8"
    )
    with pytest.raises(ModelArtifactError):
        load_artifact(artifact_dir)
