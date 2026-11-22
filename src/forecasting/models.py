"""Minimal model specification and fit/predict helpers.

This module exists to satisfy the import chain in forecasting.transforms
(which is imported by forecasting.serving.model). It is a thin placeholder:
the real training path lives in forecasting.training and does not depend on
this module.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

import numpy as np
import pandas as pd
from sklearn.base import BaseEstimator, RegressorMixin
from sklearn.pipeline import Pipeline


@dataclass
class ModelSpec:
    """Specification of a model family and its hyperparameters."""

    family: str = "random_forest"
    params: dict[str, Any] = field(default_factory=dict)


def fit_model(spec: ModelSpec, X: pd.DataFrame, y: pd.Series) -> BaseEstimator:
    """Fit a scikit-learn pipeline according to ``spec``."""
    if spec.family == "linear":
        model: Pipeline = Pipeline(
            [
                ("imputer", SimpleImputer(strategy="median")),
                ("scaler", StandardScaler(with_mean=True, with_std=True)),
                ("regressor", LinearRegression()),
            ]
        )
    else:
        model = Pipeline(
            [
                ("imputer", SimpleImputer(strategy="median")),
                ("scaler", StandardScaler(with_mean=True, with_std=True)),
                (
                    "regressor",
                    RandomForestRegressor(
                        max_depth=spec.params.get("max_depth", 4),
                        min_samples_leaf=spec.params.get("min_samples_leaf", 4),
                        n_estimators=spec.params.get("n_estimators", 300),
                        random_state=42,
                    ),
                ),
            ]
        )
    model.fit(X, y)
    return model


def predict_model(model: BaseEstimator, X: pd.DataFrame) -> np.ndarray:
    """Predict with a fitted model."""
    return np.asarray(model.predict(X)).reshape(-1)


from sklearn.impute import SimpleImputer  # noqa: E402
from sklearn.linear_model import LinearRegression  # noqa: E402
from sklearn.ensemble import RandomForestRegressor  # noqa: E402
from sklearn.preprocessing import StandardScaler  # noqa: E402
