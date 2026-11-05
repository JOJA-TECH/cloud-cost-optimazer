# Tests for serving configuration.

from __future__ import annotations

import pytest

from forecasting.serving.config import (
    DEFAULT_GRPC_ADDR,
    load_settings,
)


def _env(monkeypatch, tmp_path, extra=None):
    model_dir = tmp_path / "model"
    model_dir.mkdir()
    env = {
        "FORECAST_MODEL_DIR": str(model_dir),
        "FORECAST_DATA_DSN": "postgresql://user:pass@127.0.0.1:5432/cco_data",
    }
    if extra:
        env.update(extra)
    for k, v in env.items():
        monkeypatch.setenv(k, v)
    return env


def test_defaults_loopback_plaintext(monkeypatch, tmp_path):
    _env(monkeypatch, tmp_path)
    monkeypatch.delenv("FORECAST_GRPC_ADDR", raising=False)
    monkeypatch.delenv("FORECAST_TLS_MODE", raising=False)
    s = load_settings()
    assert s.grpc_addr == DEFAULT_GRPC_ADDR
    assert s.is_loopback
    assert s.require_tls() is False
    assert s.tls_mode == "disabled"


def test_non_local_without_tls_fails_closed(monkeypatch, tmp_path):
    _env(monkeypatch, tmp_path, {"FORECAST_GRPC_ADDR": "0.0.0.0:50051"})
    monkeypatch.delenv("FORECAST_TLS_MODE", raising=False)
    with pytest.raises(ValueError, match="TLS"):
        load_settings()


def test_tls_required_ok(monkeypatch, tmp_path):
    cert = tmp_path / "cert.pem"
    cert.write_text("cert")
    key = tmp_path / "key.pem"
    key.write_text("key")
    _env(
        monkeypatch,
        tmp_path,
        {
            "FORECAST_GRPC_ADDR": "0.0.0.0:50051",
            "FORECAST_TLS_MODE": "required",
            "FORECAST_TLS_CERT_FILE": str(cert),
            "FORECAST_TLS_KEY_FILE": str(key),
        },
    )
    s = load_settings()
    assert s.require_tls() is True
    assert s.tls_cert_file == str(cert)


def test_mtls_with_ca(monkeypatch, tmp_path):
    cert = tmp_path / "cert.pem"
    cert.write_text("cert")
    key = tmp_path / "key.pem"
    key.write_text("key")
    ca = tmp_path / "ca.pem"
    ca.write_text("ca")
    _env(
        monkeypatch,
        tmp_path,
        {
            "FORECAST_GRPC_ADDR": "0.0.0.0:50051",
            "FORECAST_TLS_MODE": "required",
            "FORECAST_TLS_CERT_FILE": str(cert),
            "FORECAST_TLS_KEY_FILE": str(key),
            "FORECAST_TLS_CA_FILE": str(ca),
        },
    )
    s = load_settings()
    assert s.tls_ca_file == str(ca)


def test_model_dir_required(monkeypatch):
    monkeypatch.delenv("FORECAST_MODEL_DIR", raising=False)
    monkeypatch.setenv("FORECAST_DATA_DSN", "postgresql://u:p@127.0.0.1:1/d")
    with pytest.raises(ValueError, match="FORECAST_MODEL_DIR is required"):
        load_settings()


def test_model_dir_must_be_directory(monkeypatch, tmp_path):
    missing = tmp_path / "does-not-exist"
    monkeypatch.setenv("FORECAST_MODEL_DIR", str(missing))
    monkeypatch.setenv("FORECAST_DATA_DSN", "postgresql://u:p@127.0.0.1:1/d")
    with pytest.raises(ValueError, match="is not a directory"):
        load_settings()


def test_env_defaults_values(monkeypatch, tmp_path):
    _env(monkeypatch, tmp_path)
    s = load_settings()
    assert s.max_stale_minutes == 120
    assert s.supported_horizon_minutes == 60
    assert s.max_receive_message_mb == 4
    assert s.db_timeout_seconds == 10
