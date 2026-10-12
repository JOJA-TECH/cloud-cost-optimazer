"""Workload generator service.

Runs one or more controlled phases against the Azure SQL `loadtest` schema,
honoring the state machine in loadtest.workload_control. Operations are bounded
(small batches, capped payloads) and only touch loadtest. Nothing destructive
or unbounded is ever executed.

Lifecycle:
- creates the workload_run in Supabase when a phase starts;
- streams updated counters (operations_total, errors_total) to Supabase;
- acknowledges PAUSING -> PAUSED when a batch completes;
- exits on STOPPING/STOPPED/FAILED and on SIGTERM/SIGINT (exit code 143/130);
- the final flush + STOPPED persistence is owned by src.shutdown.

Usage:
    python -m src.workload.runner --service            # systemd
    python -m src.workload.runner --run --class mixed --hours 6
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import logging
import random
import signal
import sys
import threading
import time
import uuid
from typing import Dict, List, Optional

from .. import db_azure, db_supabase
from ..config import Config, from_env, load_env_file
from ..metrics import workload_observations
from . import patterns
from .controller import WorkloadController

logger = logging.getLogger(__name__)


def _now() -> dt.datetime:
    return dt.datetime.now(dt.timezone.utc)


class PhaseConfig:
    def __init__(self, raw: dict, index: int):
        self.index = index
        self.workload_class = raw.get("workload_class", "mixed")
        self.duration_seconds = int(raw.get("duration_seconds", 6 * 3600))
        self.intensity = float(raw.get("intensity", 0.7))
        self.concurrency = int(raw.get("concurrency", 1))
        self.batch_size = int(raw.get("batch_size", 10))
        patterns.validate_class(self.workload_class)

    def as_dict(self) -> dict:
        return {
            "phase": self.index,
            "workload_class": self.workload_class,
            "duration_seconds": self.duration_seconds,
            "intensity": self.intensity,
            "concurrency": self.concurrency,
            "batch_size": self.batch_size,
        }


def default_schedule(cfg: Config) -> List[PhaseConfig]:
    """Default: a single `mixed` phase covering the experiment window."""
    return [
        PhaseConfig(
            {
                "workload_class": "mixed",
                "duration_seconds": cfg.experiment_duration_hours * 3600,
                "intensity": 0.7,
                "concurrency": 1,
                "batch_size": 10,
            },
            index=1,
        )
    ]


def load_schedule(cfg: Config) -> List[PhaseConfig]:
    raw = _env_json("WORKLOAD_SCHEDULE_JSON")
    if not raw:
        return default_schedule(cfg)
    return [PhaseConfig(item, idx) for idx, item in enumerate(raw, start=1)]


def _env_json(name: str) -> Optional[list]:
    import os

    value = os.getenv(name)
    if not value:
        return None
    try:
        parsed = json.loads(value)
        return parsed if isinstance(parsed, list) else None
    except json.JSONDecodeError:
        return None


class WorkloadRunner:
    def __init__(self, cfg: Config):
        self.cfg = cfg
        self.controller = WorkloadController(cfg)
        self.rng = random.Random(seed_for(cfg))
        self._stop_requested = threading.Event()
        self.status = "CREATED"

    # -- operations ----------------------------------------------------------
    def _op_point_lookup(self, conn, worker_id: int, batch_id: int, rng: random.Random):
        start = time.monotonic()
        db_azure.execute(
            conn,
            "SELECT TOP (1) fe.event_id, fe.created_at_utc FROM loadtest.fact_events fe "
            "WHERE fe.experiment_uid = ? ORDER BY fe.created_at_utc DESC",
            (self.cfg.experiment_uid,),
        )
        return True, (time.monotonic() - start) * 1000.0

    def _op_range_scan(self, conn, worker_id: int, batch_id: int, rng: random.Random):
        start = time.monotonic()
        end = _now()
        start_utc = end - dt.timedelta(seconds=3600)
        db_azure.execute(
            conn,
            "SELECT COUNT_BIG(*) FROM loadtest.fact_events fe "
            "WHERE fe.experiment_uid = ? AND fe.created_at_utc BETWEEN ? AND ?",
            (self.cfg.experiment_uid, start_utc, end),
        )
        return True, (time.monotonic() - start) * 1000.0

    def _op_insert(self, conn, worker_id: int, batch_id: int, rng: random.Random):
        start = time.monotonic()
        payload_size = rng.randint(64, 1024)
        payload = "".join(rng.choice("abcdefghijklmnopqrstuvwxyz0123456789") for _ in range(payload_size))
        digest = hashlib.sha256(payload.encode("utf-8")).hexdigest()
        with conn.cursor() as cur:
            cur.execute(
                "INSERT INTO loadtest.fact_events "
                "(event_id, experiment_uid, created_at_utc, operation_type, "
                " payload_size_bytes, batch_id, worker_id) "
                "OUTPUT INSERTED.event_id "
                "VALUES (NEWID(), ?, SYSUTCDATETIME(), N'insert_event', ?, ?, ?)",
                (self.cfg.experiment_uid, payload_size, batch_id, worker_id),
            )
            row = cur.fetchone()
        if row is not None:
            db_azure.execute(
                conn,
                "INSERT INTO loadtest.fact_events_payload (event_id, payload, payload_sha256) "
                "VALUES (?, ?, ?)",
                (row[0], payload, digest),
            )
        conn.commit()
        return True, (time.monotonic() - start) * 1000.0

    def _op_update(self, conn, worker_id: int, batch_id: int, rng: random.Random):
        start = time.monotonic()
        db_azure.execute(
            conn,
            "UPDATE TOP (1) loadtest.fact_events SET payload_size_bytes = payload_size_bytes + 1 "
            "WHERE experiment_uid = ?",
            (self.cfg.experiment_uid,),
        )
        conn.commit()
        return True, (time.monotonic() - start) * 1000.0

    def _op_aggregate(self, conn, worker_id: int, batch_id: int, rng: random.Random):
        start = time.monotonic()
        db_azure.execute(
            conn,
            "SELECT experiment_uid, COUNT_BIG(*) AS n, AVG(payload_size_bytes) AS avg_size "
            "FROM loadtest.fact_events WHERE experiment_uid = ? GROUP BY experiment_uid",
            (self.cfg.experiment_uid,),
        )
        return True, (time.monotonic() - start) * 1000.0

    OPS: Dict[str, object] = {}

    # -- phase bookkeeping -----------------------------------------------------
    def _register_workload_run(self, phase: PhaseConfig) -> str:
        uid = str(uuid.uuid4())
        with db_supabase.transaction(self.cfg) as conn:
            db_supabase.query(
                conn,
                "insert into experiment.workload_run "
                "(workload_run_uid, experiment_uid, workload_class, started_at_utc, "
                " duration_seconds, intensity, concurrency, pattern_params_json, "
                " operations_total, errors_total, status) "
                "values (%s, %s, %s, %s, %s, %s, %s, %s, 0, 0, 'RUNNING')",
                (
                    uid,
                    self.cfg.experiment_uid,
                    phase.workload_class,
                    _now(),
                    phase.duration_seconds,
                    phase.intensity,
                    phase.concurrency,
                    json.dumps(phase.as_dict()),
                ),
            )
        return uid

    def _write_workload_stats(self, run_uid: str, ops: int, errs: int, finished: bool, reason: str = ""):
        with db_supabase.transaction(self.cfg) as conn:
            db_supabase.query(
                conn,
                "update experiment.workload_run set "
                " operations_total = %s, errors_total = %s, "
                " finished_at_utc = case when %s then now() else finished_at_utc end, "
                " status = case when %s then 'FINISHED' else status end, "
                " stop_reason = coalesce(nullif(%s, ''), stop_reason) "
                "where workload_run_uid = %s",
                (ops, errs, finished, finished, reason, run_uid),
            )

    # -- worker loop -----------------------------------------------------------
    def _worker(self, run_uid: str, phase: PhaseConfig, stop_event: threading.Event,
                stats: dict, worker_id: int):
        rng = random.Random(seed_for(self.cfg) + run_uid.encode() + bytes([worker_id]))
        conn = db_azure.connect(self.cfg)
        try:
            ops_per_min = 15.0 * phase.intensity * phase.concurrency
            batch = max(1, phase.batch_size)

            while not stop_event.is_set() and not self._stop_requested.is_set():
                status = self.controller.azure_status()
                if status == "PAUSING":
                    try:
                        self.controller.acknowledge_paused()
                    except Exception:  # noqa: BLE001
                        pass
                    time.sleep(2)
                    continue
                if status in ("STOPPING", "STOPPED", "FAILED"):
                    break
                if status in ("CREATED", "PAUSED"):
                    time.sleep(5)
                    continue

                elapsed = time.monotonic() - stats["started_at"]
                fraction = min(1.0, elapsed / max(1.0, phase.duration_seconds))
                multiplier = patterns.shape_value(phase.workload_class, fraction)
                effective_rate = max(1.0, ops_per_min * multiplier)

                for i in range(batch):
                    if stop_event.is_set():
                        break
                    key, _ = patterns.pick_operation(rng, phase.workload_class)
                    handler = self.OPS.get(key)
                    if handler is None:
                        continue
                    try:
                        ok, _latency = handler(
                            conn, worker_id=worker_id, batch_id=stats["batch_seq"], rng=rng
                        )
                        workload_observations.append_observation(
                            self.cfg, key, _latency, ok
                        )
                        with stats["lock"]:
                            if ok:
                                stats["ops"] += 1
                            else:
                                stats["errs"] += 1
                    except Exception as exc:  # noqa: BLE001
                        logger.warning("operation %s failed: %s", key, exc)
                        with stats["lock"]:
                            stats["errs"] += 1
                        conn.rollback()
                    with stats["lock"]:
                        stats["batch_seq"] += 1

                sleep_seconds = max(0.2, (batch / effective_rate) * 60.0)
                stop_event.wait(sleep_seconds)
        finally:
            conn.close()

    def run_phases(self) -> int:
        self.controller.ensure_experiment(
            workload_class="schedule",
            random_seed=seed_for(self.cfg),
            config_json={"schedule": [p.as_dict() for p in self._schedule]},
        )
        logger.info("experiment %s ensured", self.cfg.experiment_uid)

        for phase in self._schedule:
            if self._stop_requested.is_set():
                break
            logger.info(
                "phase %d/%d class=%s duration=%ds intensity=%s concurrency=%d",
                phase.index, len(self._schedule), phase.workload_class,
                phase.duration_seconds, phase.intensity, phase.concurrency,
            )
            run_uid = self._register_workload_run(phase)
            stop_event = threading.Event()
            stats = {
                "ops": 0,
                "errs": 0,
                "batch_seq": 1,
                "started_at": time.monotonic(),
                "lock": threading.Lock(),
            }
            threads = [
                threading.Thread(
                    target=self._worker,
                    args=(run_uid, phase, stop_event, stats, w + 1),
                    daemon=True,
                    name=f"worker-{w}",
                )
                for w in range(max(1, phase.concurrency))
            ]
            for t in threads:
                t.start()

            phase_end = time.monotonic() + phase.duration_seconds
            last_report = time.monotonic()
            while time.monotonic() < phase_end:
                if self._stop_requested.is_set():
                    break
                status = self.controller.azure_status()
                if status in ("STOPPING", "STOPPED", "FAILED"):
                    break
                now = time.monotonic()
                if now - last_report > 30:
                    self._write_workload_stats(run_uid, stats["ops"], stats["errs"], False)
                    last_report = now
                time.sleep(2)

            stop_event.set()
            for t in threads:
                t.join(timeout=15)
            reason = "phase_completed"
            if self._stop_requested.is_set():
                reason = "stop_requested"
            elif self.controller.azure_status() in ("STOPPING", "STOPPED", "FAILED"):
                reason = self.controller.azure_status().lower()
            self._write_workload_stats(run_uid, stats["ops"], stats["errs"], True, reason)
            logger.info("phase %d finished: ops=%d errs=%d", phase.index, stats["ops"], stats["errs"])

        return 0

    # -- service entry ---------------------------------------------------------
    def serve(self) -> int:
        signal.signal(signal.SIGTERM, self._signal_handler)
        signal.signal(signal.SIGINT, self._signal_handler)
        logger.info("workload service started (experiment=%s)", self.cfg.experiment_uid)
        try:
            self.run_phases()
        except Exception as exc:  # noqa: BLE001
            logger.error("workload service failed: %s", exc)
            try:
                self.controller.mark_failed(reason=str(exc)[:200])
            except Exception:  # noqa: BLE001
                pass
            return 1
        return 0

    def _signal_handler(self, signum, _frame):
        logger.info("received signal %s; stopping workers", signum)
        self._stop_requested.set()


# Operation registry: template_key (from loadtest.query_templates) -> handler.
# All handlers are bounded and touch only the loadtest schema.
WorkloadRunner.OPS = {
    "point_lookup": WorkloadRunner._op_point_lookup,
    "range_scan": WorkloadRunner._op_range_scan,
    "insert_event": WorkloadRunner._op_insert,
    "update_row": WorkloadRunner._op_update,
    "aggregate": WorkloadRunner._op_aggregate,
}


def seed_for(cfg: Config) -> int:
    """Deterministic seed per experiment so phases are reproducible."""
    digest = hashlib.sha256(cfg.experiment_uid.encode("utf-8")).hexdigest()
    return int(digest[:8], 16)


def _setup_logging(verbose: bool = False) -> None:
    logging.basicConfig(
        level=logging.DEBUG if verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="CCO workload generator")
    parser.add_argument("--service", action="store_true", help="run as a systemd service")
    parser.add_argument("--run", action="store_true", help="single run (implies --service loop semantics)")
    parser.add_argument("--class", dest="workload_class", default=None)
    parser.add_argument("--hours", type=float, default=None)
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    _setup_logging(args.verbose)
    load_env_file()
    cfg = from_env()

    if args.workload_class:
        patterns.validate_class(args.workload_class)
        hours = args.hours if args.hours else cfg.experiment_duration_hours
        cfg_overrides = [PhaseConfig(
            {"workload_class": args.workload_class, "duration_seconds": int(hours * 3600)},
            index=1,
        )]
    else:
        cfg_overrides = load_schedule(cfg)

    runner = WorkloadRunner(cfg)
    runner._schedule = cfg_overrides
    return runner.serve()


if __name__ == "__main__":
    raise SystemExit(main())