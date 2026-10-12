"""SQL diagnostics reader (inside the system under test).

Runs bounded DMV queries through the workload connection:
- sys.dm_db_resource_stats  -> cpu / data io / log write / memory
- sys.dm_exec_sessions      -> active user sessions

Requires VIEW DATABASE STATE, granted to the workload login by migration 004.
Any failure degrades gracefully (returns what it can).
"""

from __future__ import annotations

import datetime as dt
import logging
from typing import List

from .. import db_azure
from ..config import Config
from .normalizer import MetricReading, canonical_metric, unit_for

logger = logging.getLogger(__name__)

RESOURCE_STATS_QUERY = (
    "SELECT TOP (1) "
    " avg_cpu_percent, avg_data_io_percent, avg_log_write_percent, "
    " avg_memory_usage_percent, end_time "
    "FROM sys.dm_db_resource_stats ORDER BY end_time DESC"
)

SESSIONS_QUERY = (
    "SELECT COUNT(*) AS sessions FROM sys.dm_exec_sessions "
    "WHERE is_user_process = 1 AND session_id <> @@SPID"
)


def _reading(code: str, value, observed: dt.datetime, interval: int, payload: dict) -> MetricReading:
    return MetricReading(
        metric_code=code,
        value=float(value),
        unit=unit_for(code),
        observed_at_utc=observed,
        interval_seconds=interval,
        source="sql_diagnostics",
        source_record_id=None,
        source_payload=payload,
    )


RESOURCE_COLUMNS = [
    "avg_cpu_percent",
    "avg_data_io_percent",
    "avg_log_write_percent",
    "avg_memory_usage_percent",
]


def fetch(cfg: Config) -> List[MetricReading]:
    readings: List[MetricReading] = []
    conn = None
    try:
        conn = db_azure.connect(cfg)
        with conn.cursor() as cur:
            cur.execute(RESOURCE_STATS_QUERY)
            row = cur.fetchone()
        if row is not None:
            observed = row[len(RESOURCE_COLUMNS)]
            if isinstance(observed, dt.datetime) and observed.tzinfo is None:
                observed = observed.replace(tzinfo=dt.timezone.utc)
            for index, column in enumerate(RESOURCE_COLUMNS):
                code = canonical_metric(column)
                if code is None or row[index] is None:
                    continue
                readings.append(
                    _reading(code, row[index], observed, 60, {"raw_column": column})
                )
    except Exception as exc:  # noqa: BLE001
        logger.warning("sql_diagnostics resource stats unavailable: %s", exc)
    finally:
        if conn is not None:
            conn.close()

    # Sessions
    conn = None
    try:
        conn = db_azure.connect(cfg)
        value = db_azure.scalar(conn, SESSIONS_QUERY)
        if value is not None:
            readings.append(
                _reading(
                    "sessions_count",
                    value,
                    dt.datetime.now(dt.timezone.utc),
                    60,
                    {"raw_column": "sessions"},
                )
            )
    except Exception as exc:  # noqa: BLE001
        logger.debug("sql_diagnostics sessions unavailable: %s", exc)
    finally:
        if conn is not None:
            conn.close()

    return readings