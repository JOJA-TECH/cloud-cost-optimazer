# DESIGN-DATA-001 — Diseño técnico del módulo Gestión de Datos

## Componente

`backend/internal/data/` — paquete Go que implementa el módulo Gestión de Datos (módulo 4). Es la **única capa de acceso a `cco_data`**: centraliza escritura y lectura de telemetría, costos y snapshots de configuración, y expone APIs REST de consulta para el dashboard.

Contiene:

| Archivo | Responsabilidad |
|---------|-----------------|
| `interfaces.go` | Contratos públicos (MODULES.md): `DataWriter`, `HistoricalDataReader`, `CostRepository`, `ConfigurationRepository`, `DataQueryService`, `RunReader` + DTOs (`ResourceRef`, `Metric`, `HistoricalMetric`, `CostRecord`, `ConfigurationSnapshot`, `DataQuery`, `CoverageReport`, `RunRecord`) |
| `errors.go` | Errores de dominio, `ErrorWithCode`, `MapErrorToCode` (patrón cloud/iam/acquisition) |
| `service.go` | Casos de uso de lectura: validación de parámetros (rangos, límites), cobertura HU-05 |
| `handlers.go` | Handlers HTTP bajo `/api/v1/data/*` + `RegisterRoutes` |
| `repositories/data_repository.go` | Único repositorio de `cco_data`: row structs, `MigrateDataTables`, escrituras idempotentes (`ON CONFLICT`), consultas de lectura |
| `repositories/data_repository_test.go` | Roundtrip, idempotencia y cobertura (SQLite) |

Adicionalmente el paquete `internal/acquisition/` se **refactoriza** para depender de los contratos de `data` (ver "Refactor acquisition → data") y recibe **dos recolectores nuevos** (config ARM y costos), porque obtener datos de los proveedores es responsabilidad de Adquisición según MODULES.md; `data` solo persiste y consulta.

## Responsabilidad

Proporcionar una capa consistente para almacenar, consultar y recuperar datos históricos, costos y snapshots de configuración, sin acoplar el dominio a un motor de persistencia. No calcula percentiles ni features (eso es Analítica, módulo 5 sobre `feat.*`), no decide recomendaciones y no habla con SDKs cloud.

**Cobertura de casos de uso MODULES.md en esta iteración:**

| Caso de uso | Implementación |
|-------------|----------------|
| Persistir métricas normalizadas y resultados de calidad | `DataWriter.WriteMetricReadings` / `WriteResourceStats` / `WriteQueryStore` (movidos desde acquisition, idempotentes). La calidad (`usable/degraded/invalid`) queda **derivada en lectura** (ver Limitaciones) |
| Consultar históricos por recurso, métrica y rango temporal | `HistoricalDataReader.MetricSeries` + `GET /api/v1/data/metrics` |
| Consultar registros de costos y snapshots de configuración | `CostRepository` / `ConfigurationRepository` + endpoints de lectura; escritura alimentada por los recolectores nuevos de acquisition |
| Preparar conjuntos de datos para los módulos consumidores | **Fuera de alcance**: `feat.v_training_base` ya existe y la extracción es de Analítica (módulo 5). M4 aporta lecturas atómicas y cobertura |
| Comprobar disponibilidad y cobertura temporal de los datos (HU-05) | `DataQueryService.Coverage` + `GET /api/v1/data/coverage` |

## Decisiones de alcance (acordadas con el equipo)

1. **Refactor a contrato**: acquisition deja de escribir directamente en `cco_data` con su propio repositorio; pasa a depender de `data.DataWriter` / `data.RunReader` (MODULES.md: "Gestión de Datos mediante contratos"). El repositorio `acquisition/repositories/data_repository.go` se **mueve** a `data/repositories/`.
2. **Incluir ambos recolectores** en esta iteración: snapshots de configuración (ARM) y costos diarios (Cost Management), implementados en acquisition con el patrón existente (interfaz + adapter Azure + fake + ciclo del scheduler), que persisten vía `data.DataWriter`.

## Dirección de dependencias

```
iam ← cloud ← acquisition ──→ data ──→ platform/database (+ gorm)
                  (writer/reader contracts)      (cco_data)
```

- `data` **no importa** a `acquisition` ni a ningún otro módulo de negocio. Define sus propios DTOs de escritura; acquisition mapea sus tipos (`RawReading` → `data.Metric`, `Target` → `data.ResourceRef`) en los call sites.
- `acquisition` importa `data` (dependencia permitida por MODULES.md en ambos sentidos lógicos; el código solo necesita la dirección acquisition→data).
- `main.go` construye **una sola instancia** del repositorio de `data` sobre `CCO_DATA_DSN` y la pasa al servicio de acquisition (writer) y al de data (reader).

## Entidades MODULES.md → implementación

| Entidad | Implementación |
|---------|----------------|
| `Metric` / `HistoricalMetric` | `data.Metric` (escritura) / `data.HistoricalMetric` (lectura) sobre `raw.metric_reading` |
| `CostRecord` | `data.CostRecord` sobre `core.cost_daily` |
| `ConfigurationSnapshot` | `data.ConfigurationSnapshot` sobre `core.resource_config_snapshot` (+ lookup `catalog.sql_sku`) |
| `DataSet` | Fuera de alcance (módulo 5: `feat.v_training_base`, `ml.training_dataset`) |
| `DataQualityRecord` | Derivado en lectura: cobertura por ventana (conteo de lecturas/hora, huecos, estado `sufficient/insufficient`). No se persiste calidad por fila (ver Limitaciones) |
| `DataQuery` | `data.DataQuery` (`ResourceUID`, `MetricCode`, `FromUTC`, `ToUTC`) |

`resource_uid` ↔ `resource_id`: mismo patrón verificado de acquisition — `JOIN core.resource ON resource_uid = ?` para traducir el UUID de contrato (`cco_app.cloud.resource_bindings.data_resource_uid`) al surrogate interno. Nunca se expone `resource_id` por API.

## Interfaces

### `DataWriter` (contrato que acquisition consume)

```go
type DataWriter interface {
    EnsureResource(ctx context.Context, r ResourceRef) (int64, error) // upsert core.resource por azure_resource_id
    StartRun(ctx context.Context, source string, resourceID int64, scope string) (int64, error)
    FinishRun(ctx context.Context, runID int64, status string, rowsIngested int, errMsg string) error
    WriteMetricReadings(ctx context.Context, resourceID int64, m []Metric) (int, error)      // ON CONFLICT DO NOTHING (PK natural)
    WriteResourceStats(ctx context.Context, resourceID int64, s []ResourceStat) (int, error) // ON CONFLICT DO NOTHING
    WriteQueryStore(ctx context.Context, resourceID int64, q []QueryStoreQueryRecord, s []QueryStoreRuntimeRecord) (int, error)
    UpsertCostDaily(ctx context.Context, c []CostRecord) (int, error)                        // ON CONFLICT ux_cost_daily_source_charge
    UpsertConfigSnapshot(ctx context.Context, s ConfigurationSnapshot) (int64, error)        // ON CONFLICT (resource_id, observed_at_utc)
}
```

### Lectores (los consume el service/handlers de `data` y acquisition)

```go
type HistoricalDataReader interface {
    MetricSeries(ctx context.Context, q DataQuery, limit int) ([]HistoricalMetric, error)
}

type RunReader interface { // reads de raw.collector_run para los endpoints de acquisition
    ListRuns(ctx context.Context, uids []uuid.UUID, f RunFilter) ([]RunRecord, error) // empty uids = empty result
    GetRun(ctx context.Context, id int64) (RunRecord, error)
}

type CostRepository interface {
    CostDailyByResource(ctx context.Context, resourceUID uuid.UUID, from, to time.Time, limit int) ([]CostRecord, error)
    CostDailyByScope(ctx context.Context, scopeType, scopeName string, from, to time.Time, limit int) ([]CostRecord, error)
}

type ConfigurationRepository interface {
    LatestConfigSnapshot(ctx context.Context, resourceUID uuid.UUID) (*ConfigurationSnapshot, error)
    ConfigSnapshotHistory(ctx context.Context, resourceUID uuid.UUID, from, to time.Time, limit int) ([]ConfigurationSnapshot, error)
}

type DataQueryService interface {
    // Coverage calcula en Go (portable SQLite/Postgres): bucketiza lecturas por
    // hora/día, cuenta huecos y compara contra minCoverage (HU-05).
    Coverage(ctx context.Context, resourceUID uuid.UUID, metricCode string,
        from, to time.Time, bucket string /* "1h"|"1d" */, minCoverage float64) (CoverageReport, error)
}
```

Un solo `dataRepository{db *gorm.DB}` implementa todo (como hoy hace acquisition). `MigrateDataTables(db)` se traslada a `data/repositories` y extiende los modelos SQLite de tests con `cost_daily`, `resource_config_snapshot` y `sql_sku`.

### Idempotencia (claves únicas verificadas en `cco_data_schema.sql`)

| Tabla | Clave | Política |
|-------|-------|----------|
| `raw.metric_reading` | PK `(resource_id, metric_code, timestamp_utc, interval_seconds)` | `ON CONFLICT DO NOTHING` |
| `raw.db_resource_stats` | PK `(resource_id, end_time_utc)` | `ON CONFLICT DO NOTHING` |
| `core.cost_daily` | `ux_cost_daily_source_charge (provider_code, source_charge_id) WHERE source_charge_id IS NOT NULL` | `ON CONFLICT DO NOTHING`; `CostRecord.SourceChargeID == nil` → **error de dominio** (evita duplicar sin control) |
| `core.resource_config_snapshot` | `UNIQUE (resource_id, observed_at_utc)` | `ON CONFLICT (resource_id, observed_at_utc) DO NOTHING`; el recolector además omite la escritura si la config no cambió (ver abajo) |

## API REST (módulo data)

Todas bajo `data:read`, envueltas en `authMW.Handler(middleware.RequirePermission(perm, "data", "read")(…))`:

| Método y ruta | Parámetros | Devuelve |
|---------------|-----------|----------|
| `GET /api/v1/data/metrics` | `resource_uid` (req), `metric_code` (req), `from`/`to` (req, ISO-8601 UTC), `limit` (def 1000, máx 10000) | Serie `{resource_uid, metric_code, points: [{timestamp_utc, interval_seconds, avg, min, max, total, count}]}` ascendente |
| `GET /api/v1/data/costs` | `resource_uid` **o** `scope_type`+`scope_name` (req), `from`/`to`, `limit` | Filas diarias `{usage_date, service_name, scope_name, region_code, pre_tax_cost, currency, usage_quantity, usage_unit, pricing_model, charge_category, resource_uid?}` |
| `GET /api/v1/data/config-snapshots` | `resource_uid` (req), `limit` (def 50), opcional `from`/`to` | Historial descendente `{observed_at_utc, sku_name, tier, family, capacity_vcores, compute_model, max_size_bytes, auto_pause_delay_minutes, status, free_limit}`; el primero es el vigente |
| `GET /api/v1/data/coverage` | `resource_uid`, `metric_code`, `from`/`to`, `bucket` (`1h`/`1d`, def `1h`), `min_coverage` (def 0.8) | `{horas_ventana, horas_con_datos, huecos, total_lecturas, estado, por_cobertura}` (HU-05) |

Los endpoints existentes de adquisición (`/api/v1/acquisitions/runs`) siguen en acquisition, pero su service ahora lee vía `data.RunReader` y mapea `RunRecord` → `AcquisitionRunDTO`.

Errores: mismo patrón `ErrorWithCode` + `MapErrorToCode`; códigos nuevos `DATA_NOT_FOUND` (404), `DATA_INVALID_QUERY` (400), `DATA_STORAGE_UNAVAILABLE` (503 cuando no hay `CCO_DATA_DSN`, igual que acquisition).

## Recolectores nuevos (en `internal/acquisition`)

### A. Snapshot de configuración (ARM)

- Interfaz: `ConfigProvider { FetchConfigSnapshot(ctx, creds, azureResourceID) (ConfigReading, error) }`.
- REST verificado: `GET https://management.azure.com{armId}?api-version=2025-01-01` (Microsoft.Sql/servers/databases). Rol **Reader** en la suscripción (el SP de módulo 2 ya lo tiene).
- Mapeo por nombre JSON exacto: `sku.name`, `sku.tier`, `sku.family`, `sku.capacity` → `capacity_vcores`; `properties.maxSizeBytes`, `properties.autoPauseDelay`, `properties.status`, `properties.licenseType`, `properties.currentServiceObjectiveName`, `properties.kind`.
- Resolución de SKU: `sku.name` (p.ej. `GP_S_Gen5_2`) → `catalog.sql_sku.sku_id` por lookup; si no está en el catálogo, se inserta con `sku_id = NULL` y el `sku_name` textual (la tabla lo permite).
  `shortcut: sku fuera del catálogo queda con sku_id NULL, se corrige sembrando catalog.sql_sku; upgrade: alta automática desde ARM.`
- **Dedupe**: el service lee `LatestConfigSnapshot` y solo inserta si algún campo cambió → `core.resource_config_snapshot` crece solo con cambios reales, no una fila por ciclo.
- Frecuencia: cada ciclo del scheduler existente (por target, barato: 1 GET por recurso).

### B. Costos diarios (Cost Management)

- Interfaz: `CostProvider { FetchDailyCosts(ctx, creds, subscriptionID string, from, to time.Time) ([]data.CostRecord, error) }`.
- REST verificado: `POST https://management.azure.com/subscriptions/{subId}/providers/Microsoft.CostManagement/query?api-version=2026-06-01`, cuerpo `{type: "Usage", timeframe: "Custom", timePeriod: {from, to}, dataset: {granularity: "Daily", aggregation: {totalCost: {name: "Cost", function: "Sum"}, usageQuantity: {name: "UsageQuantity", function: "Sum"}}, grouping: [{type: "Dimension", name: "ResourceId"}, {type: "Dimension", name: "ServiceName"}, {type: "Dimension", name: "MeterId"}, {type: "Dimension", name: "ChargeType"}]}}`. Rol **Cost Management Reader** en la suscripción (nuevo requisito de RBAC para el SP — documentarlo en el despliegue).
- Respuesta: `properties.columns` + `properties.rows`; **mapear columnas por nombre** (el orden no es fijo). `Currency` es columna; paginar con `nextLink`.
- Mapeo a `CostRecord`: `ResourceID` ARM → `resource_uid`/`resource_id` si coincide con `core.resource.azure_resource_id`, si no `resource_id = NULL` (costos sin mapeo: Storage, VNet — la columna lo permite). `scope_type = "subscription"`, `scope_name = subscriptionId`.
- **Idempotencia**: `source_charge_id` sintético determinista: `"cost:" + subscriptionID + ":" + usageDate + ":" + sha1(resourceArmID|serviceName|meterID|chargeCategory)[:12]` → re-ejecuciones del lookback no duplican.
- **Lag de datos**: Cost Management completa el día con 8–24 h de retraso (hasta ~72 h en pay-as-you-go). El job re-consulta una ventana de `CCO_ACQ_COST_LOOKBACK_DAYS` (def 3) y el upsert idempotente corrige cargos tardíos.
- Frecuencia: ticker propio en `scheduler.go` (def 6 h), independiente del ciclo de métricas. Un scope por suscripción (agrupa targets por `SubscriptionID`), no por recurso.
- `shortcut: costos no asignan resource_group como scope separado; upgrade: segunda query agrupada por ResourceGroup cuando se necesite.`

### Cambios en acquisition

| Archivo | Cambio |
|---------|--------|
| `interfaces.go` | Se elimina `AcquisitionRepository`; `NewService` recibe `data.DataWriter` + `data.RunReader`. Nuevas interfaces `ConfigProvider`, `CostProvider`. Se agregan mapeos `RawReading→data.Metric`, `Target→data.ResourceRef`, `RunRecord→AcquisitionRunDTO` |
| `service.go` | `RunCycle` agrega paso de config snapshot (fetch → dedupe → upsert). Nuevo `RunCostCycle(subscriptionID)` |
| `scheduler.go` | Segundo ticker para costos; agrupación de targets por suscripción |
| `adapters/azure_config.go` (nuevo) | GET ARM database + mapeo |
| `adapters/azure_cost.go` (nuevo) | POST Cost Management query + paginación + mapeo de columnas |
| `adapters/fake.go` | Fakes de `ConfigProvider` y `CostProvider` para dev/tests |
| `repositories/data_repository.go` | **Eliminado** (movido a `data/repositories/`). `target_repository.go` permanece (lee `cco_app`) |

## Permisos

- Nuevo permiso `data:read` en los mapas `rolePermissions` / `membershipRolePermissions` de `iam/auth.go`: system roles `db_admin`, `analyst`, `viewer` (`platform_admin` ya cubre con `*:*`); org roles `owner`, `admin`, `member`, `viewer`.
- Las escrituras del módulo ocurren vía service (scheduler/collectors), no por HTTP → no requieren permiso nuevo; `collector` conserva `metrics:write`.
- Seeds de permisos en `test/iam_integration_test.go` si los tests del módulo las requieren (patrón actual: las tablas `iam.permissions` se siembran por test).

## Variables de entorno nuevas

| Variable | Defecto | Uso |
|----------|---------|-----|
| `CCO_ACQ_COST_INTERVAL_SECONDS` | `21600` (6 h) | Ticker del job de costos |
| `CCO_ACQ_COST_LOOKBACK_DAYS` | `3` | Ventana de re-consulta por lag de Cost Management |

`CCO_DATA_DSN` ya existe y se reutiliza (una conexión a `cco_data` compartida por acquisition y data). Sin `CCO_DATA_DSN`: endpoints de lectura responden 503 `DATA_STORAGE_UNAVAILABLE` y los recolectores no arrancan (patrón acquisition).

## Consultas de lectura (SQL portátil)

- Series y costos: `SELECT … WHERE resource_id = ? AND metric_code = ? AND timestamp_utc >= ? AND timestamp_utc < ? ORDER BY timestamp_utc ASC LIMIT ?` — SQL llano, funciona igual en SQLite (tests) y Postgres (prod) vía `database.TableName(db, schema, table)`. Índices existentes suficientes (PK por partición + BRIN).
- Cobertura: **se calcula en Go** (bucket `1h`/`1d` sobre los timestamps de la ventana, conteo de huecos contra una grilla virtual) porque `generate_series`/`date_trunc` no existen en SQLite y el patrón del repo exige tests de integración SQLite. Volumen de laboratorio (≤ 90 días × 24 h) es trivial.
  `shortcut: cobertura en memoria en Go, no SQL analítico; upgrade: mover a vista SQL en cco_data si la ventana crece órdenes de magnitud.`
- Config snapshots: `ORDER BY observed_at_utc DESC LIMIT ?` sobre índice `idx_config_snapshot_resource_time`.
- Costos agregados por mes: no se implementan (el frontend agrega sobre filas diarias); `GROUP BY usage_date` es la única agregación necesaria y es portable.

## Fuera de alcance (explícito)

- Cálculo de percentiles, features, snapshots `feat.*`, datasets y vistas de entrenamiento → módulo 5 (Analítica).
- Recomendaciones, escenarios, riesgo, DES → módulos 6–7 (`ml.*`, `opt.*`).
- Participaciones mensuales de costos y alertas de presupuesto.
- Particiones futuras de `raw.*`: operación existente (`raw.ensure_monthly_partitions`) sigue siendo tarea mensual manual/cron, no código Go.

## T2–T6 completados ✅

| # | Tarea | Estado | Detalle |
|---|-------|--------|---------|
| T2 | Refactor acquisition → data contracts | ✅ | `AcquisitionRepository` eliminado; acquisition ahora consume `data.DataWriter`/`RunReader`. Eliminado `acquisition/repositories/data_repository.go` (~500 líneas). |
| T3 | API lectura `/api/v1/data/*` | ✅ | 4 endpoints: `metrics`, `costs`, `config-snapshots`, `coverage` (HU-05). Permiso `data:read` en roles system/org. |
| T4 | Recolector ARM config | ✅ | `ConfigProvider` + `AzureConfigProvider` (GET `.../databases?api-version=2025-01-01`). Dedupe por `observed_at_utc`. Fake incluido. |
| T5 | Recolector Cost Management | ✅ | `CostProvider` + `AzureCostProvider` (POST `/providers/Microsoft.CostManagement/query?api-version=2026-06-01`). `source_charge_id` sintético, lookback 3 días, paginación `nextLink`. Fake incluido. |
| T6 | Docs | ✅ | `API.md` actualizado (sección 14), `.env.example` con `CCO_ACQ_COST_INTERVAL_SECONDS`, `CCO_ACQ_COST_LOOKBACK_DAYS`. |

**Tests**: 11/11 paquetes OK (acquisition, adapters, data, iam, cloud, test). Integración end-to-end:
- `TestAcquisitionStackEndToEnd` (4 fuentes: monitor + config + diag + qstore)
- `TestDataStackEndToEnd` (métricas, costos, snapshots, cobertura, 404 por recurso desconocido)

## Limitaciones conocidas (shortcuts)

1. `shortcut: calidad por fila no persistida (normalizer la descarta), cobertura derivada en lectura; upgrade: columna quality en raw.metric_reading + tabla de warnings cuando se exija auditoría por fila.`
2. `shortcut: percentiles exactos NO existen aún (requieren raw.metric_sample que nadie llena); módulo 4 no los calcula — es frontera con Analítica.`
3. `shortcut: sku fuera de catalog.sql_sku se guarda con sku_id NULL; upgrade: sincronización del catálogo.`
4. `shortcut: costos requieren rol Cost Management Reader manual en el SP (deploy), no automatizado por Terraform en esta iteración.`
5. Lag de Cost Management (8–24 h, hasta 72 h PAYG): el dashboard puede no tener el día de hoy; la cobertura de costos se comunica por la propia ausencia de filas.
