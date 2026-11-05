# Online inference engine for the forecasting artifact.
# Builds the input matrix from observed feature-store rows (no future labels),
# validates temporal quality, and runs the loaded artifact once per request.
# Supports multiple model versions so different metrics/targets can be served
# and inferred independently.

from __future__ import annotations

from datetime import UTC, datetime
from typing import Any

import numpy as np
import pandas as pd

from forecasting.features import (
    FeatureSpec,
    add_derived_features,
    build_modeling_frame,
    feature_columns,
)
from forecasting.serving.data import (
    REQUIRED_HISTORY_HOURS,
    FeatureStoreReader,
)
from forecasting.serving.model import DEFAULT_VERSION, ModelArtifact


class InferenceError(RuntimeError):
    # Raised when a prediction cannot be produced.
    pass


class InferenceEngine:
    def __init__(
        self,
        artifacts: dict[str, ModelArtifact],
        reader: FeatureStoreReader,
        max_stale_minutes: int,
        supported_horizon_minutes: int,
        default_version: str = DEFAULT_VERSION,
    ) -> None:
        self.artifacts = artifacts
        self.reader = reader
        self.max_stale_minutes = max_stale_minutes
        self.supported_horizon_minutes = supported_horizon_minutes
        self.default_version = default_version or DEFAULT_VERSION
        self.spec = FeatureSpec()

    # ------------------------------------------------------------------ #
    # artifact resolution
    # ------------------------------------------------------------------ #
    def get_artifact(self, version: str | None) -> ModelArtifact | None:
        """Return the artifact for ``version``, or the default artifact."""
        key = version or self.default_version
        return self.artifacts.get(key)

    def available_versions(self) -> list[str]:
        return sorted(self.artifacts.keys())

    # ------------------------------------------------------------------ #
    # request validation
    # ------------------------------------------------------------------ #
    def validate_target_horizon(
        self, target: str, horizon_minutes: int, version: str | None = None
    ) -> None:
        artifact = self.get_artifact(version)
        if artifact is None:
            raise InferenceError(
                "model version %r is not loaded" % (version or self.default_version)
            )
        if target != artifact.target:
            raise InferenceError(
                "unsupported target %r; version %r serves %r"
                % (target, artifact.version, artifact.target)
            )
        if horizon_minutes != self.supported_horizon_minutes:
            raise InferenceError(
                "unsupported horizon %d min; only %d min is supported"
                % (horizon_minutes, self.supported_horizon_minutes)
            )

    # ------------------------------------------------------------------ #
    # data quality
    # ------------------------------------------------------------------ #
    @staticmethod
    def _validate_history(frame: pd.DataFrame) -> tuple[bool, str]:
        # Check ordering, duplicates and contiguity of the trailing window.
        if frame.empty:
            return False, "insufficient_data"

        ts = frame["period_start_utc"]
        if not ts.is_monotonic_increasing:
            return False, "insufficient_data"

        if ts.duplicated().any():
            return False, "insufficient_data"

        tail = frame.tail(REQUIRED_HISTORY_HOURS + 1)
        if len(tail) < REQUIRED_HISTORY_HOURS + 1:
            return False, "insufficient_data"

        diffs = tail["period_start_utc"].diff().dropna()
        expected = pd.Timedelta(hours=1)
        if not (diffs == expected).all():
            return False, "insufficient_data"

        return True, "ok"

    def _is_stale(self, last_ts: pd.Timestamp, now: datetime) -> bool:
        age_minutes = (now - last_ts.to_pydatetime()).total_seconds() / 60.0
        return age_minutes > self.max_stale_minutes

    # ------------------------------------------------------------------ #
    # feature matrix
    # ------------------------------------------------------------------ #
    def build_input_matrix(
        self,
        frame: pd.DataFrame,
        cutoff: pd.Timestamp | None,
        artifact: ModelArtifact,
    ) -> tuple[Any, str]:
        # Build the single-row input matrix for the latest usable window.
        # Returns (X, status). X is None when data is insufficient/stale.
        if frame.empty:
            return None, "insufficient_data"

        if cutoff is not None:
            frame = frame[frame["period_start_utc"] <= cutoff].reset_index(drop=True)
            if frame.empty:
                return None, "insufficient_data"

        enriched = add_derived_features(frame.copy(), self.spec)
        modeling = build_modeling_frame(enriched, artifact.target, self.spec)

        if modeling.empty:
            return None, "insufficient_data"

        last = modeling.tail(1)
        required_nan = [
            c
            for c in ("cpu_avg", "workload_intensity")
            if c in last.columns and bool(last[c].isna().any())
        ]
        if required_nan:
            return None, "insufficient_data"

        available = feature_columns(modeling, self.spec)
        for column in artifact.feature_list:
            if column not in available:
                return None, "insufficient_data"

        X = last[artifact.feature_list].copy()
        return X, "ok"

    # ------------------------------------------------------------------ #
    # predict
    # ------------------------------------------------------------------ #
    def predict(
        self,
        resource_uid: str,
        target: str,
        horizon_minutes: int,
        request_id: str | None,
        cutoff_time: str | None,
        model_version: str | None = None,
        now: datetime | None = None,
    ) -> dict[str, Any]:
        # Run one prediction and return a response payload dict.
        now = now or datetime.now(UTC)
        artifact = self.get_artifact(model_version)
        if artifact is None:
            raise InferenceError(
                "model version %r is not available"
                % (model_version or self.default_version)
            )

        self.validate_target_horizon(target, horizon_minutes, model_version)

        cutoff = None
        if cutoff_time:
            cutoff = pd.to_datetime(cutoff_time, utc=True)

        frame = self.reader.fetch_hourly_features(resource_uid)

        if frame.empty:
            return self._response(
                artifact,
                resource_uid,
                target,
                horizon_minutes,
                request_id,
                status="insufficient_data",
            )

        last_ts = frame["period_start_utc"].iloc[-1]
        if self._is_stale(last_ts, now):
            return self._response(
                artifact,
                resource_uid,
                target,
                horizon_minutes,
                request_id,
                status="stale_data",
            )

        ok, _ = self._validate_history(frame)
        if not ok:
            return self._response(
                artifact,
                resource_uid,
                target,
                horizon_minutes,
                request_id,
                status="insufficient_data",
            )

        X, status = self.build_input_matrix(frame, cutoff, artifact)
        if X is None:
            return self._response(
                artifact,
                resource_uid,
                target,
                horizon_minutes,
                request_id,
                status=status,
            )

        # Convert Inf -> NaN so the trained imputer handles missing values;
        # never impute with constants that differ from training.
        X = X.replace([np.inf, -np.inf], np.nan)

        try:
            values = artifact.predict(X)
        except Exception as exc:
            raise InferenceError("model prediction failed: %s" % exc) from exc

        value = float(np.asarray(values).reshape(-1)[0])
        if not np.isfinite(value):
            raise InferenceError("model produced a non-finite prediction")

        period_start = frame["period_start_utc"].iloc[-1]
        period_future = period_start + pd.Timedelta(hours=1)

        return self._response(
            artifact,
            resource_uid,
            target,
            horizon_minutes,
            request_id,
            status="ok",
            value=value,
            period_start_utc=period_future,
            interval=self._build_interval(artifact, value),
        )

    # ------------------------------------------------------------------ #
    # response assembly
    # ------------------------------------------------------------------ #
    def _response(
        self,
        artifact: ModelArtifact,
        resource_uid: str,
        target: str,
        horizon_minutes: int,
        request_id: str | None,
        *,
        status: str,
        value: float | None = None,
        period_start_utc: pd.Timestamp | None = None,
        interval: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        now = datetime.now(UTC)
        payload: dict[str, Any] = {
            "resource_uid": resource_uid,
            "target": target,
            "unit": "percent",
            "horizon_minutes": horizon_minutes,
            "generated_at_utc": now,
            "model_name": artifact.model_name,
            "model_version": artifact.model_version,
            "status": status,
            "experimental": True,
            "request_id": request_id or "",
            "interval": interval,
        }
        if status == "ok" and value is not None:
            payload["value"] = value
        if period_start_utc is not None:
            payload["period_start_utc"] = period_start_utc
        return payload

    def _build_interval(self, artifact: ModelArtifact, value: float) -> dict[str, Any] | None:
        config = artifact.interval_config
        if not config:
            return None
        lower_offset = config.get("lower_offset")
        upper_offset = config.get("upper_offset")
        if lower_offset is None or upper_offset is None:
            return None
        try:
            lo = float(value + float(lower_offset))
            hi = float(value + float(upper_offset))
        except (TypeError, ValueError):
            return None
        if not (np.isfinite(lo) and np.isfinite(hi)):
            return None
        return {
            "lower": lo,
            "upper": hi,
            "method": str(config.get("primary_method", "conformal")),
            "nominal_coverage": float(config.get("formal_guarantee_level", 0.0)),
        }
