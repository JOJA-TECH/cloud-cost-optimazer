# Tests for the protobuf contract (descriptor + fields).

from __future__ import annotations

from datetime import UTC, datetime

from google.protobuf.timestamp_pb2 import Timestamp
from google.protobuf.wrappers_pb2 import DoubleValue

from forecasting.proto.forecast.v1 import forecast_pb2, forecast_pb2_grpc


def service_methods():
    return forecast_pb2.DESCRIPTOR.services_by_name["ForecastService"]


def test_forecast_service_and_rpcs():
    svc = service_methods()
    assert svc.full_name == "forecast.v1.ForecastService"
    names = {m.name for m in svc.methods}
    assert names == {"Predict", "GetModelInfo"}


def test_predict_request_fields():
    req = forecast_pb2.PredictRequest()
    expected = {
        "resource_uid": str,
        "target": str,
        "horizon_minutes": int,
        "request_id": str,
    }
    for field, _ in expected.items():
        assert req.DESCRIPTOR.fields_by_name[field] is not None
    # cutoff_time is optional: not required to be set.
    assert req.DESCRIPTOR.fields_by_name["cutoff_time"] is not None
    assert not req.HasField("cutoff_time")

    req.resource_uid = "uid-abc"
    req.target = "target_cpu_avg_next1h"
    req.horizon_minutes = 60
    req.request_id = "corr-123"
    ct = Timestamp()
    ct.FromDatetime(datetime(2026, 9, 10, 23, 0, tzinfo=UTC))
    req.cutoff_time.CopyFrom(ct)

    # Round-trip serialization.
    blob = req.SerializeToString()
    back = forecast_pb2.PredictRequest()
    back.ParseFromString(blob)
    assert back.resource_uid == "uid-abc"
    assert back.target == "target_cpu_avg_next1h"
    assert back.horizon_minutes == 60
    assert back.request_id == "corr-123"
    assert back.HasField("cutoff_time")


def test_predict_response_fields():
    resp = forecast_pb2.PredictResponse()
    for field in (
        "resource_uid",
        "target",
        "unit",
        "horizon_minutes",
        "period_start_utc",
        "generated_at_utc",
        "value",
        "model_name",
        "model_version",
        "status",
        "experimental",
        "request_id",
    ):
        assert resp.DESCRIPTOR.fields_by_name[field] is not None
    assert resp.DESCRIPTOR.fields_by_name["interval"] is not None

    resp.resource_uid = "uid-abc"
    resp.target = "target_cpu_avg_next1h"
    resp.unit = "percent"
    resp.horizon_minutes = 60
    resp.status = "ok"
    resp.experimental = True
    resp.request_id = "corr-123"
    resp.value.CopyFrom(DoubleValue(value=42.5))
    resp.model_name = "test_forecast"
    resp.model_version = "1.0.0"
    resp.interval.lower = 41.0
    resp.interval.upper = 44.0
    resp.interval.method = "jackknife_plus"
    resp.interval.nominal_coverage = 0.8

    blob = resp.SerializeToString()
    back = forecast_pb2.PredictResponse()
    back.ParseFromString(blob)
    assert back.status == "ok"
    assert back.experimental is True
    assert back.value.value == 42.5
    assert back.interval.method == "jackknife_plus"
    assert back.interval.nominal_coverage == 0.8


def test_get_model_info_fields():
    req = forecast_pb2.GetModelInfoRequest()
    resp = forecast_pb2.GetModelInfoResponse()
    for field in (
        "model_name",
        "model_version",
        "target",
        "horizon_minutes",
        "unit",
        "feature_list",
        "ready",
        "experimental",
        "prediction_interval_method",
    ):
        assert resp.DESCRIPTOR.fields_by_name[field] is not None
    resp.ready = True
    resp.experimental = True
    resp.feature_list.extend(["cpu_avg", "workload_intensity"])
    blob = resp.SerializeToString()
    back = forecast_pb2.GetModelInfoResponse()
    back.ParseFromString(blob)
    assert back.ready is True
    assert back.feature_list == ["cpu_avg", "workload_intensity"]


def test_stub_service_name():
    # The generated gRPC stub must expose the ForecastService stub class.
    assert forecast_pb2_grpc.ForecastServiceStub is not None
