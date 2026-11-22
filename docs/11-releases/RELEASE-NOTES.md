# REL-1.0-slice-3 — Release Notes (Slice 3: ML training and model registry)

- **Versión:** 1.0.0-slice-3
- **Fecha:** 2026-11-22
- **Estado:** Implementado (backend + servidor Python)

## Resumen

Se completa el ciclo de vida de modelos ML del Release 1.0: el servidor Python ML puede ahora entrenar modelos candidatos, registrarlos en `ml.training_run` / `ml.model_version`, y promoverlos a `champion`. El backend Go invoca el entrenamiento a través del handler de Jobs `ml:train` y consulta las versiones registradas con `GET /api/v1/ml/models`.

## Nuevas funcionalidades

- `src/forecasting/training.py` (nuevo): `train_model()` lee `feat.feature_snapshot`, entrena un modelo baseline (Random Forest o Regresión Lineal) con partición temporal 85/15, persiste `model.joblib` + `model_metadata.json` bajo `FORECAST_MODEL_DIR/<target>/<version>/`, y registra la corrida en `ml.training_run` y el candidato en `ml.model_version` (stage `candidate`). `list_models()` y `promote_model()` proporcionan la capa de gestión.
- `src/forecasting/models.py` (nuevo): `ModelSpec`/`fit_model`/`predict_model` mínimos para satisfacer la cadena de importación preexistente en `forecasting.transforms`.
- `src/forecasting/serving/server.py`: agrega el RPC `Train` a `ForecastService`. Llama a `training.train_model` y devuelve `training_run_id`, `model_version_id`, `mae` y `artefact_path`.
- `src/forecasting/proto/forecast/v1/forecast.proto`: agrega el RPC `Train` con mensajes `TrainRequest`/`TrainResponse`. Stubs Python y Go regenerados.
- `src/forecasting/serving/data.py`: corrige sintaxis de `SET statement_timeout` (PostgreSQL no acepta parámetros en `SET`); recorta `FEATURE_SNAPSHOT_COLUMNS` a las columnas que existen en el esquema base de `cco_data`.
- `backend/internal/analytics/adapters/grpc_ml_client.go`: exporta `GrpcMLClient` y agrega el método `Train()` para operaciones de gestión (separado del contrato `MLPort` de inferencia).
- `backend/internal/analytics/repositories/analytics_repository.go`: agrega `ListModels()` que consulta `ml.model_version` unido con `ml.training_run`.
- `backend/cmd/cco-app/main.go`: agrega el handler de Jobs `ml:train` (llama a `GrpcMLClient.Train`) y el endpoint `GET /api/v1/ml/models` (lista versiones de modelo, requiere `analytics:read`). `mlClient` se sacó del bloque `if CCO_DATA_DSN` para que los handlers de Jobs puedan acceder a él.

## Cambios

- `MLPort` sigue siendo el contrato de inferencia únicamente; `Train` es una operación de gestión en el mismo cliente gRPC (pendiente de separar en `MLManagementPort` en Slice 4).
- `docs/04-architecture/decisions/ADR-012-analytics-module.md`: documenta el entrenamiento, registro y promoción de modelos (Slice 3).
- `docs/04-architecture/MODULES.md`: módulo 6 pasa de "pendiente (Slice 3)" a "entrenamiento y registro disponibles (Slice 3)".

## Correcciones

Ninguna.

## Requisitos modificados

Ninguno (se implementa capacidad prevista en REQ de ML/analítica).

## Arquitectura modificada

- `docs/04-architecture/decisions/ADR-012-analytics-module.md`: actualizado para Slice 3.
- `docs/04-architecture/MODULES.md`: actualizado para Slice 3.

## Experimentos / pruebas

- `go build ./...` y `go vet` pasan.
- `go test ./internal/analytics/...` y `./internal/recommendation/...` pasan sin regressión.
- Prueba directa de Python: `GetModelInfo`, `Predict` (devuelve `insufficient_data` honestamente cuando no hay historial contiguo suficiente), y `Train` (MAE 4.07, registrado en `ml.training_run` y `ml.model_version`) funcionan contra `cco_data` en Docker.

## Limitaciones conocidas

- No existe aún una interfaz `MLManagementPort` separada; `Train` comparte el servicio gRPC con `Predict`. Document dice que deben dividirse en Slice 4.
- La promoción a `champion` no aplica umbrales de calidad automáticos (Slice 4); cualquier versión puede promoverse con `promote_model()`.
- No hay validación de `dataset_hash` contra `ml.training_dataset` durante el entrenamiento.
- No hay API para eliminación de modelos o rollback.
- El cliente gRPC usa canal insegura (loopback por defecto); TLS/mTLS pendiente.

## Migraciones / despliegue

No hay migraciones de base de datos en este slice (el esquema `ml.*` ya existía en `cco_data_schema.sql`). Variables de entorno nuevas (opcionales): `FORECAST_MODEL_DIR`, `FORECAST_DATA_DSN`, `FORECAST_GRPC_ADDR`. En el backend Go, `CCO_ML_MODE=grpc` y `CCO_ML_ENDPOINT` son requeridas para usar el servicio Python productivo.

## Documentación relacionada

- `docs/04-architecture/decisions/ADR-012-analytics-module.md`
- `docs/04-architecture/MODULES.md`
- `backend/API.md` §15 (endpoints de analítica)

---

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
