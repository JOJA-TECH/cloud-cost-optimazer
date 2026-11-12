# DESIGN-AMDA-001 — Diseño técnico del módulo Adquisición y Monitoreo de Datos

## Componente

`backend/internal/acquisition/` — paquete Go que implementa el módulo Adquisición y Monitoreo de Datos del backend monolito modular (Release 1.0). Es el port a Go del collector Python de la rama `dataset-metrics-collector` (solo la parte de adquisición núcleo; el generador de workload, Telegram y el spool quedan fuera — ver "Fuera de alcance").

Contiene:

| Archivo | Responsabilidad |
|---------|-----------------|
| `interfaces.go` | Contratos públicos: `MetricsProvider`, `DiagnosticsProvider`, `QueryStoreProvider`, `AcquisitionRepository`, `TargetRepository`, DTOs (`MetricReading`, `AcquisitionRunDTO`, `CollectorRunRow`) |
| `errors.go` | Errores de dominio, `ErrorWithCode`, `MapErrorToCode` (patrón cloud/iam) |
| `service.go` | Caso de uso `RunCycle`: resolver targets → abrir `collector_run` → fetch → normalizar/validar → persistir idempotente → cerrar run |
| `scheduler.go` | Goroutine ticker (intervalo por env) + trigger manual; un ciclo a la vez por recurso (mutex) |
| `normalizer.go` | Mapa nombre crudo → código canónico (`catalog.metric_catalog`), unidades y validación de rangos (bounds + tolerancia 5%) |
| `handlers.go` | Handlers HTTP bajo `/api/v1/acquisitions/*` |
| `repositories/data_repository.go` | Escrituras en `cco_data` (`raw.*`, resolución `core.resource`) vía segunda conexión Postgres |
| `repositories/target_repository.go` | Lecturas en `cco_app`: bindings monitoreados, recursos y cuentas (credenciales ARM) |
| `adapters/azure_monitor.go` | REST Azure Monitor Metrics (`azidentity` + `net/http`), igual patrón que `internal/cloud/adapters/azure.go` |
| `adapters/sql_diagnostics.go` | DMVs y Query Store por conexión SQL directa (`go-mssqldb`) |
| `adapters/fake.go` | Adaptador en memoria para dev/tests |

## Responsabilidad

Obtener métricas operativas de los recursos configurados (bindings con `is_monitored=true`), validar y normalizar los datos recibidos, y registrar el resultado de cada proceso de adquisición. No decide recomendaciones, no calcula features ni percentiles, no genera reportes y no muta recursos en Azure (solo lectura).

Fuentes cubiertas en esta iteración (decisión del equipo: paridad completa con el collector Python):

| Fuente | Qué lee | Tabla destino (`cco_data.raw`) |
|--------|---------|-------------------------------|
| Azure Monitor | cpu_percent, physical_data_read_percent, log_write_percent, storage_percent, storage, workers_percent, sessions_percent, sessions_count, deadlock, dtu_consumption_percent (PT1M) | `metric_reading` |
| SQL DMVs | `sys.dm_db_resource_stats` (incremental) y `sys.dm_exec_sessions` | `db_resource_stats`, `metric_reading` (sesiones) |
| Query Store | Top-20 runtime stats por `last_execution_time` + textos de query | `query_store_runtime_stats`, `query_store_query` |

Entidades MODULES.md → implementación:

| Entidad MODULES.md | Implementación |
|--------------------|----------------|
| `AcquisitionJob` | Ciclo del scheduler (definición en memoria + trigger manual) |
| `AcquisitionResult` / `AcquisitionStatus` | Fila en `raw.collector_run` (`ok`/`partial`/`error`) |
| `MetricSample` / `MetricBatch` | Lote de `raw.metric_reading` |
| `MetricDefinition` | `catalog.metric_catalog` (contrato: solo se ingesta lo declarado ahí) |
| `DataQualityResult` | Validación de bounds en `normalizer.go` (usable/degradado/inválido) |

## Interfaces

### `MetricsProvider` (Azure Monitor)

```go
type MetricPoint struct {
    Timestamp         time.Time
    Average, Minimum  *float64
    Maximum, Total    *float64
    Count             *float64
}

type MetricsProvider interface {
    // FetchPlatformMetrics consulta PT1M para las métricas del catálogo en la
    // ventana dada y devuelve lecturas crudas ya mapeadas a código canónico.
    FetchPlatformMetrics(ctx context.Context, creds cloud.ConnectionCredentials,
        azureResourceID string, start, end time.Time) ([]RawReading, error)
}
```

Implementaciones: `adapters.FakeMonitor` (dev/tests), `adapters.AzureMonitor` (REST, `CCO_CLOUD_PROVIDER_MODE=azure`).

Detalles REST verificados (Microsoft Learn, api-version `2023-10-01`):

- `GET https://management.azure.com{armId}/providers/Microsoft.Insights/metrics?timespan={start}/{end}&interval=PT1M&metricnames=...&aggregation=average,minimum,maximum,total,count&api-version=2023-10-01`
- Auth: `azidentity.ClientSecretCredential` + scope `https://management.azure.com/.default` (el SP ya tiene ARM en módulo 2; se requiere rol **Monitoring Reader**).
- Métrica no soportada por el tipo de recurso: la API responde **200** con `errorCode != "Success"` en ese bloque → se omite esa métrica (no falla el ciclo). HTTP 400 real = params malformados.
- `storage` viene en **Bytes**; el resto en Percent/Count.
- 429 → respetar `Retry-After` y reintentar (máx 3).

### `DiagnosticsProvider` (DMVs) y `QueryStoreProvider`

```go
type DiagnosticsProvider interface {
    // FetchResourceStats lee sys.dm_db_resource_stats incremental
    // (WHERE end_time > lastSeen) y sesiones actuales.
    FetchResourceStats(ctx context.Context, sqlConn SQLConfig, target Target) ([]RawReading, []DBResourceStat, error)
}

type QueryStoreProvider interface {
    // FetchRuntimeStats lee TOP 20 runtime stats + textos de query.
    FetchRuntimeStats(ctx context.Context, sqlConn SQLConfig, target Target) ([]QueryStoreQuery, []QueryStoreRuntimeStat, error)
}
```

Conexión SQL: `github.com/microsoft/go-mssqldb` (única dependencia nueva), `Encrypt=true`, login **workload** con `VIEW DATABASE STATE`. Credencial por variables de entorno `CCO_ACQ_SQL_SERVER/USER/PASSWORD` (una sola para el laboratorio; ver Limitaciones).

### `TargetRepository` (cco_app, lectura) y `AcquisitionRepository` (cco_data, escritura)

```go
type Target struct {
    ResourceUID     uuid.UUID // contrato estable cco_app ↔ cco_data
    AzureResourceID string
    RegionCode      string
    // ... campos de cloud.resources
}

type TargetRepository interface {
    ListMonitoredTargets(ctx context.Context) ([]Target, error) // bindings is_monitored=true + recursos + cuenta
    ResolveCredentials(ctx context.Context, accountID uuid.UUID) (cloud.ConnectionCredentials, error) // reutiliza resolución env:VAR del módulo cloud
}

type AcquisitionRepository interface {
    EnsureResource(ctx context.Context, t Target) (resourceID int64, err error) // upsert cco_data.core.resource por azure_resource_id conservando resource_uid
    StartRun(ctx context.Context, source string, resourceID int64, scope string) (runID int64, err error)
    FinishRun(ctx context.Context, runID int64, status string, rows int, errMsg string) error
    InsertMetricReadings(ctx context.Context, rows []MetricReadingRow) error    // ON CONFLICT DO NOTHING
    InsertResourceStats(ctx context.Context, rows []DBResourceStatRow) error    // ON CONFLICT DO NOTHING
    InsertQueryStore(ctx context.Context, qs []QueryStoreQueryRow, rs []QueryStoreRuntimeStatRow) error
    ListRuns(ctx context.Context, f RunFilter) ([]CollectorRunRow, error)
    GetRun(ctx context.Context, id int64) (CollectorRunRow, error)
}
```

## Entradas

### HTTP (endpoints protegidos, patrón cloud: `AuthMiddleware` + `RequirePermission`)

| Endpoint | Método | Body | Permiso | Descripción |
|----------|--------|------|---------|-------------|
| `/api/v1/acquisitions/run` | POST | `{resource_uid?}` (opcional: un recurso o todos) | `acquisitions:run` | Dispara un ciclo ahora; responde con los `collector_run` creados |
| `/api/v1/acquisitions` | GET | query: `resource_uid`, `source`, `status`, `limit` | `acquisitions:read` | Lista runs de adquisición |
| `/api/v1/acquisitions/{id}` | GET | — | `acquisitions:read` | Detalle de un run (filas, error, payload de request) |

Permisos nuevos en el mapa de roles por defecto (`internal/iam/auth.go`): `owner: acquisitions:*`, `admin: acquisitions:read,acquisitions:run`, `viewer: acquisitions:read`.

### Variables de entorno

| Variable | Default | Uso |
|----------|---------|-----|
| `CCO_DATA_DSN` | — | DSN Postgres de `cco_data`. Si está vacío: scheduler deshabilitado y endpoints responden 503 `STORAGE_UNAVAILABLE` |
| `CCO_ACQ_INTERVAL_SECONDS` | 60 | Periodo del scheduler |
| `CCO_ACQ_MONITOR_DELAY_SECONDS` | 90 | Retraso del fin de ventana (métricas se publican con 1–3 min de delay) |
| `CCO_ACQ_MONITOR_LOOKBACK_SECONDS` | 300 | Solape hacia atrás para recuperar puntos tardíos (la idempotencia deduplica) |
| `CCO_ACQ_SOURCES` | `azure_monitor,dmv,query_store` | Fuentes activas |
| `CCO_ACQ_SQL_USER/PASSWORD` | — | Login SQL workload (con `VIEW DATABASE STATE`) para DMVs/Query Store; servidor y base salen del target (`server.database.windows.net`). Sin ellas solo corre Azure Monitor |

## Salidas

- Filas normalizadas e idempotentes en `cco_data.raw.*` (ver Datos).
- `raw.collector_run` por (ciclo × recurso × fuente): `status`, `rows_ingested`, `error_message`, `request_payload` (timespan, interval, métricas pedidas).
- Respuestas HTTP de runs; errores en formato `{"code": "...", "message": "..."}`.

## Dependencias

| Paquete | Uso |
|---------|-----|
| `github.com/Azure/azure-sdk-for-go/sdk/azidentity` | Token ARM (ya en go.mod) |
| `github.com/microsoft/go-mssqldb` | **Nueva**: conexión SQL directa para DMVs/Query Store |
| `gorm.io/gorm` + driver postgres | Repositorios (patrón ADR-003); `net/http` para handlers |

Internas: `internal/cloud` (reutiliza `ConnectionCredentials` y la resolución de `credential_reference` env:VAR — misma función, extraída a helper compartido o duplicada mínima), `internal/iam` (`PermissionChecker`, `IdentityContext`), `internal/platform/database` (segunda conexión).

## Datos

Segunda conexión Postgres a `cco_data` (ADR-002: sin FKs entre bases; el puente son los UUID de contrato). Los modelos GORM de `cco_data` viven en el paquete `repositories` del módulo (no en `platform/database/models.go`, que es de `cco_app`).

| Tabla destino | Clave de idempotencia (ON CONFLICT DO NOTHING) |
|---------------|------------------------------------------------|
| `raw.metric_reading` | PK `(resource_id, metric_code, timestamp_utc, interval_seconds)` |
| `raw.db_resource_stats` | PK `(resource_id, end_time_utc)` |
| `raw.query_store_runtime_stats` | PK `(resource_id, query_id, interval_start_utc)` |
| `raw.query_store_query` | PK `(resource_id, query_id, snapshot_at_utc)` |
| `raw.collector_run` | INSERT (identity), referenciado en `request_payload` de cada ciclo |

`raw.metric_reading.metric_code` tiene FK a `catalog.metric_catalog`: el normalizador solo emite códigos declarados; nombre desconocido → se descarta y se registra en el log del run (nunca se inventa un código). Semillas a añadir al DDL (`databases/cco_data/cco_data_schema.sql`, `ON CONFLICT DO NOTHING`): `sessions_count` (Azure Monitor, Count) y `dtu_consumption_percent` (Percent).

Resolución `resource_uid` → `resource_id`: `EnsureResource` hace upsert en `core.resource` por `azure_resource_id` (INSERT con el `resource_uid` de cco_app; ON CONFLICT UPDATE `lifecycle_status`/`last_seen` y RETURNING `resource_id`), asegurando antes la fila en `catalog.region`. Esto absorbe la sincronización que el diseño de cloud delegaba al módulo Python.

### Flujo del ciclo

1. Scheduler dispara (o POST manual). Mutex global: si hay un ciclo en curso, el trigger manual espera o rechaza (422 `CYCLE_IN_PROGRESS`).
2. `ListMonitoredTargets` (cco_app). Sin targets → run vacío `ok`.
3. Por target: `EnsureResource` → `resource_id`.
4. Por fuente activa: `StartRun` → fetch → normalizar + validar bounds → insert idempotente → `FinishRun(ok/partial)`. Un fallo de una fuente no aborta las demás; el ciclo nunca tumba el scheduler.
5. Ventana Azure Monitor: `end = now - delay`, `start = end - lookback`; `timestamp_utc` tal cual llega (Azure ya alinea a minuto). DMV incremental: cursor `last_end_time` en memoria por recurso; solo se avanza si la escritura tuvo éxito (reintento natural en el siguiente ciclo — **sustituye al spool del collector Python**: todo es re-obtenible y los upserts deduplican).

## Errores

| Error | HTTP | Código |
|-------|------|--------|
| `ErrStorageUnavailable` (CCO_DATA_DSN vacío) | 503 | `STORAGE_UNAVAILABLE` |
| `ErrCycleInProgress` | 422 | `CYCLE_IN_PROGRESS` |
| `ErrResourceNotFound` | 404 | `RESOURCE_NOT_FOUND` |
| `ErrInvalidRequest` | 400 | `INVALID_REQUEST` |
| `ErrRunNotFound` | 404 | `ACQUISITION_RUN_NOT_FOUND` |
| Fallo de fuente puntual (Azure/SQL) | — | run `partial`/`error` + `error_message`; HTTP del trigger 200 con estado por fuente |

## Seguridad

- Solo lectura contra Azure (métricas) y contra las DMVs (`VIEW DATABASE STATE`); nunca muta bases productivas (§7.2).
- Secretos: ARM por `credential_reference` env (módulo 2); SQL por variables de entorno; nada en logs ni en `error_message` (RNF-02, RNF-15). `response_payload` verbatim de `collector_run` se deja **desactivado** por tamaño.
- Endpoints tras `AuthMiddleware` + `RequirePermission`; aislamiento por organización en `ListRuns`/`GetRun` (el servicio filtra por `OrgID` del `IdentityContext` → 404 ante cruce, patrón cloud).
- Headers de trazabilidad `X-Request-ID`/`X-Correlation-ID` propagados (RNF-13).

## Rendimiento

| Operación | Notas |
|-----------|-------|
| Ciclo completo | O(targets × fuentes); 20 recursos en paralelo objetivo RNF-04 → fetch concurrente por recurso con `errgroup` y semáforo (8 goroutines) |
| Inserts | Lotes (`ON CONFLICT DO NOTHING`, batch de ~500 filas) |
| Presupuesto RNF-01 | Ventana PT1M con delay 90s: el ciclo debe terminar antes del siguiente tick; el mutex lo garantiza |

## Decisiones relacionadas

| ID | Decisión | Documento |
|----|----------|-----------|
| ADR-002 | Dos bases PostgreSQL sin FKs entre sí | `docs/04-architecture/decisions/ADR-002-two-postgresql-databases.md` |
| Port a Go | Decisión del equipo (nov 2026): la adquisición es un módulo del backend Go; Python queda para ML/workload | acta reunión / `MODULES.md` §3 |
| RF-03 / RF-04 | Recolección periódica; separación dato crudo (raw) vs derivado (feat) | `Proyecto_De_Ingenieria.md` §10.1 |
| RNF-01 / RNF-04 | Pipeline < 5 min por recurso; ≥20 recursos paralelos | `Proyecto_De_Ingenieria.md` §10.2 |
| HU-05 | Informar cuando la evidencia sea insuficiente: lecturas fuera de rango se marcan/descartan y quedan auditadas en el run | `Proyecto_De_Ingenieria.md` §9.1 |
| DESIGN-CLOUD-001 | "Sync bajo demanda → Módulo 3 posee el scheduler" | `docs/05-design/DESIGN-CLOUD-001.md` |

## Pruebas relacionadas

| ID | Prueba | Archivo | Estado |
|----|--------|---------|--------|
| TEST-AMDA-001 | Normalizador: mapeo crudo→canónico, códigos desconocidos descartados, bounds (degradado ±5%, inválido fuera) | `backend/internal/acquisition/normalizer_test.go` | Implementada |
| TEST-AMDA-002 | Servicio: ciclo feliz con fakes escribe en `cco_data` SQLite, deduplica al repetir el ciclo, fallo de una fuente no aborta las demás, cursor DMV avanza | `backend/internal/acquisition/service_test.go` | Implementada |
| TEST-AMDA-003 | Adaptador Azure Monitor: 200 con `errorCode` por métrica, 429 + Retry-After, 400 | `backend/internal/acquisition/adapters/azure_monitor_test.go` (httptest) | Implementada |
| TEST-AMDA-004 | Scheduler: corre y para con contexto cancelado | `backend/internal/acquisition/scheduler_test.go` | Implementada |
| TEST-AMDA-005 | Repositorios: idempotencia `EnsureResource`/`InsertMetricReadings`, `ListRuns` org-scoped, `GetRun` 404 | `backend/internal/acquisition/repositories/*_test.go` | Implementada |
| TEST-AMDA-006 | Integración end-to-end: cloud (cuenta→binding) + ciclo con fakes + consultas org-scoped sobre SQLite | `backend/test/acquisition_integration_test.go` | Implementada |
| — | Ejecución: `go test ./... -count=1` desde `backend/` | — | — |

## Limitaciones conocidas y caminos de upgrade

| Limitación | Techo | Camino de upgrade |
|------------|-------|-------------------|
| Una credencial SQL para todas las bases monitoreadas (env) | No soporta recursos con logins distintos | Credencial SQL por recurso (columna en `cloud.resources` o tabla de conexiones) |
| Cursor DMV en memoria | Se reinicia al rearrancar el proceso → relee la última hora (idempotente, sin pérdida) | Persistir `last_end_time` en una tabla de estado |
| Sin spool local | Si `cco_data` está caída, solo se recupera lo re-obtenible por lookback; DMV se recupera por cursor no-avanzado | Portar spool con checksum si aparece una fuente no re-obtenible |
| Calidad: bounds solo al ingreso | No hay columna de `quality_status` en `raw.*` del esquema actual | Añadir columnas de calidad o portar `ops.metric_rejection` |
| Sin agregación horaria | `feat.metric_hourly` no se llena aquí | Módulo 4 (Gestión de Datos) / 5 (Analítica) |
| Costos no incluidos | `raw.collector_run.source` ya reserva `retail_prices`/`cost_management` | Fase 2 de este módulo o Gestión de Datos (RF-06) |

`shortcut: credencial SQL única por env → credenciales por recurso cuando haya multi-tenant; cursor DMV en memoria → tabla de estado si el reinicio molesta; sin spool → todo re-obtenible por lookback + upsert idempotente; agregación horaria y costos → módulos 4/5 y fase 2.`

## Fuera de alcance (explícito)

- Generador de carga, observaciones de workload (`workload_observation`), máquina de estados de experimentos y Telegram: quedan en la rama Python del experimento, no en el backend.
- `feat.metric_hourly`, percentiles y features: módulos 4/5.
- Activity Log y Advisor: tablas reservadas en `raw.*`, sin collector en esta iteración.

## Notas de implementación

- Orden de trabajo en 5 pasos: (1) normalizador + tests, (2) repositorios cco_data + `EnsureResource`, (3) adaptador Azure Monitor + tests httptest, (4) service + scheduler + handlers + wiring en `main.go`, (5) adaptador SQL DMV/Query Store + integración.
- En dev sin `CCO_DATA_DSN` ni `CCO_CLOUD_PROVIDER_MODE=azure`, todo el módulo queda funcional con fakes (patrón del módulo 2).
- El helper de resolución `env:VAR` se reutiliza del módulo cloud; si está privado, se exporta un pequeño helper en `internal/cloud` en vez de duplicarlo.

## Observabilidad y logging

`slog` a stdout (patrón ADR-006): eventos `acquisition cycle started/finished` (targets, filas por fuente, duración), `acquisition source failed` (source, recurso, error sin secretos), `metric unknown discarded` (nombre crudo). Campos: `request_id`, `correlation_id`, `resource_uid`, `collector_run_id`.

## Véase también

- `docs/05-design/DESIGN-CLOUD-001.md` — estructura de referencia de este documento.
- `backend/API.md` — sección 13 (endpoints de adquisición, por append).
- `MODULES.md` §3; `Proyecto_De_Ingenieria.md` §10 (RF-03/04, RNF-01/04), §13.5 (variables del dataset).
- `databases/cco_data/cco_data_schema.sql` — tablas `raw.*`, `catalog.metric_catalog`, `core.resource`.
- Rama `dataset-metrics-collector` — collector Python de referencia (normalizer, quality, writer idempotente, fuentes DMV/Query Store).
