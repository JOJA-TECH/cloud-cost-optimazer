"""Capacity, wait and IO diagnostics (DMVs the base collector ignored).

Everything here runs through the workload connection; ``VIEW DATABASE STATE``
is enough (verified against Azure SQL DB with the non-admin login).

Design rules:

- **Rates, not totals.** Counters that only grow (waits, file IO) are stored as
  ``per second`` deltas, so a reading does not depend on the cycle length and a
  database pause/resume does not fabricate a spike.
- **Caps travel with demand.** ``cpu_limit``, ``max_dop``, ``max_sessions``,
  ``max_memory_grant`` and the log rate cap are stored as metrics, so a
  utilization percentage can be sized without a second source.
- **Slow inventory is collected once per 30 min.** Index usage, VLF count and
  tuning recommendations do not change within a tick; re-reading them every
  minute only burns free-tier vCore-seconds.
- Any failure degrades gracefully: the base collector keeps working.
"""

from __future__ import annotations

import datetime as dt
import logging
import time
from typing import Dict, List, Optional, Tuple

from .. import db_azure
from ..config import Config
from .normalizer import MetricReading, unit_for

logger = logging.getLogger(__name__)

SLOW_INTERVAL_SECONDS = 1800

# Wait types that mean the platform is enforcing a cap. They are the ones worth
# alerting on: a high SOS_SCHEDULER_YIELD is your own serialization, a high
# LOG_RATE_GOVERNOR is the service telling you to slow down.
CRITICAL_WAITS = frozenset({
    "IO_QUEUE_LIMIT",
    "IO_RETRY",
    "LOG_RATE_GOVERNOR",
    "POOL_LOG_RATE_GOVERNOR",
    "RESMGR_THROTTLED",
    "RESOURCE_SEMAPHORE",
    "RESOURCE_SEMAPHORE_QUERY_COMPILE",
    "SE_REPL_CATCHUP_THROTTLE",
    "SE_REPL_COMMIT_ACK",
    "SE_REPL_COMMIT_TURN",
    "SE_REPL_ROLLBACK_ACK",
    "SE_REPL_SLOW_SECONDARY_THROTTLE",
    "THREADPOOL",
})

# Background/idle waits. They dominate raw sums (SOS_WORK_DISPATCHER alone was
# 222 s in a 10 s sample) and would drown the real signal.
BENIGN_PREFIXES = (
    "SLEEP_",
    "HADR_",
    "XE_",
    "BROKER_",
    "SQLTRACE_",
    "QDS_",
    "PREEMPTIVE_",
    "LAZYWRITER",
    "CHECKPOINT_QUEUE",
    "LOGMGR_QUEUE",
    "DIRTY_PAGE_POLL",
    "SERVER_IDLE_CHECK",
    "RESOURCE_QUEUE",
    "SNI_",
    "WAITFOR",
    "SOS_WORK_DISPATCHER",
    "RESOURCE_GOVERNOR_IDLE",
    "FT_IFTS",
    "PARALLEL_REDO",
    "PWAIT_",
    "VDI_CLIENT",
    "KSOURCE_WAKEUP",
    "SP_SERVER_DIAGNOSTICS",
    "MEMORY_ALLOCATION_EXT",
    "STARTUP_DEPENDENCY_MANAGER",
    "ONDEMAND_TASK_QUEUE",
    "REQUEST_FOR_DEADLOCK_SEARCH",
    "DISPATCHER_QUEUE_SEMAPHORE",
    "DBMIRROR",
    "PVS_PREALLOCATE",
    "WAIT_XTP_",
    "CLR_",
    "EXECSYNC",
)

WAIT_CLASSES = ("cpu", "log", "lock", "io", "memory", "other")

_CPU_WAITS = frozenset({
    "SOS_SCHEDULER_YIELD", "CXPACKET", "CXCONSUMER", "THREADPOOL",
    "SOS_WORK_DISPATCHER",
})
_LOG_WAITS = frozenset({
    "WRITELOG", "LOG_RATE_GOVERNOR", "POOL_LOG_RATE_GOVERNOR", "LOGBUFFER",
    "WRITE_COMPLETION", "LOGMGR", "LOGMGR_RESERVE_APPEND",
})
_IO_WAITS = frozenset({
    "PAGEIOLATCH_SH", "PAGEIOLATCH_EX", "PAGEIOLATCH_UP", "IO_COMPLETION",
    "IO_QUEUE_LIMIT", "IO_RETRY", "ASYNC_IO_COMPLETION", "BACKUPIO",
    "BACKUPTHREAD",
})
_MEMORY_WAITS = frozenset({
    "RESOURCE_SEMAPHORE", "RESOURCE_SEMAPHORE_QUERY_COMPILE", "CMEMTHREAD",
    "RESMGR_THROTTLED",
})

WAITS_QUERY = "SELECT wait_type, wait_time_ms FROM sys.dm_os_wait_stats WHERE wait_time_ms > 0"

GUID_QUERY = "SELECT physical_database_guid FROM sys.dm_user_db_resource_governance"

# Gauges only. In Azure SQL the per-second counters from this DMV are NOT
# instantaneous rates (measured: they barely move while inserting 20k rows and
# can stay frozen for minutes), so storing them as per-second would be a lie.
# Throughput comes from workload_self / Query Store; log throughput is derived
# from log_write_percent x log_rate_cap_bytes_per_sec.
COUNTER_CODES: Dict[str, str] = {
    "Page life expectancy": "page_life_expectancy_sec",
    "Memory Grants Pending": "memory_grants_pending",
    "Total Server Memory (KB)": "total_server_memory_bytes",
    "Database Cache Memory (KB)": "database_cache_memory_bytes",
}
KB_COUNTERS = frozenset({"Total Server Memory (KB)", "Database Cache Memory (KB)"})

IO_QUERY = (
    "SELECT f.type_desc, v.num_of_bytes_read, v.num_of_bytes_written, "
    "       v.io_stall_read_ms, v.io_stall_write_ms "
    "FROM sys.dm_io_virtual_file_stats(DB_ID(), NULL) v "
    "JOIN sys.database_files f ON f.file_id = v.file_id"
)

# Only user requests count as congestion: internal tasks (HADR_*, XIO_*,
# PARALLEL REDO) are always "background/running" and would report a busy
# database with no workload connected. @@SPID excludes the collector itself.
REQUESTS_QUERY = (
    "SELECT COUNT(*) AS running, "
    "       SUM(CASE WHEN r.blocking_session_id <> 0 THEN 1 ELSE 0 END) AS blocked, "
    "       ISNULL(MAX(DATEDIFF(MILLISECOND, r.start_time, GETUTCDATE())), 0) AS longest_ms "
    "FROM sys.dm_exec_requests r "
    "JOIN sys.dm_exec_sessions s ON s.session_id = r.session_id "
    "WHERE s.is_user_process = 1 AND r.database_id = DB_ID() AND r.session_id <> @@SPID"
)

LOG_STATS_QUERY = (
    "SELECT total_log_size_mb, total_vlf_count FROM sys.dm_db_log_stats(DB_ID())"
)
INDEX_USAGE_QUERY = (
    "SELECT ISNULL(SUM(user_seeks), 0), ISNULL(SUM(user_scans), 0), "
    "       ISNULL(SUM(user_lookups), 0), ISNULL(SUM(user_updates), 0) "
    "FROM sys.dm_db_index_usage_stats WHERE database_id = DB_ID()"
)
TUNING_QUERY = "SELECT COUNT(*) FROM sys.dm_db_tuning_recommendations"
TEMPDB_QUERY = (
    "SELECT ISNULL(SUM(user_object_reserved_page_count + internal_object_reserved_page_count "
    "            + version_store_reserved_page_count), 0) * 8192 "
    "FROM sys.dm_db_file_space_usage"
)
SCHEDULERS_QUERY = (
    "SELECT ISNULL(SUM(runnable_tasks_count), 0), ISNULL(SUM(work_queue_count), 0) "
    "FROM sys.dm_os_schedulers WHERE status = 'VISIBLE ONLINE'"
)
CAPS_QUERY = (
    "SELECT cpu_limit, max_dop, max_sessions, max_memory, max_memory_grant, "
    "       primary_max_log_rate, instance_max_log_rate "
    "FROM sys.dm_user_db_resource_governance"
)

_PREV_WAITS: Dict[str, Tuple[float, dict]] = {}
_PREV_IO: Dict[str, Tuple[float, dict]] = {}
_SLOW_AT: Dict[str, float] = {}


def wait_class(wait_type: str) -> Optional[str]:
    """Classify a wait type, or ``None`` when it is background noise."""
    name = (wait_type or "").strip().upper()
    if name.startswith(BENIGN_PREFIXES):
        return None
    if name.startswith("LCK_"):
        return "lock"
    if name.startswith("PAGEIOLATCH_") or name in _IO_WAITS:
        return "io"
    if name in _LOG_WAITS:
        return "log"
    if name in _CPU_WAITS:
        return "cpu"
    if name in _MEMORY_WAITS:
        return "memory"
    return "other"


def rates(current: dict, previous: Optional[dict], elapsed: float) -> dict:
    """Per-second deltas. Empty when there is no baseline or a counter reset."""
    if previous is None or elapsed <= 0:
        return {}
    if any(value < previous.get(key, 0.0) for key, value in current.items()):
        # sys.dm_os_wait_stats and file stats restart when the database resumes.
        return {}
    return {
        key: (value - previous.get(key, 0.0)) / elapsed
        for key, value in current.items()
    }


def _snapshot(state: dict, resource_uid: str, current: dict) -> Tuple[Optional[dict], float]:
    now = time.monotonic()
    previous = state.get(resource_uid)
    state[resource_uid] = (now, current)
    if previous is None:
        return None, 0.0
    return previous[1], now - previous[0]


def _reading(code: str, value, resource_uid: str, interval: int, payload: dict) -> MetricReading:
    return MetricReading(
        metric_code=code,
        value=float(value),
        unit=unit_for(code),
        observed_at_utc=dt.datetime.now(dt.timezone.utc),
        interval_seconds=interval,
        source="sql_diagnostics",
        resource_uid=resource_uid,
        source_payload=payload,
    )


def fetch(cfg: Config, resource_uid: str, interval: int = 60,
          sku_name: Optional[str] = None) -> List[MetricReading]:
    readings: List[MetricReading] = []
    conn = None
    try:
        conn = db_azure.connect(cfg)
        cur = conn.cursor()
        readings += _waits(cur, resource_uid, interval)
        readings += _counters(cur, resource_uid, interval, sku_name)
        readings += _io(cur, resource_uid, interval)
        readings += _requests(cur, resource_uid, interval)
        readings += _slow(cur, resource_uid, sku_name)
    except Exception as exc:  # noqa: BLE001 - extras must never break capture
        logger.warning("sql_extras unavailable for %s: %s", resource_uid, exc)
    finally:
        if conn is not None:
            conn.close()
    return readings


def _waits(cur, resource_uid: str, interval: int) -> List[MetricReading]:
    cur.execute(WAITS_QUERY)
    current = {str(row[0]).strip(): float(row[1]) for row in cur.fetchall()}
    previous, elapsed = _snapshot(_PREV_WAITS, resource_uid, current)
    delta = rates(current, previous, elapsed)
    if not delta:
        return []

    per_class = {name: 0.0 for name in WAIT_CLASSES}
    critical = 0.0
    for wait_type, per_second in delta.items():
        if wait_type in CRITICAL_WAITS:
            critical += per_second
        name = wait_class(wait_type)
        if name is not None:
            per_class[name] += per_second

    payload = {"raw_column": "dm_os_wait_stats", "elapsed_seconds": round(elapsed, 3)}
    readings = [
        _reading(f"wait_{name}_ms_per_sec", value, resource_uid, interval, payload)
        for name, value in per_class.items()
    ]
    readings.append(
        _reading("wait_critical_ms_per_sec", critical, resource_uid, interval, payload)
    )
    return readings


def _counter_priority(instance: str, guid: str) -> int:
    value = (instance or "").strip().upper()
    if value and value == guid:
        return 0
    if not value:
        return 1
    if value == "_TOTAL":
        return 2
    return 3


def _counters(cur, resource_uid: str, interval: int,
              sku_name: Optional[str]) -> List[MetricReading]:
    cur.execute(GUID_QUERY)
    row = cur.fetchone()
    guid = str(row[0]).strip().upper() if row and row[0] else ""

    names = list(COUNTER_CODES)
    placeholders = ", ".join("?" for _ in names)
    cur.execute(
        "SELECT LTRIM(RTRIM(counter_name)), LTRIM(RTRIM(instance_name)), cntr_value "
        f"FROM sys.dm_os_performance_counters "
        f"WHERE LTRIM(RTRIM(counter_name)) IN ({placeholders})",
        names,
    )
    best: Dict[str, Tuple[int, float]] = {}
    for name, instance, value in cur.fetchall():
        name = str(name).strip()
        rank = _counter_priority(instance, guid)
        if name not in best or rank < best[name][0]:
            best[name] = (rank, float(value))

    readings: List[MetricReading] = []
    for name, (_, value) in best.items():
        code = COUNTER_CODES[name]
        if name in KB_COUNTERS:
            value *= 1024.0
        payload = {"raw_counter": name}
        if sku_name is not None:
            payload["sku_name"] = sku_name
        readings.append(_reading(code, value, resource_uid, interval, payload))
    return readings


def _io(cur, resource_uid: str, interval: int) -> List[MetricReading]:
    cur.execute(IO_QUERY)
    current = {
        "rows_read_bytes": 0.0,
        "rows_write_bytes": 0.0,
        "rows_stall_ms": 0.0,
        "log_stall_ms": 0.0,
    }
    for type_desc, read_bytes, write_bytes, stall_read, stall_write in cur.fetchall():
        if str(type_desc).upper() == "LOG":
            # Log bytes written are virtualised by the platform and under-report
            # by orders of magnitude; only the stall accumulator is meaningful.
            current["log_stall_ms"] += float(stall_write)
        else:
            current["rows_read_bytes"] += float(read_bytes)
            current["rows_write_bytes"] += float(write_bytes)
            current["rows_stall_ms"] += float(stall_read) + float(stall_write)

    previous, elapsed = _snapshot(_PREV_IO, resource_uid, current)
    delta = rates(current, previous, elapsed)
    if not delta:
        return []
    payload = {"raw_column": "dm_io_virtual_file_stats", "elapsed_seconds": round(elapsed, 3)}
    return [
        _reading(f"{key}_per_sec", value, resource_uid, interval, payload)
        for key, value in delta.items()
    ]


def _requests(cur, resource_uid: str, interval: int) -> List[MetricReading]:
    cur.execute(REQUESTS_QUERY)
    running, blocked, longest = cur.fetchone()
    payload = {"raw_column": "dm_exec_requests"}
    return [
        _reading("running_requests", running or 0, resource_uid, interval, payload),
        _reading("blocked_requests", blocked or 0, resource_uid, interval, payload),
        _reading("longest_running_ms", longest or 0, resource_uid, interval, payload),
    ]


def _slow(cur, resource_uid: str,
          sku_name: Optional[str]) -> List[MetricReading]:
    last = _SLOW_AT.get(resource_uid)
    if last is not None and time.monotonic() - last < SLOW_INTERVAL_SECONDS:
        return []
    _SLOW_AT[resource_uid] = time.monotonic()

    readings: List[MetricReading] = []
    interval = SLOW_INTERVAL_SECONDS

    def emit(sql: str, codes: List[str], scales=None) -> None:
        try:
            cur.execute(sql)
            row = cur.fetchone()
        except Exception as exc:  # noqa: BLE001 - one missing DMV must not stop the rest
            logger.debug("sql_extras slow query skipped: %s", exc)
            return
        if not row:
            return
        scales = scales or [1.0] * len(codes)
        for code, value, scale in zip(codes, row, scales):
            if value is None:
                continue
            readings.append(
                _reading(code, float(value) * scale, resource_uid, interval,
                         {"raw_column": "slow_inventory"})
            )

    # dm_db_log_stats reports MB and a plain count: scale per column.
    emit(LOG_STATS_QUERY, ["log_size_bytes", "log_vlf_count"],
         scales=[1024 * 1024, 1.0])
    emit(INDEX_USAGE_QUERY,
         ["index_seeks_total", "index_scans_total", "index_lookups_total",
          "index_updates_total"])
    emit(TUNING_QUERY, ["tuning_recommendations_count"])
    emit(TEMPDB_QUERY, ["tempdb_used_bytes"])
    emit(SCHEDULERS_QUERY, ["runnable_tasks", "work_queue_count"])

    # Caps: the denominator every utilisation percentage needs.
    try:
        cur.execute(CAPS_QUERY)
        row = cur.fetchone()
    except Exception as exc:  # noqa: BLE001
        logger.debug("sql_extras governance unavailable: %s", exc)
        row = None
    if row:
        payload = {"raw_column": "dm_user_db_resource_governance"}
        if sku_name is not None:
            payload["sku_name"] = sku_name
        for code, value in zip(
            ["cpu_limit_vcores", "max_dop", "max_sessions", "max_memory_percent",
             "max_memory_grant_percent", "log_rate_cap_bytes_per_sec",
             "instance_max_log_rate_bytes_per_sec"],
            row,
        ):
            if value is None:
                continue
            readings.append(_reading(code, value, resource_uid, interval, payload))
    return readings


def reset(resource_uid: Optional[str] = None) -> None:
    """Drop the delta baselines (tests / new experiment)."""
    for state in (_PREV_WAITS, _PREV_IO, _SLOW_AT):
        if resource_uid is None:
            state.clear()
        else:
            state.pop(resource_uid, None)
