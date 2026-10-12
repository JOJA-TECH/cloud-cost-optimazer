from src.metrics.quality import (
    compute_missing_fraction,
    evaluate_reading,
    percentiles_from_values,
)


def test_evaluate_reading_valid():
    assert evaluate_reading("cpu_percent", 50.0, "percent") == ("usable", [])


def test_evaluate_reading_missing_value():
    status, warnings = evaluate_reading("cpu_percent", None, "percent")
    assert status == "not_available"
    assert "missing_value" in warnings


def test_evaluate_reading_unknown_metric_is_invalid():
    status, warnings = evaluate_reading("nope", 1.0, "percent")
    assert status == "invalid"
    assert warnings == ["unknown_metric:nope"]


def test_evaluate_reading_slightly_out_of_range_is_degraded():
    status, warnings = evaluate_reading("cpu_percent", 104.0, "percent")
    assert status == "degraded"
    assert "below_expected_range" not in warnings


def test_evaluate_reading_far_out_of_range_is_invalid():
    status, _ = evaluate_reading("cpu_percent", 200.0, "percent")
    assert status == "invalid"


def test_percentiles_interpolate():
    result = percentiles_from_values([1, 2, 3, 4], points=(50.0,))
    assert result[50.0] == 2.5


def test_percentiles_empty_is_none_not_invented():
    result = percentiles_from_values([], points=(50.0, 95.0))
    assert result == {50.0: None, 95.0: None}


def test_compute_missing_fraction():
    assert compute_missing_fraction(60, 60) == 0.0
    assert compute_missing_fraction(30, 60) == 0.5
    assert compute_missing_fraction(0, 60) == 1.0
