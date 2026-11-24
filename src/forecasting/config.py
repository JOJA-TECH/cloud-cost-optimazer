"""Configuration handling.

Configuration is layered, from lowest to highest precedence:

1. packaged defaults (:data:`DEFAULTS`);
2. YAML file (e.g. ``configs/real_v1.yaml``);
3. environment variables (``.env`` file plus process environment);
4. explicit command line overrides.

The module never returns secrets in a printable form: :func:`redact_dsn` is
used everywhere a DSN may reach a log file or an artifact.
"""

from __future__ import annotations

import os
import re
from collections.abc import Mapping
from dataclasses import dataclass, field, replace
from pathlib import Path
from typing import Any

import yaml

REPO_ROOT = Path(__file__).resolve().parents[2]
CONFIG_DIR = REPO_ROOT / "configs"
ARTIFACTS_DIR = REPO_ROOT / "artifacts"
SQL_DIR = REPO_ROOT / "sql"

#: Environment file read by :func:`load_env_file` (git-ignored).
ENV_PATH = Path(os.environ.get("CCO_ENV_FILE", REPO_ROOT / ".env"))

DEFAULTS: dict[str, Any] = {
    "experiment_uid": "",
    "resource_uid": "",
    "resource_name": "cco-lab-sql-database",
    "resource_sku_code": "",
    "resource_compute_model": "",
    "resource_vcores": None,
    "resource_region_code": "australiaeast",
    "resource_provider": "azure",
    "random_seed": 42,
    "target": "target_cpu_avg_next1h",
    "secondary_targets": ["target_workload_next1h", "target_storage_next1h"],
    "period_type": "1h",
    "split": {"train": 0.70, "validation": 0.15, "test": 0.15},
    "artifacts_dir": str(ARTIFACTS_DIR),
    "local_dsn": "",
    "source_dsn": "",
    "source_sslmode": "require",
    "postgres_port": 55432,
    "postgres_db": "cco_forecasting",
    "postgres_user": "cco_forecasting",
    "postgres_password": "",
}

#: Features whose absence invalidates the row (row is dropped, never imputed).
REQUIRED_FEATURES: tuple[str, ...] = (
    "cpu_avg",
    "workload_intensity",
)

#: Features that are imputed when missing and always paired with a
#: ``feature_missing_<name>`` indicator.
OPTIONAL_FEATURES: tuple[str, ...] = (
    "cpu_max",
    "data_io_avg",
    "log_write_avg",
    "memory_percent",
    "sessions_count",
    "sessions_percent",
    "workers_avg",
    "storage_used_pct",
    "storage_used_bytes",
    "storage_growth_bytes",
    "deadlock_count",
    "latency_p95_ms",
)

#: Derived (lag / rolling / calendar) features computed on the hourly frame.
DERIVED_FEATURES: tuple[str, ...] = (
    "cpu_avg_lag_1h",
    "cpu_avg_lag_2h",
    "cpu_avg_lag_3h",
    "cpu_avg_rolling_3h",
    "cpu_avg_rolling_6h",
    "workload_intensity_lag_1h",
    "workload_intensity_rolling_3h",
    "storage_growth_lag_1h",
    "hour_sin",
    "hour_cos",
    "day_of_week",
    "is_peak_hour",
)

_DSN_SECRET = re.compile(r"://(?P<user>[^:/@]+):(?P<password>[^@]*)@")


def redact_dsn(dsn: str) -> str:
    """Return ``dsn`` without the password, safe to log or persist."""
    if not dsn:
        return ""
    match = _DSN_SECRET.search(dsn)
    if not match:
        return dsn
    return dsn[: match.start()] + f"://{match.group('user')}:***@" + dsn[match.end() :]


def load_env_file(path: str | Path = ENV_PATH) -> dict[str, str]:
    """Parse a ``.env`` file without overriding the real environment."""
    path = Path(path)
    values: dict[str, str] = {}
    if not path.is_file():
        return values
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if key and key not in os.environ:
            values[key] = value
    return values


def _env_value(env: Mapping[str, str], *names: str, default: str = "") -> str:
    for name in names:
        value = env.get(name)
        if value:
            return value
    return default


@dataclass(frozen=True)
class Settings:
    """Resolved configuration for a pipeline run."""

    experiment_uid: str = ""
    resource_uid: str = ""
    resource_name: str = "cco-lab-sql-database"
    resource_sku_code: str = ""
    resource_compute_model: str = ""
    resource_vcores: float | None = None
    resource_region_code: str = "australiaeast"
    resource_provider: str = "azure"
    random_seed: int = 42
    target: str = "target_cpu_avg_next1h"
    secondary_targets: tuple[str, ...] = ()
    period_type: str = "1h"
    split: Mapping[str, float] = field(default_factory=lambda: dict(DEFAULTS["split"]))
    artifacts_dir: str = str(ARTIFACTS_DIR)
    local_dsn: str = ""
    source_dsn: str = ""
    source_sslmode: str = "require"
    postgres_port: int = 55432
    postgres_db: str = "cco_forecasting"
    postgres_user: str = "cco_forecasting"
    postgres_password: str = ""
    extra: Mapping[str, Any] = field(default_factory=dict)

    # -- derived helpers ----------------------------------------------------
    @property
    def artifacts_path(self) -> Path:
        return Path(self.artifacts_dir)

    def with_overrides(self, **kwargs: Any) -> Settings:
        clean = {k: v for k, v in kwargs.items() if v is not None}
        return replace(self, **clean)  # type: ignore[arg-type]


def _read_yaml(path: str | Path) -> dict[str, Any]:
    path = Path(path)
    if not path.is_file():
        raise FileNotFoundError(f"configuration file not found: {path}")
    data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if not isinstance(data, dict):
        raise ValueError(f"configuration file must contain a mapping: {path}")
    return data


def _build_local_dsn(env: Mapping[str, str], data: Mapping[str, Any]) -> str:
    explicit = str(data.get("local_dsn") or _env_value(env, "LOCAL_DSN"))
    if explicit:
        return explicit
    user = str(
        data.get("postgres_user")
        or _env_value(env, "POSTGRES_USER", default="cco_forecasting")
    )
    password = str(
        data.get("postgres_password") or _env_value(env, "POSTGRES_PASSWORD")
    )
    host = str(
        data.get("postgres_host")
        or _env_value(env, "POSTGRES_HOST", default="127.0.0.1")
    )
    port = int(
        data.get("postgres_port") or _env_value(env, "POSTGRES_PORT", default="55432")
    )
    dbname = str(
        data.get("postgres_db")
        or _env_value(env, "POSTGRES_DB", default="cco_forecasting")
    )
    return f"postgresql://{user}:{password}@{host}:{port}/{dbname}"


def load_settings(
    config_path: str | Path | None = None,
    *,
    env_path: str | Path = ENV_PATH,
    use_env: bool = True,
    **overrides: Any,
) -> Settings:
    """Build a :class:`Settings` instance from defaults, YAML, env and overrides."""
    env: dict[str, str] = dict(load_env_file(env_path)) if use_env else {}
    env.update({k: v for k, v in os.environ.items() if isinstance(v, str)})

    data: dict[str, Any] = dict(DEFAULTS)
    if config_path is not None:
        data.update(_read_yaml(config_path))

    split = dict(DEFAULTS["split"])
    split.update(data.get("split") or {})
    for key, value in split.items():
        split[key] = float(value)

    settings = Settings(
        experiment_uid=str(
            data.get("experiment_uid") or _env_value(env, "EXPERIMENT_UID")
        ),
        resource_uid=str(data.get("resource_uid") or _env_value(env, "RESOURCE_UID")),
        resource_name=str(
            data.get("resource_name")
            or _env_value(env, "RESOURCE_NAME", default="cco-lab-sql-database")
        ),
        resource_sku_code=str(
            data.get("resource_sku_code") or _env_value(env, "RESOURCE_SKU_CODE")
        ),
        resource_compute_model=str(
            data.get("resource_compute_model")
            or _env_value(env, "RESOURCE_COMPUTE_MODEL")
        ),
        resource_vcores=(
            float(data["resource_vcores"])
            if data.get("resource_vcores") not in (None, "")
            else None
        ),
        resource_region_code=str(
            data.get("resource_region_code")
            or _env_value(env, "RESOURCE_REGION_CODE", default="australiaeast")
        ),
        resource_provider=str(
            data.get("resource_provider")
            or _env_value(env, "RESOURCE_PROVIDER", default="azure")
        ),
        random_seed=int(
            data.get("random_seed") or _env_value(env, "RANDOM_SEED", default="42")
        ),
        target=str(data.get("target") or "target_cpu_avg_next1h"),
        secondary_targets=tuple(data.get("secondary_targets") or ()),
        period_type=str(data.get("period_type") or "1h"),
        split=split,
        artifacts_dir=str(data.get("artifacts_dir") or ARTIFACTS_DIR),
        local_dsn=_build_local_dsn(env, data),
        source_dsn=str(data.get("source_dsn") or _env_value(env, "SOURCE_DSN")),
        source_sslmode=str(
            data.get("source_sslmode")
            or _env_value(env, "SOURCE_SSLMODE", default="require")
        ),
        postgres_port=int(
            data.get("postgres_port")
            or _env_value(env, "POSTGRES_PORT", default="55432")
        ),
        postgres_db=str(
            data.get("postgres_db")
            or _env_value(env, "POSTGRES_DB", default="cco_forecasting")
        ),
        postgres_user=str(
            data.get("postgres_user")
            or _env_value(env, "POSTGRES_USER", default="cco_forecasting")
        ),
        postgres_password=str(
            data.get("postgres_password") or _env_value(env, "POSTGRES_PASSWORD")
        ),
        extra={k: v for k, v in data.items() if k not in DEFAULTS},
    )
    return settings.with_overrides(**overrides) if overrides else settings
