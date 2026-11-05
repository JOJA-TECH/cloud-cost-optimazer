"""Model artifact loader and validation.

Loads model.joblib and model_metadata.json exactly once at startup.
The serialized object may be a sklearn Pipeline or a DifferencedPredictor
wrapper; both are supported through a uniform predict interface. No training
happens here.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import joblib

from forecasting.features import FeatureSpec, feature_definition_hash
from forecasting.transforms import DifferencedPredictor

MODEL_FILENAME = "model.joblib"
METADATA_FILENAME = "model_metadata.json"


class ModelArtifactError(RuntimeError):
    """Raised when the artifact cannot be loaded or fails validation."""


@dataclass(frozen=True)
class ModelArtifact:
    """A validated, ready-to-serve model artifact."""

    model: Any
    metadata: dict[str, Any]
    model_dir: Path

    @property
    def model_name(self) -> str:
        return str(self.metadata.get("model_name", ""))

    @property
    def model_version(self) -> str:
        return str(self.metadata.get("model_version", ""))

    @property
    def model_kind(self) -> str:
        return str(self.metadata.get("model_kind", ""))

    @property
    def target(self) -> str:
        return str(self.metadata.get("target", ""))

    @property
    def period_type(self) -> str:
        return str(self.metadata.get("period_type", "1h"))

    @property
    def feature_list(self) -> list[str]:
        return [str(c) for c in self.metadata.get("feature_list", [])]

    @property
    def feature_definition_hash(self) -> str:
        return str(self.metadata.get("feature_definition_hash", ""))

    @property
    def dataset_hash(self) -> str:
        return str(self.metadata.get("dataset_hash", ""))

    @property
    def target_transform(self) -> str:
        return str(self.metadata.get("target_transform", "level"))

    @property
    def interval_config(self) -> dict[str, Any] | None:
        return self.metadata.get("prediction_interval")

    def is_differenced(self) -> bool:
        return isinstance(self.model, DifferencedPredictor)

    def predict(self, X) -> Any:
        """Uniform prediction on the level scale."""
        if isinstance(self.model, DifferencedPredictor):
            return self.model.predict(X)
        return self.model.predict(X)


def _load_json(path: Path) -> dict[str, Any]:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as exc:
        raise ModelArtifactError(f"metadata not found: {path}") from exc
    except json.JSONDecodeError as exc:
        raise ModelArtifactError(f"invalid metadata JSON {path}: {exc}") from exc


def _validate_object(model: Any, metadata: dict[str, Any]) -> None:
    kind = str(metadata.get("model_kind", "")).lower()
    is_differenced = isinstance(model, DifferencedPredictor)
    inner = getattr(model, "model", model) if is_differenced else model
    if not hasattr(inner, "predict") or not callable(inner.predict):
        raise ModelArtifactError(
            "serialized model has no callable predict: not a Pipeline or DifferencedPredictor"
        )
    if (
        kind in ("random_forest", "gradient_boosting", "ridge", "linear")
        and not is_differenced
    ):
        if not hasattr(model, "named_steps"):
            raise ModelArtifactError("serialized level model is not a sklearn Pipeline")


def _validate_metadata(metadata: dict[str, Any]) -> None:
    required = ["model_name", "model_version", "model_kind", "feature_list", "target"]
    missing = [k for k in required if not metadata.get(k)]
    if missing:
        raise ModelArtifactError(f"metadata missing required fields: {missing}")
    feature_list = metadata.get("feature_list")
    if not isinstance(feature_list, list) or not feature_list:
        raise ModelArtifactError("metadata.feature_list must be a non-empty list")
    for column in feature_list:
        if not isinstance(column, str):
            raise ModelArtifactError("metadata.feature_list must contain only strings")


def load(model_dir: str | Path) -> ModelArtifact:
    """Load and validate the artifact from ``model_dir``.

    Raises ModelArtifactError if the artifact is missing, incompatible or fails
    validation. The caller must treat the service as not ready.
    """
    directory = Path(model_dir)
    model_path = directory / MODEL_FILENAME
    metadata_path = directory / METADATA_FILENAME

    if not model_path.is_file():
        raise ModelArtifactError(f"model artifact not found: {model_path}")
    if not metadata_path.is_file():
        raise ModelArtifactError(f"model metadata not found: {metadata_path}")

    metadata = _load_json(metadata_path)
    _validate_metadata(metadata)

    try:
        model = joblib.load(model_path)
    except Exception as exc:
        raise ModelArtifactError(f"cannot load model.joblib: {exc}") from exc

    _validate_object(model, metadata)

    expected_hash = feature_definition_hash(metadata["feature_list"], FeatureSpec())
    if (
        metadata.get("feature_definition_hash")
        and expected_hash != metadata["feature_definition_hash"]
    ):
        raise ModelArtifactError(
            "feature_definition_hash mismatch: artifact was built with a "
            "different feature definition than the current serving code"
        )

    return ModelArtifact(model=model, metadata=metadata, model_dir=directory)
