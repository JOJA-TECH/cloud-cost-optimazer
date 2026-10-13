from src.metrics.quality import (
    compute_missing_fraction,
    coverage_by_metric,
    evaluate_reading,
    percentiles_from_values,
    window_quality,
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


def test_coverage_by_metric_covers_catalog():
    coverage = coverage_by_metric({"cpu_percent": 60}, 60)
    assert coverage["cpu_percent"] == 0.0
    assert coverage["data_io_percent"] == 1.0


def test_window_quality_missing_mandatory_is_not_available():
    coverage = coverage_by_metric({"cpu_percent": 60}, 60)
    status, warnings = window_quality(coverage)
    assert status == "not_available"
    assert "mandatory_metric_missing" in warnings


def test_window_quality_full_coverage_is_usable():
    counts = {code: 60 for code in coverage_by_metric({}, 60)}
    status, _ = window_quality(coverage_by_metric(counts, 60))
    assert status == "usable"
