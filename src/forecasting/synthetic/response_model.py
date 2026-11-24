"""Response model: workload intensity → resource metrics.

The response model learns the relationship between the applied load
(workload_intensity, concurrency, batch_size, operation_mix) and the
observed metrics (cpu_avg, data_io_avg, log_write_avg, memory_percent,
latency_avg_ms, latency_p95_ms, error_rate, storage_growth_bytes).

For the first prototype we use:
    observed_response + load_effect + reproducible_residual

Each target metric is modelled independently with a simple regressor.
Constraints are enforced by clipping and the number of clipped predictions
is recorded.
"""

from __future__ import annotations

import warnings
from collections.abc import Sequence
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import joblib
import numpy as np
import pandas as pd
from sklearn.ensemble import GradientBoostingRegressor
from sklearn.impute import SimpleImputer
from sklearn.linear_model import Ridge
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from forecasting.config import Settings
from forecasting.features import FeatureSpec, impute_optional


@dataclass
class ResponseTarget:
    """Configuration for one response target."""

    name: str
    lower: float
    upper: float
    unit: str
    feature: str = "workload_intensity"

    def as_dict(self) -> dict[str, Any]:
        return {
            "name": self.name,
            "lower": self.lower,
            "upper": self.upper,
            "unit": self.unit,
            "feature": self.feature,
        }


DEFAULT_TARGETS: list[ResponseTarget] = [
    ResponseTarget("cpu_avg", 0.0, 100.0, "percent"),
    ResponseTarget("data_io_avg", 0.0, 100.0, "percent"),
    ResponseTarget("log_write_avg", 0.0, 100.0, "percent"),
    ResponseTarget("memory_percent", 0.0, 100.0, "percent"),
    ResponseTarget("latency_avg_ms", 0.0, 5000.0, "ms"),
    ResponseTarget("latency_p95_ms", 0.0, 5000.0, "ms"),
    ResponseTarget("error_rate", 0.0, 1.0, "ratio"),
    ResponseTarget("storage_growth_bytes", -1e9, 1e9, "bytes"),
]


@dataclass
class ResponseModel:
    """Fitted response model with metadata."""

    target_models: dict[str, Pipeline] = field(default_factory=dict)
    targets: list[ResponseTarget] = field(default_factory=lambda: list(DEFAULT_TARGETS))
    feature_names: list[str] = field(default_factory=list)
    clipped_counts: dict[str, int] = field(default_factory=dict)
    residuals: dict[str, np.ndarray] = field(default_factory=dict)
    metadata_: dict[str, Any] = field(default_factory=dict)
    observed_support: dict[str, list[float]] = field(default_factory=dict)

    @classmethod
    def fit(
        cls,
        hourly: pd.DataFrame,
        settings: Settings,
        *,
        targets: Sequence[ResponseTarget] = DEFAULT_TARGETS,
        feature_spec: FeatureSpec | None = None,
    ) -> ResponseModel:
        """Fit one model per target metric from the real hourly frame."""
        feature_spec = feature_spec or FeatureSpec()
        hourly = hourly.sort_values("period_start_utc").reset_index(drop=True)

        # Build features: use the same derived features as the forecasting model
        enriched = add_response_features(hourly, feature_spec)

        # Impute optional features with train median (use full frame as "train")
        imputed, _ = impute_response_features(enriched, enriched)

        model = cls(targets=list(targets))
        model.feature_names = [t.feature for t in targets]

        for target in targets:
            if target.name not in imputed.columns:
                warnings.warn(f"target {target.name} not in frame, skipping")
                continue

            y = imputed[target.name].astype(float)
            X = imputed[[target.feature]].copy()
            # Ensure the feature column has no NaN (safety)
            valid = y.notna() & X[target.feature].notna()
            if valid.sum() < 5:
                warnings.warn(f"too few observations for {target.name}")
                continue

            X_train = X.loc[valid, [target.feature]]
            y_train = y.loc[valid]

            # Observed support of the driving feature: any scenario outside
            # this range is extrapolation without empirical support.
            feature_values = X_train[target.feature]
            if target.feature not in model.observed_support:
                model.observed_support[target.feature] = [
                    float(feature_values.min()),
                    float(feature_values.max()),
                ]

            # Correlation feature<->target in the fitting window: quantifies
            # how much signal the response model actually has to learn.
            # (postgres numerics arrive as Decimal: force float64 first)
            feature_array = feature_values.to_numpy(dtype=float)
            target_array = y_train.to_numpy(dtype=float)
            if feature_array.std() > 0 and target_array.std() > 0:
                corr_pearson = float(np.corrcoef(feature_array, target_array)[0, 1])
                corr_spearman = float(
                    pd.Series(feature_array).corr(
                        pd.Series(target_array), method="spearman"
                    )
                )
            else:
                corr_pearson = float("nan")
                corr_spearman = float("nan")

            # Try linear first, then tree ensemble if non-linear
            with warnings.catch_warnings():
                warnings.simplefilter("ignore")
                lin = Pipeline(
                    [
                        ("scaler", StandardScaler()),
                        ("model", Ridge(alpha=1.0)),
                    ]
                )
                lin.fit(X_train, y_train)
                lin_pred = lin.predict(X_train)
                lin_rmse = float(np.sqrt(np.mean((y_train - lin_pred) ** 2)))

            # Tree ensemble for non-linear relationships
            with warnings.catch_warnings():
                warnings.simplefilter("ignore")
                tree = Pipeline(
                    [
                        ("imputer", SimpleImputer(strategy="median")),
                        (
                            "model",
                            GradientBoostingRegressor(
                                n_estimators=100,
                                max_depth=3,
                                learning_rate=0.1,
                                random_state=settings.random_seed,
                            ),
                        ),
                    ]
                )
                tree.fit(X_train, y_train)
                tree_pred = tree.predict(X_train)
                tree_rmse = float(np.sqrt(np.mean((y_train - tree_pred) ** 2)))

            # Select the better in-sample model
            if tree_rmse < lin_rmse * 0.95:
                best = tree
                kind = "gradient_boosting"
            else:
                best = lin
                kind = "ridge"

            # Residuals on the training data
            preds = best.predict(X_train)
            residuals = y_train.to_numpy(dtype=float) - preds
            n_clipped = int(np.sum((preds < target.lower) | (preds > target.upper)))

            model.target_models[target.name] = best
            model.residuals[target.name] = residuals
            model.clipped_counts[target.name] = n_clipped
            model.metadata_[target.name] = {
                "model_kind": kind,
                "in_sample_rmse": float(
                    tree_rmse if kind == "gradient_boosting" else lin_rmse
                ),
                "n_obs": int(valid.sum()),
                "n_clipped": n_clipped,
                "corr_pearson_with_feature": corr_pearson,
                "corr_spearman_with_feature": corr_spearman,
            }

        return model

    def predict(
        self,
        scenario_frame: pd.DataFrame,
        settings: Settings,
        *,
        seed: int | None = None,
    ) -> pd.DataFrame:
        """Predict all target metrics for a scenario frame.

        Adds reproducible residual sampling.
        """
        out = scenario_frame.copy()
        # Handle both 'ts' and 'period_start_utc' column names
        if "ts" in out.columns and "period_start_utc" not in out.columns:
            out = out.rename(columns={"ts": "period_start_utc"})
        out = out.sort_values("period_start_utc").reset_index(drop=True)

        for target in self.targets:
            if target.name not in self.target_models:
                continue
            model = self.target_models[target.name]
            X = scenario_frame[[target.feature]].copy()
            preds = model.predict(X)
            # Clip
            preds = np.clip(preds, target.lower, target.upper)
            # Add reproducible residual
            if target.name in self.residuals and self.residuals[target.name].size > 0:
                residuals = self.residuals[target.name]
                if seed is not None:
                    rng = np.random.default_rng(seed + hash(target.name) % 10000)
                else:
                    rng = np.random.default_rng()
                sampled = rng.choice(residuals, size=len(preds), replace=True)
                preds = preds + sampled
                preds = np.clip(preds, target.lower, target.upper)
            out[target.name] = preds
        return out

    def save(self, path: str | Path) -> None:
        joblib.dump(self, path)

    @classmethod
    def load(cls, path: str | Path) -> ResponseModel:
        return joblib.load(path)

    def metadata(self) -> dict[str, Any]:
        return {
            "targets": {t.name: t.as_dict() for t in self.targets},
            "feature_names": self.feature_names,
            "clipped_counts": self.clipped_counts,
            "per_target": self.metadata_,
            "observed_support": self.observed_support,
        }


# =============================================================================
# Helpers (internal)
# =============================================================================
def add_response_features(frame: pd.DataFrame, spec: FeatureSpec) -> pd.DataFrame:
    """Add derived features for the response model (same as forecasting)."""
    from forecasting.features import add_derived_features

    return add_derived_features(frame.copy(), spec)


def impute_response_features(
    train_frame: pd.DataFrame, target_frame: pd.DataFrame
) -> tuple[pd.DataFrame, dict[str, Any]]:
    """Impute missing features with train median (same logic as forecasting)."""
    from forecasting.features import FeatureSpec

    # Collect all numeric columns except the target names
    targets = [t.name for t in DEFAULT_TARGETS]
    feature_cols = [
        c
        for c in train_frame.columns
        if c not in targets
        and c not in ("period_start_utc", "period_end_utc", "formula", "ts")
        and pd.api.types.is_numeric_dtype(train_frame[c])
    ]
    FeatureSpec(optional=tuple(feature_cols))
    return impute_optional(train_frame, target_frame, feature_cols)
