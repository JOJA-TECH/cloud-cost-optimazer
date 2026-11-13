# ADR-012 — Módulo Analítica y Optimización (Module 5)

- **Estado:** Aceptado e implementado
- **Fecha:** 2026-11-12
- **Decisores:** Equipo del proyecto
- **Relacionado con:** ADR-001, ADR-003, ADR-004, ADR-011, MODULES.md (módulo 5)

## Contexto

El módulo Analítica y Optimización convierte métricas, costos, configuración e históricos en análisis de utilización, tendencias, patrones, capacidad y costos. Produce evidencia analítica, pero no toma por sí solo la decisión final de recomendación (esa es el Motor de Recomendaciones, Module 7).

## Problema

Definir el diseño del motor analítico en Report 1.0:

1. ¿Qué análisis se computan y cómo se exponen por API?
2. ¿Cómo se integra con el servicio ML (Module 6) sin acoplarse a Python?
3. ¿Qué ocurre si `cco_data` o el servicio ML no están disponibles?

## Alternativas consideradas

### Alternativa A: Lógica de analítica embebida en cada handler HTTP

**Desventajas:** Difícil de probar, reutilizar y mantener; mezcla transporte con lógica de negocio.

### Alternativa B: Servicio analítico con `AnalyticsEngine` + `AnalyticsResultRepository` (seleccionada)

**Ventajas**
- `AnalyticsEngine` computa evidencia (snapshots, percentiles, tendencias, patrones, capacidad, costos, DES, datasets, forecasts).
- `AnalyticsResultRepository` lee evidencia para consumidores (Module 7, dashboards, ML).
- `MLPort` es el contrato hacia el servicio ML externo (Module 6); cliente fake en desarrollo.
- Todos los endpoints requieren `analytics:read` (lectura) o `analytics:write` (cómputo).
- Sin `CCO_DATA_DSN`, responden `503 ANALYTICS_STORAGE_UNAVAILABLE`; sin ML, `503 ANALYTICS_ML_UNAVAILABLE`.

## Decisión

Se decide **Alternativa B**:

1. **Contratos (`backend/internal/analytics/interfaces.go`):**
   - `AnalyticsEngine`: `ComputeFeatureSnapshot`, `ComputePercentiles`, `ComputeTrends`, `DetectWorkloadPatterns`, `AnalyzeCapacity`, `AnalyzeCosts`, `BuildTrainingDataset`, `RequestForecast`, `ComputeDESComponents`.
   - `AnalyticsResultRepository`: lectores de `feature_snapshot`, `percentile_statistic`, `trend`, `workload_pattern`, `capacity`, `cost`, `des`, `training_dataset`, `forecasts`.
   - `MLPort`: `Forecast`, `ValidateContract` (hacia Module 6).
2. **Endpoints de API (17 REST, todos protegidos):**
   - Escritura (`analytics:write`): `POST /analytics/{snapshots,percentiles,trends,patterns,capacity,costs,des,datasets,forecasts}`.
   - Lectura (`analytics:read`): `GET /analytics/{snapshots,percentiles,trends,patterns,capacity,costs,des,datasets,forecasts}`.
3. **Integración ML:** `RequestForecast` valida el contrato (`ml.inference.v1`) y pide predicción al ML service; el ML escribe `ml.forecast` y devuelve `forecast_uid`.
4. **DES (Database Efficiency Score):** `capacity_e`, `cost_e`, `performance_e`; pesos `v1_equal` (1/3 cada uno) en Release 1.0; Module 7 aplica los pesos para la recomendación final.
5. **Portabilidad:** mismos contratos y repositorios en SQLite y PostgreSQL.

## Justificación

Separar el motor analítico del transporte HTTP permite probar la lógica de forma aislada y reutilizarla desde otros módulos. El contrato `MLPort` evita acoplar el dominio a Python o a un framework de ML concreto. La separación de "evidencia" (Module 5) y "decisión" (Module 7) cumple MODULES.md.

## Consecuencias

### Positivas

- 17 endpoints REST documentados en `API.md` §15.
- Motor analítico testeable sin depender del servicio ML (cliente fake).
- Contrato estable hacia Module 6 (ML) y Module 7 (recomendaciones).

### Negativas

- Sin `CCO_DATA_DSN` no hay datos de entrada; los endpoints responden `503`.
- El servicio ML es opcional (fake en dev); sin él, los forecasts no están disponibles.

### Riesgos

- La lógica de DES y percentiles usa shortcuts (peso igual, computación on-the-fil) documentados como tales; Module 7 debería refinarlos antes de producción.

## Evidencia

- `backend/internal/analytics/` — handlers, service, interfaces, adapters (FakeMLClient), repositories.
- `backend/cmd/cco-app/main.go` — wiring del módulo analytics.
- `API.md` §15 — referencia de endpoints de analítica.
- `docs/MODULES.md` — Módulo 5, Analítica y Optimización.

## Revisión

- **Fecha prevista de revisión:** Cuando se implemente el servicio ML real (Module 6) o el Motor de Recomendaciones (Module 7) consuma la evidencia analítica.

## Referencias

[1]: docs/MODULES.md — Módulo 5, Analítica y Optimización.
