"""Hourly aggregation into feat.metric_hourly.

Only persisted readings/samples are used. Exact percentiles come from
raw.metric_sample; when there are no samples the percentile columns stay NULL
and the quality status reflects it (never an invented number).

The upsert is idempotent on (experiment_uid, resource_uid, period_start_utc),
so re-running an hour is safe.
"""

from __future__ import annotations

import datetime as dt
import logging
from typing import Dict, List, Optional

from .. import db_supabase
from ..config import Config
from .quality import compute_missing_fraction, percentiles_from_values

logger = logging.getLogger(__name__)

HOUR = dt.timedelta(hours=1)
EXPECTED_READINGS_PER_HOUR = 60  # one reading per minute


def _hour_start(moment: dt.datetime) -> dt.datetime:
    return moment.replace(minute=0, second=0, microsecond=0)


def aggregate_hour(cfg: Config, hour_start: dt.datetime) -> Optional[dict]:
    start = _hour_start(hour_start)
    end = start + HOUR
    resource_uid = cfg.resource_uid()

    with db_supabase.transaction(cfg) as conn:
        stats = db_supabase.query(
            conn,
            """
            select metric_code,
                   avg(value) as avg_value,
                   max(value) as max_value,
                   sum(value) as sum_value,
                   count(*)   as n
            from raw.metric_reading
            where experiment_uid = %s and resource_uid = %s
              and observed_at_utc >= %s and observed_at_utc < %s
            group by metric_code
            """,
            (cfg.experiment_uid, resource_uid, start, end),
        )
        samples = db_supabase.query(
            conn,
            """
            select metric_code, value
            from raw.metric_sample
            where experiment_uid = %s and resource_uid = %s
              and observed_at_utc >= %s and observed_at_utc < %s
            """,
            (cfg.experiment_uid, resource_uid, start, end),
        )
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

    if not stats and not samples:
        return None

    by_code: Dict[str, dict] = {row["metric_code"]: row for row in stats}

    def avg(code: str) -> Optional[float]:
        row = by_code.get(code)
        return row["avg_value"] if row else None

    def maxv(code: str) -> Optional[float]:
        row = by_code.get(code)
        return row["max_value"] if row else None

    def sumv(code: str) -> float:
        row = by_code.get(code)
        return float(row["sum_value"] or 0.0) if row else 0.0

    def count(code: str) -> int:
        row = by_code.get(code)
        return int(row["n"]) if row else 0

    cpu_samples = [row["value"] for row in samples if row["metric_code"] == "cpu_percent"]
    latency_samples = [
        row["value"] for row in samples if row["metric_code"] == "query_duration_sample_ms"
    ]

    cpu_percentiles = percentiles_from_values(cpu_samples, points=(50.0, 95.0, 99.0))
    latency_percentiles = percentiles_from_values(latency_samples, points=(95.0,))

    cpu_n = count("cpu_percent")
    missing_fraction = compute_missing_fraction(cpu_n, EXPECTED_READINGS_PER_HOUR)

    warnings: List[str] = []
    if cpu_n == 0:
        quality = "not_available"
        warnings.append("no_cpu_readings")
    else:
        quality = "usable"
        if not cpu_samples:
            warnings.append("percentiles_not_available")
        if missing_fraction > 0.3:
            quality = "degraded"
            warnings.append("sparse_cpu_coverage")

    storage_bytes = maxv("storage_used_bytes")
    storage_bytes_value = int(storage_bytes) if storage_bytes is not None else None

    growth = None
    if storage_bytes_value is not None and previous:
        prev_bytes = previous[0].get("storage_used_bytes")
        if prev_bytes is not None:
            growth = storage_bytes_value - int(prev_bytes)

    return {
        "experiment_uid": cfg.experiment_uid,
        "resource_uid": resource_uid,
        "period_start_utc": start,
        "period_end_utc": end,
        "workload_class": _workload_class(cfg, start),
        "cpu_avg": avg("cpu_percent"),
        "cpu_p50": cpu_percentiles.get(50.0),
        "cpu_p95": cpu_percentiles.get(95.0),
        "cpu_p99": cpu_percentiles.get(99.0),
        "cpu_max": maxv("cpu_percent"),
        "data_io_avg": avg("data_io_percent"),
        "log_write_avg": avg("log_write_percent"),
        "sessions_avg": avg("sessions_count") if by_code.get("sessions_count") else avg("sessions_percent"),
        "workers_avg": avg("workers_percent"),
        "storage_used_pct": maxv("storage_percent"),
        "storage_used_bytes": storage_bytes_value,
        "storage_growth_bytes": growth,
        "workload_intensity": avg("workload_intensity"),
        "deadlock_count": int(sumv("deadlock_count")),
        "latency_p95_ms": latency_percentiles.get(95.0),
        "sample_count": len(samples),
        "missing_fraction": missing_fraction,
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
    with db_supabase.transaction(cfg) as conn:
        db_supabase.query(
            conn,
            """
            insert into feat.metric_hourly (
                experiment_uid, resource_uid, period_start_utc, period_end_utc,
                workload_class, cpu_avg, cpu_p50, cpu_p95, cpu_p99, cpu_max,
                data_io_avg, log_write_avg, sessions_avg, workers_avg,
                storage_used_pct, storage_used_bytes, storage_growth_bytes,
                workload_intensity, deadlock_count, latency_p95_ms,
                sample_count, missing_fraction, quality_status, quality_warnings
            ) values (
                %(experiment_uid)s, %(resource_uid)s, %(period_start_utc)s, %(period_end_utc)s,
                %(workload_class)s, %(cpu_avg)s, %(cpu_p50)s, %(cpu_p95)s, %(cpu_p99)s, %(cpu_max)s,
                %(data_io_avg)s, %(log_write_avg)s, %(sessions_avg)s, %(workers_avg)s,
                %(storage_used_pct)s, %(storage_used_bytes)s, %(storage_growth_bytes)s,
                %(workload_intensity)s, %(deadlock_count)s, %(latency_p95_ms)s,
                %(sample_count)s, %(missing_fraction)s, %(quality_status)s, %(quality_warnings)s
            )
            on conflict (experiment_uid, resource_uid, period_start_utc) do update set
                period_end_utc = excluded.period_end_utc,
                workload_class = excluded.workload_class,
                cpu_avg = excluded.cpu_avg,
                cpu_p50 = excluded.cpu_p50,
                cpu_p95 = excluded.cpu_p95,
                cpu_p99 = excluded.cpu_p99,
                cpu_max = excluded.cpu_max,
                data_io_avg = excluded.data_io_avg,
                log_write_avg = excluded.log_write_avg,
                sessions_avg = excluded.sessions_avg,
                workers_avg = excluded.workers_avg,
                storage_used_pct = excluded.storage_used_pct,
                storage_used_bytes = excluded.storage_used_bytes,
                storage_growth_bytes = excluded.storage_growth_bytes,
                workload_intensity = excluded.workload_intensity,
                deadlock_count = excluded.deadlock_count,
                latency_p95_ms = excluded.latency_p95_ms,
                sample_count = excluded.sample_count,
                missing_fraction = excluded.missing_fraction,
                quality_status = excluded.quality_status,
                quality_warnings = excluded.quality_warnings
            """,
            row,
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