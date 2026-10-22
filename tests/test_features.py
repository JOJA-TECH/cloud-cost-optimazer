"""Tests for the features module."""

import numpy as np
import pandas as pd
import pytest

from forecasting.features import (
    FeatureSpec,
    add_derived_features,
    build_modeling_frame,
    dataset_hash,
    feature_columns,
    feature_definition_hash,
    impute_optional,
    intensity_band,
    peak_band,
)


def test_add_derived_features_lag_no_future():
    """Lag features must not look at future rows."""
    df = pd.DataFrame(
        {
            "period_start_utc": pd.date_range("2026-01-01", periods=10, freq="h", tz="UTC"),
            "cpu_avg": [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0],
            "workload_intensity": [100] * 10,
        }
    )
    enriched = add_derived_features(df)
    # cpu_avg_lag_1h at row i should equal cpu_avg at row i-1
    assert enriched.loc[1, "cpu_avg_lag_1h"] == 1.0
    assert enriched.loc[5, "cpu_avg_lag_1h"] == 5.0
    # First row has no lag -> NaN
    assert np.isnan(enriched.loc[0, "cpu_avg_lag_1h"])


def test_add_derived_features_rolling_includes_current():
    """Rolling windows include the current row (which is observed at forecast time)."""
    df = pd.DataFrame(
        {
            "period_start_utc": pd.date_range("2026-01-01", periods=10, freq="h", tz="UTC"),
            "cpu_avg": [1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0],
            "workload_intensity": [100] * 10,
        }
    )
    enriched = add_derived_features(df)
    # rolling_3h at row 2 = mean of rows 0,1,2 = (1+2+3)/3 = 2.0
    assert np.isclose(enriched.loc[2, "cpu_avg_rolling_3h"], 2.0)
    # rolling_3h at row 0 has only 1 value -> NaN (min_periods=window)
    assert np.isnan(enriched.loc[0, "cpu_avg_rolling_3h"])
    # rolling_3h at row 1 has 2 values -> NaN
    assert np.isnan(enriched.loc[1, "cpu_avg_rolling_3h"])


def test_intensity_band():
    """Workload intensity bucketing."""
    s = pd.Series([50.0, 150.0, 300.0, 600.0, 1200.0, np.nan])
    bands = intensity_band(s)
    assert bands.iloc[0] == "lt_100"
    assert bands.iloc[1] == "lt_250"
    assert bands.iloc[2] == "lt_500"
    assert bands.iloc[3] == "lt_1000"
    assert bands.iloc[4] == "gte_1000"
    assert bands.iloc[5] == "unknown"


def test_peak_band():
    """Peak vs non-peak hour bucketing."""
    hours = pd.Series([8, 9, 17, 18, 0, 12])
    assert peak_band(hours).tolist() == ["non_peak", "peak", "peak", "non_peak", "non_peak", "peak"]


def test_build_modeling_frame_drops_missing_required():
    """Rows missing a required feature are dropped."""
    df = pd.DataFrame(
        {
            "period_start_utc": pd.date_range("2026-01-01", periods=5, freq="h", tz="UTC"),
            "cpu_avg": [1.0, 2.0, np.nan, 4.0, 5.0],
            "workload_intensity": [100, 100, 100, 100, 100],
            "data_io_avg": [0.0, 0.0, 0.0, 0.0, 0.0],
        }
    )
    spec = FeatureSpec(required=("cpu_avg", "workload_intensity"), optional=("data_io_avg",))
    out = build_modeling_frame(df, "target", spec)
    assert len(out) == 4  # row 2 dropped


def test_impute_optional_train_median():
    """Optional features imputed with train median and missing flag added."""
    train = pd.DataFrame(
        {
            "cpu_avg": [1.0, 2.0, 3.0],
            "data_io_avg": [0.0, 0.0, 0.0],
            "workload_intensity": [100, 100, 100],
        }
    )
    target = pd.DataFrame(
        {
            "cpu_avg": [4.0, 5.0],
            "data_io_avg": [np.nan, np.nan],
            "workload_intensity": [100, 100],
        }
    )
    out, report = impute_optional(train, target, ["data_io_avg"])
    assert np.isclose(out["data_io_avg"].iloc[0], 0.0)
    assert out["feature_missing_data_io_avg"].sum() == 2
    assert not report["imputed"]["data_io_avg"]["dropped"]


def test_dataset_hash_deterministic():
    """Same frame yields same hash."""
    df = pd.DataFrame({"a": [1, 2], "b": [3.0, 4.0]})
    h1 = dataset_hash(df)
    h2 = dataset_hash(df)
    assert h1 == h2
    assert len(h1) == 64  # sha256 hex


def test_feature_definition_hash():
    """Hash is stable across runs."""
    cols = ["cpu_avg", "cpu_avg_lag_1h"]
    spec = FeatureSpec()
    h1 = feature_definition_hash(cols, spec)
    h2 = feature_definition_hash(cols, spec)
    assert h1 == h2
    assert len(h1) == 64


def test_feature_columns_order():
    """Returned columns include required, optional, derived, and missing flags."""
    df = pd.DataFrame(
        {
            "cpu_avg": [1.0, 2.0],
            "workload_intensity": [100, 100],
            "cpu_avg_lag_1h": [np.nan, 1.0],
            "cpu_avg_rolling_3h": [np.nan, np.nan],
            "feature_missing_data_io_avg": [0, 1],
            "hour_sin": [0.0, 0.0],
        }
    )
    spec = FeatureSpec()
    cols = feature_columns(df, spec)
    assert "cpu_avg" in cols
    assert "workload_intensity" in cols
    assert "cpu_avg_lag_1h" in cols
    assert "feature_missing_data_io_avg" in cols
    assert "hour_sin" in cols