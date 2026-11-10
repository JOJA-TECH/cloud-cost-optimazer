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

from ..workload.controller import WorkloadController
from ..workload.runner import seed_for

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


def _targets(cfg: Config) -> List[dict]:
    """Lista de targets a recolectar. piloto: el único target configurado."""
    targets = cfg.azure_sql_targets or []
    if not targets:
        # Fallback a target único (legacy / pilot sin targets_json).
        rid = cfg.azure_monitor_resource_id()
        if rid:
            targets = [{
                "target_key": "default",
                "resource_id": rid,
                "resource_uid": cfg.resource_uid(),
                "sku_name": cfg.sql_sku_name or None,
            }]
    return targets


def _monitor_window(cfg: Config) -> Tuple[dt.datetime, dt.datetime]:
    """Ventana retrasada + lookback.

    end   = ahora - demora          (evita métricas aún no publicadas)
    start = end - lookback          (recupera puntos publicados con retraso)
    La deduplicación (idempotencia en observed_at_utc) evita repetir puntos.
    """
    end = _now() - dt.timedelta(seconds=cfg.azure_monitor_delay_seconds)
    start = end - dt.timedelta(seconds=cfg.azure_monitor_lookback_seconds)
    return start, end


def collect_once(cfg: Config, collector_run_uid: str) -> writer.WriteResult:
    start, end = _monitor_window(cfg)
    targets = _targets(cfg)

    readings: List[MetricReading] = []
    rejected: List[dict] = []
    for target in targets:
        resource_id = target.get("resource_id") or cfg.azure_monitor_resource_id()
        resource_uid = target.get("resource_uid") or cfg.resource_uid()
        sku_name = target.get("sku_name")
        readings.extend(
            azure_monitor.fetch(
                cfg,
                resource_id,
                resource_uid,
                start,
                end,
                cfg.capture_raw_interval_seconds,
                rejected=rejected,
                sku_name=sku_name,
            )
        )
        if cfg.collect_sql_diagnostics:
            readings.extend(sql_diagnostics.fetch(cfg, resource_uid, sku_name=sku_name))
            readings.extend(query_store.fetch(cfg, resource_uid, sku_name=sku_name))

    observations, workload_stats = workload_observations.drain(cfg, start, end)
    # Las observaciones de workload pertenecen al experimento; se etiquetan con
    # el target primario (en piloto es el único).
    primary_uid = cfg.primary_resource_uid()
    for r in observations + workload_stats:
        if r.resource_uid is None:
            r.resource_uid = primary_uid
    readings.extend(observations)
    readings.extend(workload_stats)

    intervals, samples = _apply_quality(readings)
    logger.info(
        "cycle: %d interval readings, %d samples, %d rejected (targets=%d)",
        len(intervals), len(samples), len(rejected), len(targets),
    )

    result = writer.persist(
        cfg, collector_run_uid, intervals, samples, rejected=rejected
    )
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

    # Ensure the experiment exists in both stores so the collector's FKs hold
    # even if the workload runner or migrations never ran. Idempotent.
    try:
        WorkloadController(cfg).ensure_experiment(
            workload_class="collector",
            random_seed=seed_for(cfg),
            config_json={"profile": cfg.deployment_profile},
        )
    except Exception as exc:  # noqa: BLE001 - collector still runs, spools data
        logger.warning("cannot ensure experiment: %s", exc)

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
    # The Azure SDK logs every HTTP request/response at INFO; keep our own INFO
    # signal readable unless we are explicitly debugging.
    for noisy in ("azure", "azure.core", "azure.identity", "azure.monitor.query"):
        logging.getLogger(noisy).setLevel(
            logging.DEBUG if verbose else logging.WARNING
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
