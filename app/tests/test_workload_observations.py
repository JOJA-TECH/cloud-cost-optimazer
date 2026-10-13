import datetime as dt
import os
import time

from src.metrics import workload_observations as wo


def test_append_then_drain_produces_samples_and_intensity(cfg):
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


def test_drain_ignores_active_writer(cfg):
    wo.append_observation(cfg, "aggregate", 1.0, True)
    window_end = dt.datetime.now(dt.timezone.utc)
    window_start = window_end - dt.timedelta(seconds=60)
    samples, stats = wo.drain(cfg, window_start, window_end)
    assert samples == []
    assert stats == []
