# Handoff: gRPC forecasting service (Python) -> future Go client

This document describes the versioned gRPC contract implemented on the Python
side so a different agent/person can implement the Go client without guessing
service names, messages, error codes or security. No Go files are changed or
generated here.

## Scope

- Language: Python only. Branch: `feature/python-forecast-grpc-server`.
- The service is a standalone process; it does NOT embed in the Go backend.
- The Go backend (later task) calls `Predict` from its server. Browsers/
  frontends must NOT connect directly to this gRPC service.
- End-to-end with Go is NOT part of this task.

## Contract source

- Proto: `src/forecasting/proto/forecast/v1/forecast.proto` (package
  `forecast.v1`, service `ForecastService`).
- Generated Python stubs (committed):
  `src/forecasting/proto/forecast/v1/forecast_pb2.py`
  `src/forecasting/proto/forecast/v1/forecast_pb2_grpc.py`

## Regenerating stubs

Python (this repo):
```
python -m grpc_tools.protoc -I src \
  --python_out=src --grpc_python_out=src \
  src/forecasting/proto/forecast/v1/forecast.proto
```
Requires: `grpcio-tools`, `protobuf` (bundles `protoc`).

Go (future agent, NOT done here):
```
protoc --go_out=./backend --go_opt=paths=source_relative \
  --go-grpc_out=./backend --go-grpc_opt=paths=source_relative \
  src/forecasting/proto/forecast/v1/forecast.proto
```
Requires: `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc`.
Do NOT commit `.pb.go` stubs as part of this task.

## Service and RPCs

Service: `forecast.v1.ForecastService`

| RPC | Request | Response |
|-----|---------|----------|
| `Predict` | `PredictRequest` | `PredictResponse` |
| `GetModelInfo` | `GetModelInfoRequest` | `GetModelInfoResponse` |

Health: `grpc.health.v1.Health` is also served. Use service name
`""` (overall) or `"forecast.v1.ForecastService"`. It reports `SERVING`
only when the model artifact is loaded.

## PredictRequest — fields

| Field | Type | Presence | Meaning |
|-------|------|----------|---------|
| `resource_uid` | string | required | Public, immutable resource id (`core.resource.resource_uid` in `cco_data`). Never the internal `resource_id`. |
| `target` | string | required | Target to forecast. Must equal the loaded artifact's target (currently `target_cpu_avg_next1h`). |
| `horizon_minutes` | int32 | required | Forecast horizon in minutes. Only `60` (1 hour) is supported in this release. |
| `request_id` | string | optional | Correlation / request id for tracing. Echoed in the response. Not an authorization token. |
| `cutoff_time` | google.protobuf.Timestamp | optional | Latest fully observed window at or before this instant (UTC). If omitted, the server uses the latest observed hourly window for the resource. |

## PredictResponse — fields

| Field | Type | Presence | Meaning |
|-------|------|----------|---------|
| `resource_uid` | string | always | Echo of the request. |
| `target` | string | always | Echo of the request. |
| `unit` | string | always | Unit of the point value, e.g. `"percent"`. |
| `horizon_minutes` | int32 | always | Echo of the request. |
| `period_start_utc` | Timestamp | present when status == ok | Future period being forecast (start of the window, UTC) = last observed window + 1h. |
| `generated_at_utc` | Timestamp | always | Server generation timestamp (UTC). |
| `value` | google.protobuf.DoubleValue | present when status == ok | Point prediction. Presence is explicit (wrapper type). |
| `model_name` | string | always | Loaded model name. |
| `model_version` | string | always | Loaded model version. |
| `status` | string | always | `ok` | `insufficient_data` | `stale_data`. |
| `experimental` | bool | always | `true` in this release (single SKU, short history; see limitations). |
| `request_id` | string | always | Echo of the request correlation id (empty if not sent). |
| `interval` | PredictionInterval | present only if calibrated | Optional prediction interval. Absent when the artifact carries no calibration. |

`PredictionInterval`: `lower`, `upper` (double), `method` (string), `nominal_coverage`
(double, 0 < v < 1). Same unit as `value`. Returned only when the loaded artifact
carries calibrated interval offsets; never invented.

## Target / horizon support

- Only the target recorded in `model_metadata.json` (`"target"`) is accepted.
  Any other target -> `INVALID_ARGUMENT`.
- Only `horizon_minutes == 60` is accepted. Anything else -> `INVALID_ARGUMENT`.
- No forecasts for SKUs, costs, savings, or other targets not backed by the
  loaded artifact.

## Data source (read-only)

- The service owns a dedicated read-only connection to the `cco_data`
  database (NOT `cco_app`), configured by `FORECAST_DATA_DSN`.
- It never connects to `cco_app`, never runs GORM migrations, never alters
  schemas.
- SQL is parameterized. `resource_uid` is resolved internally to the internal
  `resource_id` via a join with `core.resource`; the internal id never leaves
  the service and is never accepted from the caller.
- Only `feat.feature_snapshot` (observed features, no future labels) is read.
  `feat.v_training_base` (which carries `target_*` future labels) is NOT used
  for inference.
- Logged: correlation id, resource uid, target, staleness/coverage, model
  version. Never full feature values or credentials.

## gRPC error codes -> suggested HTTP mapping

| gRPC code | When | Suggested HTTP |
|-----------|------|----------------|
| `INVALID_ARGUMENT` | bad target/horizon, malformed resource_uid | 400 |
| `NOT_FOUND` | resource_uid not registered in `core.resource` | 404 |
| `FAILED_PRECONDITION` | model artifact not loaded (service not ready) | 503 |
| `UNAVAILABLE` | feature store unreachable / DB failure | 503 |
| `DEADLINE_EXCEEDED` | caller deadline / cancellation | 504 |
| `INTERNAL` | unexpected prediction failure | 500 |

No stack traces, private paths, DSNs or secrets are ever sent to the client.

## Deadlines, cancellation, correlation ID

- Honor gRPC deadlines/cancellation (`context` in Go). The server aborts
  promptly when the RPC context is cancelled.
- Propagate `request_id` as a correlation/tracing id only; never treat it as
  authorization.

## Security / transport

- Default bind: `FORECAST_GRPC_ADDR` = `127.0.0.1:50051` (loopback, plaintext,
  development only). The listener is NOT exposed to the internet by default.
- Non-local bind requires `FORECAST_TLS_MODE=required` (fail-closed).
- TLS via `FORECAST_TLS_CERT_FILE` / `FORECAST_TLS_KEY_FILE` (paths or secret
  mounts). Optional `FORECAST_TLS_CA_FILE` enables mTLS client verification.
- No secrets are hardcoded. The DSN is redacted before any logging.

## Environment variables (Python service)

| Variable | Default | Meaning |
|----------|---------|---------|
| `FORECAST_GRPC_ADDR` | `127.0.0.1:50051` | Bind address. Loopback by default. |
| `FORECAST_MODEL_DIR` | (required) | Directory with `model.joblib` + `model_metadata.json`. |
| `FORECAST_DATA_DSN` | (required) | Read-only DSN to `cco_data`. |
| `FORECAST_MAX_STALE_MINUTES` | `120` | Max age of the last observed sample. |
| `FORECAST_SUPPORTED_HORIZON_MINUTES` | `60` | Only supported horizon. |
| `FORECAST_MAX_RECEIVE_MESSAGE_MB` | `4` | Max request/response size. |
| `FORECAST_MAX_CONCURRENT_STREAMS` | `100` | Concurrency limit. |
| `FORECAST_DB_TIMEOUT_SECONDS` | `10` | Statement timeout. |
| `FORECAST_TLS_MODE` | `disabled` | `disabled` | `required`. |
| `FORECAST_TLS_CERT_FILE` / `_KEY_FILE` / `_CA_FILE` | (none) | TLS cert/key/CA paths. |

## Entrypoint

```
python -m forecasting.serving.server
# or, once installed:
forecast-grpc-server
PYTHONPATH=src
```

Readiness is positive only when the model artifact and metadata load and the
data DSN is configured. Missing/incompatible artifact -> clear startup error,
service not ready, no silent default/mock predictions.

## Limitations (experimental)

- Single SKU, short history: 46 hourly windows from one Azure SQL serverless
  database (~2 days). Empirical interval coverage was ~29% on 7 test points
  (nominal 80%).
- No cross-SKU validation; no cost/savings forecasting.
- Prediction intervals are optional and rely on exchangeability (an
  approximation for time series). Reported with measured coverage, not as a
  guarantee.
- Label: `experimental = true` while evidence does not support a stronger
  claim.

## Runbook for the Go implementer

1. Generate Go stubs from the `.proto` (command above).
2. Call `forecast.v1.ForecastService.Predict` with `resource_uid` (public),
   `target="target_cpu_avg_next1h"`, `horizon_minutes=60`, optional
   `request_id` and `cutoff_time`.
3. Read `status`: only `ok` carries a point `value`; treat
   `insufficient_data` / `stale_data` as non-fatal but not a forecast.
4. Honor `experimental=true`; do not build cross-resource or cost decisions on
   this output.
5. Use TLS/mTLS in non-local environments; loopback plaintext is dev-only.

## Blockers (pending)

- No real trained artifact is available in this repo: tests use a synthetic
  fixture. A real `model.joblib` + `model_metadata.json` must be produced by
  `train-real` against `cco_data` before real inference.
- No live `cco_data` connection is available in this environment: `FORECAST_DATA_DSN`
  must be provisioned with a read-only user before the service can serve real
  resources. Until then, readiness reports not-ready for real inference.
