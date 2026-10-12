"""Workload self-observation reader.

The workload generator appends one JSON line per operation to
``{LOCAL_SPOOL_DIR}/workload_obs/obs-<pid>.jsonl``. The collector drains those
files to obtain two things that no platform metric can give:

- individual operation durations -> ``raw.metric_sample`` with code
  ``query_duration_sample_ms`` (the ONLY exact-percentile source);
- the operation count over the window -> a ``workload_intensity`` reading
  (operations per minute, source ``workload_self``).

A file is only consumed once it has been idle for a couple of seconds, so a
writer that is still appending is never truncated mid-line.
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
        path = directory / f"obs-{os.getpid()}.jsonl"
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
) -> Tuple[List[MetricReading], Optional[MetricReading]]:
    directory = obs_dir(cfg)
    if not directory.exists():
        return [], None

    samples: List[MetricReading] = []
    count = 0
    now = time.time()
    for path in sorted(directory.glob("obs-*.jsonl")):
        try:
            if now - path.stat().st_mtime < IDLE_SECONDS:
                continue  # writer may still be appending
            lines = path.read_text(encoding="utf-8").splitlines()
        except OSError:
            continue
        for line in lines:
            line = line.strip()
            if not line:
                continue
            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                continue
            observed = _parse_ts(record.get("ts", ""))
            if observed is None:
                continue
            count += 1
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
        try:
            path.unlink(missing_ok=True)
        except OSError:
            pass

    intensity: Optional[MetricReading] = None
    if count:
        window_seconds = max(1.0, (window_end - window_start).total_seconds())
        ops_per_min = count * 60.0 / window_seconds
        intensity = make_reading(
            "ops_per_min",
            ops_per_min,
            window_end,
            cfg.capture_raw_interval_seconds,
            "workload_self",
            source_payload={"operations": count},
        )
    return samples, intensity
