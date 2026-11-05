# Standalone gRPC server for the forecasting module.
# Exposes forecast.v1.ForecastService (Predict + GetModelInfo) and a
# grpc.health.v1.Health endpoint. Service-to-service only: the Go backend
# is the intended client; browsers/frontends must not connect directly.

from __future__ import annotations

import argparse
import logging
import signal
import sys
from concurrent import futures
from datetime import UTC, datetime

import grpc

from forecasting.proto.forecast.v1 import forecast_pb2, forecast_pb2_grpc
from forecasting.serving.config import load_settings
from forecasting.serving.data import FeatureStoreReader
from forecasting.serving.engine import InferenceEngine, InferenceError
from forecasting.serving.model import ModelArtifactError, load

try:  # grpc.health.v1 ships with grpcio
    from grpc_health.v1 import health, health_pb2, health_pb2_grpc

    HEALTH_AVAILABLE = True
except Exception:  # noqa: BLE001 - health is optional
    HEALTH_AVAILABLE = False

log = logging.getLogger("forecast.grpc")


class ForecastServicer(forecast_pb2_grpc.ForecastServiceServicer):
    def __init__(self, engine: InferenceEngine, reader: FeatureStoreReader) -> None:
        self.engine = engine
        self.reader = reader

    def Predict(self, request, context):
        request_id = request.request_id or ""
        log.info(
            "Predict resource_uid=%s target=%s horizon=%s request_id=%s",
            request.resource_uid, request.target, request.horizon_minutes,
            request_id,
        )

        if self.engine.artifact is None:
            context.abort(grpc.StatusCode.FAILED_PRECONDITION,
                          "model artifact not loaded")

        # Target / horizon are validated against the loaded artifact only.
        try:
            self.engine.validate_target_horizon(
                request.target, request.horizon_minutes
            )
        except InferenceError as exc:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))

        # Resolve the public uid: unknown resource -> NOT_FOUND.
        try:
            if not self.reader.resource_exists(request.resource_uid):
                context.abort(
                    grpc.StatusCode.NOT_FOUND,
                    "resource not found: %s" % request.resource_uid,
                )
        except Exception as exc:  # noqa: BLE001 - DB failure
            log.error("resource lookup failed: %s", exc)
            context.abort(grpc.StatusCode.UNAVAILABLE, "feature store unavailable")

        cutoff = None
        if request.HasField("cutoff_time"):
            cutoff = request.cutoff_time.ToDatetime().astimezone(UTC).isoformat()

        try:
            payload = self.engine.predict(
                resource_uid=request.resource_uid,
                target=request.target,
                horizon_minutes=request.horizon_minutes,
                request_id=request_id,
                cutoff_time=cutoff,
            )
        except InferenceError as exc:
            context.abort(grpc.StatusCode.INTERNAL, str(exc))
        except Exception as exc:  # noqa: BLE001 - unexpected failure
            log.error("prediction failed: %s", exc)
            context.abort(grpc.StatusCode.INTERNAL, "prediction failed")

        return self._build_response(payload)

    @staticmethod
    def _build_response(payload: dict) -> forecast_pb2.PredictResponse:
        resp = forecast_pb2.PredictResponse()
        resp.resource_uid = payload["resource_uid"]
        resp.target = payload["target"]
        resp.unit = payload["unit"]
        resp.horizon_minutes = payload["horizon_minutes"]
        resp.status = payload["status"]
        resp.experimental = bool(payload["experimental"])
        resp.request_id = payload.get("request_id") or ""

        if payload.get("value") is not None:
            resp.value = float(payload["value"])
        if payload.get("period_start_utc") is not None:
            resp.period_start_utc.FromDatetime(
                payload["period_start_utc"].to_pydatetime()
            )
        generated = payload["generated_at_utc"]
        resp.generated_at_utc.FromDatetime(
            generated if isinstance(generated, datetime) else datetime.now(UTC)
        )
        resp.model_name = payload["model_name"]
        resp.model_version = payload["model_version"]

        interval = payload.get("interval")
        if interval:
            resp.interval.lower = float(interval["lower"])
            resp.interval.upper = float(interval["upper"])
            resp.interval.method = str(interval.get("method", ""))
            resp.interval.nominal_coverage = float(
                interval.get("nominal_coverage", 0.0)
            )
        return resp

    def GetModelInfo(self, request, context):
        artifact = self.engine.artifact
        if artifact is None:
            context.abort(grpc.StatusCode.FAILED_PRECONDITION,
                          "model artifact not loaded")
        resp = forecast_pb2.GetModelInfoResponse()
        resp.model_name = artifact.model_name
        resp.model_version = artifact.model_version
        resp.target = artifact.target
        resp.horizon_minutes = self.engine.supported_horizon_minutes
        resp.unit = "percent"
        resp.feature_list = list(artifact.feature_list)
        resp.ready = artifact is not None
        resp.experimental = True
        config = artifact.interval_config or {}
        resp.prediction_interval_method = str(config.get("primary_method", ""))
        return resp


def build_server(engine: InferenceEngine, reader: FeatureStoreReader, settings) -> grpc.Server:
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=max(4, settings.max_concurrent_streams)),
        options=[
            ("grpc.max_receive_message_length",
             settings.max_receive_message_mb * 1024 * 1024),
            ("grpc.max_send_message_length",
             settings.max_receive_message_mb * 1024 * 1024),
            ("grpc.max_concurrent_streams", settings.max_concurrent_streams),
        ],
    )
    forecast_pb2_grpc.add_ForecastServiceServicer_to_server(
        ForecastServicer(engine, reader), server
    )

    if HEALTH_AVAILABLE:
        health_servicer = health.HealthServer()
        serving = health_pb2.HealthCheckResponse.SERVING
        not_serving = health_pb2.HealthCheckResponse.NOT_SERVING
        status = serving if engine.artifact is not None else not_serving
        health_servicer.set("", status)
        health_servicer.set("forecast.v1.ForecastService", status)
        health_pb2_grpc.add_HealthServicer_to_server(health_servicer, server)

    if settings.require_tls():
        with open(settings.tls_key_file, "rb") as f:
            private_key = f.read()
        with open(settings.tls_cert_file, "rb") as f:
            certificate_chain = f.read()
        root_certificates = None
        if settings.tls_ca_file:
            with open(settings.tls_ca_file, "rb") as f:
                root_certificates = f.read()
        server_credentials = grpc.ssl_server_credentials(
            [(private_key, certificate_chain)],
            root_certificates=root_certificates,
            require_client_auth=root_certificates is not None,
        )
        server.add_secure_port(settings.grpc_addr, server_credentials)
        log.info("listening on %s (TLS%s)", settings.grpc_addr,
                 " with mTLS" if root_certificates else "")
    else:
        if not settings.is_loopback:
            raise ValueError(
                "refusing to bind non-local address %s without TLS" % settings.grpc_addr
            )
        server.add_insecure_port(settings.grpc_addr)
        log.info("listening on %s (plaintext, loopback only)", settings.grpc_addr)
    return server


def main(argv: list[str] | None = None) -> int:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    parser = argparse.ArgumentParser(prog="forecast-grpc-server")
    args = parser.parse_args(argv)

    try:
        settings = load_settings()
    except ValueError as exc:
        log.error("configuration error: %s", exc)
        return 2

    # Load the artifact once at startup. Failure -> not ready, clear error.
    artifact = None
    try:
        artifact = load(settings.model_dir)
        log.info(
            "model loaded name=%s version=%s target=%s features=%d",
            artifact.model_name, artifact.model_version, artifact.target,
            len(artifact.feature_list),
        )
    except ModelArtifactError as exc:
        log.error("model not ready: %s", exc)
        artifact = None

    reader = FeatureStoreReader(settings.data_dsn, timeout_seconds=settings.db_timeout_seconds)
    engine = InferenceEngine(
        artifact=artifact,
        reader=reader,
        max_stale_minutes=settings.max_stale_minutes,
        supported_horizon_minutes=settings.supported_horizon_minutes,
    )

    server = build_server(engine, reader, settings)

    def handle_signal(signum, frame):
        log.info("received signal %s, shutting down", signum)
        server.stop(grace=5)

    signal.signal(signal.SIGTERM, handle_signal)
    signal.signal(signal.SIGINT, handle_signal)

    server.start()
    log.info("server started")
    server.wait_for_termination()
    log.info("server stopped")
    return 0


if __name__ == "__main__":
    sys.exit(main())
