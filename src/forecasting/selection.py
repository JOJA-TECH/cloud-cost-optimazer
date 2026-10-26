"""Feature selection for the tiny-data regime (n_rows << n_features).

With 43 usable rows and 39 engineered features the design matrix is wider
than long, which invites overfitting. The selection is deliberately simple,
deterministic and free of test-set contact:

1. **Variance prune** — columns that are constant over the calibration window
   (train + validation) carry no information; this removes, for example, the
   ``feature_missing_*`` flags of fully observed metrics and one-hot bands
   that were never observed in the window.
2. **Collinearity prune** — a column that is an exact duplicate (or perfect
   negative) of an already kept column is dropped; this removes, for
   example, ``peak_band_peak`` vs ``is_peak_hour`` or ``sessions_percent``
   vs ``sessions_count`` when the divisor is constant.
3. **Permutation ranking** — the surviving columns are ranked by the
   degradation of the MAE when their values are permuted, averaged over
   the folds of an expanding window (TimeSeriesSplit) and over the
   candidate models of the catalogue. The top ``max_features`` columns with
   strictly positive importance are kept, with a floor of ``MIN_SELECTED``
   features so the ranking noise can never degenerate the model.

The calibration window ends where the test set begins: the test rows are
never read by any step of the selection.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Sequence

import numpy as np
import pandas as pd
from sklearn.base import clone
from sklearn.inspection import permutation_importance
from sklearn.model_selection import TimeSeriesSplit

from .models import ModelSpec, build_estimator

#: Variance below which a column is considered constant.
VARIANCE_EPS: float = 1e-12
#: Absolute correlation above which two columns are considered duplicates.
COLLINEARITY_THRESHOLD: float = 0.999
#: Never return fewer features than this, whatever the ranking says.
MIN_SELECTED: int = 3
#: Permutation repeats per fold (36-row windows: cheap and stable enough).
DEFAULT_N_REPEATS: int = 10


@dataclass
class SelectionReport:
    """Everything the artifacts need to explain why columns were dropped."""

    method: str
    max_features: int
    n_candidates: int
    n_rows_calibration: int
    n_folds: int
    dropped_variance: list[str]
    dropped_collinear: dict[str, str]
    importances: list[dict[str, Any]]
    selected: list[str]
    notes: list[str] = field(default_factory=list)

    def as_dict(self) -> dict[str, Any]:
        return {
            "method": self.method,
            "max_features": self.max_features,
            "n_candidates": self.n_candidates,
            "n_rows_calibration": self.n_rows_calibration,
            "n_folds": self.n_folds,
            "dropped_variance": self.dropped_variance,
            "dropped_collinear": self.dropped_collinear,
            "importances": self.importances,
            "selected": self.selected,
            "notes": self.notes,
        }


def variance_prune(X: pd.DataFrame, eps: float = VARIANCE_EPS) -> list[str]:
    """Constant columns of the window: they cannot explain anything."""
    dropped: list[str] = []
    for column in X.columns:
        values = X[column].to_numpy(dtype=float)
        finite = values[np.isfinite(values)]
        if finite.size == 0 or float(np.var(finite)) < eps:
            dropped.append(column)
    return dropped


def _safe_corr(a: pd.Series, b: pd.Series) -> float:
    """Pearson correlation, 0.0 when undefined (constants are caught earlier)."""
    if a.nunique() < 2 or b.nunique() < 2:
        return 0.0
    corr = float(np.corrcoef(a.to_numpy(dtype=float), b.to_numpy(dtype=float))[0, 1])
    return corr if np.isfinite(corr) else 0.0


def collinearity_prune(
    X: pd.DataFrame,
    columns: Sequence[str],
    threshold: float = COLLINEARITY_THRESHOLD,
) -> dict[str, str]:
    """Exact duplicates or perfect negatives of an already kept column.

    The first column (in the original order) is kept and later twins are
    dropped; the mapping records which kept column absorbed each drop.
    """
    kept: list[str] = []
    dropped: dict[str, str] = {}
    for column in columns:
        twin: str | None = None
        for other in kept:
            if abs(_safe_corr(X[column], X[other])) >= threshold:
                twin = other
                break
        if twin is None:
            kept.append(column)
        else:
            dropped[column] = twin
    return dropped


def _fold_mean_importance(
    estimator: Any,
    X: pd.DataFrame,
    y: pd.Series,
    *,
    n_splits: int,
    n_repeats: int,
    seed: int,
) -> np.ndarray:
    """Mean permutation importance (MAE degradation) over expanding-window folds."""
    matrix = X.to_numpy(dtype=float)
    labels = y.to_numpy(dtype=float)
    splitter = TimeSeriesSplit(n_splits=n_splits)
    fold_scores: list[np.ndarray] = []
    for train_idx, test_idx in splitter.split(matrix):
        fitted = clone(estimator)
        fitted.fit(matrix[train_idx], labels[train_idx])
        result = permutation_importance(
            fitted,
            matrix[test_idx],
            labels[test_idx],
            scoring="neg_mean_absolute_error",
            n_repeats=n_repeats,
            random_state=seed,
        )
        fold_scores.append(np.asarray(result.importances_mean, dtype=float))
    return np.mean(fold_scores, axis=0)


def permutation_ranking(
    specs: Sequence[ModelSpec],
    X: pd.DataFrame,
    y: pd.Series,
    *,
    n_splits: int,
    seed: int,
    n_repeats: int = DEFAULT_N_REPEATS,
) -> list[dict[str, Any]]:
    """Rank features by MAE degradation, averaged across models and folds."""
    rows: dict[str, dict[str, float]] = {column: {} for column in X.columns}
    for spec in specs:
        scores = _fold_mean_importance(
            build_estimator(spec), X, y, n_splits=n_splits, n_repeats=n_repeats, seed=seed
        )
        for column, score in zip(X.columns, scores):
            rows[column][f"importance_{spec.name}"] = float(score)
    ranking: list[dict[str, Any]] = []
    for column, per_model in rows.items():
        values = list(per_model.values())
        ranking.append(
            {
                "feature": column,
                "importance_mean": float(np.mean(values)) if values else 0.0,
                **per_model,
            }
        )
    ranking.sort(key=lambda row: row["importance_mean"], reverse=True)
    return ranking


def select_features(
    X: pd.DataFrame,
    y: pd.Series,
    specs: Sequence[ModelSpec],
    *,
    max_features: int = 15,
    seed: int = 42,
    n_repeats: int = DEFAULT_N_REPEATS,
) -> tuple[list[str], SelectionReport]:
    """Return the selected column list (original order) and the full report."""
    dropped_variance = variance_prune(X)
    after_variance = [c for c in X.columns if c not in dropped_variance]
    dropped_collinear = collinearity_prune(X, after_variance)
    survivors = [c for c in after_variance if c not in dropped_collinear]

    notes = [
        "La ventana de calibracion es train + validacion: el conjunto de prueba "
        "no participa en ninguna etapa de la seleccion.",
        "El ranking promedia la degradacion de MAE por permutacion sobre los pliegues "
        "expansivos (TimeSeriesSplit) y sobre todos los modelos candidatos.",
    ]
    if not survivors:
        notes.append("Sin columnas supervivientes: se conservan todas las candidatas.")
        selected = list(X.columns)
        return selected, SelectionReport(
            method="variance+collinearity+permutation",
            max_features=max_features,
            n_candidates=len(X.columns),
            n_rows_calibration=len(X),
            n_folds=0,
            dropped_variance=dropped_variance,
            dropped_collinear=dropped_collinear,
            importances=[],
            selected=selected,
            notes=notes,
        )

    n_splits = max(2, min(5, len(X) // 12))
    ranking = permutation_ranking(
        specs, X[survivors], y, n_splits=n_splits, seed=seed, n_repeats=n_repeats
    )
    positive = [row["feature"] for row in ranking if row["importance_mean"] > 0.0]
    if len(positive) >= MIN_SELECTED:
        chosen = positive[:max_features]
    else:
        chosen = [row["feature"] for row in ranking[:MIN_SELECTED]]
        notes.append(
            "Menos de MIN_SELECTED caracteristicas con importancia positiva: "
            "se conservan las mejores por ranking (piso de 3)."
        )
    selected = [c for c in X.columns if c in set(chosen)]
    return selected, SelectionReport(
        method="variance+collinearity+permutation",
        max_features=max_features,
        n_candidates=len(X.columns),
        n_rows_calibration=len(X),
        n_folds=n_splits,
        dropped_variance=dropped_variance,
        dropped_collinear=dropped_collinear,
        importances=ranking,
        selected=selected,
        notes=notes,
    )
