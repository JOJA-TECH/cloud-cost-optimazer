"""Supabase PostgreSQL connectivity (analytical persistence).

Only the VM talks to Supabase. The connection uses psycopg2 with SSL and a
connect timeout so failures surface quickly into the retry/spool path.
"""

from __future__ import annotations

import logging
import time
from contextlib import contextmanager
from typing import TYPE_CHECKING, Iterator

from .config import Config

if TYPE_CHECKING:  # pragma: no cover - annotations only
    import psycopg2

logger = logging.getLogger(__name__)


class SupabaseConnectionError(RuntimeError):
    pass


def _psycopg2():
    """Import psycopg2 lazily so the package imports without the driver."""
    try:
        import psycopg2
        import psycopg2.extras  # noqa: F401
        return psycopg2
    except ImportError as exc:
        raise SupabaseConnectionError("psycopg2 is not installed") from exc


def connect(cfg: Config, timeout: int = 15) -> "psycopg2.extensions.connection":
    pg = _psycopg2()
    dsn = cfg.supabase_dsn()
    try:
        conn = pg.connect(dsn)
        conn.autocommit = False
        return conn
    except pg.Error as exc:
        logger.error("Supabase connection failed: %s", exc)
        raise SupabaseConnectionError(str(exc)) from exc


@contextmanager
def transaction(cfg: Config) -> Iterator["psycopg2.extensions.connection"]:
    conn = connect(cfg)
    try:
        yield conn
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()


def insert_dict(conn, table: str, data: dict) -> int:
    """Insert one row; returns affected rows (0/1)."""
    columns = [k for k in data.keys()]
    values = [data[k] for k in columns]
    placeholders = ", ".join(["%s"] * len(values))
    sql = (
        f"insert into {table} ({', '.join(columns)}) "
        f"values ({placeholders})"
    )
    with conn.cursor() as cur:
        cur.execute(sql, values)
    return max(cur.rowcount, 0)


def insert_dict_on_conflict(
    conn, table: str, data: dict, conflict_keys: list[str],
    do_nothing: bool = True,
) -> int:
    """INSERT ... ON CONFLICT for idempotent retries (returns rowcount)."""
    columns = [k for k in data.keys()]
    values = [data[k] for k in columns]
    placeholders = ", ".join(["%s"] * len(values))
    conflict = ", ".join(conflict_keys)
    tail = "DO NOTHING" if do_nothing else "DO UPDATE SET updated_at = now()"
    sql = (
        f"insert into {table} ({', '.join(columns)}) "
        f"values ({placeholders}) "
        f"on conflict ({conflict}) {tail}"
    )
    with conn.cursor() as cur:
        cur.execute(sql, values)
    return max(cur.rowcount, 0)


def query(conn, sql: str, params: tuple = ()) -> list[dict]:
    psycopg2 = _psycopg2()
    with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
        cur.execute(sql, params)
        return list(cur.fetchall())


def ping(cfg: Config) -> float:
    """Return latency in ms or raise SupabaseConnectionError."""
    start = time.monotonic()
    with transaction(cfg) as conn:
        with conn.cursor() as cur:
            cur.execute("select 1")
            cur.fetchone()
    return (time.monotonic() - start) * 1000.0
