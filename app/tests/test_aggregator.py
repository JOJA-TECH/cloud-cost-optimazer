from src.metrics.aggregator import select_canonical


def _row(source, code, avg):
    return {
        "source": source,
        "metric_code": code,
        "avg_value": avg,
        "max_value": avg,
        "sum_value": avg,
        "n": 60,
    }


def test_canonical_prefers_dmv_for_cpu_percent():
    canonical, sources = select_canonical(
        [
            _row("sql_diagnostics", "cpu_percent", 40.0),
            _row("azure_monitor", "cpu_percent", 42.0),
        ]
    )
    assert sources["cpu_percent"] == "sql_diagnostics"
    assert canonical["cpu_percent"]["avg_value"] == 40.0


def test_canonical_prefers_dmv_for_io_metrics():
    for code in ("data_io_percent", "log_write_percent"):
        _, sources = select_canonical(
            [
                _row("sql_diagnostics", code, 40.0),
                _row("azure_monitor", code, 42.0),
            ]
        )
        assert sources[code] == "sql_diagnostics"


def test_canonical_prefers_azure_monitor_for_other_metrics():
    _, sources = select_canonical(
        [
            _row("sql_diagnostics", "sessions_count", 40.0),
            _row("azure_monitor", "sessions_count", 42.0),
        ]
    )
    assert sources["sessions_count"] == "azure_monitor"


def test_canonical_falls_back_when_preferred_source_absent():
    _, sources = select_canonical([_row("sql_diagnostics", "memory_percent", 10.0)])
    assert sources["memory_percent"] == "sql_diagnostics"


def test_canonical_keeps_one_source_per_metric_no_averaging():
    canonical, _ = select_canonical(
        [
            _row("azure_monitor", "sessions_count", 5.0),
            _row("sql_diagnostics", "sessions_count", 9.0),
        ]
    )
    # The canonical row is a single source's aggregate, never a blend of both.
    assert canonical["sessions_count"]["avg_value"] in (5.0, 9.0)
