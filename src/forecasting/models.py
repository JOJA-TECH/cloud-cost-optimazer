"""Learned models for the 1h-ahead CPU forecast.

The catalogue is intentionally small and ordered by increasing capacity, as
required by the plan: linear regression with lags first, then tree ensembles
when the volume of data allows it. Deep learning, RL and AutoML are out of
scope for this delivery.

Every model is wrapped in a ``Pipeline`` that imputes the optional features
with the **training** median and standardises the inputs for the linear
models, so the preprocessing is fitted on the train split only.
"""

from __future__ import annotations

from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from typing import Any

import numpy as np
import pandas as pd
from sklearn.ensemble import GradientBoostingRegressor, RandomForestRegressor
from sklearn.impute import SimpleImputer
from sklearn.linear_model import LinearRegression, Ridge
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

DEFAULT_RANDOM_SEED = 42


@dataclass
class ModelSpec:
    """Declarative description of a candidate model."""

    name: str
    kind: str
    params: dict[str, Any] = field(default_factory=dict)

    def describe(self) -> str:
        return f"{self.name}({self.kind})"


def default_catalogue(seed: int = DEFAULT_RANDOM_SEED) -> list[ModelSpec]:
    """Models evaluated in this delivery, in the order of the plan."""
    return [
        ModelSpec("linear_lags", "linear", {}),
        ModelSpec("ridge_lags", "ridge", {"alpha": 1.0}),
        ModelSpec(
            "random_forest",
            "random_forest",
            {
                "n_estimators": 300,
                "max_depth": 3,
                "min_samples_leaf": 2,
                "random_state": seed,
            },
        ),
        ModelSpec(
            "gradient_boosting",
            "gradient_boosting",
            {
                "n_estimators": 200,
                "max_depth": 2,
                "learning_rate": 0.05,
                "random_state": seed,
            },
        ),
    ]


def build_estimator(spec: ModelSpec) -> Pipeline:
    """Instantiate the sklearn pipeline of a model specification."""
    steps: list[tuple[str, Any]] = [
        ("imputer", SimpleImputer(strategy="median", keep_empty_features=True)),
    ]
    if spec.kind == "linear":
        steps.append(("scaler", StandardScaler()))
        steps.append(("model", LinearRegression()))
    elif spec.kind == "ridge":
        steps.append(("scaler", StandardScaler()))
        steps.append(("model", Ridge(alpha=float(spec.params.get("alpha", 1.0)))))
    elif spec.kind == "random_forest":
        steps.append(
            (
                "model",
                RandomForestRegressor(
                    n_estimators=int(spec.params.get("n_estimators", 300)),
                    max_depth=spec.params.get("max_depth"),
                    min_samples_leaf=int(spec.params.get("min_samples_leaf", 1)),
                    random_state=int(
                        spec.params.get("random_state", DEFAULT_RANDOM_SEED)
                    ),
                ),
            )
        )
    elif spec.kind == "gradient_boosting":
        steps.append(
            (
                "model",
                GradientBoostingRegressor(
                    n_estimators=int(spec.params.get("n_estimators", 200)),
                    max_depth=spec.params.get("max_depth"),
                    learning_rate=float(spec.params.get("learning_rate", 0.05)),
                    random_state=int(
                        spec.params.get("random_state", DEFAULT_RANDOM_SEED)
                    ),
                ),
            )
        )
    else:
        raise ValueError(f"unknown model kind: {spec.kind}")
    return Pipeline(steps)


def fit_model(spec: ModelSpec, X: pd.DataFrame, y: pd.Series) -> Pipeline:
    """Fit a model on the given matrices."""
    estimator = build_estimator(spec)
    estimator.fit(X.to_numpy(dtype=float), y.to_numpy(dtype=float))
    return estimator


def predict_model(model: Pipeline, X: pd.DataFrame) -> np.ndarray:
    """Predict with a fitted model."""
    return np.asarray(model.predict(X.to_numpy(dtype=float)), dtype=float)


def feature_importance(model: Pipeline, columns: Sequence[str]) -> pd.DataFrame:
    """Extract the feature importance of tree models (empty for linear ones)."""
    estimator = model.named_steps.get("model")
    importances = getattr(estimator, "feature_importances_", None)
    if importances is None:
        coefficients = getattr(estimator, "coef_", None)
        if coefficients is None:
            return pd.DataFrame(columns=["feature", "importance"])
        return pd.DataFrame(
            {
                "feature": list(columns),
                "importance": np.abs(np.asarray(coefficients, dtype=float)),
            }
        ).sort_values("importance", ascending=False)
    return pd.DataFrame(
        {"feature": list(columns), "importance": np.asarray(importances, dtype=float)}
    ).sort_values("importance", ascending=False)


def select_columns(frame: pd.DataFrame, candidates: Sequence[str]) -> list[str]:
    """Keep the candidate columns that exist and are not entirely null."""
    keep: list[str] = []
    for column in candidates:
        if column in frame.columns and not frame[column].isna().all():
            keep.append(column)
    return keep


def build_matrix(
    frame: pd.DataFrame, columns: Sequence[str], target: str
) -> tuple[pd.DataFrame, pd.Series]:
    """Split a frame into the design matrix and the label."""
    X = frame[list(columns)].copy()
    y = frame[target].astype(float).copy()
    return X, y


def describe_model(model: Pipeline) -> dict[str, Any]:
    """Serialisable description of a fitted model."""
    estimator = model.named_steps["model"]
    return {
        "estimator": type(estimator).__name__,
        "steps": [name for name, _ in model.steps],
        "params": {
            key: (
                value
                if isinstance(value, (int, float, str, bool, type(None)))
                else str(value)
            )
            for key, value in estimator.get_params().items()
            if key
            in {
                "n_estimators",
                "max_depth",
                "min_samples_leaf",
                "learning_rate",
                "alpha",
                "random_state",
            }
        },
    }


def model_metadata(
    spec: ModelSpec,
    columns: Sequence[str],
    *,
    training_rows: int,
    validation_rows: int,
    test_rows: int,
    training_period: tuple[str | None, str | None],
    validation_period: tuple[str | None, str | None],
    test_period: tuple[str | None, str | None],
    dataset_hash: str,
    feature_definition_hash: str,
    random_seed: int,
    extra: Mapping[str, Any] | None = None,
) -> dict[str, Any]:
    """Metadata block persisted as ``model_metadata.json``."""
    metadata: dict[str, Any] = {
        "model_name": spec.name,
        "model_version": "1.0.0",
        "model_kind": spec.kind,
        "hyperparameters": dict(spec.params),
        "feature_list": list(columns),
        "training_rows": training_rows,
        "validation_rows": validation_rows,
        "test_rows": test_rows,
        "training_period": {
            "start_utc": training_period[0],
            "end_utc": training_period[1],
        },
        "validation_period": {
            "start_utc": validation_period[0],
            "end_utc": validation_period[1],
        },
        "test_period": {"start_utc": test_period[0], "end_utc": test_period[1]},
        "dataset_hash": dataset_hash,
        "feature_definition_hash": feature_definition_hash,
        "random_seed": random_seed,
    }
    if extra:
        metadata.update(extra)
    return metadata
