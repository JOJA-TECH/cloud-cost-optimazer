from src.metrics.normalizer import (
    METRIC_CATALOG,
    METRIC_CATALOG_VERSION,
    canonical_metric,
    classify,
    make_reading,
    participates_in_features,
    unit_for,
)


def test_canonical_metric_is_case_insensitive_and_maps_aliases():
    assert canonical_metric("CPU_PERCENT") == "cpu_percent"
    assert canonical_metric("operation_duration_ms") == "workload_operation_latency_ms"
    assert canonical_metric("avg_duration_ms") == "query_duration_avg_ms"
    assert canonical_metric("offered_ops") == "offered_operations"
    assert canonical_metric("successful_ops") == "successful_operations"


def test_canonical_metric_unknown_returns_none():
    assert canonical_metric("totally_unknown") is None
    assert canonical_metric("") is None


def test_classify_reports_reason():
    assert classify("cpu_percent") == ("cpu_percent", None)
    assert classify("totally_unknown") == (None, "unknown_metric")
    assert classify("") == (None, "empty_metric_name")


def test_catalog_version_is_a_string():
    assert isinstance(METRIC_CATALOG_VERSION, str)
    assert METRIC_CATALOG_VERSION


def test_unit_and_features():
    assert unit_for("cpu_percent") == "percent"
    assert unit_for("storage_used_bytes") == "bytes"
    assert unit_for("unknown") == "unknown"
    assert participates_in_features("cpu_percent") is True
    assert participates_in_features("dtu_consumption_percent") is False


def test_make_reading_drops_unknown_and_keeps_catalog_codes():
    import datetime as dt

    ts = dt.datetime(2024, 1, 1, tzinfo=dt.timezone.utc)
    assert make_reading("bogus_metric", 1.0, ts, 60, "test") is None
    reading = make_reading("cpu_percent", 5.0, ts, 60, "test")
    assert reading is not None
    assert reading.metric_code == "cpu_percent"
    assert reading.unit in {unit for unit, *_ in METRIC_CATALOG.values()}
