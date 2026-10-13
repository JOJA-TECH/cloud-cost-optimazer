"""Hourly aggregation into feat.metric_hourly.

Design rules enforced here:

- **Canonical source per metric.** Metrics that arrive from more than one
  source (``cpu_percent``, ``data_io_percent``, ``log_write_percent``,
  ``sessions_count``) are not averaged together: the highest-priority source
  present in the window is chosen and recorded in ``canonical_sources``
  (Azure Monitor > SQL DMVs > Query Store > workload self-observation).
- **Percentiles are labelled.** CPU percentiles are *empirical quantiles of the
  60 s interval readings* (``empirical_interval_quantile``), not exact
  per-moment quantiles. Latency p95 comes from individual operation samples
  (``exact_sample``). When the inputs are missing the method is
  ``not_available`` and the value stays NULL.
- **Sessions are not mixed.** Count and utilization live in separate columns.
- **Coverage is per metric**, not just CPU; window quality follows from it.

The upsert is idempotent on (experiment_uid, resource_uid, period_start_utc).
"""

from __future__ import annotations

import datetime as dt
import logging
from typing import Dict, List, Optional

from .. import db_supabase
from ..config import Config
from .normalizer import METRIC_CATALOG_VERSION
from .quality import (
    coverage_by_metric,
    percentiles_from_values,
    window_quality,
)

logger = logging.getLogger(__name__)

HOUR = dt.timedelta(hours=1)
EXPECTED_READINGS_PER_HOUR = 60  # one reading per minute

# Lower value wins when a metric is present from multiple sources.
SOURCE_PRIORITY = {
    "azure_monitor": 0,
    "sql_diagnostics": 1,
    "query_store_execution": 2,
    "workload_self": 3,
    "workload_observation": 4,
}
DEFAULT_SOURCE_PRIORITY = 9


def _hour_start(moment: dt.datetime) -> dt.datetime:
    return moment.replace(minute=0, second=0, microsecond=0)


def select_canonical(per_source: List[dict]) -> tuple:
    """Choose one canonical source per metric code (highest priority present).

    Returns ``(canonical, canonical_sources)`` where both are keyed by metric
    code; ``canonical`` holds the source row and ``canonical_sources`` the
    winning source name. Sources are never averaged together.
    """
    by_code: Dict[str, Dict[str, dict]] = {}
    for row in per_source:
        by_code.setdefault(row["metric_code"], {})[row["source"]] = row

    canonical: Dict[str, dict] = {}
    canonical_sources: Dict[str, str] = {}
    for code, sources in by_code.items():
        best = min(
            sources,
            key=lambda s: (SOURCE_PRIORITY.get(s, DEFAULT_SOURCE_PRIORITY), s),
        )
        canonical[code] = sources[best]
        canonical_sources[code] = best
    return canonical, canonical_sources


def aggregate_hour(cfg: Config, hour_start: dt.datetime) -> Optional[dict]:
    start = _hour_start(hour_start)
    end = start + HOUR
    resource_uid = cfg.resource_uid()

    with db_supabase.transaction(cfg) as conn:
        per_source = db_supabase.query(
            conn,
            """
            select source, metric_code,
                   avg(value) as avg_value,
                   max(value) as max_value,
                   sum(value) as sum_value,
                   count(*)   as n
            from raw.metric_reading
            where experiment_uid = %s and resource_uid = %s
              and observed_at_utc >= %s and observed_at_utc < %s
            group by source, metric_code
            """,
            (cfg.experiment_uid, resource_uid, start, end),
        )
        cpu_rows = db_supabase.query(
            conn,
            """
            select source, value
            from raw.metric_reading
            where experiment_uid = %s and resource_uid = %s
              and observed_at_utc >= %s and observed_at_utc < %s
              and metric_code = 'cpu_percent'
            """,
            (cfg.experiment_uid, resource_uid, start, end),
        )
        latency_values = [
            row["value"]
            for row in db_supabase.query(
                conn,
                """
                select value
                from raw.metric_sample
                where experiment_uid = %s and resource_uid = %s
                  and observed_at_utc >= %s and observed_at_utc < %s
                  and metric_code = 'workload_operation_latency_ms'
                """,
                (cfg.experiment_uid, resource_uid, start, end),
            )
        ]
        previous = db_supabase.query(
            conn,
            """
            select storage_used_bytes
            from feat.metric_hourly
            where experiment_uid = %s and resource_uid = %s
              and period_start_utc = %s
            """,
            (cfg.experiment_uid, resource_uid, start - HOUR),
        )

    if not per_source:
        return None

    # ---- canonical source per metric code ---------------------------------
    canonical, canonical_sources = select_canonical(per_source)

    def avg(code: str) -> Optional[float]:
        row = canonical.get(code)
        return float(row["avg_value"]) if row and row["avg_value"] is not None else None

    def maxv(code: str) -> Optional[float]:
        row = canonical.get(code)
        return float(row["max_value"]) if row and row["max_value"] is not None else None

    def sumv(code: str) -> float:
        row = canonical.get(code)
        return float(row["sum_value"] or 0.0) if row else 0.0

    def count(code: str) -> int:
        row = canonical.get(code)
        return int(row["n"]) if row else 0

    # ---- percentiles -------------------------------------------------------
    cpu_source = canonical_sources.get("cpu_percent")
    cpu_values = [float(r["value"]) for r in cpu_rows if r["source"] == cpu_source]
    cpu_percentiles = percentiles_from_values(cpu_values, points=(50.0, 95.0, 99.0))
    cpu_method = "empirical_interval_quantile" if cpu_values else "not_available"

    latency_percentiles = percentiles_from_values(latency_values, points=(95.0,))
    latency_method = "exact_sample" if latency_values else "not_available"

    # ---- coverage and quality ---------------------------------------------
    counts = {code: count(code) for code in canonical}
    coverage = coverage_by_metric(counts, EXPECTED_READINGS_PER_HOUR)
    quality, warnings = window_quality(coverage)

    # ---- storage -----------------------------------------------------------
    capacity = cfg.storage_capacity_bytes()
    storage_bytes = maxv("storage_used_bytes")
    storage_bytes_value = int(storage_bytes) if storage_bytes is not None else None
    storage_pct = maxv("storage_percent")
    if capacity and storage_bytes_value is not None and storage_pct is not None:
        implied_pct = storage_bytes_value / capacity * 100.0
        if abs(implied_pct - storage_pct) > 3.0:
            warnings.append("storage_metric_inconsistent")
            if quality == "usable":
                quality = "degraded"

    growth = None
    if storage_bytes_value is not None and previous:
        prev_bytes = previous[0].get("storage_used_bytes")
        if prev_bytes is not None:
            growth = storage_bytes_value - int(prev_bytes)

    # ---- workload outcomes -------------------------------------------------
    offered = int(sumv("offered_operations"))
    successful = int(sumv("successful_operations"))
    failed = int(sumv("failed_operations"))
    error_rate = (failed / offered) if offered else None

    return {
        "experiment_uid": cfg.experiment_uid,
        "resource_uid": resource_uid,
        "period_start_utc": start,
        "period_end_utc": end,
        "workload_class": _workload_class(cfg, start),
        # CPU
        "cpu_avg": avg("cpu_percent"),
        "cpu_p50": cpu_percentiles.get(50.0),
        "cpu_p95": cpu_percentiles.get(95.0),
        "cpu_p99": cpu_percentiles.get(99.0),
        "cpu_max": maxv("cpu_percent"),
        "cpu_p50_method": cpu_method,
        "cpu_p95_method": cpu_method,
        "cpu_p99_method": cpu_method,
        "cpu_p95_over_interval_readings": cpu_percentiles.get(95.0),
        # I/O and memory
        "data_io_avg": avg("data_io_percent"),
        "log_write_avg": avg("log_write_percent"),
        "memory_avg": avg("memory_percent"),
        # Sessions and workers (kept in separate units)
        "sessions_avg_count": avg("sessions_count"),
        "sessions_utilization_avg_pct": avg("sessions_percent"),
        "workers_avg": avg("workers_percent"),
        # Storage
        "storage_used_pct": storage_pct,
        "storage_used_bytes": storage_bytes_value,
        "storage_capacity_bytes": capacity,
        "storage_growth_bytes": growth,
        # Workload
        "workload_intensity": avg("workload_intensity"),
        "offered_operations": offered,
        "successful_operations": successful,
        "failed_operations": failed,
        "error_rate": error_rate,
        "throughput_ops_per_second": avg("throughput_ops_per_second"),
        "deadlock_count": int(sumv("deadlock_count")),
        # Latency
        "workload_operation_latency_p95_ms": latency_percentiles.get(95.0),
        "latency_percentile_method": latency_method,
        # Quality
        "sample_count": len(latency_values),
        "missing_fraction": coverage.get("cpu_percent", 0.0),
        "coverage_json": coverage,
        "canonical_sources": canonical_sources,
        "catalog_version": METRIC_CATALOG_VERSION,
        "quality_status": quality,
        "quality_warnings": warnings,
    }


def _workload_class(cfg: Config, hour_start: dt.datetime) -> str:
    try:
        with db_supabase.transaction(cfg) as conn:
            rows = db_supabase.query(
                conn,
                """
                select workload_class
                from experiment.workload_run
                where experiment_uid = %s
                  and started_at_utc < %s
                  and (finished_at_utc is null or finished_at_utc >= %s)
                order by started_at_utc desc
                limit 1
                """,
                (cfg.experiment_uid, hour_start + HOUR, hour_start),
            )
        if rows and rows[0].get("workload_class"):
            return rows[0]["workload_class"]
    except Exception as exc:  # noqa: BLE001 - class is metadata, not critical
        logger.warning("cannot resolve workload_class for %s: %s", hour_start, exc)
    return "unknown"


def upsert_hour(cfg: Config, row: dict) -> None:
    from psycopg2.extras import Json

    payload = dict(row)
    payload["coverage_json"] = Json(row["coverage_json"])
    payload["canonical_sources"] = Json(row["canonical_sources"])
    with db_supabase.transaction(cfg) as conn:
        db_supabase.query(
            conn,
            """
            insert into feat.metric_hourly (
                experiment_uid, resource_uid, period_start_utc, period_end_utc,
                workload_class,
                cpu_avg, cpu_p50, cpu_p95, cpu_p99, cpu_max,
                cpu_p50_method, cpu_p95_method, cpu_p99_method,
                cpu_p95_over_interval_readings,
                data_io_avg, log_write_avg, memory_avg,
                sessions_avg_count, sessions_utilization_avg_pct, workers_avg,
                storage_used_pct, storage_used_bytes, storage_capacity_bytes,
                storage_growth_bytes,
                workload_intensity,
                offered_operations, successful_operations, failed_operations,
                error_rate, throughput_ops_per_second,
                deadlock_count,
                workload_operation_latency_p95_ms, latency_percentile_method,
                sample_count, missing_fraction, coverage_json,
                canonical_sources, catalog_version,
                quality_status, quality_warnings
            ) values (
                %(experiment_uid)s, %(resource_uid)s, %(period_start_utc)s, %(period_end_utc)s,
                %(workload_class)s,
                %(cpu_avg)s, %(cpu_p50)s, %(cpu_p95)s, %(cpu_p99)s, %(cpu_max)s,
                %(cpu_p50_method)s, %(cpu_p95_method)s, %(cpu_p99_method)s,
                %(cpu_p95_over_interval_readings)s,
                %(data_io_avg)s, %(log_write_avg)s, %(memory_avg)s,
                %(sessions_avg_count)s, %(sessions_utilization_avg_pct)s, %(workers_avg)s,
                %(storage_used_pct)s, %(storage_used_bytes)s, %(storage_capacity_bytes)s,
                %(storage_growth_bytes)s,
                %(workload_intensity)s,
                %(offered_operations)s, %(successful_operations)s, %(failed_operations)s,
                %(error_rate)s, %(throughput_ops_per_second)s,
                %(deadlock_count)s,
                %(workload_operation_latency_p95_ms)s, %(latency_percentile_method)s,
                %(sample_count)s, %(missing_fraction)s, %(coverage_json)s,
                %(canonical_sources)s, %(catalog_version)s,
                %(quality_status)s, %(quality_warnings)s
            )
            on conflict (experiment_uid, resource_uid, period_start_utc) do update set
                period_end_utc = excluded.period_end_utc,
                workload_class = excluded.workload_class,
                cpu_avg = excluded.cpu_avg,
                cpu_p50 = excluded.cpu_p50,
                cpu_p95 = excluded.cpu_p95,
                cpu_p99 = excluded.cpu_p99,
                cpu_max = excluded.cpu_max,
                cpu_p50_method = excluded.cpu_p50_method,
                cpu_p95_method = excluded.cpu_p95_method,
                cpu_p99_method = excluded.cpu_p99_method,
                cpu_p95_over_interval_readings = excluded.cpu_p95_over_interval_readings,
                data_io_avg = excluded.data_io_avg,
                log_write_avg = excluded.log_write_avg,
                memory_avg = excluded.memory_avg,
                sessions_avg_count = excluded.sessions_avg_count,
                sessions_utilization_avg_pct = excluded.sessions_utilization_avg_pct,
                workers_avg = excluded.workers_avg,
                storage_used_pct = excluded.storage_used_pct,
                storage_used_bytes = excluded.storage_used_bytes,
                storage_capacity_bytes = excluded.storage_capacity_bytes,
                storage_growth_bytes = excluded.storage_growth_bytes,
                workload_intensity = excluded.workload_intensity,
                offered_operations = excluded.offered_operations,
                successful_operations = excluded.successful_operations,
                failed_operations = excluded.failed_operations,
                error_rate = excluded.error_rate,
                throughput_ops_per_second = excluded.throughput_ops_per_second,
                deadlock_count = excluded.deadlock_count,
                workload_operation_latency_p95_ms = excluded.workload_operation_latency_p95_ms,
                latency_percentile_method = excluded.latency_percentile_method,
                sample_count = excluded.sample_count,
                missing_fraction = excluded.missing_fraction,
                coverage_json = excluded.coverage_json,
                canonical_sources = excluded.canonical_sources,
                catalog_version = excluded.catalog_version,
                quality_status = excluded.quality_status,
                quality_warnings = excluded.quality_warnings
            """,
            payload,
        )


def aggregate_recent(cfg: Config, hours_back: int = 6) -> int:
    """Aggregate every completed hour in the last ``hours_back`` hours."""
    current_hour = _hour_start(dt.datetime.now(dt.timezone.utc))
    aggregated = 0
    for offset in range(1, hours_back + 1):
        hour_start = current_hour - offset * HOUR
        try:
            row = aggregate_hour(cfg, hour_start)
            if row is None:
                continue
            upsert_hour(cfg, row)
            aggregated += 1
        except Exception as exc:  # noqa: BLE001
            logger.warning("aggregation failed for %s: %s", hour_start, exc)
    return aggregated
