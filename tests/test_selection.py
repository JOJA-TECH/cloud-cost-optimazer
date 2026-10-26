"""Tests for the feature selection module."""

import numpy as np
import pandas as pd

from forecasting.models import ModelSpec, default_catalogue
from forecasting.selection import (
    collinearity_prune,
    permutation_ranking,
    select_features,
    variance_prune,
)


def _linear_spec() -> list[ModelSpec]:
    """Cheap model for the ranking tests: a single linear regression."""
    return [ModelSpec("linear_lags", "linear", {})]


def _toy_frame(n: int = 36, seed: int = 0) -> tuple[pd.DataFrame, pd.Series]:
    """Informative target driven by column 'a', plus noise columns."""
    rng = np.random.default_rng(seed)
    X = pd.DataFrame(
        {
            "a": np.linspace(0.0, 10.0, n) + rng.normal(0, 0.1, n),
            "noise_1": rng.normal(0, 1, n),
            "noise_2": rng.normal(0, 1, n),
        }
    )
    y = pd.Series(X["a"].to_numpy() + rng.normal(0, 0.05, n))
    return X, y


def test_variance_prune_drops_constant_columns():
    """Columns constant over the window cannot explain anything."""
    X = pd.DataFrame(
        {
            "cpu": [1.0, 2.0, 3.0, 4.0],
            "constant_zero": [0.0, 0.0, 0.0, 0.0],
            "constant_flag": [5.0, 5.0, 5.0, 5.0],
        }
    )
    dropped = variance_prune(X)
    assert set(dropped) == {"constant_zero", "constant_flag"}


def test_variance_prune_keeps_varying_columns():
    """Anything with variance survives."""
    X, _ = _toy_frame()
    assert variance_prune(X) == []


def test_collinearity_prune_drops_exact_duplicates_and_negatives():
    """Twins (corr = +1 or -1) are dropped, the first column is kept."""
    X = pd.DataFrame(
        {
            "is_peak_hour": [0.0, 1.0, 0.0, 1.0, 1.0, 0.0],
            "peak_band_peak": [0.0, 1.0, 0.0, 1.0, 1.0, 0.0],  # exact duplicate
            "peak_band_non_peak": [1.0, 0.0, 1.0, 0.0, 0.0, 1.0],  # perfect negative
        }
    )
    dropped = collinearity_prune(X, list(X.columns))
    assert dropped == {"peak_band_peak": "is_peak_hour", "peak_band_non_peak": "is_peak_hour"}


def test_collinearity_prune_keeps_moderate_correlation():
    """0.9 correlation is not a duplicate and must be kept."""
    rng = np.random.default_rng(1)
    base = rng.normal(0, 1, 50)
    X = pd.DataFrame(
        {
            "base": base,
            "correlated": 0.9 * base + 0.1 * rng.normal(0, 1, 50),
        }
    )
    assert collinearity_prune(X, list(X.columns)) == {}


def test_permutation_ranking_prefers_informative_feature():
    """The feature that carries the signal must rank first."""
    X, y = _toy_frame()
    ranking = permutation_ranking(
        _linear_spec(), X, y, n_splits=3, seed=42, n_repeats=5
    )
    assert ranking[0]["feature"] == "a"
    assert ranking[0]["importance_mean"] > ranking[-1]["importance_mean"]


def test_select_features_end_to_end():
    """Constant, collinear and ranking-cut columns are all excluded."""
    rng = np.random.default_rng(7)
    base = np.linspace(0, 10, 36)
    X = pd.DataFrame(
        {
            "signal": base + rng.normal(0, 0.1, 36),
            "helper": base * 2.0,  # near-exact twin of signal (corr ~0.9994)
            "noise_1": rng.normal(0, 1, 36),
            "noise_2": rng.normal(0, 1, 36),
            "noise_3": rng.normal(0, 1, 36),
            "constant": [3.0] * 36,
            "twin_of_signal": base,  # exact duplicate
        }
    )
    y = pd.Series(base + rng.normal(0, 0.05, 36))
    selected, report = select_features(
        X, y, _linear_spec(), max_features=3, seed=42, n_repeats=5
    )
    assert "constant" not in selected
    assert "twin_of_signal" not in selected
    assert "helper" not in selected
    assert len(selected) <= 3
    assert report.n_candidates == len(X.columns)
    assert "constant" in report.dropped_variance
    assert report.dropped_collinear["twin_of_signal"] == "signal"
    assert report.dropped_collinear["helper"] == "signal"
    assert len(report.importances) == 4  # 7 candidates - 1 variance - 2 collinear


def test_select_features_respects_max_features():
    """The cap is honoured even when more features carry signal."""
    rng = np.random.default_rng(3)
    base = np.linspace(0, 10, 36)
    data = {f"signal_{i}": base + rng.normal(0, 0.3, 36) for i in range(8)}
    data["pure_noise"] = rng.normal(0, 1, 36)
    X = pd.DataFrame(data)
    y = pd.Series(base)
    selected, report = select_features(
        X, y, _linear_spec(), max_features=4, seed=42, n_repeats=5
    )
    assert len(selected) <= 4
    assert report.max_features == 4


def test_select_features_minimum_floor():
    """With no informative column at all the floor keeps 3 features."""
    rng = np.random.default_rng(11)
    X = pd.DataFrame({f"n{i}": rng.normal(0, 1, 36) for i in range(6)})
    y = pd.Series(rng.normal(0, 1, 36))  # independent of X
    selected, report = select_features(
        X, y, _linear_spec(), max_features=15, seed=42, n_repeats=3
    )
    assert len(selected) >= 3
    assert any("piso" in note or "MIN_SELECTED" in note for note in report.notes)


def test_select_features_with_full_catalogue_is_fast_and_ordered():
    """The real catalogue runs on a toy frame and returns original column order."""
    X, y = _toy_frame(seed=2)
    selected, report = select_features(
        X, y, default_catalogue(seed=42), max_features=2, seed=42, n_repeats=3
    )
    assert set(selected) <= set(X.columns)
    assert selected == [c for c in X.columns if c in set(selected)]
    assert report.method == "variance+collinearity+permutation"
    payload = report.as_dict()
    assert payload["selected"] == selected
    assert payload["n_rows_calibration"] == len(X)
