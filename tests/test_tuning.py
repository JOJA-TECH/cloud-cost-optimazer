"""Tests for the walk-forward tuning module."""

import numpy as np
import pandas as pd

from forecasting.models import ModelSpec, build_estimator
from forecasting.tuning import (
    CVResult,
    evaluation_points,
    evaluate_candidates,
    grid_catalogue,
    grid_description,
    walk_forward_scores,
)


def _exact_linear_data(n: int = 36) -> tuple[pd.DataFrame, pd.Series]:
    """y = 2x with no noise: a linear model must be perfect on every fold."""
    X = pd.DataFrame({"x": np.linspace(0.0, 10.0, n)})
    y = pd.Series(2.0 * X["x"].to_numpy())
    return X, y


def test_grid_catalogue_contents_and_counts():
    """18 configurations spanning the four kinds, all biased towards bias."""
    specs = grid_catalogue(seed=42)
    assert len(specs) == 18
    kinds = {spec.kind for spec in specs}
    assert kinds == {"linear", "ridge", "random_forest", "gradient_boosting"}
    assert len({spec.name for spec in specs}) == len(specs)

    for spec in specs:
        build_estimator(spec)  # every configuration must instantiate

    for spec in specs:
        if spec.kind == "random_forest":
            assert spec.params["max_depth"] <= 4
            assert spec.params["min_samples_leaf"] >= 2
            assert spec.params["random_state"] == 42
        if spec.kind == "gradient_boosting":
            assert spec.params["learning_rate"] <= 0.10
            assert spec.params["max_depth"] <= 3
            assert spec.params["random_state"] == 42


def test_grid_catalogue_names_are_readable():
    """The name encodes the tuned hyperparameters."""
    specs = {spec.name: spec for spec in grid_catalogue(seed=42)}
    assert "ridge_a2.0" in specs
    assert specs["ridge_a2.0"].params["alpha"] == 2.0
    assert "random_forest_d3_l4" in specs
    assert specs["random_forest_d3_l4"].params == {
        "n_estimators": 300,
        "max_depth": 3,
        "min_samples_leaf": 4,
        "random_state": 42,
    }
    assert "gradient_boosting_lr0.05_d2" in specs


def test_grid_description_is_serialisable():
    """The search space can be embedded in the metadata artifacts."""
    description = grid_description()
    assert set(description) == {"linear", "ridge", "random_forest", "gradient_boosting"}
    assert len(description["ridge"]) == 5
    assert len(description["random_forest"]) == 6
    assert len(description["gradient_boosting"]) == 6


def test_evaluation_points_math():
    """TimeSeriesSplit(5) on 36 rows evaluates 5 x 6 = 30 points."""
    assert evaluation_points(36, 5) == 30
    assert evaluation_points(24, 3) == 18
    assert evaluation_points(7, 5) == 0  # too small to split


def test_walk_forward_scores_perfect_linear_fit():
    """With y = 2x a linear model must be (near) exact on every fold."""
    X, y = _exact_linear_data()
    linear = ModelSpec("linear_lags", "linear", {})
    scores = walk_forward_scores(linear, X, y, n_splits=3)
    assert len(scores) == 3
    assert all(score < 1e-6 for score in scores)


def test_walk_forward_scores_deterministic():
    """Same inputs, same seed: identical fold MAEs."""
    rng = np.random.default_rng(0)
    X = pd.DataFrame({"a": rng.normal(0, 1, 36), "b": rng.normal(0, 1, 36)})
    y = pd.Series(X["a"].to_numpy() * 3.0 + rng.normal(0, 0.5, 36))
    rf = ModelSpec("random_forest", "random_forest",
                   {"n_estimators": 100, "max_depth": 2, "min_samples_leaf": 2,
                    "random_state": 42})
    first = walk_forward_scores(rf, X, y, n_splits=3)
    second = walk_forward_scores(rf, X, y, n_splits=3)
    assert first == second


def test_evaluate_candidates_ranks_best_first():
    """A perfect linear model must beat a degenerate ridge on y = 2x."""
    X, y = _exact_linear_data()
    specs = [
        ModelSpec("ridge_degenerate", "ridge", {"alpha": 1e12}),
        ModelSpec("linear_lags", "linear", {}),
    ]
    results = evaluate_candidates(specs, X, y, n_splits=3)
    assert results[0].spec.name == "linear_lags"
    assert results[0].mean_mae < results[1].mean_mae
    assert all(result.mean_mae <= results[i].mean_mae for i, result in enumerate(results))


def test_cv_result_row_is_flat_and_sorted():
    """as_row() exposes the fold MAEs as flat columns for the CSV artifact."""
    X, y = _exact_linear_data()
    specs = [ModelSpec("linear_lags", "linear", {})]
    results = evaluate_candidates(specs, X, y, n_splits=3)
    row = results[0].as_row()
    assert row["name"] == "linear_lags"
    assert row["n_folds"] == 3
    assert "fold_1_mae" in row and "fold_3_mae" in row
    assert row["cv_mae_mean"] == float(np.mean([row["fold_1_mae"], row["fold_2_mae"], row["fold_3_mae"]]))
    assert isinstance(results[0], CVResult)
