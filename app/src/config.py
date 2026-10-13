"""Centralized configuration.

All secrets come from the environment (or a local .env file loaded by
python-dotenv). Nothing here is committed; the .env is created on the VM by
cloud-init with mode 0600.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path
from typing import FrozenSet, Optional

try:
    from dotenv import load_dotenv
except ImportError:  # pragma: no cover - dotenv is optional for import-time use
    def load_dotenv(*_args, **_kwargs):
        return False

ENV_PATH = Path(os.environ.get("CCO_ENV_FILE", ".env"))


def load_env_file(path: str | Path = ENV_PATH) -> None:
    load_dotenv(path, override=False)


def _env(name: str, default: str = "") -> str:
    value = os.getenv(name)
    return value.strip() if value is not None else default


def _env_int(name: str, default: int) -> int:
    raw = _env(name, str(default))
    try:
        return int(raw)
    except ValueError:
        return default


def _env_float(name: str, default: float) -> float:
    raw = _env(name, str(default))
    try:
        return float(raw)
    except ValueError:
        return default


def _env_bool(name: str, default: bool) -> bool:
    return _env(name, str(default)).strip().lower() in {
        "1",
        "true",
        "yes",
        "on",
    }


def _env_list(name: str) -> list[str]:
    raw = _env(name, "")
    if not raw:
        return []
    return [item.strip() for item in raw.split(",") if item.strip()]


@dataclass(frozen=True)
class Config:
    # Experiment identity
    experiment_uid: str
    run_environment: str = "azure-vm"

    # Azure SQL (system under test)
    azure_sql_server: str = ""
    azure_sql_database: str = "loadtest"
    azure_sql_port: int = 1433
    azure_sql_admin_user: str = ""
    azure_sql_admin_password: str = ""
    azure_sql_user: str = ""
    azure_sql_password: str = ""
    azure_sql_driver: str = ""
    azure_sql_encrypt: bool = True
    azure_sql_trust_server_certificate: bool = False

    # Supabase (analytical persistence)
    supabase_db_host: str = ""
    supabase_db_port: int = 5432
    supabase_db_name: str = "postgres"
    supabase_db_user: str = ""
    supabase_db_password: str = ""
    supabase_db_sslmode: str = "require"

    # Telegram control
    telegram_bot_token: str = ""
    telegram_allowed_chat_ids: FrozenSet[str] = frozenset()

    # Cadence
    capture_raw_interval_seconds: int = 60
    operational_aggregation_minutes: int = 5
    feature_period: str = "1h"
    forecast_horizon: str = "1h"

    # Lifecycle
    experiment_duration_hours: int = 336
    max_experiment_hours: int = 336
    auto_stop_after_hours: int = 360
    final_flush_timeout_seconds: int = 120

    # Workload load control
    workload_intensity: float = 0.7
    workload_intensity_jitter: float = 0.0

    # Resilience
    supabase_retry_max_attempts: int = 8
    supabase_retry_base_seconds: int = 5
    local_spool_max_mb: int = 500
    local_spool_dir: str = "spool"

    # Safety
    auto_start: bool = False
    max_storage_percent: int = 80

    # Azure control plane (for Azure Monitor reads via managed identity)
    azure_sql_resource_id: str = ""
    azure_subscription_id: str = ""
    azure_resource_group: str = ""
    azure_location: str = ""
    azure_sql_max_size_gb: int = 5

    # Supabase connection string used by psycopg2/asyncpg or the pooler.
    def supabase_dsn(self) -> str:
        return (
            f"host={self.supabase_db_host} port={self.supabase_db_port} "
            f"dbname={self.supabase_db_name} user={self.supabase_db_user} "
            f"password={self.supabase_db_password} sslmode={self.supabase_db_sslmode}"
            f" connect_timeout=15"
        )

    def azure_monitor_resource_id(self) -> Optional[str]:
        """ARM resource id used by Azure Monitor metric queries.

        Prefers an explicit AZURE_SQL_RESOURCE_ID; otherwise reconstructs it
        from subscription/resource group/server/database when available.
        """
        if self.azure_sql_resource_id:
            return self.azure_sql_resource_id
        if self.azure_subscription_id and self.azure_resource_group:
            rid = (
                f"/subscriptions/{self.azure_subscription_id}"
                f"/resourceGroups/{self.azure_resource_group}"
                f"/providers/Microsoft.Sql/servers/{self.azure_sql_server.split('.')[0]}"
                f"/databases/{self.azure_sql_database}"
            )
            return rid
        return None

    @property
    def sql_server_bare(self) -> str:
        """Logical server name without the .database.windows.net suffix."""
        return self.azure_sql_server.split(".")[0]

    def resource_uid(self) -> str:
        """Stable, deterministic public id for the monitored resource.

        Derived from the ARM resource id so the same Azure resource always maps
        to the same UUID without storing any extra secret.
        """
        import hashlib
        import uuid

        base = self.azure_monitor_resource_id() or (
            f"{self.azure_sql_server}/{self.azure_sql_database}"
        )
        digest = hashlib.sha256(base.encode("utf-8")).hexdigest()[:32]
        return str(uuid.UUID(digest))

    def storage_capacity_bytes(self) -> int:
        """Configured maximum storage (bytes) used to validate storage metrics."""
        return int(self.azure_sql_max_size_gb) * 1024 * 1024 * 1024

    def has_required_secrets(self) -> bool:
        required = [
            self.azure_sql_server,
            self.azure_sql_user,
            self.azure_sql_password,
            self.supabase_db_host,
            self.supabase_db_user,
            self.supabase_db_password,
        ]
        return all(required)


def from_env() -> Config:
    allowed = frozenset(_env_list("TELEGRAM_ALLOWED_CHAT_IDS"))
    return Config(
        experiment_uid=_env("EXPERIMENT_UID", ""),
        run_environment=_env("RUN_ENVIRONMENT", "azure-vm"),
        azure_sql_server=_env("AZURE_SQL_SERVER"),
        azure_sql_database=_env("AZURE_SQL_DATABASE", "loadtest"),
        azure_sql_port=_env_int("AZURE_SQL_PORT", 1433),
        azure_sql_admin_user=_env("AZURE_SQL_ADMIN_USER"),
        azure_sql_admin_password=_env("AZURE_SQL_ADMIN_PASSWORD"),
        azure_sql_user=_env("AZURE_SQL_USER"),
        azure_sql_password=_env("AZURE_SQL_PASSWORD"),
        azure_sql_driver=_env("AZURE_SQL_DRIVER", "ODBC Driver 18 for SQL Server"),
        azure_sql_encrypt=_env_bool("AZURE_SQL_ENCRYPT", True),
        azure_sql_trust_server_certificate=_env_bool(
            "AZURE_SQL_TRUST_SERVER_CERTIFICATE", False
        ),
        supabase_db_host=_env("SUPABASE_DB_HOST"),
        supabase_db_port=_env_int("SUPABASE_DB_PORT", 5432),
        supabase_db_name=_env("SUPABASE_DB_NAME", "postgres"),
        supabase_db_user=_env("SUPABASE_DB_USER"),
        supabase_db_password=_env("SUPABASE_DB_PASSWORD"),
        supabase_db_sslmode=_env("SUPABASE_DB_SSLMODE", "require"),
        telegram_bot_token=_env("TELEGRAM_BOT_TOKEN"),
        telegram_allowed_chat_ids=allowed,
        capture_raw_interval_seconds=_env_int("CAPTURE_RAW_INTERVAL_SECONDS", 60),
        operational_aggregation_minutes=_env_int(
            "OPERATIONAL_AGGREGATION_MINUTES", 5
        ),
        feature_period=_env("FEATURE_PERIOD", "1h"),
        forecast_horizon=_env("FORECAST_HORIZON", "1h"),
        experiment_duration_hours=_env_int("EXPERIMENT_DURATION_HOURS", 336),
        max_experiment_hours=_env_int("MAX_EXPERIMENT_HOURS", 336),
        auto_stop_after_hours=_env_int("AUTO_STOP_AFTER_HOURS", 360),
        final_flush_timeout_seconds=_env_int("FINAL_FLUSH_TIMEOUT_SECONDS", 120),
        workload_intensity=_env_float("WORKLOAD_INTENSITY", 0.7),
        workload_intensity_jitter=_env_float("WORKLOAD_INTENSITY_JITTER", 0.0),
        supabase_retry_max_attempts=_env_int("SUPABASE_RETRY_MAX_ATTEMPTS", 8),
        supabase_retry_base_seconds=_env_int("SUPABASE_RETRY_BASE_SECONDS", 5),
        local_spool_max_mb=_env_int("LOCAL_SPOOL_MAX_MB", 500),
        local_spool_dir=_env("LOCAL_SPOOL_DIR", "spool"),
        auto_start=_env_bool("AUTO_START", False),
        max_storage_percent=_env_int("MAX_STORAGE_PERCENT", 80),
        azure_sql_resource_id=_env("AZURE_SQL_RESOURCE_ID"),
        azure_subscription_id=_env("AZURE_SUBSCRIPTION_ID"),
        azure_resource_group=_env("AZURE_RESOURCE_GROUP"),
        azure_location=_env("AZURE_LOCATION"),
        azure_sql_max_size_gb=_env_int("AZURE_SQL_MAX_SIZE_GB", 5),
    )


def _redact(text: str) -> str:
    return text if not text else "***"


class ConfigSafety:
    """Helpers to render config without leaking secrets (for logs/status)."""

    @staticmethod
    def public(config: Config) -> dict:
        return {
            "experiment_uid": config.experiment_uid,
            "run_environment": config.run_environment,
            "azure_sql_server": config.azure_sql_server,
            "azure_sql_database": config.azure_sql_database,
            "az_monitor_resource_id": config.azure_monitor_resource_id(),
            "supabase_db_host": config.supabase_db_host,
            "supabase_db_port": config.supabase_db_port,
            "supabase_db_name": config.supabase_db_name,
            "capture_raw_interval_seconds": config.capture_raw_interval_seconds,
            "experiment_duration_hours": config.experiment_duration_hours,
            "max_experiment_hours": config.max_experiment_hours,
            "auto_stop_after_hours": config.auto_stop_after_hours,
            "auto_start": config.auto_start,
            "workload_intensity": config.workload_intensity,
            "workload_intensity_jitter": config.workload_intensity_jitter,
            "azure_sql_max_size_gb": config.azure_sql_max_size_gb,
        }
