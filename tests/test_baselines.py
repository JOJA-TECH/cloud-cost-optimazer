"""Tests for the baselines module."""

import numpy as np
import pandas as pd

from forecasting.baselines import (
    BaselineSpec,
    build_baselines,
    moving_average,
    naive_last,
    predict_baseline,
    seasonal_naive,
)


def test_naive_last():
    """naive_last returns the last observed value for all positions."""
    s = pd.Series([1.0, 2.0, 3.0, 4.0])
    out = naive_last(s)
    assert np.allclose(out, [1.0, 2.0, 3.0, 4.0])


def test_moving_average_window_3():
    """Moving average with window=3 uses the last 3 values."""
    s = pd.Series([1.0, 2.0, 3.0, 4.0, 5.0])
    out = moving_average(s, window=3)
    # i=0: mean([1]) = 1
    # i=1: mean([1,2]) = 1.5
    # i=2: mean([1,2,3]) = 2
    # i=3: mean([2,3,4]) = 3
    # i=4: mean([3,4,5]) = 4
    expected = np.array([1.0, 1.5, 2.0, 3.0, 4.0])
    assert np.allclose(out, expected)


def test_seasonal_naive_period_24():
    """Seasonal naive with period=24 looks back 24 steps from the target."""
    # Build a series where value at t is t. Target at t is value at t+1.
    # seasonal_naive predicts value at (t+1 - 24) = t - 23.
    s = pd.Series(range(50))  # 0..49
    out = seasonal_naive(s, period=24)
    # out[23] = s[23+1-24] = s[0] = 0
    # out[24] = s[24+1-24] = s[1] = 1
    assert np.isnan(out[22])
    assert out[23] == 0
    assert out[24] == 1


def test_predict_baseline_dispatch():
    """predict_baseline dispatches correctly."""
    s = pd.Series([1.0, 2.0, 3.0])
    spec = BaselineSpec("test", "naive_last", {})
    assert np.allclose(predict_baseline(spec, s), s)
    spec = BaselineSpec("test", "moving_average", {"window": 2})
    assert np.allclose(predict_baseline(spec, s), moving_average(s, 2))
    spec = BaselineSpec("test", "seasonal_naive", {"period": 1})
    assert np.allclose(predict_baseline(spec, s), seasonal_naive(s, 1))


def test_build_baselines_catalogue():
    """Default catalogue contains the three standard baselines."""
    s = pd.Series([1.0] * 10)
    cat = build_baselines(s)
    assert "naive_last" in cat
    assert "moving_average" in cat
    assert "seasonal_naive" in cat
    assert cat["moving_average"].params["window"] == 3
    assert cat["seasonal_naive"].params["period"] == 24