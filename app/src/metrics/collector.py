"""Metrics collector service.

One cycle (every CAPTURE_RAW_INTERVAL_SECONDS seconds):
  1. register an ``ops.collector_run``;
  2. gather readings from Azure Monitor + SQL DMVs + Query Store;
  3. drain workload self-observations (samples for exact percentiles + ops/min);
  4. validate every reading against the metric catalog (quality policy);
  5. persist interval readings and samples idempotently (spool on failure);
  6. close the collector_run with ok/partial/error.

Every OPERATIONAL_AGGREGATION_MINUTES the collector also rebuilds the last few
completed hours into ``feat.metric_hourly`` (idempotent upsert).

Usage:
    python -m src.metrics.collector --service     # systemd
    python -m src.metrics.collector --once
"""

from __future__ import annotations

import argparse
import datetime as dt
import logging
import signal
import sys
import time
from typing import List, Tuple

from ..config import Config, from_env, load_env_file
from ..sync import writer
from . import aggregator, azure_monitor, query_store, sql_diagnostics, workload_observations
from .normalizer import MetricReading
from .quality import evaluate_reading

logger = logging.getLogger(__name__)

SAMPLE_SOURCES = frozenset({"workload_observation"})


def _now() -> dt.datetime:
    return dt.datetime.now(dt.timezone.utc)


def _apply_quality(readings: List[MetricReading]) -> Tuple[List[MetricReading], List[MetricReading]]:
    """Split into (interval_readings, samples), tagging quality on each."""
    intervals: List[MetricReading] = []
    samples: List[MetricReading] = []
    for reading in readings:
        status, warnings = evaluate_reading(reading.metric_code, reading.value, reading.unit)
        reading.quality_status = status
        reading.quality_warnings = warnings
        if reading.source in SAMPLE_SOURCES:
            samples.append(reading)
        else:
            intervals.append(reading)
    return intervals, samples


def collect_once(cfg: Config, collector_run_uid: str) -> writer.WriteResult:
    end = _now()
    start = end - dt.timedelta(seconds=cfg.capture_raw_interval_seconds)

    readings: List[MetricReading] = []
    readings.extend(azure_monitor.fetch(cfg, start, end, cfg.capture_raw_interval_seconds))
    readings.extend(sql_diagnostics.fetch(cfg))
    readings.extend(query_store.fetch(cfg))

    observations, intensity = workload_observations.drain(cfg, start, end)
    readings.extend(observations)
    if intensity is not None:
        readings.append(intensity)

    intervals, samples = _apply_quality(readings)
    logger.info(
        "cycle: %d interval readings, %d samples", len(intervals), len(samples)
    )

    result = writer.persist(cfg, collector_run_uid, intervals, samples)
    if result.error:
        logger.warning("persist error: %s", result.error)
    return result


def run_forever(cfg: Config, interval_seconds: int | None = None) -> int:
    interval = interval_seconds or cfg.capture_raw_interval_seconds
    aggregation_every = max(1, cfg.operational_aggregation_minutes) * 60
    stop = {"requested": False}

    def _handler(signum, _frame):
        logger.info("received signal %s; finishing cycle", signum)
        stop["requested"] = True

    signal.signal(signal.SIGTERM, _handler)
    signal.signal(signal.SIGINT, _handler)

    logger.info(
        "collector started (experiment=%s interval=%ds agg_every=%ds)",
        cfg.experiment_uid, interval, aggregation_every,
    )
    last_aggregation = 0.0
    while not stop["requested"]:
        cycle_start = time.monotonic()
        run_uid = writer.start_collector_run(cfg, "collector")
        try:
            result = collect_once(cfg, run_uid)
            status = "partial" if result.spooled else "ok"
            writer.finish_collector_run(
                cfg, run_uid,
                status=status,
                rows_observed=result.rows_observed,
                rows_written=result.rows_written,
                error=result.error or "",
            )
        except Exception as exc:  # noqa: BLE001 - a bad cycle must not kill the service
            logger.error("collector cycle failed: %s", exc)
            writer.finish_collector_run(
                cfg, run_uid,
                status="error",
                rows_observed=0, rows_written=0, error=str(exc)[:500],
            )

        if time.monotonic() - last_aggregation >= aggregation_every:
            try:
                count = aggregator.aggregate_recent(cfg, hours_back=2)
                logger.info("aggregated %d hourly rows", count)
            except Exception as exc:  # noqa: BLE001
                logger.warning("hourly aggregation failed: %s", exc)
            last_aggregation = time.monotonic()

        elapsed = time.monotonic() - cycle_start
        sleep_for = max(1.0, interval - elapsed)
        # Sleep in small slices so signals are honored promptly.
        deadline = time.monotonic() + sleep_for
        while not stop["requested"] and time.monotonic() < deadline:
            time.sleep(min(1.0, deadline - time.monotonic()))

    # Final flush of any spooled data before exit.
    try:
        flushed = writer.flush_spool(cfg)
        logger.info("final spool flush: %s", flushed)
    except Exception as exc:  # noqa: BLE001
        logger.warning("final spool flush failed: %s", exc)
    logger.info("collector stopped")
    return 0


def _setup_logging(verbose: bool = False) -> None:
    logging.basicConfig(
        level=logging.DEBUG if verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="CCO metrics collector")
    parser.add_argument("--service", action="store_true", help="run continuously")
    parser.add_argument("--once", action="store_true", help="run a single cycle")
    parser.add_argument("--interval", type=int, default=None)
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    _setup_logging(args.verbose)
    load_env_file()
    cfg = from_env()

    if args.once:
        run_uid = writer.start_collector_run(cfg, "collector")
        try:
            result = collect_once(cfg, run_uid)
        except Exception as exc:  # noqa: BLE001
            writer.finish_collector_run(
                cfg, run_uid, status="error", rows_observed=0, rows_written=0,
                error=str(exc)[:500],
            )
            logger.error("collector cycle failed: %s", exc)
            return 1
        writer.finish_collector_run(
            cfg, run_uid,
            status="partial" if result.spooled else "ok",
            rows_observed=result.rows_observed,
            rows_written=result.rows_written,
            error=result.error or "",
        )
        return 0

    return run_forever(cfg, args.interval)


if __name__ == "__main__":
    raise SystemExit(main())
