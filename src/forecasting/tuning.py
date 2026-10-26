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
from .transforms import DifferencedPredictor

#: Suffix appended to the configuration name of differenced candidates.
DIFF_SUFFIX = "_diff"

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


@dataclass
class Candidate:
    """A grid configuration plus its target representation (level or diff).

    ``level_index`` is the design-matrix position of the level column
    (``cpu_avg``) used by the differenced representation; it is required
    when ``differenced`` is true.
    """

    spec: ModelSpec
    differenced: bool = False
    level_index: int | None = None

    @property
    def name(self) -> str:
        return self.spec.name + (DIFF_SUFFIX if self.differenced else "")

    @property
    def transform_name(self) -> str:
        return "diff" if self.differenced else "level"

    def clone(self) -> "Candidate":
        return Candidate(
            spec=self.spec, differenced=self.differenced, level_index=self.level_index
        )


def as_candidate(item: ModelSpec | Candidate) -> Candidate:
    """Auto-wrap a bare ``ModelSpec`` as a level candidate (back-compat)."""
    if isinstance(item, Candidate):
        return item
    return Candidate(spec=item)


def candidate_catalogue(
    seed: int = 42,
    *,
    level_index: int | None = None,
) -> list[Candidate]:
    """Expand the grid into candidates in both target representations.

    Differenced variants are only produced when ``level_index`` is given,
    i.e. when the level column survived the feature selection.
    """
    specs = grid_catalogue(seed)
    candidates = [Candidate(spec=spec) for spec in specs]
    if level_index is not None:
        candidates += [
            Candidate(spec=spec, differenced=True, level_index=level_index)
            for spec in specs
        ]
    return candidates


def fit_candidate(candidate: Candidate, X: pd.DataFrame, y: pd.Series):
    """Fit a candidate and return a predictor of the LEVEL scale.

    Level candidates return the sklearn ``Pipeline``; differenced candidates
    return a :class:`~forecasting.transforms.DifferencedPredictor`. Both are
    picklable; use :func:`predict_candidate` for a uniform predict interface.
    """
    candidate = as_candidate(candidate)
    if candidate.differenced:
        if candidate.level_index is None:
            raise ValueError("differenced candidate requires a level column index")
        return DifferencedPredictor(candidate.spec, candidate.level_index).fit(X, y)
    return fit_model(candidate.spec, X, y)


def predict_candidate(predictor: Any, X: pd.DataFrame) -> np.ndarray:
    """Uniform prediction of the level scale for both representations."""
    if isinstance(predictor, DifferencedPredictor):
        return predictor.predict(X)
    return predict_model(predictor, X)


def walk_forward_scores(
    candidate: ModelSpec | Candidate,
    X: pd.DataFrame,
    y: pd.Series,
    *,
    n_splits: int = 5,
) -> list[float]:
    """Per-fold MAE on the level scale of one expanding-window evaluation.

    Differenced candidates are automatically reintegrated to the level scale
    so every configuration competes on the same metric.
    """
    resolved = as_candidate(candidate)
    matrix = X.to_numpy(dtype=float)
    labels = y.to_numpy(dtype=float)
    splitter = TimeSeriesSplit(n_splits=n_splits)
    scores: list[float] = []
    for train_idx, test_idx in splitter.split(matrix):
        predictor = fit_candidate(
            resolved.clone(),
            pd.DataFrame(matrix[train_idx]),
            pd.Series(labels[train_idx]),
        )
        prediction = predict_candidate(predictor, pd.DataFrame(matrix[test_idx]))
        scores.append(ev.mae(labels[test_idx], prediction))
    return scores


@dataclass
class CVResult:
    """Walk-forward outcome of a single candidate configuration."""

    candidate: Candidate
    fold_maes: list[float] = field(default_factory=list)
    mean_mae: float = float("inf")

    @property
    def spec(self) -> ModelSpec:
        return self.candidate.spec

    def as_row(self) -> dict[str, Any]:
        row: dict[str, Any] = {
            "name": self.candidate.name,
            "kind": self.candidate.spec.kind,
            "params": json.dumps(self.candidate.spec.params, sort_keys=True),
            "target_transform": self.candidate.transform_name,
            "n_folds": len(self.fold_maes),
            "cv_mae_mean": self.mean_mae,
        }
        for index, score in enumerate(self.fold_maes, start=1):
            row[f"fold_{index}_mae"] = float(score)
        return row


def evaluate_candidates(
    candidates: Sequence[ModelSpec | Candidate],
    X: pd.DataFrame,
    y: pd.Series,
    *,
    n_splits: int = 5,
) -> list[CVResult]:
    """Walk-forward MAE of every candidate, sorted best first.

    Both target representations compete on the level scale; ties are broken
    by the original catalogue order so the result is fully deterministic.
    """
    resolved = [as_candidate(item) for item in candidates]
    results: list[CVResult] = []
    for order, candidate in enumerate(resolved):
        scores = walk_forward_scores(candidate, X, y, n_splits=n_splits)
        results.append(
            CVResult(
                candidate=candidate,
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
