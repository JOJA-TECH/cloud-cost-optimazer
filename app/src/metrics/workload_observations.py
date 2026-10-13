"""Workload self-observation reader.

The workload generator appends one JSON line per operation to
``{LOCAL_SPOOL_DIR}/workload_obs/obs-<pid>.jsonl``. The collector drains those
files to obtain two things that no platform metric can give:

- individual operation durations -> ``raw.metric_sample`` with code
  ``workload_operation_latency_ms`` (the ONLY exact-percentile source);
- the operation counts over the window -> ``workload_intensity`` and outcome
  readings (source ``workload_self``).

The writer appends continuously, so consumption is tracked by byte offset: a
complete-line prefix is drained on every cycle and any trailing partial line is
left for the next cycle. This keeps per-minute intensity/outcome coverage dense
without ever truncating a line mid-write. Fully drained, idle files are removed.
"""

from __future__ import annotations

import datetime as dt
import json
import logging
import os
import time
import uuid
from pathlib import Path
from typing import List, Optional, Tuple

from ..config import Config
from .normalizer import MetricReading, make_reading

logger = logging.getLogger(__name__)

OBS_SUBDIR = "workload_obs"
IDLE_SECONDS = 2.0
ROTATE_SECONDS = 60

# path -> (inode, consumed_offset). Module-level because the collector drains on
# every cycle from the same process; inode guards against file recreation.
_OFFSETS: dict = {}


def obs_dir(cfg: Config) -> Path:
    return Path(cfg.local_spool_dir) / OBS_SUBDIR


def append_observation(cfg: Config, op_key: str, duration_ms: float, ok: bool) -> None:
    """Best-effort append by the workload generator; never raises."""
    try:
        directory = obs_dir(cfg)
        directory.mkdir(parents=True, exist_ok=True)
        record = {
            "id": uuid.uuid4().hex,
            "ts": dt.datetime.now(dt.timezone.utc).isoformat(),
            "op": op_key,
            "duration_ms": float(duration_ms),
            "ok": bool(ok),
        }
        path = directory / f"obs-{os.getpid()}-{int(time.time() // ROTATE_SECONDS)}.jsonl"
        with path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(record) + "\n")
    except Exception as exc:  # noqa: BLE001 - observation is best-effort
        logger.debug("cannot append workload observation: %s", exc)


def _parse_ts(value: str) -> Optional[dt.datetime]:
    try:
        parsed = dt.datetime.fromisoformat(value)
    except (TypeError, ValueError):
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=dt.timezone.utc)
    return parsed


def drain(
    cfg: Config,
    window_start: dt.datetime,
    window_end: dt.datetime,
) -> Tuple[List[MetricReading], List[MetricReading]]:
    directory = obs_dir(cfg)
    if not directory.exists():
        return [], []

    samples: List[MetricReading] = []
    offered = 0
    successful = 0
    now = time.time()
    for path in sorted(directory.glob("obs-*.jsonl")):
        try:
            stat = path.stat()
        except OSError:
            continue
        inode = stat.st_ino
        size = stat.st_size
        key = str(path)
        previous = _OFFSETS.get(key)
        offset = 0
        if previous is not None and previous[0] == inode and previous[1] <= size:
            offset = previous[1]

        if size <= offset:
            # Nothing new. Clean up only once the writer has been idle.
            if now - stat.st_mtime >= IDLE_SECONDS:
                try:
                    path.unlink(missing_ok=True)
                except OSError:
                    pass
                _OFFSETS.pop(key, None)
            else:
                _OFFSETS[key] = (inode, offset)
            continue

        try:
            with path.open("rb") as handle:
                handle.seek(offset)
                chunk = handle.read(size - offset)
        except OSError:
            continue

        last_newline = chunk.rfind(b"\n")
        if last_newline < 0:
            _OFFSETS[key] = (inode, offset)  # no complete line yet
            continue

        new_offset = offset + last_newline + 1
        _OFFSETS[key] = (inode, new_offset)
        for raw_line in chunk[:new_offset - offset].split(b"\n"):
            raw_line = raw_line.strip()
            if not raw_line:
                continue
            try:
                record = json.loads(raw_line.decode("utf-8"))
            except (json.JSONDecodeError, UnicodeDecodeError):
                continue
            observed = _parse_ts(record.get("ts", ""))
            if observed is None:
                continue
            offered += 1
            if record.get("ok"):
                successful += 1
            duration = record.get("duration_ms")
            if duration is None:
                continue
            reading = make_reading(
                "operation_duration_ms",
                float(duration),
                observed,
                cfg.capture_raw_interval_seconds,
                "workload_observation",
                source_record_id=record.get("id"),
                source_payload={"op": record.get("op"), "ok": record.get("ok")},
            )
            if reading is not None:
                samples.append(reading)

        if new_offset >= size and now - stat.st_mtime >= IDLE_SECONDS:
            try:
                path.unlink(missing_ok=True)
            except OSError:
                pass
            _OFFSETS.pop(key, None)

    failed = offered - successful
    window_seconds = max(1.0, (window_end - window_start).total_seconds())
    stats: List[MetricReading] = []

    def _stat(raw_name: str, value: float, payload: dict) -> None:
        reading = make_reading(
            raw_name,
            value,
            window_end,
            cfg.capture_raw_interval_seconds,
            "workload_self",
            source_payload=payload,
        )
        if reading is not None:
            stats.append(reading)

    payload = {"operations": offered}
    _stat("ops_per_min", offered * 60.0 / window_seconds, payload)
    _stat("offered_ops", float(offered), payload)
    _stat("successful_ops", float(successful), payload)
    _stat("failed_ops", float(failed), payload)
    _stat("error_rate", (failed / offered) if offered else 0.0, payload)
    _stat("throughput_ops_per_second", successful / window_seconds, payload)
    return samples, stats
