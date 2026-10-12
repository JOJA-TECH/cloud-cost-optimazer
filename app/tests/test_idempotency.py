from src.sync.idempotency import (
    build_metric_reading_row,
    build_metric_sample_row,
    reading_dedup_key,
)


def test_dedup_key_is_stable_within_a_minute(cfg, reading_factory):
    a = reading_factory(second=1)
    b = reading_factory(second=59)
    assert reading_dedup_key(cfg, a) == reading_dedup_key(cfg, b)


def test_dedup_key_differs_by_metric_and_source(cfg, reading_factory):
    cpu = reading_factory(code="cpu_percent")
    other_metric = reading_factory(code="data_io_percent")
    other_source = reading_factory(source="sql_diagnostics")
    assert reading_dedup_key(cfg, cpu) != reading_dedup_key(cfg, other_metric)
    assert reading_dedup_key(cfg, cpu) != reading_dedup_key(cfg, other_source)


def test_build_metric_reading_row_fields(cfg, reading_factory):
    reading = reading_factory(second=37)
    row = build_metric_reading_row(cfg, "collector-uid", reading)
    assert row["experiment_uid"] == cfg.experiment_uid
    assert row["collector_run_uid"] == "collector-uid"
    assert row["resource_uid"] == cfg.resource_uid()
    assert row["observed_at_utc"].second == 0
    assert row["observed_at_utc"].minute == 0
    assert row["source_record_id"]


def test_build_metric_sample_row_is_deterministic(cfg, reading_factory):
    reading = reading_factory(
        code="query_duration_sample_ms", value=12.5, source="workload_observation"
    )
    first = build_metric_sample_row(cfg, reading)
    second = build_metric_sample_row(cfg, reading)
    assert first["source_sample_id"] == second["source_sample_id"]

    other = reading_factory(
        code="query_duration_sample_ms", value=99.0, source="workload_observation"
    )
    assert build_metric_sample_row(cfg, other)["source_sample_id"] != first["source_sample_id"]
