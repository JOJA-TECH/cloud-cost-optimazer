"""SQL diagnostics reader (inside the system under test).

Runs bounded DMV queries through the workload connection:
- sys.dm_db_resource_stats  -> cpu / data io / log write / memory (all rows
  newer than the last captured end_time, incremental, ~15 s cadence)
- sys.dm_exec_sessions      -> active user sessions

Requires VIEW DATABASE STATE, granted to the workload login by migration 004.
Any failure degrades gracefully (returns what it can).

Fuera de los bloques activos de workload el llamador NO debe invocar este
lector, para no reanudar la base de datos ni consumir vCore-segundos.
"""

from __future__ import annotations

import datetime as dt
import logging
from typing import List, Optional

from .. import db_azure
from ..config import Config
from .normalizer import MetricReading, canonical_metric, unit_for

logger = logging.getLogger(__name__)

RESOURCE_STATS_QUERY = (
    "SELECT avg_cpu_percent, avg_data_io_percent, avg_log_write_percent, "
    " avg_memory_usage_percent, end_time "
    "FROM sys.dm_db_resource_stats "
    "WHERE end_time > ? "
    "ORDER BY end_time ASC"
)

SESSIONS_QUERY = (
    "SELECT COUNT(*) AS sessions FROM sys.dm_exec_sessions "
    "WHERE is_user_process = 1 AND session_id <> @@SPID"
)

# Ultimo end_time capturado por target, para captura incremental sin repetir
# filas ni perder puntos intermedios (~15 s). Clave: resource_uid.
_LAST_SEEN: dict = {}


def _reading(code: str, value, observed: dt.datetime, interval: int,
             resource_uid: str, payload: dict) -> MetricReading:
    return MetricReading(
        metric_code=code,
        value=float(value),
        unit=unit_for(code),
        observed_at_utc=observed,
        interval_seconds=interval,
        source="sql_diagnostics",
        resource_uid=resource_uid,
        source_record_id=None,
        source_payload=payload,
    )


RESOURCE_COLUMNS = [
    "avg_cpu_percent",
    "avg_data_io_percent",
    "avg_log_write_percent",
    "avg_memory_usage_percent",
]


def fetch(cfg: Config, resource_uid: str, sku_name: Optional[str] = None) -> List[MetricReading]:
    readings: List[MetricReading] = []
    last_seen = _LAST_SEEN.get(resource_uid)
    # SQL Server rejects ISO strings with a UTC offset for datetime comparisons;
    # pass a naive datetime (the column is UTC) or a safe floor.
    last_seen_sql = (
        last_seen.replace(tzinfo=None)
        if isinstance(last_seen, dt.datetime)
        else dt.datetime(1900, 1, 1)
    )

    conn = None
    try:
        conn = db_azure.connect(cfg)
        with conn.cursor() as cur:
            cur.execute(RESOURCE_STATS_QUERY, (last_seen_sql,))
            rows = cur.fetchall()
        if rows:
            new_last = last_seen
            for row in rows:
                values = list(row)
                observed = values.pop()  # end_time es la última columna
                if isinstance(observed, dt.datetime) and observed.tzinfo is None:
                    observed = observed.replace(tzinfo=dt.timezone.utc)
                if new_last is None or observed > new_last:
                    new_last = observed
                for index, column in enumerate(RESOURCE_COLUMNS):
                    code = canonical_metric(column)
                    if code is None or values[index] is None:
                        continue
                    payload = {"raw_column": column}
                    if sku_name is not None:
                        payload["sku_name"] = sku_name
                    readings.append(
                        _reading(code, values[index], observed, 15, resource_uid, payload)
                    )
            _LAST_SEEN[resource_uid] = new_last
    except Exception as exc:  # noqa: BLE001
        logger.warning("sql_diagnostics resource stats unavailable for %s: %s",
                       resource_uid, exc)
    finally:
        if conn is not None:
            conn.close()

    # Sesiones (punto actual, no incremental).
    conn = None
    try:
        conn = db_azure.connect(cfg)
        value = db_azure.scalar(conn, SESSIONS_QUERY)
        if value is not None:
            payload = {"raw_column": "sessions"}
            if sku_name is not None:
                payload["sku_name"] = sku_name
            readings.append(
                _reading(
                    "sessions_count", value, dt.datetime.now(dt.timezone.utc), 60,
                    resource_uid, payload,
                )
            )
    except Exception as exc:  # noqa: BLE001
        logger.debug("sql_diagnostics sessions unavailable for %s: %s", resource_uid, exc)
    finally:
        if conn is not None:
            conn.close()

    return readings


def reset_last_seen(resource_uid: Optional[str] = None) -> None:
    """Descansa el cursor incremental (tests / nuevo experimento)."""
    if resource_uid is None:
        _LAST_SEEN.clear()
    else:
        _LAST_SEEN.pop(resource_uid, None)
