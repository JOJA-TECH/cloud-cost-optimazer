"""Scheduled multi-day experiment: one rotating load test per tick.

The Azure SQL free tier allows ~100k vCore-seconds per month, so the database
must stay auto-paused between tests. ``cco-scheduled-experiment.timer`` starts
this module every few hours; each tick runs one workload profile from a fixed
rotation and collects metrics only during the test window. Between ticks the
process exits and the database auto-pauses.

The rotation index is persisted so a reboot or a missed tick does not repeat
the same profile forever.
"""

from __future__ import annotations

import datetime as dt
import logging
import os
import signal
import subprocess
import sys
import time
from pathlib import Path

from .. import db_azure, db_supabase
from ..config import Config, from_env, load_env_file
from ..metrics import aggregator
from .controller import TransitionError, WorkloadController
from .runner import PhaseConfig, WorkloadRunner, config_snapshot_id, seed_for

logger = logging.getLogger(__name__)

# One profile per tick, rotated in order. Durations are short on purpose: the
# free tier bills vCore-seconds while the database is awake.
PROFILES = [
    {"workload_class": "low_stable", "duration_seconds": 300, "intensity": 0.2,
     "offered_rate_ops_per_min": 5.0},
    {"workload_class": "high_stable", "duration_seconds": 300, "intensity": 0.7,
     "offered_rate_ops_per_min": 30.0},
    {"workload_class": "periodic", "duration_seconds": 300, "intensity": 0.6,
     "offered_rate_ops_per_min": 20.0},
    {"workload_class": "bursty", "duration_seconds": 300, "intensity": 0.8,
     "offered_rate_ops_per_min": 24.0},
    {"workload_class": "increasing", "duration_seconds": 300, "intensity": 0.7,
     "offered_rate_ops_per_min": 18.0},
    {"workload_class": "cpu_intensive", "duration_seconds": 300, "intensity": 0.8,
     "offered_rate_ops_per_min": 15.0},
    {"workload_class": "io_intensive", "duration_seconds": 300, "intensity": 0.7,
     "offered_rate_ops_per_min": 18.0},
    {"workload_class": "mixed", "duration_seconds": 300, "intensity": 0.6,
     "offered_rate_ops_per_min": 20.0},
]

APP_DIR = Path(__file__).resolve().parents[2]
STATE_FILE = Path(os.environ.get("SCHEDULE_STATE_FILE", "/opt/cco/schedule_index"))
TAIL_SECONDS = int(os.environ.get("SCHEDULE_TAIL_SECONDS", "120"))
AGGREGATION_LOOKBACK_HOURS = int(os.environ.get("AGGREGATION_LOOKBACK_HOURS", "12"))


def _setup_logging(verbose: bool = False) -> None:
    logging.basicConfig(
        level=logging.DEBUG if verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )
    for noisy in ("azure", "azure.core", "azure.identity", "azure.monitor.query"):
        logging.getLogger(noisy).setLevel(
            logging.DEBUG if verbose else logging.WARNING
        )


def _next_index() -> int:
    try:
        return int(STATE_FILE.read_text().strip())
    except (OSError, ValueError):
        return 0


def _save_index(index: int) -> None:
    try:
        STATE_FILE.write_text(f"{index}\n")
    except OSError as exc:  # noqa: BLE001 - index is best-effort
        logger.warning("cannot persist schedule index: %s", exc)


def _sync_supabase_running(controller: WorkloadController) -> None:
    """Repair a split state where Azure says RUNNING but Supabase says STOPPED.

    A watchdog stop that cannot reach Azure leaves the analytical record
    terminal while the operational mirror keeps RUNNING; the next tick fixes it.
    """
    if controller.supabase_status() == "RUNNING":
        return
    with db_supabase.transaction(controller.cfg) as conn:
        db_supabase.query(
            conn,
            "update experiment.experiment_run "
            "set status = 'RUNNING', stop_reason = NULL, updated_at_utc = now() "
            "where experiment_uid = %s",
            (controller.cfg.experiment_uid,),
        )
    logger.info("supabase experiment status reconciled to RUNNING")


def _wake_database(cfg: Config, attempts: int = 12, delay: int = 20) -> None:
    """A paused free-tier database resumes on the first connection attempt.

    That attempt usually times out (HYT00) while the server is resuming, so
    retry until the login succeeds before starting the test.
    """
    for attempt in range(1, attempts + 1):
        try:
            db_azure.connect(cfg).close()
            if attempt > 1:
                logger.info("database awake after %d attempts", attempt)
            return
        except db_azure.AzureSQLConnectionError as exc:
            logger.warning(
                "database not ready (attempt %d/%d): %s", attempt, attempts, exc
            )
            time.sleep(delay)
    raise TransitionError(f"database did not resume after {attempts} attempts")


def _experiment_age_hours(cfg: Config) -> float | None:
    with db_supabase.transaction(cfg) as conn:
        rows = db_supabase.query(
            conn,
            "select started_at_utc from experiment.experiment_run "
            "where experiment_uid = %s",
            (cfg.experiment_uid,),
        )
    if not rows or not rows[0]["started_at_utc"]:
        return None
    started = rows[0]["started_at_utc"]
    if started.tzinfo is None:
        started = started.replace(tzinfo=dt.timezone.utc)
    return (dt.datetime.now(dt.timezone.utc) - started).total_seconds() / 3600.0


def _ensure_running(controller: WorkloadController) -> None:
    status = controller.azure_status()
    if status is None:
        controller.ensure_experiment(
            workload_class="schedule",
            random_seed=seed_for(controller.cfg),
            config_json={"scheduled": True},
        )
        status = controller.azure_status()
    if status == "RUNNING":
        _sync_supabase_running(controller)
        return
    if status in ("CREATED", "PAUSED", "PAUSING", "FAILED"):
        controller.transition(
            "RUNNING", requested_by="scheduler", reason="scheduled_tick"
        )
        return
    raise TransitionError(f"experiment is {status}; scheduled tick cannot start")


def _start_collector() -> subprocess.Popen:
    logger.info("starting collector burst")
    return subprocess.Popen(
        [sys.executable, "-m", "src.metrics.collector", "--service"],
        cwd=str(APP_DIR),
    )


def _stop_collector(proc: subprocess.Popen) -> None:
    if proc.poll() is not None:
        logger.warning("collector exited early with code %s", proc.returncode)
        return
    proc.send_signal(signal.SIGTERM)
    try:
        proc.wait(timeout=200)
    except subprocess.TimeoutExpired:
        logger.warning("collector did not stop; killing")
        proc.kill()
        proc.wait()


def main() -> int:
    _setup_logging(os.environ.get("SCHEDULE_VERBOSE", "").lower() in {"1", "true"})
    load_env_file()
    cfg = from_env()

    index = _next_index()
    profile = PROFILES[index % len(PROFILES)]
    logger.info(
        "scheduled tick #%d profile=%s rate=%.1f ops/min",
        index, profile["workload_class"], profile["offered_rate_ops_per_min"],
    )

    controller = WorkloadController(cfg)
    try:
        limit_hours = cfg.auto_stop_after_hours or cfg.experiment_duration_hours
        age = _experiment_age_hours(cfg)
        if age is not None and limit_hours and age >= limit_hours:
            logger.info(
                "experiment window over (age=%.1fh limit=%dh); no tick",
                age, limit_hours,
            )
            return 0
        _wake_database(cfg)
        _ensure_running(controller)
    except TransitionError as exc:
        logger.error("scheduled tick skipped: %s", exc)
        return 0

    collector = _start_collector()
    try:
        phase = PhaseConfig(
            {
                **profile,
                "warmup_seconds": 30,
                "measure_seconds": 240,
                "cooldown_seconds": 30,
            },
            index=1,
            config_snapshot_id=config_snapshot_id([profile]),
        )
        runner = WorkloadRunner(cfg)
        runner._schedule = [phase]
        runner.run_phases()
        logger.info("workload finished; collecting tail for %ds", TAIL_SECONDS)
        time.sleep(TAIL_SECONDS)
    finally:
        _stop_collector(collector)

    try:
        count = aggregator.aggregate_recent(
            cfg, hours_back=AGGREGATION_LOOKBACK_HOURS
        )
        logger.info("aggregated %d hourly rows", count)
    except Exception as exc:  # noqa: BLE001 - next tick retries
        logger.warning("hourly aggregation failed: %s", exc)

    _save_index(index + 1)
    logger.info("scheduled tick #%d done", index)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
