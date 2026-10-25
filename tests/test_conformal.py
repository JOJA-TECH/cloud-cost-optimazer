"""Tests for the conformal prediction module."""

import numpy as np
import pandas as pd

from forecasting.conformal import (
    ConformalInterval,
    conformal_rank,
    jackknife_plus_interval,
    jackknife_plus_scores,
    split_conformal_interval,
)


def _mean_predictor(X_tr, y_tr, X_new):
    """Deterministic fit_predict: always predicts the training mean."""
    return np.full(len(X_new), float(np.mean(y_tr)))


def test_conformal_rank_finite_sample_correction():
    """k = ceil((n+1)*coverage), clamped to [1, n]."""
    # n=6, 80%: ceil(7*0.8) = 6 -> the largest score.
    assert conformal_rank(6, 0.80) == 6
    # n=36, 80%: ceil(37*0.8) = 30
    assert conformal_rank(36, 0.80) == 30
    # n=36, 90%: ceil(37*0.9) = 34
    assert conformal_rank(36, 0.90) == 34
    # tiny coverage clamps to 1.
    assert conformal_rank(6, 0.01) == 1
    # coverage beyond n/(n+1) saturates the rank.
    assert conformal_rank(6, 0.95) == 6


def test_conformal_rank_rejects_invalid_inputs():
    """Guard clauses of the rank helper."""
    for bad_n, bad_cov in [(0, 0.8), (6, 0.0), (6, 1.0), (6, -0.5), (6, 1.5)]:
        try:
            conformal_rank(bad_n, bad_cov)
        except ValueError:
            continue
        raise AssertionError(f"expected ValueError for n={bad_n}, coverage={bad_cov}")


def test_split_conformal_uses_kth_absolute_residual():
    """With n=6 and 80% the half width is the largest absolute residual."""
    residuals = [1.0, -3.0, 2.0, -0.5, 4.0, -2.0]
    interval = split_conformal_interval(residuals)
    assert interval.method == "split_conformal"
    assert interval.n_scores == 6
    assert interval.finite_sample_rank == 6
    assert np.isclose(interval.half_width, 4.0)
    assert np.isclose(interval.lower_offset, -4.0)
    assert np.isclose(interval.upper_offset, 4.0)
    assert not interval.clamped  # (6+1)*0.8 = 5.6 <= 6


def test_split_conformal_larger_calibration_set():
    """With n=36 and 80% the half width is the 30th sorted absolute residual."""
    rng = np.random.default_rng(42)
    residuals = rng.normal(0.0, 1.0, size=36)
    interval = split_conformal_interval(residuals)
    expected = float(np.sort(np.abs(residuals))[29])
    assert interval.finite_sample_rank == 30
    assert np.isclose(interval.half_width, expected)


def test_split_conformal_empty_residuals():
    """No calibration scores -> degenerate interval."""
    interval = split_conformal_interval([])
    assert interval.n_scores == 0
    assert interval.half_width == 0.0


def test_split_conformal_asymmetric_bonferroni():
    """Each tail is calibrated at alpha/2; joint level stays at 1-alpha."""
    residuals = np.linspace(-5.0, 5.0, 36)
    symmetric = split_conformal_interval(residuals)
    asymmetric = split_conformal_interval(residuals, symmetric=False)
    assert asymmetric.method == "split_conformal_asymmetric"
    # Bonferroni tails are at least as wide as the symmetric construction.
    assert asymmetric.lower_offset <= symmetric.lower_offset
    assert asymmetric.upper_offset >= symmetric.upper_offset
    assert asymmetric.lower_offset <= 0.0
    assert asymmetric.upper_offset >= 0.0


def test_split_conformal_coverage_guarantee_monte_carlo():
    """Empirical coverage never falls below the nominal level (exchangeable data)."""
    rng = np.random.default_rng(0)
    trials, n_cal, n_test = 1500, 36, 20
    covered = 0
    total = 0
    for _ in range(trials):
        calibration = rng.normal(0.0, 1.0, size=n_cal)
        fresh = rng.normal(0.0, 1.0, size=n_test)
        interval = split_conformal_interval(calibration)
        lower, upper = interval.apply(np.zeros(n_test))
        covered += int(np.sum((fresh >= lower) & (fresh <= upper)))
        total += n_test
    assert covered / total >= 0.80


def test_jackknife_plus_scores_with_mean_predictor():
    """LOO score of a mean predictor: |y_i - mean(y_{-i})|."""
    X = pd.DataFrame({"a": np.arange(8, dtype=float)})
    y = pd.Series([1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0])
    scores = jackknife_plus_scores(_mean_predictor, X, y)
    values = y.to_numpy()
    for i in range(len(y)):
        expected = abs(values[i] - float(np.mean(np.delete(values, i))))
        assert np.isclose(scores[i], expected)


def test_jackknife_plus_scores_too_few_rows():
    """Fewer than two rows -> no scores can be computed."""
    X = pd.DataFrame({"a": [1.0]})
    y = pd.Series([1.0])
    assert jackknife_plus_scores(_mean_predictor, X, y).size == 0


def test_jackknife_plus_interval_rank():
    """The half width is the k-th sorted score with the corrected rank."""
    scores = np.arange(1.0, 37.0)  # 1..36
    interval = jackknife_plus_interval(scores)
    assert interval.method == "jackknife_plus"
    assert interval.finite_sample_rank == 30
    assert np.isclose(interval.half_width, 30.0)
    assert interval.n_scores == 36


def test_apply_produces_symmetric_band():
    """apply() shifts the point predictions by the calibrated offsets."""
    residuals = [1.0, -2.0, 3.0, -4.0, 5.0, -6.0]
    interval = split_conformal_interval(residuals)
    lower, upper = interval.apply(np.array([10.0, 20.0]))
    assert np.allclose(lower, [4.0, 14.0])
    assert np.allclose(upper, [16.0, 26.0])


def test_as_dict_is_serialisable():
    """as_dict() exposes every field needed by the metadata artifacts."""
    interval = split_conformal_interval([0.5, -1.0, 2.0, -3.0, 4.0, -5.0])
    payload = interval.as_dict()
    assert payload["method"] == "split_conformal"
    assert payload["n_calibration_scores"] == 6
    assert payload["finite_sample_rank"] == 6
    assert isinstance(payload["guarantee"], str)
    assert isinstance(interval, ConformalInterval)
