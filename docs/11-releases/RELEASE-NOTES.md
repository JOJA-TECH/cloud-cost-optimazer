# REL-1.0-slice-1 — Release Notes (Slice 1: puente de transporte ML)

- **Versión:** 1.0.0-slice-1
- **Fecha:** 2026-11-22
- **Estado:** Implementado (backend)

## Resumen

Se implementa el puente de transporte entre el backend Go y el servicio Python ML (Module 6) previsto en la formalización del Release 1.0. El backend invoca al servicio ML externo a través del contrato `MLPort` existente, usando un cliente gRPC real cuando se configura `CCO_ML_MODE=grpc`, manteniendo el cliente fake como valor por defecto para desarrollo. No se modifica `cco_app`, no se exponen credenciales de Azure y no hay cambios automáticos en recursos cloud.

## Nuevas funcionalidades

- `backend/internal/analytics/adapters/grpc_ml_client.go`: adaptador gRPC que implementa `analytics.MLPort` (`Forecast`, `ValidateContract`) contra `forecast.v1.ForecastService.Predict` del servicio Python. Traduce `ForecastRequest` → `PredictRequest` (mapea targets `cpu_avg`/`workload_intensity`/`storage_used_bytes`, valida horizonte 1×1h → 60 min, propaga `request_id`/`cutoff_time`) y traduce códigos gRPC a errores tipados de analytics (`ANALYTICS_INVALID_QUERY`, `ANALYTICS_NOT_FOUND`, `ANALYTICS_ML_UNAVAILABLE`).
- `backend/internal/forecast/v1/`: stubs Go generados del proto `src/forecasting/proto/forecast/v1/forecast.proto` (paquete `forecastv1`).
- `backend/cmd/cco-app/main.go`: nuevo modo `case "grpc"` en la selección del cliente ML, lee `CCO_ML_ENDPOINT` y `CCO_ML_REQUEST_TIMEOUT_SECONDS`; cierra el canal al salir.
- `backend/internal/analytics/interfaces.go`: `ForecastRequest` añade `RequestID` y `CorrelationID` para trazabilidad.
- `backend/go.mod` / `go.sum`: añade `google.golang.org/grpc` y `google.golang.org/protobuf`.

## Cambios

- `MLPort` sigue siendo el contrato de inferencia; se añade implementación productiva gRPC junto al fake existente.
- El modo `grpc` es fatal si `CCO_ML_ENDPOINT` no conecta (fail-fast al arrancar con `CCO_DATA_DSN` configurado).

## Correcciones

Ninguna.

## Requisitos modificados

Ninguno (se implementa capacidad prevista en REQ de ML/analítica).

## Arquitectura modificada

- `docs/04-architecture/decisions/ADR-012-analytics-module.md`: documenta el cliente gRPC productivo y sus límites (Slice 1 sin persistencia en `ml.forecast`).
- `docs/04-architecture/MODULES.md`: módulo 6 pasa de "proveedor productivo pendiente" a "cliente gRPC implementado (Slice 1)".

## Experimentos / pruebas

- `go build ./...` y `go vet` pasan.
- `go test ./internal/analytics/...` pasa (73 tests previos sin regressión).

## Limitaciones conocidas

- El `forecast_uid` generado por el cliente gRPC aún no se persiste en `ml.forecast` ni es consultable vía `contract.v_forecast` (pendiente Slice 2: rol `app_mlservice` + escritura idempotente).
- El cliente gRPC usa canal insegura (loopback por defecto); TLS/mTLS pendiente.
- `CCO_ML_MODE=grpc` requiere que el servicio Python esté levantado y sirviendo un artefacto real; de lo contrario `ValidateContract`/`Forecast` devuelven `ANALYTICS_ML_UNAVAILABLE`.

## Migraciones / despliegue

No hay migraciones de base de datos en este slice. Variables de entorno nuevas (opcionales): `CCO_ML_ENDPOINT`, `CCO_ML_REQUEST_TIMEOUT_SECONDS`. Sin ellas se mantiene el comportamiento actual (cliente fake).

## Documentación relacionada

- `docs/04-architecture/decisions/ADR-012-analytics-module.md`
- `docs/04-architecture/MODULES.md`
- `backend/API.md` §15 (endpoints de analítica)

---

# REL-1.0-slice-2 — Release Notes (Slice 2: persistencia ML en ml.forecast)

- **Versión:** 1.0.0-slice-2
- **Fecha:** 2026-11-22
- **Estado:** Implementado (backend + servidor Python)

## Resumen

El servicio Python ML (Module 6) ahora persiste pronósticos en `ml.forecast` de `cco_data` y devuelve el `forecast_uid` al backend Go a través del contrato gRPC `forecast.v1.ForecastService.Predict`. El cliente Go lee el `forecast_uid` de la respuesta del servidor en lugar de generarlo localmente, cumpliendo el requisito de que el `forecast_uid` sea la referencia pública compartida consultable vía `contract.v_forecast`.

## Nuevas funcionalidades

- `src/forecasting/serving/data.py`: `ForecastWriter` escribe en `ml.forecast` de forma idempotente (`ON CONFLICT ... DO NOTHING RETURNING` sobre la clave lógica `model_version_id, resource_id, target_metric, horizon_steps, step_unit, period_start_utc`). Resuelve `resource_uid` → `resource_id` y `model_name`/`version` → `model_version_id` (crea fila mínima en `ml.model_version` como bridge; Slice 3 lo reemplaza con `ml.training_run` + campeón).
- `src/forecasting/serving/engine.py`: `predict()` persiste el forecast vía `ForecastWriter` y devuelve `forecast_uid` en el payload. Fallo de persistencia no falla la inferencia (warning, no error).
- `src/forecasting/serving/server.py`: crea `ForecastWriter`, lo inyecta en el engine, pasa `dataset_hash` al predict, devuelve `forecast_uid` en la respuesta gRPC (`PredictResponse.forecast_uid`, campo 14).
- `src/forecasting/proto/forecast/v1/forecast.proto`: agrega `dataset_hash` (request, campo 7) y `forecast_uid` (response, campo 14). Stubs Python y Go regenerados.
- `backend/internal/analytics/adapters/grpc_ml_client.go`: lee `forecast_uid` de la respuesta del servidor; falla si el servidor no devolvió `forecast_uid` (no se presenta evidencia inexistente).
- `src/forecasting/` (base del servidor Python): servidor gRPC standalone, engine de inferencia, carga de artefactos `model.joblib` + `model_metadata.json`, lector read-only de `feat.feature_snapshot`, configuración desde env, TLS opcional, health check.
- `sql/create_forecasting_schema.sql`, `docker-compose.yml`, `requirements.txt`, `pyproject.toml`, `.env.example`, `GO_CLIENT_HANDOFF.md`, `DOCUMENTACION.md`.

## Cambios

- `MLPort` sigue siendo el contrato de inferencia; el cliente gRPC ahora devuelve el `forecast_uid` generado por el servidor Python (no un UUID local de Go).
- `docs/04-architecture/decisions/ADR-012-analytics-module.md`: documenta que el servidor Python es propietario de la escritura en `ml.*` y que el `forecast_uid` es consultable vía `contract.v_forecast`.
- `docs/04-architecture/MODULES.md`: módulo 6 pasa de "cliente gRPC implementado (Slice 1)" a "servidor Python persiste en ml.forecast (Slice 2)".

## Correcciones

Ninguna.

## Requisitos modificados

Ninguno (se implementa capacidad prevista en REQ de ML/analítica).

## Arquitectura modificada

- `docs/04-architecture/decisions/ADR-012-analytics-module.md`: actualizado para Slice 2.
- `docs/04-architecture/MODULES.md`: actualizado para Slice 2.

## Experimentos / pruebas

- `go build ./...` y `go vet` pasan.
- `go test ./internal/analytics/...` y `./internal/recommendation/...` pasan sin regressión.
- `py_compile` de los módulos Python modificados (`serving/data.py`, `serving/engine.py`, `serving/server.py`, stubs proto) pasa.

## Limitaciones conocidas

- `resolve_model_version_id` crea una fila mínima en `ml.model_version` cuando el artefacto no está registrado vía entrenamiento (bridge de Slice 2; Slice 3 lo reemplaza con `ml.training_run` + promoción a `champion`).
- No hay validación de `dataset_hash` contra `ml.training_dataset` todavía.
- El cliente gRPC usa canal insegura (loopback por defecto); TLS/mTLS pendiente.
- `CCO_ML_MODE=grpc` requiere que el servicio Python esté levantado y sirviendo un artefacto real; de lo contrario `ValidateContract`/`Forecast` devuelven `ANALYTICS_ML_UNAVAILABLE`.
- Los módulos de entrenamiento (`training.py`, `cli.py train-real`, etc.) de la rama base existen pero no están integrados con el backend Go todavía (Slice 3).

## Migraciones / despliegue

No hay migraciones de base de datos en este slice (el esquema `ml.*` ya existía en `cco_data_schema.sql`). Variables de entorno nuevas (opcionales): `CCO_ML_ENDPOINT`, `CCO_ML_REQUEST_TIMEOUT_SECONDS`. Para el servidor Python: `FORECAST_MODEL_DIR`, `FORECAST_DATA_DSN`, `FORECAST_GRPC_ADDR`. Sin ellas se mantiene el comportamiento actual (cliente fake en Go).

## Documentación relacionada

- `docs/04-architecture/decisions/ADR-012-analytics-module.md`
- `docs/04-architecture/MODULES.md`
- `backend/API.md` §15 (endpoints de analítica)
- `src/forecasting/GO_CLIENT_HANDOFF.md` (contrato para cliente Go)
