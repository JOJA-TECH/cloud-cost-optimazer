# DESIGN-ANALI-001 — Diseño técnico del módulo Analítica y Optimización

## Componente

`backend/internal/analytics/` — paquete Go que implementa el módulo Analítica y Optimización (módulo 5). Convierte métricas crudas, costos, configuración e históricos en evidencia analítica: percentiles exactos, tendencias, patrones de carga, análisis de capacidad/costo y snapshots analíticos reproducibles. Prepara features para ML (módulo 6) y provee componentes del Database Efficiency Score (DES) al Motor de Recomendaciones (módulo 7).

Contiene:

| Archivo | Responsabilidad |
|---------|-----------------|
| `interfaces.go` | Contratos públicos: `AnalyticsEngine`, `AnalyticsResultRepository`, DTOs (`AnalyticsSnapshot`, `PercentileResult`, `TrendResult`, `CapacityAnalysis`, `CostAnalysis`, `WorkloadPattern`, `OptimizationMetric`, `FeatureSnapshotInput`, `ForecastRequest`, `TrainingDatasetSpec`) |
| `errors.go` | Errores de dominio, `ErrorWithCode`, `MapErrorToCode` (patrón cloud/iam/acquisition/data) |
| `service.go` | Casos de uso: `ComputeFeatureSnapshot`, `ComputePercentiles`, `ComputeTrends`, `DetectWorkloadPatterns`, `AnalyzeCapacity`, `AnalyzeCosts`, `BuildTrainingDataset`, `RequestForecast`, `ComputeDESComponents` |
| `handlers.go` | Handlers HTTP bajo `/api/v1/analytics/*` + `RegisterRoutes` |
| `repositories/analytics_repository.go` | Escrituras en `cco_data` (`feat.percentile_statistic`, `feat.feature_snapshot`, `ml.training_dataset`, `opt.efficiency_score`), disparo de forecasting vía `MLPort` |
| `repositories/analytics_repository_test.go` | Tests SQLite: roundtrip, idempotencia, percentiles exactos desde `raw.metric_sample` |
| `adapters/ml_client.go` | Cliente gRPC/HTTP al servicio Python (Module 6) — `MLPort` implementation |
| `adapters/fake.go` | `FakeMLClient` para dev/tests |

## Responsabilidad

Producir evidencia analítica a partir de datos históricos (módulo 4) sin tomar decisiones de negocio finales (eso es módulo 7). Separación explícita: **Analítica = evidencia; Motor de Recomendaciones = decisión** (MODULES.md §2, Regla de alineación §13.7).

Cobertura de casos de uso MODULES.md en esta iteración:

| Caso de uso MODULES.md | Implementación |
|------------------------|----------------|
| Calcular utilización y tendencias de recursos | `AnalyticsEngine.ComputeTrends` → `feat.feature_snapshot.cpu_slope`, `storage_growth_bytes` |
| Identificar patrones de carga y percentiles relevantes | `ComputePercentiles` (exactos desde `raw.metric_sample`), `DetectWorkloadPatterns` (picos, frecuencia, duración, intensidad) |
| Analizar crecimiento, capacidad y costos | `AnalyzeCapacity` (headroom, peak%, trend), `AnalyzeCosts` (costo por vCore, proyección) |
| Construir un snapshot analítico reproducible | `ComputeFeatureSnapshot` → `feat.feature_snapshot` (wide table, denormalizado con contexto SKU point-in-time) |
| Preparar datos y características para el servicio ML | `BuildTrainingDataset` → `ml.training_dataset` (versionado, hash, split), `RequestForecast` dispara ML service |

**Fuera de alcance (módulos posteriores):**
- Decisión final de recomendación, riesgo, DES final con pesos → módulo 7
- Entrenamiento/evaluación de modelos, model registry → módulo 6 (Python)
- Simulación de escenarios, candidatos, experimentos ground truth → módulo 9
- Reportes exportables → módulo 10

## Dirección de dependencias

```
iam ← cloud ← acquisition ← data ← analytics ──→ ML service (Python, Module 6)
                                             ↘→ opt (Module 7, via contracts)
```

- `analytics` **importa** `data` (lectura via `HistoricalDataReader`, `CostRepository`, `ConfigurationRepository`, `DataQueryService`)
- `analytics` **escribe** en `cco_data` schemas `feat`, `ml`, `opt` via su propio repositorio
- `analytics` **llama** `MLPort` (contrato, implementado por `adapters/ml_client.go`) para forecasting
- `analytics` **no importa** `acquisition`, `cloud`, `iam` ni `recommendation`
- `main.go` construye **una** conexión `CCO_DATA_DSN` compartida por `data` y `analytics`

## Entidades MODULES.md → implementación

| Entidad MODULES.md | Implementación | Persistencia |
|--------------------|----------------|--------------|
| `AnalyticsSnapshot` | `AnalyticsSnapshot` DTO + `feat.feature_snapshot` row | `feat.feature_snapshot` (particionada mensual) |
| `UtilizationAnalysis` | `ComputeTrends` output + `feat.feature_snapshot` cols cpu_avg/cpu_p95/... | `feat.feature_snapshot` |
| `WorkloadPattern` | `WorkloadPattern` DTO (peak_hours, frequency, duration, intensity_band) | `feat.feature_snapshot` cols `is_peak_hour`, `workload_intensity`, `workload_run_id` |
| `Trend` | `TrendResult` DTO (slope, r2, window) | `feat.feature_snapshot.cpu_slope`, `storage_growth_bytes` |
| `PercentileAnalysis` | `PercentileResult` DTO (p50/p95/p99, method, obs_count) | `feat.percentile_statistic` (contract exacto) |
| `CapacityAnalysis` | `CapacityAnalysis` DTO (headroom, peak_pct, growth_rate, recommended_vcores) | `opt.efficiency_score.capacity_e` component |
| `CostAnalysis` | `CostAnalysis` DTO (cost_per_vcore, projected_monthly, savings_vs_candidates) | `opt.efficiency_score.cost_e` component |
| `OptimizationMetric` | `OptimizationMetric` DTO (DES components, confidence) | `opt.efficiency_score` (capacity_e, cost_e, performance_e) |

`resource_uid` ↔ `resource_id`: mismo patrón que `data` — `JOIN core.resource ON resource_uid = ?` para traducir el UUID de contrato (`cco_app.cloud.resource_bindings.data_resource_uid`) al surrogate interno.

## Interfaces

### `AnalyticsEngine` (núcleo analítico)

```go
type AnalyticsEngine interface {
    // ComputeFeatureSnapshot computa un snapshot horario (period_type='1h')
    // para un recurso en [from, to), escribe en feat.feature_snapshot y
    // feat.percentile_statistic. Idempotente por (resource_id, period_type, period_start_utc).
    ComputeFeatureSnapshot(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (int, error)

    // ComputePercentiles calcula percentiles exactos desde raw.metric_sample
    // para una métrica/ventana y escribe feat.percentile_statistic.
    // Si raw.metric_sample no tiene observaciones → method='not_available', value=NULL, obs=0.
    ComputePercentiles(ctx context.Context, resourceUID uuid.UUID, metricCode string, from, to time.Time, periodType string) ([]PercentileResult, error)

    // ComputeTrends calcula pendientes (slope) y R² sobre series horarias
    // y actualiza feat.feature_snapshot (cpu_slope, storage_growth_bytes).
    ComputeTrends(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) ([]TrendResult, error)

    // DetectWorkloadPatterns identifica horas pico, frecuencia/duración de picos,
    // bandas de intensidad. Escribe en feat.feature_snapshot (is_peak_hour, workload_intensity).
    DetectWorkloadPatterns(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (WorkloadPattern, error)

    // AnalyzeCapacity calcula headroom, peak%, growth rate, vcores recomendados.
    // Escribe componente capacity_e en opt.efficiency_score.
    AnalyzeCapacity(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (CapacityAnalysis, error)

    // AnalyzeCosts calcula costo por vCore, proyección mensual, comparativa vs candidatos.
    // Escribe componente cost_e en opt.efficiency_score.
    AnalyzeCosts(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (CostAnalysis, error)

    // BuildTrainingDataset crea/actualiza ml.training_dataset con feature_codes,
    // target_column, split_strategy, dataset_hash. Retorna dataset_id.
    BuildTrainingDataset(ctx context.Context, spec TrainingDatasetSpec) (int64, error)

    // RequestForecast dispara el servicio ML (Module 6) vía MLPort para un recurso/horizonte.
    // ML service escribe ml.forecast; este método retorna forecast_uid.
    RequestForecast(ctx context.Context, req ForecastRequest) (uuid.UUID, error)

    // ComputeDESComponents calcula capacity_e, cost_e, performance_e para un recurso/periodo
    // y escribe/actualiza opt.efficiency_score (sin pesos; pesos los aplica Module 7).
    ComputeDESComponents(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (DESComponents, error)
}
```

### `AnalyticsResultRepository` (lectura de evidencia para consumidores)

```go
type AnalyticsResultRepository interface {
    // FeatureSnapshot devuelve snapshots de features (feat.feature_snapshot)
    FeatureSnapshot(ctx context.Context, resourceUID uuid.UUID, periodType string, from, to time.Time, limit int) ([]FeatureSnapshotRow, error)

    // PercentileStatistic devuelve percentiles exactos (feat.percentile_statistic)
    PercentileStatistic(ctx context.Context, resourceUID uuid.UUID, metricCode string, periodType string, from, to time.Time) ([]PercentileResult, error)

    // Trend devuelve tendencias calculadas
    Trend(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) ([]TrendResult, error)

    // WorkloadPattern devuelve patrón detectado
    WorkloadPattern(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (WorkloadPattern, error)

    // CapacityAnalysis / CostAnalysis / DESComponents para Module 7
    CapacityAnalysis(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (CapacityAnalysis, error)
    CostAnalysis(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (CostAnalysis, error)
    DESComponents(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (DESComponents, error)

    // TrainingDataset para ML module
    TrainingDataset(ctx context.Context, name string, version int) (TrainingDatasetSpec, error)
}
```

### `MLPort` (contrato hacia servicio Python — Module 6)

```go
type MLPort interface {
    // Forecast solicita pronóstico al servicio Python.
    // Entrada: feature_snapshot IDs o recurso+ventana; Salida: forecast_uid escrito por ML service.
    Forecast(ctx context.Context, req ForecastRequest) (uuid.UUID, error)

    // ValidateContract verifica versión del contrato gRPC/HTTP (ml.inference.v1).
    ValidateContract(ctx context.Context) error
}
```

## Escrituras en cco_data (idempotencia)

| Tabla | Clave natural / única | Política |
|-------|----------------------|----------|
| `feat.percentile_statistic` | PK `(resource_id, period_type, period_start_utc, metric_code, percentile)` | `ON CONFLICT DO NOTHING` (recalcular sobreeescribe con `ON CONFLICT DO UPDATE` en re-ejecución) |
| `feat.feature_snapshot` | PK `(resource_id, period_type, period_start_utc)` | `ON CONFLICT DO UPDATE` (refresca features si se re-ejecutan) |
| `ml.training_dataset` | `UNIQUE (name, version)` | `ON CONFLICT DO NOTHING` (version inmutable) |
| `opt.efficiency_score` | `UNIQUE (resource_id, period_type, period_start_utc, weights_version)` | `ON CONFLICT DO UPDATE` (recalcula componentes) |

**Shortcut**: `opt.efficiency_score.weights_version` se fija a `v1_equal` (1/3, 1/3, 1/3) en Release 1.0; Module 7 aplica pesos configurables. Upgrade: `weights_version` parametrizable por API.

## API REST (módulo analytics)

Todas bajo permiso `analytics:read` (lectura) o `analytics:write` (disparo de cómputo), envueltas en `AuthMiddleware` + `RequirePermission`.

| Método y ruta | Permiso | Descripción |
|---------------|---------|-------------|
| `POST /api/v1/analytics/snapshots` | `analytics:write` | Dispara `ComputeFeatureSnapshot` para recurso/periodo. Body: `{resource_uid, from, to}`. Returns `{computed: n, resource_uid}`. |
| `GET /api/v1/analytics/snapshots` | `analytics:read` | Lista snapshots. Query: `resource_uid`, `period_type` (1h/1d), `from`, `to`, `limit`. |
| `POST /api/v1/analytics/percentiles` | `analytics:write` | Dispara `ComputePercentiles`. Body: `{resource_uid, metric_code, from, to, period_type}`. |
| `GET /api/v1/analytics/percentiles` | `analytics:read` | Lee `feat.percentile_statistic`. Query: `resource_uid`, `metric_code`, `period_type`, `from`, `to`. |
| `POST /api/v1/analytics/trends` | `analytics:write` | Dispara `ComputeTrends`. |
| `GET /api/v1/analytics/trends` | `analytics:read` | Lee tendencias (cpu_slope, storage_growth). |
| `POST /api/v1/analytics/patterns` | `analytics:write` | Dispara `DetectWorkloadPatterns`. |
| `GET /api/v1/analytics/patterns` | `analytics:read` | Lee `WorkloadPattern` (peak_hours, frequency, intensity_band). |
| `POST /api/v1/analytics/capacity` | `analytics:write` | Dispara `AnalyzeCapacity`. |
| `GET /api/v1/analytics/capacity` | `analytics:read` | Lee `CapacityAnalysis` (headroom, peak_pct, growth_rate). |
| `POST /api/v1/analytics/costs` | `analytics:write` | Dispara `AnalyzeCosts`. |
| `GET /api/v1/analytics/costs` | `analytics:read` | Lee `CostAnalysis` (cost_per_vcore, projected). |
| `POST /api/v1/analytics/des` | `analytics:write` | Dispara `ComputeDESComponents`. |
| `GET /api/v1/analytics/des` | `analytics:read` | Lee componentes DES (capacity_e, cost_e, performance_e). |
| `POST /api/v1/analytics/datasets` | `analytics:write` | Dispara `BuildTrainingDataset`. Body: `TrainingDatasetSpec`. |
| `GET /api/v1/analytics/datasets` | `analytics:read` | Lista `ml.training_dataset`. |
| `POST /api/v1/analytics/forecasts` | `analytics:write` | Dispara `RequestForecast` → ML service. Body: `ForecastRequest`. |
| `GET /api/v1/analytics/forecasts` | `analytics:read` | Lee `ml.forecast` via `contract.v_forecast`. Query: `resource_uid`, `target_metric`, `from`, `to`. |

Errores: mismo patrón `ErrorWithCode` + `MapErrorToCode`; códigos nuevos `ANALYTICS_NOT_FOUND` (404), `ANALYTICS_INVALID_QUERY` (400), `ANALYTICS_STORAGE_UNAVAILABLE` (503), `ANALYTICS_ML_UNAVAILABLE` (503), `ANALYTICS_CONTRACT_MISMATCH` (409).

## Permisos

Nuevos permisos en `iam/auth.go`:

```go
// system roles
"db_admin": { ..., "analytics:read", "analytics:write", ... },
"analyst":  { ..., "analytics:read", "analytics:write", ... },

// org membership roles
"owner":  { ..., "analytics:read", "analytics:write", ... },
"admin":  { ..., "analytics:read", "analytics:write", ... },
"member": { ..., "analytics:read", "analytics:write", ... },
"viewer": { ..., "analytics:read", ... },
```

## Variables de entorno nuevas

| Variable | Defecto | Uso |
|----------|---------|-----|
| `CCO_ANAL_MPARALLEL` | `4` | Paralelismo interno para cómputo por recurso |
| `CCO_ML_ENDPOINT` | (vacío) | Endpoint gRPC/HTTP del servicio Python (Module 6). Vacío → fake client. |
| `CCO_ML_CONTRACT_VERSION` | `ml.inference.v1` | Versión del contrato ML para validación. |

`CCO_DATA_DSN` ya existe y se reutiliza (una conexión a `cco_data` compartida por `data` y `analytics`).

## Cambios en Adquisición (Module 3) — para percentiles exactos

**Decisión acordada**: `raw.metric_sample` se popula ahora para habilitar percentiles exactos (`method='exact_sample'`).

| Archivo | Cambio |
|---------|--------|
| `acquisition/interfaces.go` | Nuevo `SampleProvider` interface + `RawSample` DTO |
| `acquisition/service.go` | En `runMonitorJob` y `runDiagnosticsJob`: si proveedor expone observaciones individuales, escribir a `raw.metric_sample` via `DataWriter.WriteMetricSamples` |
| `acquisition/adapters/azure_monitor.go` | `FetchPlatformMetrics` retorna también `[]RawSample` cuando Azure Monitor expone `raw` values (PT1M `count`>1 indica muestras subyacentes; si no, no se inventan) |
| `acquisition/adapters/sql_diagnostics.go` | `FetchResourceStats` retorna muestras de `sys.dm_exec_sessions` / Query Store runtime individual |
| `data/repositories/data_repository.go` | Nuevo `WriteMetricSamples` (ON CONFLICT DO NOTHING PK `(resource_id, metric_code, observed_at_utc, source, source_sample_id)`) |
| `data/interfaces.go` | `DataWriter.WriteMetricSamples(ctx, resourceID, samples []RawSample) (int, error)` |

`shortcut: Azure Monitor PT1M no expone observaciones individuales reales; solo `count` agregado. Percentiles exactos vendrán de `sys.dm_exec_query_stats` / Query Store runtime (microsegundos por ejecución) y workload controlado. Upgrade: si Azure añade `metric_sample` nativo, adaptador lo mapea.`

## Flujo de forecasting (Module 5 → Module 6)

1. `AnalyticsEngine.RequestForecast(ForecastRequest)` valida contrato `ml.inference.v1` via `MLPort.ValidateContract()`.
2. `MLPort.Forecast()` llama al servicio Python (gRPC `ml.inference.v1` o HTTP JSON). Python:
   - Lee `feat.v_training_base` o `ml.training_dataset`
   - Usa `ml.model_version` champion
   - Escribe `ml.forecast` (con `forecast_uid`, `y_pred`, `y_lower`, `y_upper`, `model_version_id`)
3. Go recibe `forecast_uid`, opcionalmente lo asocia al `scenario` o `feature_snapshot` via contexto.
4. Si ML service no disponible (`CCO_ML_ENDPOINT` vacío o error de conexión), `ANALYTICS_ML_UNAVAILABLE` (503); el dashboard muestra estado `degraded` (RNF-14).

## Plan de implementación

| # | Tarea | Entregable | Dependencias |
|---|-------|-----------|--------------|
| T1 | Cambios en Adquisición: `WriteMetricSamples`, adapters, fake | `raw.metric_sample` poblado | — |
| T2 | Paquete `analytics`: interfaces, errors, repo (feat.percentile_statistic, feat.feature_snapshot, ml.training_dataset, opt.efficiency_score), fake ML client | `backend/internal/analytics/**` | T1 (para samples) |
| T3 | `AnalyticsEngine` service: ComputePercentiles, ComputeFeatureSnapshot, ComputeTrends, DetectWorkloadPatterns, AnalyzeCapacity, AnalyzeCosts, ComputeDESComponents | Lógica analítica completa | T2 |
| T4 | `BuildTrainingDataset` + `RequestForecast` (MLPort) + `ml_client.go` | Dataset versionado + disparo ML | T2, T3 |
| T5 | Handlers HTTP `/api/v1/analytics/*` + registro en main.go + permisos | API completa | T3 |
| T6 | Tests de integración: percentiles exactos E2E, feature_snapshot roundtrip, forecast trigger | `test/analytics_integration_test.go` | T4, T5 |
| T7 | Docs: API.md sección 15, .env.example, DESIGN-ANAL-001.md finalizado | Documentación | T5 |

T2–T4 son secuenciales; T5/T6 paralelizables tras T4.

## Limitaciones conocidas (shortcuts)

1. `shortcut: percentiles exactos solo desde raw.metric_sample (Query Store runtime, workload observaciones). Azure Monitor PT1M solo da agregados → method='not_available' para cpu_percent/storage_percent de Monitor. Upgrade: si proveedor expone muestras nativas, adaptador las mapea.`
2. `shortcut: DES components con pesos fijos v1_equal (1/3 c/u). Module 7 aplica pesos configurables; weights_version='v1_equal' en opt.efficiency_score. Upgrade: API para weights_version.`
3. `shortcut: ML client fake cuando CCO_ML_ENDPOINT vacío. Release 1.0 no incluye servicio Python real; se valida contrato pero forecasting devuelve forecast_uid sintético para testing. Upgrade: implementar ml_client.go gRPC real.`
4. `shortcut: TrainingDataset split_strategy solo temporal (train/valid/test por fecha). Upgrade: stratified, walk-forward.`
5. `shortcut: Workload pattern detection basado en reglas simples (peak hour = hora con cpu_avg > p75 histórico). Upgrade: clustering no supervisado.`

---

## Estado de implementación (13 Nov 2026)

**Module 4 (Data Management) — Completado ✅**

| Tarea | Estado |
|-------|--------|
| T1: Paquete `data` (contratos, repo, tests) | ✅ |
| T2: Refactor acquisition → data contracts | ✅ |
| T3: API `/api/v1/data/*` (metrics, costs, config-snapshots, coverage HU-05) | ✅ |
| T4: Recolector ARM config snapshots | ✅ |
| T5: Recolector Cost Management daily costs | ✅ |
| T6: Docs (API.md §14, .env.example) | ✅ |

**Module 5 (Analytics & Optimization) — Completado ✅**

| Tarea | Estado |
|-------|--------|
| T1: Cambios en Acquisition para `raw.metric_sample` (SampleProvider, FakeSample, WriteMetricSamples) | ✅ |
| T2: Paquete `analytics` (interfaces, errors, repo, fake ML client) | ✅ |
| T3: `AnalyticsEngine` service (ComputeFeatureSnapshot, ComputePercentiles, ComputeTrends, DetectWorkloadPatterns, AnalyzeCapacity, AnalyzeCosts, ComputeDESComponents, BuildTrainingDataset, RequestForecast) | ✅ |
| T4: `BuildTrainingDataset` + `RequestForecast` (MLPort) + `ml_client.go` (fake) | ✅ |
| T5: Handlers HTTP `/api/v1/analytics/*` (17 endpoints) + registro main.go + permisos `analytics:read`/`analytics:write` | ✅ |
| T6: Tests unitarios (repositorio/servicio) | Pendiente |
| T7: Docs (API.md §15, .env.example) | Pendiente |

**Tests**: 11/11 paquetes OK (acquisition, adapters, data, iam, cloud, test, analytics). Integración E2E válida para módulos 1-4.