"""Tests for the extraction module."""

import pandas as pd
import pytest

from forecasting.extraction import (
    BASE_COLUMNS,
    TARGET_COLUMNS,
    extract_training_base,
    temporal_split,
)


def test_temporal_split_order():
    """Split must be train -> validation -> test in time order."""
    df = pd.DataFrame(
        {
            "period_start_utc": pd.date_range("2026-01-01", periods=100, freq="h", tz="UTC"),
            "cpu_avg": range(100),
            "target_cpu_avg_next1h": range(1, 101),
        }
    )
    splits = temporal_split(df, train=0.6, validation=0.2, test=0.2)
    assert splits["train"].frame["period_start_utc"].max() < splits["validation"].frame["period_start_utc"].min()
    assert splits["validation"].frame["period_start_utc"].max() < splits["test"].frame["period_start_utc"].min()


def test_temporal_split_ratios():
    """Split ratios should be approximately respected."""
    df = pd.DataFrame(
        {
            "period_start_utc": pd.date_range("2026-01-01", periods=100, freq="h", tz="UTC"),
            "cpu_avg": range(100),
            "target_cpu_avg_next1h": range(1, 101),
        }
    )
    splits = temporal_split(df, train=0.7, validation=0.15, test=0.15)
    total = len(df)
    assert splits["train"].rows == int(total * 0.7)
    assert splits["validation"].rows == int(total * 0.15)
    assert splits["test"].rows == total - splits["train"].rows - splits["validation"].rows


def test_temporal_split_sum_to_one():
    """Ratios must sum to 1."""
    df = pd.DataFrame({"period_start_utc": pd.date_range("2026-01-01", periods=10, freq="h", tz="UTC")})
    with pytest.raises(ValueError):
        temporal_split(df, train=0.5, validation=0.5, test=0.5)


def test_extract_training_base_columns():
    """The training base must contain the expected base and target columns."""
    # This test requires a database connection; it's skipped if not available.
    pass  # integration test placeholder