"""Model artifact loader and validation.

Loads model.joblib and model_metadata.json exactly once at startup.
The serialized object may be a sklearn Pipeline or a DifferencedPredictor
wrapper; both are supported through a uniform predict interface. No training
happens here.

Model storage layout
--------------------
Artifacts are stored in folders independent per target (the metric being
predicted) and, inside each target, in folders per version::

    model_dir/
        <target>/                       <- name of the metric, e.g. target_cpu_avg_next1h
            <version>/                  <- model_version, e.g. 1.0.0, 1.0.1
                model.joblib
                model_metadata.json

A new version is created when:

* the user explicitly indicates a version, or
* training adds different values that alter the model behaviour
  (different features, hyperparameters, target or dataset hash).

The :class:`ModelRegistry` discovers every ``<target>/<version>/`` pair under
``FORECAST_MODEL_DIR`` and loads them once at startup.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
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
    model_dir: Path       # directory that contains this artifact (<target>/<version>/)
    target: str           # metric this artifact predicts
    version: str          # model_version of this artifact

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


@dataclass
class ModelRegistry:
    """In-memory index of all model artifacts discovered in ``model_dir``.

    Keys are target names; values map version strings to :class:`ModelArtifact`.
    Loaded exactly once at startup; no training happens here.
    """

    model_dir: Path
    by_target: dict[str, dict[str, ModelArtifact]] = field(default_factory=dict)

    def targets(self) -> list[str]:
        """Return the sorted list of targets (metrics) with at least one version."""
        return sorted(self.by_target.keys())

    def versions(self, target: str) -> list[str]:
        """Return the sorted list of versions available for ``target``."""
        return sorted(self.by_target.get(target, {}).keys())

    def default_version(self, target: str) -> str | None:
        """Return the default version for ``target``: the latest by version string."""
        vs = self.versions(target)
        if not vs:
            return None
        return vs[-1]

    def get(self, target: str, version: str | None = None) -> ModelArtifact | None:
        """Return the artifact for ``target``/``version``.

        When ``version`` is None the default (latest) version for the target is
        returned. Returns None when the target or version does not exist.
        """
        versions_map = self.by_target.get(target)
        if not versions_map:
            return None
        if version is None:
            version = self.default_version(target)
        if version is None:
            return None
        return versions_map.get(version)

    def has_target(self, target: str) -> bool:
        return target in self.by_target


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
    if kind in ("random_forest", "gradient_boosting", "ridge", "linear") and not is_differenced:
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


def _validate_single(directory: Path, metadata: dict[str, Any], model: Any) -> ModelArtifact:
    """Validate and build a ModelArtifact for one artifact directory."""
    expected_hash = feature_definition_hash(metadata["feature_list"], FeatureSpec())
    if (
        metadata.get("feature_definition_hash")
        and expected_hash != metadata["feature_definition_hash"]
    ):
        raise ModelArtifactError(
            "feature_definition_hash mismatch: artifact was built with a "
            "different feature definition than the current serving code"
        )
    target = str(metadata.get("target", ""))
    version = str(metadata.get("model_version", "") or directory.name)
    return ModelArtifact(
        model=model,
        metadata=metadata,
        model_dir=directory,
        target=target,
        version=version,
    )


def load_artifact(
    directory: Path, target: str | None = None
) -> ModelArtifact:
    """Load and validate a single artifact from ``directory``.

    ``directory`` is expected to contain ``model.joblib`` and
    ``model_metadata.json``. When ``target`` is given it overrides the
    target read from metadata (useful when the folder name is the canonical
    metric name).
    """
    directory = Path(directory)
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
        raise ModelArtifactError(f"cannot load model artifact: {model_path}: {exc}") from exc

    _validate_object(model, metadata)
    artifact = _validate_single(directory, metadata, model)
    if target is not None:
        artifact = ModelArtifact(
            model=artifact.model,
            metadata=artifact.metadata,
            model_dir=artifact.model_dir,
            target=target,
            version=artifact.version,
        )
    return artifact


def discover_artifacts(model_dir: str | Path) -> list[ModelArtifact]:
    """Discover every ``<target>/<version>/`` artifact under ``model_dir``.

    Returns a list of validated :class:`ModelArtifact`. Any directory that does
    not contain a valid artifact is skipped and logged.
    """
    root = Path(model_dir)
    if not root.is_dir():
        return []
    artifacts: list[ModelArtifact] = []
    for target_dir in sorted(root.iterdir()):
        if not target_dir.is_dir():
            continue
        target = target_dir.name
        for version_dir in sorted(target_dir.iterdir()):
            if not version_dir.is_dir():
                continue
            if not (version_dir / MODEL_FILENAME).is_file():
                continue
            if not (version_dir / METADATA_FILENAME).is_file():
                continue
            try:
                artifact = load_artifact(version_dir, target=target_dir.name)
                artifacts.append(artifact)
            except ModelArtifactError as exc:  # noqa: BLE001 - skip bad artifacts
                import logging

                logging.getLogger("forecast.grpc").warning(
                    "skipping artifact %s/%s: %s", target, version_dir.name, exc
                )
    return artifacts


def load_registry(model_dir: str | Path) -> ModelRegistry:
    """Load every artifact under ``model_dir`` into a :class:`ModelRegistry`.

    This is the recommended entry point for the server: it discovers all
    targets and versions once at startup.
    """
    root = Path(model_dir)
    if not root.is_dir():
        raise ModelArtifactError(f"model directory not found: {root}")
    artifacts = discover_artifacts(root)
    by_target: dict[str, dict[str, ModelArtifact]] = {}
    for artifact in artifacts:
        by_target.setdefault(artifact.target, {})[artifact.version] = artifact
    return ModelRegistry(model_dir=root, by_target=by_target)
