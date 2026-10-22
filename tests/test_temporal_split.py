"""Tests for temporal split functionality."""

import pandas as pd

from forecasting.extraction import temporal_split, walk_forward_splits


def test_temporal_split_train_before_val_before_test():
    """Time order must be train < validation < test."""
    df = pd.DataFrame(
        {
            "period_start_utc": pd.date_range("2026-01-01", periods=20, freq="h", tz="UTC"),
            "y": range(20),
        }
    )
    splits = temporal_split(df, train=0.5, validation=0.25, test=0.25)
    assert splits["train"].frame["period_start_utc"].max() < splits["validation"].frame["period_start_utc"].min()
    assert splits["validation"].frame["period_start_utc"].max() < splits["test"].frame["period_start_utc"].min()


def test_temporal_split_no_shuffle():
    """Split must not shuffle rows."""
    df = pd.DataFrame(
        {
            "period_start_utc": pd.date_range("2026-01-01", periods=20, freq="h", tz="UTC"),
            "y": range(20),
        }
    )
    splits = temporal_split(df, train=0.5, validation=0.25, test=0.25)
    # First train row should be the first row of the original frame
    assert splits["train"].frame["period_start_utc"].iloc[0] == df["period_start_utc"].iloc[0]


def test_walk_forward_splits_expanding():
    """Walk-forward splits expand the training window."""
    df = pd.DataFrame(
        {
            "period_start_utc": pd.date_range("2026-01-01", periods=20, freq="h", tz="UTC"),
            "y": range(20),
        }
    )
    splits = walk_forward_splits(df, n_splits=3, min_train=5, validation_size=3)
    assert len(splits) == 3
    # First split: train=5, val=3
    assert len(splits[0][0]) == 5
    assert len(splits[0][1]) == 3
    # Second split: train=8, val=3
    assert len(splits[1][0]) == 8
    assert len(splits[1][1]) == 3
    # Third split: train=11, val=3
    assert len(splits[2][0]) == 11
    assert len(splits[2][1]) == 3