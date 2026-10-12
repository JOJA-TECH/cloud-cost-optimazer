"""Query Store reader.

Query Store keeps per-interval aggregates (avg/min/max/stdev/count), not the
individual observations. Those aggregates are persisted as interval readings
and MUST NOT be turned into p95/p99: the percentile method for them is
'not_available'. Exact latency percentiles come from workload observations
(raw.metric_sample, source = workload_observation).
"""

from __future__ import annotations

import datetime as dt
import logging
from typing import List

from .. import db_azure
from ..config import Config
from .normalizer import MetricReading, unit_for

logger = logging.getLogger(__name__)

# avg_duration is microseconds in Query Store -> convert to milliseconds.
QUERY_STORE_QUERY = (
    "SELECT TOP (20) "
    " CAST(qsrs.avg_duration / 1000.0 AS float) AS avg_duration_ms, "
    " qsrs.count_executions, "
    " qsrs.last_execution_time "
    "FROM sys.query_store_runtime_stats qsrs "
    "JOIN sys.query_store_plan qsp ON qsp.plan_id = qsrs.plan_id "
    "ORDER BY qsrs.last_execution_time DESC"
)


def fetch(cfg: Config) -> List[MetricReading]:
    readings: List[MetricReading] = []
    conn = None
    try:
        conn = db_azure.connect(cfg)
        with conn.cursor() as cur:
            cur.execute(QUERY_STORE_QUERY)
            rows = cur.fetchall()
        # Weighted average duration across the most recent intervals.
        total_exec = 0
        weighted = 0.0
        latest = None
        for row in rows:
            avg_ms = row[0]
            executions = row[1] or 0
            if avg_ms is None or executions <= 0:
                continue
            weighted += float(avg_ms) * executions
            total_exec += int(executions)
            latest = row[2] or latest
        if total_exec > 0:
            observed = latest or dt.datetime.now(dt.timezone.utc)
            if isinstance(observed, dt.datetime) and observed.tzinfo is None:
                observed = observed.replace(tzinfo=dt.timezone.utc)
            readings.append(
                MetricReading(
                    metric_code="query_duration_avg_ms",
                    value=weighted / total_exec,
                    unit=unit_for("query_duration_avg_ms"),
                    observed_at_utc=observed,
                    interval_seconds=60,
                    source="query_store_execution",
                    source_record_id=None,
                    source_payload={
                        "intervals": len(rows),
                        "executions": total_exec,
                        "percentile_method": "not_available",
                    },
                )
            )
    except Exception as exc:  # noqa: BLE001
        logger.debug("query_store unavailable: %s", exc)
    finally:
        if conn is not None:
            conn.close()
    return readings