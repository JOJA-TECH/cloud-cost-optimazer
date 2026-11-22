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
