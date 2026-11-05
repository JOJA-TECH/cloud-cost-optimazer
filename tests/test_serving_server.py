# Tests for the gRPC server (in-process, loopback only).

from __future__ import annotations

from datetime import UTC, datetime

import grpc
import pytest
from conftest import FEATURE_LIST, TARGET, make_artifact, make_observed_frame

from forecasting.proto.forecast.v1 import forecast_pb2, forecast_pb2_grpc
from forecasting.serving.config import ServingSettings
from forecasting.serving.data import FeatureStoreReader
from forecasting.serving.engine import InferenceEngine
from forecasting.serving.model import load
from forecasting.serving.server import ForecastServicer


class FakeReader(FeatureStoreReader):
    def __init__(self, frame, exists=True):
        self.frame = frame
        self.exists = exists
        self.calls = []

    def resource_exists(self, resource_uid: str) -> bool:
        self.calls.append(("exists", resource_uid))
        return self.exists

    def fetch_hourly_features(self, resource_uid: str):
        self.calls.append(("fetch", resource_uid))
        return self.frame.copy()


class FakeContext:
    def __init__(self):
        self.code = None
        self.details = None

    def abort(self, code, details):
        self.code = code
        self.details = details
        raise _Abort(code, details)


class _Abort(Exception):
    def __init__(self, code, details):
        super().__init__(details)
        self.code = code
        self.details = details


def make_settings():
    return ServingSettings(
        grpc_addr="127.0.0.1:0",
        model_dir="/tmp/forecast-model-dir",
        data_dsn="postgresql://fake:fake@127.0.0.1:1/fake",
        max_stale_minutes=1000000,
        supported_horizon_minutes=60,
        max_receive_message_mb=4,
        max_concurrent_streams=100,
        db_timeout_seconds=10,
        tls_mode="disabled",
        tls_cert_file=None,
        tls_key_file=None,
        tls_ca_file=None,
        default_model_version="",
    )


def build_engine_reader(
    tmp_path, frame, kind="level", exists=True, max_stale_minutes=1000000
):
    artifact = load(
        str(make_artifact(tmp_path, kind=kind, with_interval=True).model_dir)
    )
    reader = FakeReader(frame, exists=exists)
    engine = InferenceEngine(
        artifacts={artifact.version: artifact},
        reader=reader,
        max_stale_minutes=max_stale_minutes,
        supported_horizon_minutes=60,
    )
    return engine, reader


def servicer_predict(engine, reader, target=TARGET, horizon=60, resource_uid="uid-1"):
    servicer = ForecastServicer(engine, reader)
    req = forecast_pb2.PredictRequest(
        resource_uid=resource_uid, target=target, horizon_minutes=horizon
    )
    ctx = FakeContext()
    try:
        resp = servicer.Predict(req, ctx)
    except _Abort as exc:
        return exc.code, exc.details, None
    return ctx.code, ctx.details, resp


def test_grpc_success(tmp_path):
    frame = make_observed_frame(20)
    engine, reader = build_engine_reader(tmp_path, frame)
    code, details, resp = servicer_predict(engine, reader)
    assert code is None
    assert resp.status == "ok"
    assert resp.experimental is True
    assert resp.request_id == ""
    assert resp.unit == "percent"
    assert resp.horizon_minutes == 60
    assert resp.model_name == "test_forecast"
    assert resp.value.value > 0
    assert resp.interval.lower < resp.value.value < resp.interval.upper


def test_grpc_invalid_target(tmp_path):
    frame = make_observed_frame(20)
    engine, reader = build_engine_reader(tmp_path, frame)
    code, details, _ = servicer_predict(engine, reader, target="target_storage_next1h")
    assert code == grpc.StatusCode.INVALID_ARGUMENT


def test_grpc_invalid_horizon(tmp_path):
    frame = make_observed_frame(20)
    engine, reader = build_engine_reader(tmp_path, frame)
    code, details, _ = servicer_predict(engine, reader, horizon=30)
    assert code == grpc.StatusCode.INVALID_ARGUMENT


def test_grpc_not_found(tmp_path):
    frame = make_observed_frame(20)
    engine, reader = build_engine_reader(tmp_path, frame, exists=False)
    code, details, _ = servicer_predict(engine, reader)
    assert code == grpc.StatusCode.NOT_FOUND


def test_grpc_insufficient_data(tmp_path):
    frame = make_observed_frame(3)
    engine, reader = build_engine_reader(tmp_path, frame)
    code, details, resp = servicer_predict(engine, reader)
    assert code is None
    assert resp.status == "insufficient_data"


def test_grpc_stale_data(tmp_path):
    frame = make_observed_frame(12)
    engine, reader = build_engine_reader(
        tmp_path,
        frame,
        max_stale_minutes=1,
    )
    payload = engine.predict(
        "uid-1",
        TARGET,
        60,
        "",
        None,
        now=datetime(2026, 9, 20, 0, 0, tzinfo=UTC),
    )
    assert payload["status"] == "stale_data"


def test_grpc_get_model_info(tmp_path):
    frame = make_observed_frame(20)
    engine, reader = build_engine_reader(tmp_path, frame)
    servicer = ForecastServicer(engine, reader)
    ctx = FakeContext()
    resp = servicer.GetModelInfo(forecast_pb2.GetModelInfoRequest(), ctx)
    assert ctx.code is None
    assert resp.ready is True
    assert resp.experimental is True
    assert resp.target == TARGET
    assert resp.horizon_minutes == 60
    assert list(resp.feature_list) == FEATURE_LIST
    assert resp.prediction_interval_method == "jackknife_plus"


def test_grpc_not_ready_without_artifact(tmp_path):
    reader = FakeReader(make_observed_frame(12))
    engine = InferenceEngine(
        artifacts={},
        reader=reader,
        max_stale_minutes=120,
        supported_horizon_minutes=60,
    )
    code, details, _ = servicer_predict(engine, reader)
    assert code == grpc.StatusCode.FAILED_PRECONDITION


def _start_server(engine, reader):
    """Build and start an in-process gRPC server on a loopback ephemeral port."""
    import concurrent.futures

    settings = make_settings()
    server = grpc.server(
        concurrent.futures.ThreadPoolExecutor(max_workers=4),
        options=[
            ("grpc.max_receive_message_length", 4 * 1024 * 1024),
            ("grpc.max_send_message_length", 4 * 1024 * 1024),
            ("grpc.max_concurrent_streams", 100),
        ],
    )
    forecast_pb2_grpc.add_ForecastServiceServicer_to_server(
        ForecastServicer(engine, reader), server
    )
    port = server.add_insecure_port("127.0.0.1:0")
    server.start()
    return server, port


def test_grpc_network_roundtrip(tmp_path):
    frame = make_observed_frame(20)
    engine, reader = build_engine_reader(tmp_path, frame)
    server, port = _start_server(engine, reader)
    try:
        channel = grpc.insecure_channel(f"127.0.0.1:{port}")
        grpc.channel_ready_future(channel).result(timeout=5)
        stub = forecast_pb2_grpc.ForecastServiceStub(channel)
        req = forecast_pb2.PredictRequest(
            resource_uid="uid-1",
            target=TARGET,
            horizon_minutes=60,
            request_id="net-req-1",
        )
        resp, call = stub.Predict.with_call(req, timeout=5)
        assert resp.status == "ok"
        assert resp.request_id == "net-req-1"
        assert resp.experimental is True
        info = stub.GetModelInfo(forecast_pb2.GetModelInfoRequest(), timeout=5)
        assert info.ready is True
        channel.close()
    finally:
        server.stop(grace=1)


def test_grpc_cancelled_call(tmp_path):
    frame = make_observed_frame(20)
    engine, reader = build_engine_reader(tmp_path, frame)
    server, port = _start_server(engine, reader)
    try:
        channel = grpc.insecure_channel(f"127.0.0.1:{port}")
        stub = forecast_pb2_grpc.ForecastServiceStub(channel)
        req = forecast_pb2.PredictRequest(
            resource_uid="uid-1",
            target=TARGET,
            horizon_minutes=60,
        )
        fut = stub.Predict.future(req, timeout=5)
        fut.cancel()
        try:
            fut.result()
        except (grpc.RpcError, grpc.FutureCancelledError) as exc:
            code = getattr(exc, "code", lambda: grpc.StatusCode.CANCELLED)()
            assert code == grpc.StatusCode.CANCELLED
            return
        pytest.fail("expected the cancelled call to raise")
    finally:
        server.stop(grace=1)


def test_grpc_real_artifact(tmp_path):
    """A real trained artifact (artifacts/real_model) is usable via gRPC."""
    real_model_dir = "artifacts/real_model"
    artifact = load(real_model_dir)
    reader = FakeReader(make_observed_frame(20), exists=True)
    engine = InferenceEngine(
        artifacts={artifact.version: artifact},
        reader=reader,
        max_stale_minutes=1000000,
        supported_horizon_minutes=60,
        default_version=artifact.version,
    )
    server, port = _start_server(engine, reader)
    try:
        channel = grpc.insecure_channel(f"127.0.0.1:{port}")
        grpc.channel_ready_future(channel).result(timeout=5)
        stub = forecast_pb2_grpc.ForecastServiceStub(channel)

        # GetModelInfo reports the real model metadata.
        info = stub.GetModelInfo(forecast_pb2.GetModelInfoRequest(), timeout=5)
        assert info.ready is True
        assert info.target == "target_cpu_avg_next1h"
        assert info.model_name == "ridge_a2.0"
        assert info.model_version == "1.0.0"
        assert info.available_versions == ["default"]
        assert info.default_version == "default"

        # Predict returns a numeric point forecast for the real model.
        req = forecast_pb2.PredictRequest(
            resource_uid="uid-1",
            target="target_cpu_avg_next1h",
            horizon_minutes=60,
            request_id="real-artifact-check",
        )
        resp, _call = stub.Predict.with_call(req, timeout=5)
        assert resp.status == "ok"
        assert resp.experimental is True
        assert resp.request_id == "real-artifact-check"
        assert resp.value.value > 0
        assert resp.model_name == "ridge_a2.0"
        assert resp.model_version == "1.0.0"
        assert resp.interval.lower < resp.value.value < resp.interval.upper
        channel.close()
    finally:
        server.stop(grace=1)
