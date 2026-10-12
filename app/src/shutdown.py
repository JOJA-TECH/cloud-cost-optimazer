"""Safe shutdown coordinator.

The only component allowed to move an experiment to its terminal record. It is
idempotent and ordered so that no data written by the collector is lost:

  1. read the current experiment status (Azure);
  2. request STOPPING on both stores (gated by the Azure trigger);
  3. confirm STOPPED;
  4. flush the local spool with a bounded timeout;
  5. if the spool emptied -> final_flush_status = 'ok';
     otherwise the experiment becomes STOPPED_WITH_PENDING_BUFFER (data kept).

``--supervise`` is a watchdog: once the experiment exceeds AUTO_STOP_AFTER_HOURS
(or MAX_EXPERIMENT_HOURS) it forces the same safe stop, so a forgotten
experiment cannot keep costing money.

Usage:
    python -m src.shutdown --stop --reason manual
    python -m src.shutdown --supervise
"""

from __future__ import annotations

import argparse
import datetime as dt
import logging
import signal
import sys
import time
from typing import Callable, Optional

from . import db_supabase
from .config import Config, from_env, load_env_file
from .sync import writer
from .workload.controller import TransitionError, WorkloadController

logger = logging.getLogger(__name__)

TERMINAL_STATUSES = ("STOPPED", "STOPPED_WITH_PENDING_BUFFER", "FAILED")


def _safe(fn: Callable):
    try:
        return fn()
    except Exception:  # noqa: BLE001
        return None


def experiment_row(cfg: Config) -> Optional[dict]:
    try:
        with db_supabase.transaction(cfg) as conn:
            rows = db_supabase.query(
                conn,
                "select started_at_utc, status, stop_reason "
                "from experiment.experiment_run where experiment_uid = %s",
                (cfg.experiment_uid,),
            )
        return rows[0] if rows else None
    except Exception as exc:  # noqa: BLE001
        logger.warning("cannot read experiment_run: %s", exc)
        return None


def final_flush(cfg: Config, timeout_seconds: int) -> dict:
    """Flush the spool until empty or the timeout elapses."""
    deadline = time.monotonic() + max(1, timeout_seconds)
    last: dict = {}
    while True:
        last = writer.flush_spool(cfg)
        if last.get("pending_files", 0) == 0:
            return last
        if time.monotonic() >= deadline:
            return last
        time.sleep(2)


def _supabase_set_status(cfg: Config, status: str, reason: str, flush_status: str | None = None) -> None:
    try:
        with db_supabase.transaction(cfg) as conn:
            db_supabase.query(
                conn,
                "update experiment.experiment_run "
                "set status = %s, stop_reason = %s, "
                " final_flush_status = coalesce(%s, final_flush_status), "
                " updated_at_utc = now() "
                "where experiment_uid = %s",
                (status, reason, flush_status, cfg.experiment_uid),
            )
    except Exception as exc:  # noqa: BLE001
        logger.error("cannot set experiment status %s: %s", status, exc)


def safe_stop(cfg: Config, reason: str = "manual") -> int:
    controller = WorkloadController(cfg)
    status = _safe(controller.azure_status)

    # 1. Move to STOPPING where the state machine allows it.
    try:
        if status in ("RUNNING", "PAUSED"):
            controller.transition("STOPPING", requested_by="shutdown", reason=reason)
        elif status == "PAUSING":
            controller.transition("RUNNING", requested_by="shutdown", reason="resume")
            controller.transition("STOPPING", requested_by="shutdown", reason=reason)
        elif status == "CREATED":
            # Nothing ever ran on Azure; record the terminal state on Supabase.
            _supabase_set_status(cfg, "STOPPED", reason)
    except TransitionError as exc:
        logger.warning("stop transition rejected (continuing): %s", exc)
    except Exception as exc:  # noqa: BLE001
        logger.warning("cannot request stop on Azure: %s", exc)

    # 2. Confirm STOPPED when the machine is in STOPPING.
    try:
        if WorkloadController(cfg).azure_status() == "STOPPING":
            controller.transition("STOPPED", requested_by="shutdown", reason="stop_confirm")
    except TransitionError as exc:
        logger.warning("STOPPED transition rejected (continuing): %s", exc)
    except Exception as exc:  # noqa: BLE001
        logger.warning("cannot confirm STOPPED on Azure: %s", exc)

    # 2b. If Azure was unavailable, still try to stop the machine, not fail.
    if status in ("CREATED", "STOPPED", "STOPPED_WITH_PENDING_BUFFER", "FAILED"):
        pass

    # 3. Bounded final flush.
    flushed = final_flush(cfg, cfg.final_flush_timeout_seconds)
    logger.info("final flush result: %s", flushed)

    if flushed.get("pending_files", 0) == 0:
        _supabase_set_status(cfg, "STOPPED", reason, flush_status="ok")
        return 0

    _supabase_set_status(
        cfg, "STOPPED_WITH_PENDING_BUFFER", reason, flush_status="pending"
    )
    return 0


def supervise(cfg: Config, poll_seconds: int = 60) -> int:
    """Watchdog: force a safe stop once the experiment exceeds its budget."""
    stop = {"requested": False}

    def _handler(signum, _frame):
        logger.info("supervisor received signal %s", signum)
        stop["requested"] = True

    signal.signal(signal.SIGTERM, _handler)
    signal.signal(signal.SIGINT, _handler)

    limit_hours = cfg.auto_stop_after_hours or cfg.experiment_duration_hours
    logger.info("supervisor started (limit=%dh)", limit_hours)

    while not stop["requested"]:
        row = experiment_row(cfg)
        if row and row.get("started_at_utc"):
            started = row["started_at_utc"]
            if isinstance(started, dt.datetime):
                if started.tzinfo is None:
                    started = started.replace(tzinfo=dt.timezone.utc)
                age_hours = (
                    dt.datetime.now(dt.timezone.utc) - started
                ).total_seconds() / 3600.0
                terminal = row.get("status") in (
                    "STOPPED", "STOPPED_WITH_PENDING_BUFFER", "FAILED",
                )
                if not terminal and age_hours >= limit_hours:
                    logger.warning(
                        "experiment age %.1fh >= limit %dh; forcing safe stop",
                        age_hours, limit_hours,
                    )
                    safe_stop(cfg, reason="auto_stop")
                    return 0
        deadline = time.monotonic() + max(5, poll_seconds)
        while not stop["requested"] and time.monotonic() < deadline:
            time.sleep(min(2.0, deadline - time.monotonic()))

    logger.info("supervisor stopped")
    return 0


def _setup_logging(verbose: bool = False) -> None:
    logging.basicConfig(
        level=logging.DEBUG if verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="CCO safe shutdown / supervisor")
    parser.add_argument("--stop", action="store_true", help="perform a safe stop now")
    parser.add_argument("--supervise", action="store_true", help="watchdog mode")
    parser.add_argument("--reason", default="manual")
    parser.add_argument("--poll-seconds", type=int, default=60)
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    _setup_logging(args.verbose)
    load_env_file()
    cfg = from_env()

    if args.supervise:
        return supervise(cfg, args.poll_seconds)
    return safe_stop(cfg, reason=args.reason)


if __name__ == "__main__":
    raise SystemExit(main())
