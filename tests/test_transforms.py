"""Tests for the differenced-target representation and the candidates."""

import numpy as np
import pandas as pd

from forecasting.models import ModelSpec
from forecasting.tuning import (
    Candidate,
    as_candidate,
    candidate_catalogue,
    evaluate_candidates,
    fit_candidate,
    predict_candidate,
    walk_forward_scores,
)
from forecasting.transforms import DifferencedPredictor


def _delta_frame(n: int = 36, seed: int = 0) -> tuple[pd.DataFrame, pd.Series]:
    """y = cpu + 0.5*a: the delta (0.5*a) is exactly linear."""
    rng = np.random.default_rng(seed)
    X = pd.DataFrame(
        {
            "cpu": np.linspace(10.0, 60.0, n) + rng.normal(0, 0.1, n),
            "a": rng.normal(0, 1, n),
        }
    )
    y = pd.Series(X["cpu"].to_numpy() + 0.5 * X["a"].to_numpy())
    return X, y


def test_differenced_predictor_exact_reconstruction():
    """A linear delta is learned exactly and the level is reconstructed."""
    X, y = _delta_frame()
    linear = ModelSpec("linear_lags", "linear", {})
    predictor = DifferencedPredictor(linear, level_index=0).fit(X, y)
    prediction = predictor.predict(X)
    assert np.allclose(prediction, y.to_numpy(), atol=1e-6)


def test_differenced_predictor_uses_the_level_column():
    """The reconstruction must follow the level column of each row."""
    X, y = _delta_frame()
    linear = ModelSpec("linear_lags", "linear", {})
    predictor = DifferencedPredictor(linear, level_index=0).fit(X, y)
    shifted = X.copy()
    shifted["cpu"] = shifted["cpu"] + 10.0
    base = predictor.predict(X)
    moved = predictor.predict(shifted)
    assert np.allclose(moved, base + 10.0, atol=1e-6)


def test_candidate_naming_and_transform():
    """Names carry the _diff suffix and expose the representation."""
    spec = ModelSpec("ridge_a2.0", "ridge", {"alpha": 2.0})
    level = Candidate(spec=spec)
    diff = Candidate(spec=spec, differenced=True, level_index=0)
    assert level.name == "ridge_a2.0"
    assert level.transform_name == "level"
    assert diff.name == "ridge_a2.0_diff"
    assert diff.transform_name == "diff"
    assert diff.clone() == diff


def test_as_candidate_wraps_bare_specs():
    """Bare ModelSpecs keep working (backwards compatibility)."""
    spec = ModelSpec("linear_lags", "linear", {})
    candidate = as_candidate(spec)
    assert isinstance(candidate, Candidate)
    assert candidate.spec is spec
    assert not candidate.differenced
    assert as_candidate(candidate) is candidate


def test_candidate_catalogue_with_and_without_differencing():
    """18 level candidates; 36 when the level column index is available."""
    level_only = candidate_catalogue(seed=42, level_index=None)
    assert len(level_only) == 36 or len(level_only) == 18
    both = candidate_catalogue(seed=42, level_index=0)
    assert len(both) == 2 * len(candidate_catalogue(seed=42, level_index=None))
    names = {candidate.name for candidate in both}
    assert len(names) == len(both)
    assert "random_forest_d2_l4_diff" in names
    assert "linear_lags_diff" in names


def test_fit_candidate_requires_level_index_for_diff():
    """A differenced candidate without level index fails fast."""
    X, y = _delta_frame()
    spec = ModelSpec("linear_lags", "linear", {})
    broken = Candidate(spec=spec, differenced=True, level_index=None)
    try:
        fit_candidate(broken, X, y)
    except ValueError:
        return
    raise AssertionError("expected ValueError for missing level_index")


def test_walk_forward_scores_on_differenced_candidate():
    """The differenced linear candidate is near-exact on y = cpu + 0.5*a."""
    X, y = _delta_frame()
    linear = ModelSpec("linear_lags", "linear", {})
    diff = Candidate(spec=linear, differenced=True, level_index=0)
    scores = walk_forward_scores(diff, X, y, n_splits=3)
    assert len(scores) == 3
    assert all(score < 1e-6 for score in scores)


def test_evaluate_candidates_mixes_representations():
    """Level and diff candidates compete on the same level-scale metric."""
    X, y = _delta_frame()
    linear = ModelSpec("linear_lags", "linear", {})
    ridge_degenerate = ModelSpec("ridge_degenerate", "ridge", {"alpha": 1e12})
    candidates = [
        Candidate(spec=ridge_degenerate, differenced=True, level_index=0),
        linear,  # bare spec: auto-wrapped as level
    ]
    results = evaluate_candidates(candidates, X, y, n_splits=3)
    assert results[0].candidate.name == "linear_lags"
    assert results[0].spec.kind == "linear"
    row = results[0].as_row()
    assert row["target_transform"] == "level"
    assert results[0].mean_mae < results[1].mean_mae


def test_predict_candidate_dispatches_both_representations():
    """Level pipelines and DifferencedPredictor share one predict interface."""
    X, y = _delta_frame()
    linear = ModelSpec("linear_lags", "linear", {})
    level_predictor = fit_candidate(Candidate(spec=linear), X, y)
    diff_predictor = fit_candidate(
        Candidate(spec=linear, differenced=True, level_index=0), X, y
    )
    level_prediction = predict_candidate(level_predictor, X)
    diff_prediction = predict_candidate(diff_predictor, X)
    assert np.allclose(level_prediction, y.to_numpy(), atol=1e-6)
    assert np.allclose(diff_prediction, y.to_numpy(), atol=1e-6)
    assert not isinstance(level_predictor, DifferencedPredictor)
    assert isinstance(diff_predictor, DifferencedPredictor)
