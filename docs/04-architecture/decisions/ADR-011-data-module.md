# ADR-011 — Módulo Gestión de Datos (Module 4)

- **Estado:** Aceptado e implementado
- **Fecha:** 2026-11-12
- **Decisores:** Equipo del proyecto
- **Relacionado con:** ADR-001, ADR-003, ADR-004, ADR-010, MODULES.md (módulo 4)

## Contexto

El módulo Gestión de Datos es la única puerta de entrada a `cco_data` (base de telemetría). Almacena y sirve históricos de métricas, registros de costos, snapshots de configuración y comprueba disponibilidad/cobertura de datos (HU-05). Nunca habla con SDKs de cloud y nunca computa features o percentiles (eso es Analítica, Module 5).

## Problema

Definir el diseño de lectura y escritura sobre `cco_data` en Report 1.0:

1. ¿Qué contratos expone Gestión de Datos para que Adquisición escriba y Analítica lea?
2. ¿Qué consultas de solo lectura se exponen por API?
3. ¿Cómo se garantiza portabilidad entre SQLite (tests) y PostgreSQL (prod)?

## Alternativas consideradas

### Alternativa A: Cada módulo consulta `cco_data` directamente con SQL/GORM

**Desventajas:** Duplica lógica de consulta, acopla a GORM y al esquema, dificulta pruebas y cambia de contrato si el esquema evoluciona.

### Alternativa B: Capa de servicio con interfaces de repositorio (seleccionada)

**Ventajas**
- `DataWriter` (escritura idempotente) para Adquisición; lectores (`HistoricalDataReader`, `CostRepository`, `ConfigurationRepository`, `RunReader`) para Analítica y API.
- `DataQueryService` para preguntas de disponibilidad (coverage, HU-05).
- La agregación de cobertura (bucketización) se computa en Go, no en SQL, para correr igual en SQLite y PostgreSQL.
- Todos los endpoints de API son solo lectura y requieren permiso `data:read`.
- Sin `CCO_DATA_DSN`, los endpoints responden `503 DATA_STORAGE_UNAVAILABLE`.

## Decisión

Se decide **Alternativa B**:

1. **Contratos (`backend/internal/data/interfaces.go`):**
   - `DataWriter`: `EnsureResource`, `StartRun`, `FinishRun`, `WriteMetricReadings`, `WriteMetricSamples`, `WriteResourceStats`, `WriteQueryStore`, `UpsertCostDaily`, `UpsertConfigSnapshot`.
   - `HistoricalDataReader`: `MetricSeries`.
   - `CostRepository`: `CostDailyByResource`, `CostDailyByScope`.
   - `ConfigurationRepository`: `LatestConfigSnapshot`, `ConfigSnapshotHistory`.
   - `RunReader`: `ListRuns`, `GetRun`.
   - `DataQueryService`: `Coverage` (HU-05).
2. **Endpoints de API (solo lectura, permiso `data:read`):**
   - `GET /api/v1/data/metrics` — serie histórica de una métrica (`resource_uid`, `metric_code`, `from`, `to`, `limit`).
   - `GET /api/v1/data/costs` — costos diarios por recurso o por scope (`subscription`/`resource_group`/`resource`).
   - `GET /api/v1/data/config-snapshots` — historial de configuración ARM de un recurso.
   - `GET /api/v1/data/coverage` — disponibilidad de una métrica en una ventana (HU-05): `sufficient` / `insufficient`.
3. **Portabilidad:** bucketización y agregación en Go; mismos modelos y repositorios en SQLite y PostgreSQL; UUIDs generados en Go.
4. **Persistencia:** `cco_data` con tablas `raw.*`, `core.*`, `catalog.*`.

## Justificación

La capa de servicio con interfaces desacopla el dominio de GORM y del esquema físico, permite pruebas con fakes y ofrece un contrato estable a Adquisición y Analítica. La cobertura computada en Go garantiza el mismo resultado en dev (SQLite) y prod (PostgreSQL).

## Consecuencias

### Positivas

- Un solo punto de entrada a `cco_data`; fácil de mantener y probar.
- Escrituras idempotentes; no hay duplicados por reintentos.
- API de solo lectura; sin riesgo de escritura accidental desde el módulo de datos.

### Negativas

- Solo lectura por API; las escrituras solo las hace Adquisición a través de `DataWriter`.
- `cco_data` es obligatorio para que el módulo funcione (responde `503` sin DSN).

### Riesgos

- Si `cco_data` no está configurado, Adquisición no puede persistir y Analítica no tiene datos que leer.

## Evidencia

- `backend/internal/data/` — handlers, service, interfaces, repositories.
- `backend/internal/acquisition/service.go` — consume `data.DataWriter` / `data.RunReader`.
- `backend/internal/analytics/service.go` — consume lectores de `data`.
- `backend/cmd/cco-app/main.go` — abre `cco_data` y construye los servicios.
- `API.md` §14 — referencia de endpoints de datos.

## Revisión

- **Fecha prevista de revisión:** Cuando se requieran escrituras de datos desde módulos distintos a Adquisición, o cuando el esquema de `cco_data` evolucione significativamente.

## Referencias

[1]: docs/MODULES.md — Módulo 4, Gestión de Datos.
