import datetime as dt
import os
import time

from src.metrics import workload_observations as wo


def test_append_then_drain_produces_samples_and_intensity(cfg):
    wo._OFFSETS.clear()
    wo.append_observation(cfg, "point_lookup", 12.5, True)
    wo.append_observation(cfg, "insert_event", 30.0, False)

    directory = wo.obs_dir(cfg)
    files = list(directory.glob("obs-*.jsonl"))
    assert len(files) == 1
    old = time.time() - (wo.IDLE_SECONDS + 5)
    os.utime(files[0], (old, old))

    window_end = dt.datetime.now(dt.timezone.utc)
    window_start = window_end - dt.timedelta(seconds=60)
    samples, stats = wo.drain(cfg, window_start, window_end)

    durations = sorted(s.value for s in samples)
    assert durations == [12.5, 30.0]
    assert all(s.source == "workload_observation" for s in samples)
    assert all(s.metric_code == "workload_operation_latency_ms" for s in samples)

    by_code = {r.metric_code: r.value for r in stats}
    assert by_code["workload_intensity"] == 2.0  # 2 ops over 60s => 2 ops/min
    assert by_code["offered_operations"] == 2.0
    assert by_code["successful_operations"] == 1.0
    assert by_code["failed_operations"] == 1.0
    assert by_code["error_rate"] == 0.5
    assert all(r.source == "workload_self" for r in stats)

    assert list(directory.glob("obs-*.jsonl")) == []


def test_drain_reads_complete_lines_and_keeps_partial(cfg):
    wo._OFFSETS.clear()
    wo.append_observation(cfg, "aggregate", 1.0, True)
    path = list(wo.obs_dir(cfg).glob("obs-*.jsonl"))[0]
    with path.open("a", encoding="utf-8") as handle:
        handle.write('{"id": "partial"')  # no trailing newline

    window_end = dt.datetime.now(dt.timezone.utc)
    window_start = window_end - dt.timedelta(seconds=60)
    samples, stats = wo.drain(cfg, window_start, window_end)

    assert [s.value for s in samples] == [1.0]
    assert {r.metric_code: r.value for r in stats}["workload_intensity"] == 1.0
    # The complete line was consumed: a second drain of the same bytes emits no
    # samples again (the partial line is simply ignored for now).
    samples, _ = wo.drain(cfg, window_start, window_end)
    assert samples == []

    # A later cycle picks up the rest once the line is completed.
    with path.open("a", encoding="utf-8") as handle:
        handle.write('}\n')
    samples, stats = wo.drain(cfg, window_start, window_end)
    assert samples == []  # no duration_ms in the partial record
    assert {r.metric_code: r.value for r in stats}["offered_operations"] == 0.0


def test_drain_emits_zero_intensity_when_idle(cfg):
    wo._OFFSETS.clear()
    directory = wo.obs_dir(cfg)
    directory.mkdir(parents=True, exist_ok=True)
    window_end = dt.datetime.now(dt.timezone.utc)
    window_start = window_end - dt.timedelta(seconds=60)
    samples, stats = wo.drain(cfg, window_start, window_end)
    assert samples == []
    by_code = {r.metric_code: r.value for r in stats}
    assert by_code["workload_intensity"] == 0.0
    assert by_code["error_rate"] == 0.0
