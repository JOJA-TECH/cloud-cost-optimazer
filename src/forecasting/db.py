"""PostgreSQL access helpers.

Two databases are involved and the roles are strictly separated:

* the **source** database holds the real collected telemetry. It is opened
  read-only and is never written to.
* the **local** database (docker compose) is the working copy: the schema, the
  imported raw rows, the rebuilt hourly features and the synthetic rows.

The local schema is created by the branch baseline
(``databases/cco_data/cco_data_schema.sql``) plus the idempotent forecasting
migration (``sql/create_forecasting_schema.sql``).
"""

from __future__ import annotations

import contextlib
from collections.abc import Iterable, Iterator, Sequence
from pathlib import Path
from typing import Any

import pandas as pd
import psycopg
from psycopg import sql as pgsql

from .config import SQL_DIR, Settings, redact_dsn

SCHEMA_BASELINE = (
    Path(__file__).resolve().parents[2]
    / "databases"
    / "cco_data"
    / "cco_data_schema.sql"
)
FORECASTING_MIGRATION = SQL_DIR / "create_forecasting_schema.sql"


class DatabaseError(RuntimeError):
    """Raised when a database operation cannot be completed."""


def connect(
    dsn: str, *, read_only: bool = False, autocommit: bool = True
) -> psycopg.Connection:
    """Open a connection to ``dsn``.

    ``read_only=True`` forces a read-only transaction so the source database
    cannot be modified even by accident.
    """
    if not dsn:
        raise DatabaseError("empty DSN: cannot connect")
    try:
        conn = psycopg.connect(dsn, autocommit=autocommit)
    except psycopg.Error as exc:  # pragma: no cover - depends on environment
        raise DatabaseError(f"cannot connect to {redact_dsn(dsn)}: {exc}") from exc
    if read_only:
        with conn.cursor() as cur:
            cur.execute("set default_transaction_read_only = on")
    return conn


@contextlib.contextmanager
def connection(
    dsn: str, *, read_only: bool = False, autocommit: bool = True
) -> Iterator[psycopg.Connection]:
    """Context manager that always closes the connection."""
    conn = connect(dsn, read_only=read_only, autocommit=autocommit)
    try:
        yield conn
    finally:
        conn.close()


def query(
    conn: psycopg.Connection, statement: str, params: Sequence[Any] | None = None
) -> list[dict[str, Any]]:
    """Run ``statement`` and return the rows as plain dictionaries."""
    with conn.cursor() as cur:
        cur.execute(statement, params or ())
        if cur.description is None:
            return []
        columns = [c.name for c in cur.description]
        return [dict(zip(columns, row)) for row in cur.fetchall()]


def read_frame(
    conn: psycopg.Connection, statement: str, params: Sequence[Any] | None = None
) -> pd.DataFrame:
    """Run ``statement`` and return the result as a pandas DataFrame."""
    with conn.cursor() as cur:
        cur.execute(statement, params or ())
        if cur.description is None:
            return pd.DataFrame()
        columns = [c.name for c in cur.description]
        rows = cur.fetchall()
    if not rows:
        return pd.DataFrame(columns=columns)
    return pd.DataFrame(rows, columns=columns)


def execute_script(conn: psycopg.Connection, script_path: str | Path) -> None:
    """Execute a SQL script file as a single statement batch."""
    path = Path(script_path)
    if not path.is_file():
        raise DatabaseError(f"SQL script not found: {path}")
    text = path.read_text(encoding="utf-8")
    with conn.cursor() as cur:
        cur.execute(text)


def apply_migrations(conn: psycopg.Connection) -> list[str]:
    """Apply the forecasting migration(s). Idempotent by construction."""
    applied: list[str] = []
    for script in (FORECASTING_MIGRATION,):
        if script.is_file():
            execute_script(conn, script)
            applied.append(script.name)
    return applied


def initialize_local_database(conn: psycopg.Connection) -> dict[str, Any]:
    """Ensure the local database has the base schema and the migration applied."""
    result: dict[str, Any] = {"baseline": str(SCHEMA_BASELINE), "migrations": []}

    exists = read_frame(
        conn,
        "select 1 as ok from information_schema.tables "
        "where table_schema = 'catalog' and table_name = 'metric_catalog'",
    )
    if exists.empty:
        if not SCHEMA_BASELINE.is_file():
            raise DatabaseError(f"baseline schema not found: {SCHEMA_BASELINE}")
        execute_script(conn, SCHEMA_BASELINE)
        result["baseline_applied"] = True
    else:
        result["baseline_applied"] = False

    result["migrations"] = apply_migrations(conn)
    return result


def upsert(
    conn: psycopg.Connection,
    statement: str,
    rows: Iterable[Sequence[Any]],
    *,
    page_size: int = 500,
) -> int:
    """Execute ``statement`` once per row, in batches. Returns rows written."""
    total = 0
    batch: list[Sequence[Any]] = []
    for row in rows:
        batch.append(row)
        if len(batch) >= page_size:
            _executemany(conn, statement, batch)
            total += len(batch)
            batch = []
    if batch:
        _executemany(conn, statement, batch)
        total += len(batch)
    return total


def _executemany(
    conn: psycopg.Connection, statement: str, batch: list[Sequence[Any]]
) -> None:
    with conn.cursor() as cur:
        cur.executemany(statement, batch)


def table_row_counts(
    conn: psycopg.Connection,
    schemas: Sequence[str] = (
        "catalog",
        "core",
        "raw",
        "feat",
        "ml",
        "opt",
        "synthetic",
        "forecasting",
    ),
) -> pd.DataFrame:
    """Return the row count of every base table of the given schemas."""
    rows = query(
        conn,
        """
        select table_schema, table_name
        from information_schema.tables
        where table_schema = any(%s) and table_type = 'BASE TABLE'
        order by table_schema, table_name
        """,
        (list(schemas),),
    )
    if not rows:
        return pd.DataFrame(columns=["table_schema", "table_name", "row_count"])
    counts = []
    for row in rows:
        name = pgsql.Identifier(row["table_schema"], row["table_name"]).as_string(conn)
        value = query(conn, f"select count(*) as n from {name}")[0]["n"]
        counts.append(
            {
                "table_schema": row["table_schema"],
                "table_name": row["table_name"],
                "row_count": int(value),
            }
        )
    return pd.DataFrame(counts)


def local_connection(settings: Settings) -> psycopg.Connection:
    """Open the local working-copy connection."""
    return connect(settings.local_dsn)


def source_connection(settings: Settings) -> psycopg.Connection:
    """Open the read-only source connection."""
    dsn = settings.source_dsn
    if not dsn:
        raise DatabaseError(
            "no source DSN configured: set SOURCE_DSN in .env to read the real telemetry"
        )
    return connect(dsn, read_only=True)
