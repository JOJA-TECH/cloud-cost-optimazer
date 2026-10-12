"""Idempotent database migrations.

Two independent stores with independent runners:

- Azure SQL (system under test): migrations are T-SQL files that assume the
  `loadtest` schema exists (created by the bootstrap step). They may use the
  secret placeholders __AZURE_SQL_WORKLOAD_USER__ / __AZURE_SQL_WORKLOAD_PASSWORD__,
  substituted at runtime from Config — never stored in the repository.
- Supabase (analytical persistence): migrations are PostgreSQL files executed
  against the Supabase project with the configured database user.

Each runner records applied migrations (name + sha256) in a marker table so
re-runs are no-ops.

Usage:
    python -m src.migrations --apply          # apply both stores
    python -m src.migrations --apply-azure    # apply Azure SQL only
    python -m src.migrations --apply-supabase # apply Supabase only
    python -m src.migrations --status         # print applied migrations
"""

from __future__ import annotations

import argparse
import hashlib
import logging
import sys
from pathlib import Path

from .config import Config, from_env, load_env_file
from . import db_azure, db_supabase

logger = logging.getLogger(__name__)

MIGRATIONS_ROOT = Path(__file__).resolve().parent.parent / "migrations"

AZURE_MARKER = "loadtest._schema_migrations"
SUPABASE_MARKER = "ops.schema_migrations"


# ---------------------------------------------------------------------------
# Azure SQL
# ---------------------------------------------------------------------------
def _bootstrap_azure(cfg: Config) -> None:
    conn = db_azure.connect_admin(cfg)
    try:
        db_azure.execute(
            conn,
            "IF NOT EXISTS (SELECT 1 FROM sys.schemas WHERE name = N'loadtest') "
            "EXEC(N'CREATE SCHEMA loadtest')",
        )
        db_azure.execute(
            conn,
            f"IF OBJECT_ID(N'{AZURE_MARKER}', N'U') IS NULL "
            "EXEC('CREATE TABLE "
            f"{AZURE_MARKER} (name nvarchar(255) NOT NULL PRIMARY KEY, "
            "checksum nvarchar(64) NOT NULL, applied_at_utc datetime2 "
            "NOT NULL DEFAULT SYSUTCDATETIME())')",
        )
    finally:
        conn.close()


def _read_azure_migration(cfg: Config, path: Path) -> str:
    sql = path.read_text(encoding="utf-8")
    sql = sql.replace("__AZURE_SQL_WORKLOAD_USER__", cfg.azure_sql_user)
    sql = sql.replace("__AZURE_SQL_WORKLOAD_PASSWORD__", cfg.azure_sql_password)
    return sql


def apply_azure_sql(cfg: Config) -> list[str]:
    _bootstrap_azure(cfg)
    conn = db_azure.connect_admin(cfg)
    applied: list[str] = []
    try:
        files = sorted((MIGRATIONS_ROOT / "azure_sql").glob("*.sql"))
        for path in files:
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            already = db_azure.scalar(
                conn,
                f"SELECT 1 FROM {AZURE_MARKER} WHERE name = ?",
                (path.name,),
            )
            if already:
                continue
            sql = _read_azure_migration(cfg, path)
            conn.autocommit = False
            try:
                db_azure.execute(conn, sql)
                db_azure.execute(
                    conn,
                    f"INSERT INTO {AZURE_MARKER} (name, checksum) VALUES (?, ?)",
                    (path.name, digest),
                )
                conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.autocommit = True
            applied.append(path.name)
            logger.info("applied azure_sql/%s", path.name)
    finally:
        conn.close()
    return applied


def azure_sql_status(cfg: Config) -> list[dict]:
    try:
        conn = db_azure.connect_admin(cfg)
    except db_azure.AzureSQLConnectionError:
        return []
    try:
        rows = db_azure.execute(
            conn,
            f"SELECT name, checksum, CONVERT(varchar(33), applied_at_utc, 126) AS applied_at "
            f"FROM {AZURE_MARKER} ORDER BY name",
        )
        return [
            {"name": r[0], "checksum": r[1], "applied_at_utc": r[2]} for r in rows
        ]
    finally:
        conn.close()


# ---------------------------------------------------------------------------
# Supabase
# ---------------------------------------------------------------------------
def _bootstrap_supabase(cfg: Config) -> None:
    with db_supabase.transaction(cfg) as conn:
        with conn.cursor() as cur:
            for schema in ("experiment", "raw", "feat", "ml", "ops"):
                cur.execute(f"CREATE SCHEMA IF NOT EXISTS {schema}")
            cur.execute(
                "CREATE TABLE IF NOT EXISTS ops.schema_migrations ("
                " name text PRIMARY KEY,"
                " checksum text NOT NULL,"
                " applied_at_utc timestamptz NOT NULL DEFAULT now())"
            )


def apply_supabase(cfg: Config) -> list[str]:
    _bootstrap_supabase(cfg)
    applied: list[str] = []
    files = sorted((MIGRATIONS_ROOT / "supabase").glob("*.sql"))
    with db_supabase.transaction(cfg) as conn:
        for path in files:
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            already = db_supabase.query(
                conn, "select 1 from ops.schema_migrations where name = %s", (path.name,)
            )
            if already:
                continue
            sql = path.read_text(encoding="utf-8")
            with conn.cursor() as cur:
                cur.execute(sql)
            with conn.cursor() as cur:
                cur.execute(
                    "insert into ops.schema_migrations (name, checksum) "
                    "values (%s, %s)",
                    (path.name, digest),
                )
            applied.append(path.name)
            logger.info("applied supabase/%s", path.name)
    return applied


def supabase_status(cfg: Config) -> list[dict]:
    try:
        with db_supabase.transaction(cfg) as conn:
            return db_supabase.query(
                conn,
                "select name, checksum, "
                "to_char(applied_at_utc, 'YYYY-MM-DD\\\"T\\\"HH24:MI:SS') as applied_at "
                "from ops.schema_migrations order by name",
            )
    except db_supabase.SupabaseConnectionError:
        return []


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------
def _setup_logging() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="CCO database migrations")
    parser.add_argument(
        "--apply", action="store_true", help="apply both Azure SQL and Supabase"
    )
    parser.add_argument("--apply-azure", action="store_true", help="apply Azure SQL only")
    parser.add_argument(
        "--apply-supabase", action="store_true", help="apply Supabase only"
    )
    parser.add_argument(
        "--status", action="store_true", help="print applied migrations"
    )
    args = parser.parse_args()

    _setup_logging()
    load_env_file()
    cfg = from_env()

    if args.status:
        print("Azure SQL:", azure_sql_status(cfg) or "unreachable/empty")
        print("Supabase:", supabase_status(cfg) or "unreachable/empty")
        return 0

    if not (args.apply or args.apply_azure or args.apply_supabase):
        parser.print_help()
        return 1

    try:
        if args.apply or args.apply_azure:
            az = apply_azure_sql(cfg)
            print(f"azure_sql applied: {az or 'nothing'}")
        if args.apply or args.apply_supabase:
            su = apply_supabase(cfg)
            print(f"supabase applied: {su or 'nothing'}")
    except Exception as exc:  # noqa: BLE001 - surface migration failure to systemd
        logger.error("migration failed: %s", exc)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())