"""Walk-forward model selection and hyperparameter tuning.

A single 6-point validation split makes the model choice nearly arbitrary:
one noisy hour can flip the ranking of the candidates. The selection now
uses an expanding-window walk-forward over the whole calibration window
(train + validation): ``TimeSeriesSplit(n_splits)`` evaluates every
candidate on ``n_splits * test_size`` points - the last fold being exactly
the classic train/validation split - and the mean MAE across folds decides.

The same CV drives a small hyperparameter grid that is deliberately biased
towards bias: shallower trees, larger leaves and smaller learning rates,
because with a 36-row window the risk is overfitting, not underfitting. A
deterministic grid is preferred over Optuna: no new dependency, fully
auditable and reproducible, and with 18 configurations the search space
does not need anything smarter.

The walk-forward runs on the calibration window only: the test split is
never read by any stage of the tuning.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from typing import Any, Sequence

import numpy as np
import pandas as pd
from sklearn.model_selection import TimeSeriesSplit

from . import evaluation as ev
from .models import ModelSpec, build_estimator, fit_model, predict_model

#: Hyperparameter grid per model kind. Order matters only for readability.
DEFAULT_GRIDS: dict[str, list[dict[str, Any]]] = {
    "linear": [{}],
    "ridge": [{"alpha": alpha} for alpha in (0.5, 1.0, 2.0, 5.0, 10.0)],
    "random_forest": [
        {"n_estimators": 300, "max_depth": depth, "min_samples_leaf": leaf}
        for depth in (2, 3, 4)
        for leaf in (2, 4)
    ],
    "gradient_boosting": [
        {"n_estimators": 200, "max_depth": depth, "learning_rate": rate}
        for rate in (0.03, 0.05, 0.10)
        for depth in (2, 3)
    ],
}


def _config_name(kind: str, params: dict[str, Any]) -> str:
    """Readable, unique name for a grid configuration."""
    if kind == "linear":
        return "linear_lags"
    if kind == "ridge":
        return f"ridge_a{params.get('alpha', 1.0)}"
    if kind == "random_forest":
        return (
            f"random_forest_d{params.get('max_depth')}"
            f"_l{params.get('min_samples_leaf')}"
        )
    if kind == "gradient_boosting":
        return (
            f"gradient_boosting_lr{params.get('learning_rate')}"
            f"_d{params.get('max_depth')}"
        )
    raise ValueError(f"unknown model kind: {kind}")


def grid_catalogue(seed: int = 42) -> list[ModelSpec]:
    """Expand the grids into the list of candidate configurations."""
    specs: list[ModelSpec] = []
    for kind, grid in DEFAULT_GRIDS.items():
        for params in grid:
            full = dict(params)
            if kind in ("random_forest", "gradient_boosting"):
                full["random_state"] = seed
            specs.append(ModelSpec(_config_name(kind, params), kind, full))
    return specs


def grid_description() -> dict[str, list[dict[str, Any]]]:
    """Serialisable description of the search space."""
    return {kind: [dict(params) for params in grid] for kind, grid in DEFAULT_GRIDS.items()}


def evaluation_points(n_rows: int, n_splits: int) -> int:
    """Total number of out-of-sample points the walk-forward evaluates on.

    Returns 0 when the geometry is infeasible: ``TimeSeriesSplit`` needs at
    least one training row before the first test block, i.e.
    ``n_rows - n_splits * test_size >= 1``.
    """
    if n_rows < 4 or n_splits < 1:
        return 0
    test_size = int(np.ceil(n_rows / (n_splits + 1)))
    if n_rows - n_splits * test_size < 1:
        return 0
    return int(n_splits * test_size)


def walk_forward_scores(
    spec: ModelSpec,
    X: pd.DataFrame,
    y: pd.Series,
    *,
    n_splits: int = 5,
) -> list[float]:
    """Per-fold MAE of an expanding-window evaluation of one configuration."""
    matrix = X.to_numpy(dtype=float)
    labels = y.to_numpy(dtype=float)
    splitter = TimeSeriesSplit(n_splits=n_splits)
    scores: list[float] = []
    for train_idx, test_idx in splitter.split(matrix):
        model = fit_model(spec, pd.DataFrame(matrix[train_idx]), pd.Series(labels[train_idx]))
        prediction = predict_model(model, pd.DataFrame(matrix[test_idx]))
        scores.append(ev.mae(labels[test_idx], prediction))
    return scores


@dataclass
class CVResult:
    """Walk-forward outcome of a single candidate configuration."""

    spec: ModelSpec
    fold_maes: list[float] = field(default_factory=list)
    mean_mae: float = float("inf")

    def as_row(self) -> dict[str, Any]:
        row: dict[str, Any] = {
            "name": self.spec.name,
            "kind": self.spec.kind,
            "params": json.dumps(self.spec.params, sort_keys=True),
            "n_folds": len(self.fold_maes),
            "cv_mae_mean": self.mean_mae,
        }
        for index, score in enumerate(self.fold_maes, start=1):
            row[f"fold_{index}_mae"] = float(score)
        return row


def evaluate_candidates(
    specs: Sequence[ModelSpec],
    X: pd.DataFrame,
    y: pd.Series,
    *,
    n_splits: int = 5,
) -> list[CVResult]:
    """Walk-forward MAE of every candidate, sorted best first.

    Ties are broken by the original catalogue order so the result is
    fully deterministic.
    """
    results: list[CVResult] = []
    for order, spec in enumerate(specs):
        scores = walk_forward_scores(spec, X, y, n_splits=n_splits)
        results.append(
            CVResult(
                spec=spec,
                fold_maes=scores,
                mean_mae=float(np.mean(scores)) if scores else float("inf"),
            )
        )
    ranking = sorted(
        range(len(results)),
        key=lambda index: (results[index].mean_mae, index),
    )
    return [results[index] for index in ranking]


def check_candidates(specs: Sequence[ModelSpec]) -> None:
    """Fail fast when a grid configuration cannot be instantiated."""
    for spec in specs:
        build_estimator(spec)
