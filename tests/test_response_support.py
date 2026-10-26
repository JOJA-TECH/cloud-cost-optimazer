"""Tests for the observed-support diagnostics of the response model."""

import numpy as np
import pandas as pd

from forecasting.config import Settings
from forecasting.synthetic.generator import evaluate_workload_support
from forecasting.synthetic.response_model import ResponseModel, ResponseTarget


def _toy_hourly(n: int = 40, seed: int = 0) -> pd.DataFrame:
    rng = np.random.default_rng(seed)
    times = pd.date_range("2026-10-13 02:00", periods=n, freq="h", tz="UTC")
    workload = np.clip(rng.normal(150.0, 40.0, n), 80.0, None)
    return pd.DataFrame(
        {
            "period_start_utc": times,
            "workload_intensity": workload,
            # Deliberately independent of the workload: mirrors limit #10.
            "cpu_avg": np.clip(rng.normal(5.0, 2.0, n), 0.0, 100.0),
        }
    )


def test_fit_records_observed_support_and_correlations():
    """The fitted model exposes the workload window it was calibrated on."""
    model = ResponseModel.fit(
        _toy_hourly(),
        Settings(),
        targets=[ResponseTarget("cpu_avg", 0.0, 100.0, "percent")],
    )
    support = model.observed_support["workload_intensity"]
    assert support[0] <= support[1]
    assert 80.0 <= support[0] and support[1] <= max(150.0 + 40.0 * 4, support[1])

    per_target = model.metadata()["per_target"]["cpu_avg"]
    assert "corr_pearson_with_feature" in per_target
    assert "corr_spearman_with_feature" in per_target
    # cpu_avg is independent of the workload by construction: near-zero corr.
    assert abs(per_target["corr_pearson_with_feature"]) < 0.5
    assert "observed_support" in model.metadata()


def test_fit_support_matches_the_feature_values():
    """The recorded support is exactly the min/max of the fitting column."""
    frame = _toy_hourly(seed=3)
    model = ResponseModel.fit(
        frame,
        Settings(),
        targets=[ResponseTarget("cpu_avg", 0.0, 100.0, "percent")],
    )
    low, high = model.observed_support["workload_intensity"]
    assert np.isclose(low, frame["workload_intensity"].min())
    assert np.isclose(high, frame["workload_intensity"].max())


def test_evaluate_support_detects_extrapolation():
    """A scenario outside the observed window is flagged."""
    model = ResponseModel.fit(
        _toy_hourly(),
        Settings(),
        targets=[ResponseTarget("cpu_avg", 0.0, 100.0, "percent")],
    )
    inside = evaluate_workload_support(
        pd.DataFrame({"workload_intensity": [100.0, 120.0, 150.0]}), model
    )
    outside = evaluate_workload_support(
        pd.DataFrame({"workload_intensity": [100.0, 900.0]}), model
    )
    assert inside["within_observed_support"] is True
    assert outside["within_observed_support"] is False
    assert outside["scenario_max"] == 900.0
    assert outside["observed_min"] == inside["observed_min"]


def test_evaluate_support_without_data_is_neutral():
    """Missing curve column or empty support yields a neutral (None) verdict."""
    model = ResponseModel.fit(
        _toy_hourly(),
        Settings(),
        targets=[ResponseTarget("cpu_avg", 0.0, 100.0, "percent")],
    )
    missing_column = evaluate_workload_support(pd.DataFrame({"other": [1.0]}), model)
    assert missing_column["within_observed_support"] is None

    empty_model = ResponseModel()
    no_support = evaluate_workload_support(
        pd.DataFrame({"workload_intensity": [50.0, 500.0]}), empty_model
    )
    assert no_support["within_observed_support"] is None
