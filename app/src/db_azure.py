"""Azure SQL connectivity (system under test).

Two credentials are supported:
- admin:  used ONLY for migrations and provisioning (schema + workload login).
- workload: least privilege login used by the load generator and collectors.

pyodbc + "ODBC Driver 18 for SQL Server" (installed by cloud-init).
"""

from __future__ import annotations

import logging
from typing import Optional

import pyodbc

from .config import Config

logger = logging.getLogger(__name__)


class AzureSQLConnectionError(RuntimeError):
    pass


def _server(cfg: Config) -> str:
    server = cfg.azure_sql_server
    if not server:
        raise AzureSQLConnectionError("AZURE_SQL_SERVER is empty")
    if server.startswith("tcp:"):
        return server
    return server


def build_connection_string(
    cfg: Config,
    *,
    admin: bool = False,
    database: Optional[str] = None,
    timeout: int = 30,
) -> str:
    user = cfg.azure_sql_admin_user if admin else cfg.azure_sql_user
    password = cfg.azure_sql_admin_password if admin else cfg.azure_sql_password
    db = database or cfg.azure_sql_database
    if not user or not password:
        raise AzureSQLConnectionError("Azure SQL credentials are not configured")
    return (
        f"DRIVER={{{cfg.azure_sql_driver}}};"
        f"SERVER={_server(cfg)},{cfg.azure_sql_port};"
        f"DATABASE={db};"
        f"UID={user};"
        f"PWD={password};"
        f"Encrypt={'yes' if cfg.azure_sql_encrypt else 'no'};"
        f"TrustServerCertificate={'yes' if cfg.azure_sql_trust_server_certificate else 'no'};"
        f"Connection Timeout={timeout};"
    )


def connect(
    cfg: Config,
    *,
    admin: bool = False,
    database: Optional[str] = None,
    timeout: int = 30,
) -> pyodbc.Connection:
    connection_string = build_connection_string(
        cfg, admin=admin, database=database, timeout=timeout
    )
    try:
        conn = pyodbc.connect(connection_string, autocommit=False)
        conn.autocommit = True
        return conn
    except pyodbc.Error as exc:  # pragma: no cover - depends on ODBC driver
        logger.error("Azure SQL connection failed: %s", exc)
        raise AzureSQLConnectionError(str(exc)) from exc


def connect_admin(cfg: Config, database: Optional[str] = None) -> pyodbc.Connection:
    return connect(cfg, admin=True, database=database)


def execute(conn: pyodbc.Connection, sql: str, params: tuple = ()) -> list[pyodbc.Row]:
    with conn.cursor() as cursor:
        cursor.execute(sql, params)
        if cursor.description:
            return list(cursor.fetchall())
        return []


def execute_many(conn: pyodbc.Connection, sql: str, params_list: list[tuple]) -> int:
    with conn.cursor() as cursor:
        cursor.fast_executemany = True
        cursor.executemany(sql, params_list)
        rows = max(cursor.rowcount, 0)
        conn.commit()
        return rows


def scalar(conn: pyodbc.Connection, sql: str, params: tuple = ()) -> Optional[object]:
    with conn.cursor() as cursor:
        cursor.execute(sql, params)
        row = cursor.fetchone()
        return row[0] if row else None