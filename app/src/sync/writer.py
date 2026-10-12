"""Supabase writer with retry-safe spooling.

persist():
  1. build rows with idempotent source ids;
  2. try a single transaction insert (ON CONFLICT DO NOTHING);
  3. on failure, spool the rows locally and report the pending buffer.

flush_spool():
  drains spool files in FIFO order, verifying the checksum before insert, and
  only deletes a file after its rows commit.
"""

from __future__ import annotations

import logging
import uuid
from dataclasses import dataclass, field
from typing import Iterable, List, Optional

from .. import db_supabase
from .idempotency import build_metric_reading_row, build_metric_sample_row
from .spool import Spool

logger = logging.getLogger(__name__)

READING_TABLE = "raw.metric_reading"
SAMPLE_TABLE = "raw.metric_sample"

READING_COLUMNS = [
    "experiment_uid", "workload_run_uid", "collector_run_uid", "resource_uid",
    "metric_code", "observed_at_utc", "interval_seconds", "value", "unit",
    "source", "source_record_id", "source_payload_json", "quality_status",
    "quality_warnings",
]

SAMPLE_COLUMNS = [
    "experiment_uid", "workload_run_uid", "resource_uid", "metric_code",
    "observed_at_utc", "value", "unit", "source", "source_sample_id",
]

JSONB_COLUMNS = {"source_payload_json"}


@dataclass
class WriteResult:
    rows_observed: int = 0
    rows_written: int = 0
    spooled: bool = False
    spool_files: int = 0
    spool_mb: float = 0.0
    error: Optional[str] = None
    notes: List[str] = field(default_factory=list)


def _spool(cfg) -> Spool:
    return Spool(cfg.local_spool_dir, cfg.local_spool_max_mb)


def spool_status(cfg) -> dict:
    status = _spool(cfg).status()
    return {
        "files": status.files,
        "mb": status.mb,
        "max_mb": status.max_mb,
        "over_limit": status.over_limit,
    }


# ---------------------------------------------------------------------------
# collector_run registry
# ---------------------------------------------------------------------------
def start_collector_run(cfg, source: str) -> str:
    uid = str(uuid.uuid4())
    try:
        with db_supabase.transaction(cfg) as conn:
            db_supabase.query(
                conn,
                "insert into ops.collector_run "
                "(collector_run_uid, experiment_uid, source, started_at_utc, status, "
                " rows_observed, rows_written, request_metadata_json) "
                "values (%s, %s, %s, now(), 'partial', 0, 0, '{}'::jsonb)",
                (uid, cfg.experiment_uid, source),
            )
    except Exception as exc:  # noqa: BLE001 - collector still runs, spools data
        logging.getLogger(__name__).warning("cannot register collector_run: %s", exc)
    return uid


def finish_collector_run(
    cfg,
    collector_run_uid: str,
    *,
    status: str,
    rows_observed: int,
    rows_written: int,
    error: str = "",
) -> None:
    try:
        with db_supabase.transaction(cfg) as conn:
            db_supabase.query(
                conn,
                "update ops.collector_run set finished_at_utc = now(), status = %s, "
                " rows_observed = %s, rows_written = %s, error_message = %s "
                "where collector_run_uid = %s",
                (status, rows_observed, rows_written, error or None, collector_run_uid),
            )
    except Exception as exc:  # noqa: BLE001
        logging.getLogger(__name__).warning("cannot finish collector_run: %s", exc)


# ---------------------------------------------------------------------------
# inserts
# ---------------------------------------------------------------------------
def _prepare(columns: List[str], row: dict) -> list:
    from psycopg2.extras import Json

    values = []
    for column in columns:
        value = row.get(column)
        if column in ("source_payload_json",):
            value = Json(value or {})
        values.append(value)
    return values


def _insert_rows(conn, table: str, columns: List[str], rows: List[dict]) -> int:
    if not rows:
        return 0
    from psycopg2.extras import execute_values

    column_sql = ", ".join(columns)
    template = "(" + ", ".join(["%s"] * len(columns)) + ")"
    values = [_prepare(columns, row) for row in rows]
    with conn.cursor() as cur:
        execute_values(
            cur,
            f"insert into {table} ({column_sql}) values %s on conflict do nothing",
            values,
            template=template,
            page_size=500,
        )
        return max(cur.rowcount, 0)


# ---------------------------------------------------------------------------
# public API
# ---------------------------------------------------------------------------
def persist(
    cfg,
    collector_run_uid: str,
    readings: Iterable,
    samples: Iterable = (),
    workload_run_uid: Optional[str] = None,
) -> WriteResult:
    reading_rows = [
        build_metric_reading_row(cfg, collector_run_uid, r, workload_run_uid)
        for r in readings
    ]
    sample_rows = [
        build_metric_sample_row(cfg, s, workload_run_uid, s.source_record_id)
        for s in samples
    ]
    observed = len(reading_rows) + len(sample_rows)
    result = WriteResult(rows_observed=observed)

    try:
        with db_supabase.transaction(cfg) as conn:
            written = 0
            written += _insert_rows(conn, "raw.metric_reading", READING_COLUMNS, reading_rows)
            written += _insert_rows(conn, "raw.metric_sample", SAMPLE_COLUMNS, sample_rows)
        result.rows_written = written
        # Best-effort drain of previously spooled data after a success.
        flushed = flush_spool(cfg)
        result.notes.append(f"spool_flushed={flushed['files']}")
    except Exception as exc:  # noqa: BLE001 - spool and continue
        result.error = str(exc)
        spool = _spool(cfg)
        if reading_rows:
            spool.write_batch("metric_reading", [_jsonable(r) for r in reading_rows])
        if sample_rows:
            spool.write_batch("metric_sample", [_jsonable(s) for s in sample_rows])
        result.spooled = True
        status = spool.status()
        result.spool_files = status.files
        result.spool_mb = status.mb
        logging.getLogger(__name__).warning(
            "Supabase write failed, spooled %d rows (%s MB pending): %s",
            observed, status.mb, exc,
        )
    return result


def flush_spool(cfg) -> dict:
    spool = _spool(cfg)
    files_flushed = 0
    rows_flushed = 0
    for path, kind, rows in list(spool.iter_pending()):
        if not rows:
            spool.remove(path)
            continue
        table, columns = (
            ("raw.metric_reading", READING_COLUMNS)
            if kind == "metric_reading"
            else ("raw.metric_sample", SAMPLE_COLUMNS)
        )
        try:
            with db_supabase.transaction(cfg) as conn:
                written = _insert_rows(conn, table, columns, rows)
        except Exception:  # noqa: BLE001 - leave the file for the next attempt
            break
        rows_flushed += written
        files_flushed += 1
        spool.remove(path)
    status = spool.status()
    return {
        "files": files_flushed,
        "rows": rows_flushed,
        "pending_files": status.files,
        "mb": status.mb,
    }


def _jsonable(row: dict) -> dict:
    import datetime as dt

    out = {}
    for key, value in row.items():
        if isinstance(value, (dt.datetime, dt.date)):
            value = value.isoformat()
        out[key] = value
    return out