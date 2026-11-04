"""Workload generator service.

Ejecuta fases W0-W9 contra la base de datos Azure SQL `loadtest` con control de
carga en abierto (open-loop): la tasa ofrecida es independiente del tiempo de
respuesta y nunca se multiplica por la concurrencia.

Cada observación lleva trazabilidad completa:
experiment_uid, resource_uid, workload_run_uid, config_snapshot_id,
workload_class, fase, timestamp UTC, operación y origen.

Los fallos SQL generan una observación con error y latencia/tiempo de espera.

Uso:
    python -m src.workload.runner --service            # systemd
    python -m src.workload.runner --run --class W1 --hours 1
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

INTERVAL_LABELS = ("warmup", "measure", "cooldown")
W8 = "W8"


def _now() -> dt.datetime:
    return dt.datetime.now(dt.timezone.utc)


class PhaseConfig:
    def __init__(self, raw: dict, index: int, config_snapshot_id: str):
        self.index = index
        self.workload_class = raw.get("workload_class", "mixed")
        patterns.validate_class(self.workload_class)
        self.duration_seconds = int(raw.get("duration_seconds", 6 * 3600))
        self.intensity = float(raw.get("intensity", 0.7))
        self.concurrency = max(1, int(raw.get("concurrency", 1)))
        self.batch_size = max(1, int(raw.get("batch_size", 1)))
        # Tasa ofrecida total (ops/min), independiente de la concurrencia.
        self.offered_rate_ops_per_min = float(
            raw.get("offered_rate_ops_per_min", 15.0 * self.intensity)
        )
        self.warmup_seconds = int(raw.get("warmup_seconds", 60))
        self.measure_seconds = int(raw.get("measure_seconds", self.duration_seconds))
        self.cooldown_seconds = int(raw.get("cooldown_seconds", 60))
        self.pause_seconds = int(raw.get("pause_seconds", 0))
        self.storage_cap_bytes = raw.get("storage_cap_bytes")  # tope W8
        self.config_snapshot_id = config_snapshot_id

    def as_dict(self) -> dict:
        return {
            "phase": self.index,
            "workload_class": self.workload_class,
            "duration_seconds": self.duration_seconds,
            "intensity": self.intensity,
            "concurrency": self.concurrency,
            "batch_size": self.batch_size,
            "offered_rate_ops_per_min": self.offered_rate_ops_per_min,
            "warmup_seconds": self.warmup_seconds,
            "measure_seconds": self.measure_seconds,
            "cooldown_seconds": self.cooldown_seconds,
            "pause_seconds": self.pause_seconds,
            "storage_cap_bytes": self.storage_cap_bytes,
            "config_snapshot_id": self.config_snapshot_id,
        }


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


def config_snapshot_id(schedule: List[dict]) -> str:
    """Identificador determinista del snapshot de configuración de carga."""
    material = json.dumps(schedule, sort_keys=True)
    return hashlib.sha256(material.encode("utf-8")).hexdigest()[:16]


def default_schedule(cfg: Config) -> List[PhaseConfig]:
    """Piloto: humo corto W0 (línea base) + W1 (alta estable) + pausa."""
    snapshot = config_snapshot_id(
        [
            {"workload_class": "low_stable", "duration_seconds": 5 * 60},
            {"workload_class": "high_stable", "duration_seconds": 5 * 60},
        ]
    )
    return [
        PhaseConfig(
            {
                "workload_class": "low_stable",
                "duration_seconds": 5 * 60,
                "intensity": 0.2,
                "concurrency": 1,
                "batch_size": 1,
                "offered_rate_ops_per_min": 5.0,
            },
            index=1,
            config_snapshot_id=snapshot,
        ),
        PhaseConfig(
            {
                "workload_class": "high_stable",
                "duration_seconds": 5 * 60,
                "intensity": 0.7,
                "concurrency": 1,
                "batch_size": 1,
                "offered_rate_ops_per_min": 30.0,
            },
            index=2,
            config_snapshot_id=snapshot,
        ),
    ]


def load_schedule(cfg: Config) -> List[PhaseConfig]:
    raw = _env_json("WORKLOAD_SCHEDULE_JSON")
    if not raw:
        return default_schedule(cfg)
    snapshot = config_snapshot_id(raw)
    return [PhaseConfig(item, idx, snapshot) for idx, item in enumerate(raw, start=1)]


def seed_for_phase(cfg: Config, phase_index: int) -> int:
    """Semilla determinista por experimento + fase (persistida, nunca aleatoria)."""
    digest = hashlib.sha256(
        f"{cfg.experiment_uid}:{phase_index}".encode("utf-8")
    ).hexdigest()
    # SQL Server `int` is signed 32-bit; keep the seed in range.
    return int(digest[:8], 16) & 0x7FFFFFFF


class WorkloadRunner:
    def __init__(self, cfg: Config):
        self.cfg = cfg
        self.controller = WorkloadController(cfg)
        self._stop_requested = threading.Event()
        self.status = "CREATED"
        self._schedule: List[PhaseConfig] = []

    # -- operaciones (solo tocan loadtest, acotadas) -----------------------
    def _op_point_lookup(self, conn, rng: random.Random):
        start = time.monotonic()
        db_azure.execute(
            conn,
            "SELECT TOP (1) fe.event_id FROM loadtest.fact_events fe "
            "WHERE fe.experiment_uid = ? ORDER BY fe.created_at_utc DESC",
            (self.cfg.experiment_uid,),
        )
        return True, (time.monotonic() - start) * 1000.0

    def _op_range_scan(self, conn, rng: random.Random):
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

    def _op_insert(self, conn, rng: random.Random):
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
                (self.cfg.experiment_uid, payload_size, 0, 0),
            )
            row = cur.fetchone()
        if row is not None:
            with conn.cursor() as cur:
                cur.execute(
                    "INSERT INTO loadtest.fact_events_payload (event_id, payload, payload_sha256) "
                    "VALUES (?, ?, ?)",
                    (row[0], payload, digest),
                )
        conn.commit()
        return True, (time.monotonic() - start) * 1000.0

    def _op_update(self, conn, rng: random.Random):
        start = time.monotonic()
        db_azure.execute(
            conn,
            "UPDATE TOP (1) loadtest.fact_events SET payload_size_bytes = payload_size_bytes + 1 "
            "WHERE experiment_uid = ?",
            (self.cfg.experiment_uid,),
        )
        conn.commit()
        return True, (time.monotonic() - start) * 1000.0

    def _op_aggregate(self, conn, rng: random.Random):
        start = time.monotonic()
        db_azure.execute(
            conn,
            "SELECT experiment_uid, COUNT_BIG(*) AS n, AVG(payload_size_bytes) AS avg_size "
            "FROM loadtest.fact_events WHERE experiment_uid = ? GROUP BY experiment_uid",
            (self.cfg.experiment_uid,),
        )
        return True, (time.monotonic() - start) * 1000.0

    OPS: Dict[str, str] = {}

    # -- registro de fases --------------------------------------------------
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

    def _write_workload_stats(
        self, run_uid: str, ops: int, errs: int, finished: bool, reason: str = ""
    ):
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

    # -- intervalo de la fase (warmup/measure/cooldown) ---------------------
    def _phase_interval(self, elapsed: float, phase: PhaseConfig) -> str:
        if elapsed <= phase.warmup_seconds:
            return "warmup"
        if elapsed <= phase.warmup_seconds + phase.measure_seconds:
            return "measure"
        return "cooldown"

    # -- bucle de worker (tasa ofrecida open-loop) --------------------------
    def _worker(
        self,
        run_uid: str,
        phase: PhaseConfig,
        stop_event: threading.Event,
        stats: dict,
        worker_id: int,
        phase_start: float,
        rng: random.Random,
    ):
        conn = db_azure.connect(self.cfg)
        try:
            # Cada worker ofrece offered_rate / concurrency ops/min (open-loop).
            worker_rate = max(0.1, phase.offered_rate_ops_per_min / phase.concurrency)
            seconds_per_op = 60.0 / worker_rate
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

                elapsed = time.monotonic() - phase_start
                if elapsed >= phase.duration_seconds:
                    break

                # Tope W8: si hay storage_cap y se acerca, detener la fase.
                if phase.storage_cap_bytes is not None:
                    try:
                        used = db_azure.scalar(
                            conn,
                            "SELECT SUM(size) * 8192 FROM sys.database_files "
                            "WHERE type = 0 AND name NOT LIKE N'model%'",
                        )
                        if used is not None and int(used) >= int(phase.storage_cap_bytes):
                            logger.warning(
                                "W8 storage cap alcanzado (%s >= %s); deteniendo fase %d",
                                used, phase.storage_cap_bytes, phase.index,
                            )
                            with stats["lock"]:
                                stats["stopped_by_cap"] = True
                            break
                    except Exception:  # noqa: BLE001
                        pass

                # Tope de operaciones ofrecidas en este ciclo.
                for _ in range(batch):
                    if stop_event.is_set() or self._stop_requested.is_set():
                        break
                    op_start = time.monotonic()
                    key, _ = patterns.pick_operation(rng, phase.workload_class)
                    handler_name = self.OPS.get(key)
                    handler = getattr(self, handler_name, None) if handler_name else None
                    if handler is None:
                        with stats["lock"]:
                            stats["offered"] += 1
                        continue
                    with stats["lock"]:
                        stats["offered"] += 1
                        stats["started"] += 1
                    ok = False
                    latency = 0.0
                    try:
                        ok, latency = handler(conn, rng=rng)
                        with stats["lock"]:
                            if ok:
                                stats["completed"] += 1
                            else:
                                stats["failed"] += 1
                    except Exception as exc:  # noqa: BLE001
                        logger.warning("operación %s falló: %s", key, exc)
                        try:
                            conn.rollback()
                        except Exception:  # noqa: BLE001
                            pass
                        with stats["lock"]:
                            stats["failed"] += 1
                        latency = (time.monotonic() - op_start) * 1000.0
                    op_elapsed = time.monotonic() - op_start
                    if op_elapsed > seconds_per_op:
                        with stats["lock"]:
                            stats["delayed"] += 1
                    workload_observations.append_observation(
                        self.cfg,
                        key,
                        latency,
                        ok,
                        workload_run_uid=run_uid,
                        phase=phase.index,
                        workload_class=phase.workload_class,
                        config_snapshot_id=phase.config_snapshot_id,
                        origin="worker",
                        resource_uid=self.cfg.primary_resource_id(),
                    )
                    with stats["lock"]:
                        stats["batch_seq"] += 1

                # Open-loop: dormir el resto del intervalo por operación, sin
                # depender del tiempo de respuesta (la tasa ofrecida es fija).
                sleep_for = max(0.0, seconds_per_op - (time.monotonic() - op_start))
                if sleep_for > 0:
                    stop_event.wait(sleep_for)
        finally:
            conn.close()

    def run_phases(self) -> int:
        self.controller.ensure_experiment(
            workload_class="schedule",
            random_seed=seed_for_phase(self.cfg, 0),
            config_json={"schedule": [p.as_dict() for p in self._schedule]},
        )
        logger.info("experimento %s asegurado", self.cfg.experiment_uid)

        for phase in self._schedule:
            if self._stop_requested.is_set():
                break
            logger.info(
                "fase %d/%d class=%s tasa_ofrecida=%.2f ops/min concurrency=%d "
                "warmup=%ds measure=%ds cooldown=%ds",
                phase.index, len(self._schedule), phase.workload_class,
                phase.offered_rate_ops_per_min, phase.concurrency,
                phase.warmup_seconds, phase.measure_seconds, phase.cooldown_seconds,
            )
            run_uid = self._register_workload_run(phase)
            stop_event = threading.Event()
            stats = {
                "offered": 0,
                "started": 0,
                "completed": 0,
                "failed": 0,
                "delayed": 0,
                "abandoned": 0,
                "batch_seq": 1,
                "stopped_by_cap": False,
                "lock": threading.Lock(),
            }
            phase_seed = seed_for_phase(self.cfg, phase.index)
            threads = [
                threading.Thread(
                    target=self._worker,
                    args=(
                        run_uid, phase, stop_event, stats, w + 1,
                        time.monotonic(), random.Random(phase_seed ^ (w + 1)),
                    ),
                    daemon=True,
                    name=f"worker-{w}",
                )
                for w in range(phase.concurrency)
            ]
            for t in threads:
                t.start()

            phase_wall_end = time.monotonic() + phase.duration_seconds
            last_report = time.monotonic()
            while time.monotonic() < phase_wall_end:
                if self._stop_requested.is_set():
                    break
                status = self.controller.azure_status()
                if status in ("STOPPING", "STOPPED", "FAILED"):
                    break
                with stats["lock"]:
                    if stats.get("stopped_by_cap"):
                        break
                now = time.monotonic()
                if now - last_report > 30:
                    self._write_workload_stats(
                        run_uid, stats["completed"], stats["failed"], False
                    )
                    last_report = now
                time.sleep(2)

            stop_event.set()
            for t in threads:
                t.join(timeout=15)
            with stats["lock"]:
                stopped_by_cap = stats.get("stopped_by_cap", False)
                ops = stats["completed"]
                errs = stats["failed"]
            reason = "phase_completed"
            if self._stop_requested.is_set():
                reason = "stop_requested"
            elif stopped_by_cap:
                reason = "w8_storage_cap"
            elif self.controller.azure_status() in ("STOPPING", "STOPPED", "FAILED"):
                reason = self.controller.azure_status().lower()
            self._write_workload_stats(run_uid, ops, errs, True, reason)
            logger.info(
                "fase %d terminada: ofrecido=%d iniciado=%d completado=%d "
                "fallido=%d retrasado=%d razón=%s",
                phase.index, stats["offered"], stats["started"], ops, errs,
                stats["delayed"], reason,
            )
            if phase.pause_seconds > 0 and not self._stop_requested.is_set():
                logger.info("fase %d: pausa %ds", phase.index, phase.pause_seconds)
                stop_event.wait(phase.pause_seconds)

        return 0

    # -- servicio -----------------------------------------------------------
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


# Operación registry: template_key (de loadtest.query_templates) -> handler.
# Todos los handlers son acotados y tocan solo el esquema loadtest.
WorkloadRunner.OPS = {
    "point_lookup": "_op_point_lookup",
    "range_scan": "_op_range_scan",
    "insert_event": "_op_insert",
    "update_row": "_op_update",
    "aggregate": "_op_aggregate",
}


def seed_for(cfg: Config) -> int:
    """Semilla determinista por experimento (compatibilidad)."""
    digest = hashlib.sha256(cfg.experiment_uid.encode("utf-8")).hexdigest()
    # SQL Server `int` is signed 32-bit; keep the seed in range.
    return int(digest[:8], 16) & 0x7FFFFFFF


def _setup_logging(verbose: bool = False) -> None:
    logging.basicConfig(
        level=logging.DEBUG if verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="CCO workload generator")
    parser.add_argument("--service", action="store_true", help="run as a systemd service")
    parser.add_argument("--run", action="store_true", help="single run")
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
        snapshot = config_snapshot_id(
            [{"workload_class": args.workload_class, "duration_seconds": int(hours * 3600)}]
        )
        cfg_overrides = [PhaseConfig(
            {"workload_class": args.workload_class, "duration_seconds": int(hours * 3600)},
            index=1,
            config_snapshot_id=snapshot,
        )]
    else:
        cfg_overrides = load_schedule(cfg)

    runner = WorkloadRunner(cfg)
    runner._schedule = cfg_overrides
    return runner.serve()


if __name__ == "__main__":
    raise SystemExit(main())
