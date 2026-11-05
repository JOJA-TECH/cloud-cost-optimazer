"""Serving configuration.

Read from environment variables only. Secrets (certificate/key material) are
expected as filesystem paths (secret mounts) or environment variables, never
hardcoded. The default bind address is loopback so the service is not
exposed to the network by default.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path

DEFAULT_GRPC_ADDR = "127.0.0.1:50051"
DEFAULT_MAX_STALE_MINUTES = 120
DEFAULT_SUPPORTED_HORIZON_MINUTES = 60
DEFAULT_MAX_RECEIVE_MESSAGE_MB = 4
DEFAULT_MAX_CONCURRENT_STREAMS = 100
DEFAULT_DB_TIMEOUT_SECONDS = 10
DEFAULT_TLS_MODE = "disabled"
DEFAULT_MODEL_VERSION = ""


def _env(name: str, default: str = "") -> str:
    return os.environ.get(name, default)


def _is_loopback(host: str) -> bool:
    return host in ("127.0.0.1", "::1", "localhost") or host.startswith("127.")


@dataclass(frozen=True)
class ServingSettings:
    """Resolved serving configuration."""

    grpc_addr: str
    model_dir: Path
    data_dsn: str
    max_stale_minutes: int
    supported_horizon_minutes: int
    max_receive_message_mb: int
    max_concurrent_streams: int
    db_timeout_seconds: int
    tls_mode: str
    tls_cert_file: str | None
    tls_key_file: str | None
    tls_ca_file: str | None
    default_model_version: str

    @property
    def is_loopback(self) -> bool:
        host = self.grpc_addr.rsplit(":", 1)[0]
        return _is_loopback(host)

    def require_tls(self) -> bool:
        """TLS is mandatory for non-local deployments (fail-closed)."""
        return self.tls_mode == "required" or (not self.is_loopback)


def load_settings() -> ServingSettings:
    """Build ServingSettings from the process environment.

    Raises ValueError with a clear message when required settings are missing
    or the configuration is unsafe (non-local listener without TLS).
    """
    grpc_addr = _env("FORECAST_GRPC_ADDR", DEFAULT_GRPC_ADDR)
    model_dir_raw = _env("FORECAST_MODEL_DIR")
    data_dsn = _env("FORECAST_DATA_DSN")

    if not model_dir_raw:
        raise ValueError("FORECAST_MODEL_DIR is required")
    model_dir = Path(model_dir_raw)
    if not model_dir.is_dir():
        raise ValueError(f"FORECAST_MODEL_DIR is not a directory: {model_dir}")

    try:
        max_stale = int(
            _env("FORECAST_MAX_STALE_MINUTES", str(DEFAULT_MAX_STALE_MINUTES))
        )
    except ValueError:
        max_stale = DEFAULT_MAX_STALE_MINUTES
    try:
        horizon = int(
            _env(
                "FORECAST_SUPPORTED_HORIZON_MINUTES",
                str(DEFAULT_SUPPORTED_HORIZON_MINUTES),
            )
        )
    except ValueError:
        horizon = DEFAULT_SUPPORTED_HORIZON_MINUTES
    try:
        max_msg = int(
            _env("FORECAST_MAX_RECEIVE_MESSAGE_MB", str(DEFAULT_MAX_RECEIVE_MESSAGE_MB))
        )
    except ValueError:
        max_msg = DEFAULT_MAX_RECEIVE_MESSAGE_MB
    try:
        max_streams = int(
            _env("FORECAST_MAX_CONCURRENT_STREAMS", str(DEFAULT_MAX_CONCURRENT_STREAMS))
        )
    except ValueError:
        max_streams = DEFAULT_MAX_CONCURRENT_STREAMS
    try:
        db_timeout = int(
            _env("FORECAST_DB_TIMEOUT_SECONDS", str(DEFAULT_DB_TIMEOUT_SECONDS))
        )
    except ValueError:
        db_timeout = DEFAULT_DB_TIMEOUT_SECONDS

    tls_mode = _env("FORECAST_TLS_MODE", DEFAULT_TLS_MODE)
    tls_cert = _env("FORECAST_TLS_CERT_FILE") or None
    tls_key = _env("FORECAST_TLS_KEY_FILE") or None
    tls_ca = _env("FORECAST_TLS_CA_FILE") or None

    default_model_version = _env("FORECAST_MODEL_VERSION", DEFAULT_MODEL_VERSION)

    settings = ServingSettings(
        grpc_addr=grpc_addr,
        model_dir=model_dir,
        data_dsn=data_dsn,
        max_stale_minutes=max_stale,
        supported_horizon_minutes=horizon,
        max_receive_message_mb=max_msg,
        max_concurrent_streams=max_streams,
        db_timeout_seconds=db_timeout,
        tls_mode=tls_mode,
        tls_cert_file=tls_cert,
        tls_key_file=tls_key,
        tls_ca_file=tls_ca,
        default_model_version=default_model_version,
    )

    if settings.require_tls() and tls_mode != "required":
        raise ValueError(
            "non-local gRPC listener requires FORECAST_TLS_MODE=required "
            f"(addr={grpc_addr})"
        )
    if tls_mode == "required" and not tls_cert:
        raise ValueError(
            "FORECAST_TLS_CERT_FILE is required when FORECAST_TLS_MODE=required"
        )
    if tls_mode == "required" and not tls_key:
        raise ValueError(
            "FORECAST_TLS_KEY_FILE is required when FORECAST_TLS_MODE=required"
        )

    return settings
