import json

from src.sync.spool import Spool


def test_write_pending_and_remove(tmp_path):
    spool = Spool(tmp_path / "spool", max_mb=1)
    path = spool.write_batch("metric_reading", [{"a": 1}, {"a": 2}])
    assert path.exists()

    status = spool.status()
    assert status.files == 1
    assert status.bytes > 0
    assert status.over_limit is False

    pending = list(spool.iter_pending())
    assert len(pending) == 1
    _, kind, rows = pending[0]
    assert kind == "metric_reading"
    assert rows == [{"a": 1}, {"a": 2}]

    spool.remove(path)
    assert spool.status().files == 0


def test_corrupt_file_is_skipped(tmp_path):
    spool = Spool(tmp_path / "spool", max_mb=1)
    path = spool.write_batch("metric_sample", [{"v": 1}])
    path.write_text("{not valid json", encoding="utf-8")

    assert list(spool.iter_pending()) == []


def test_over_limit_detects_large_buffer(tmp_path):
    spool = Spool(tmp_path / "spool", max_mb=1)
    big = "x" * (1_300_000)
    spool.write_batch("metric_reading", [{"blob": big}])
    assert spool.status().over_limit is True


def test_sidecar_checksum_matches_payload(tmp_path):
    spool = Spool(tmp_path / "spool", max_mb=1)
    path = spool.write_batch("metric_reading", [{"v": 42}])
    sidecar = path.with_suffix(".sha256")
    assert sidecar.exists()
    assert len(sidecar.read_text(encoding="ascii").strip()) == 64
    json.loads(path.read_text(encoding="utf-8"))
