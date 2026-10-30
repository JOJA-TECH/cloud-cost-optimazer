# RES-002 — Registro de fuentes verificadas

- **ID relacionado:** RES-002 (`../RES-002-precios-metricas-azure-sql.md`)
- **Fecha de verificación:** 2026-09-28
- **Método:** acceso directo a cada URL; contrastación con evidencia empírica interna (DATA-001) cuando existe.

## Estado de verificación

| # | Fuente | Tipo | Organismo | Verificación |
|---|---|---|---|---|
| 1 | <https://learn.microsoft.com/en-us/azure/azure-sql/database/monitoring-metrics-alerts> | Documentación oficial | Microsoft | Leída completa 2026-09-28 (última actualización 2025-02-11): catálogo de métricas con IDs, unidades y significado; advertencias de escalas no comparables y de `availability` no soportada en serverless; agregaciones disponibles |
| 2 | <https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices> | Documentación oficial | Microsoft | Accedida 2026-09-28 (URL citada previamente en EXP-001 [3] y DATA-001 [3]; endpoint verificado empíricamente en LAB-001 con 343 items) |
| 3 | <https://learn.microsoft.com/en-us/rest/api/cost-management/query/usage> | Documentación oficial | Microsoft | Accedida 2026-09-28 (URL citada previamente en EXP-001 [2] y DATA-001 [2]) |
| 4 | <https://learn.microsoft.com/en-us/azure/azure-sql/database/serverless-tier-overview> | Documentación oficial | Microsoft | Leída 2026-09-28: facturación por compute usado **por segundo** en vCore; mínimo configurable 0.5 vCores (default); auto-pausa solo en General Purpose; billing por segundo vs por hora en provisioned |
| 5 | <https://learn.microsoft.com/en-us/azure/azure-sql/database/database-watcher-data?view=azure-sql-mi> | Documentación oficial | Microsoft | Accedida 2026-09-28 (URL citada previamente en EXP-001 [4]; campos de Database watcher verificados empíricamente en DATA-001) |
| 6 | <https://learn.microsoft.com/en-us/azure/cost-management-billing/costs/understand-cost-mgt-data> | Documentación oficial | Microsoft | Accedida 2026-09-28 (URL citada previamente en LAB-001 [2]) |

## Evidencia interna contrastada

| Artefacto | Qué aporta a RES-002 |
|---|---|
| `07-experiments/LAB-001-azure-data-integration.md` | Resultado por fuente: precios (343 items, completa), métricas (5, PT15M), costos (11 filas, parcial) |
| `07-experiments/laboratory-001/db-1/DATOS_PROVEEDORES.md` | Campos reales del catálogo Retail Prices; series reales de `cpu_percent`, `storage_percent`, `sessions_percent`, `workers_percent`, `deadlock`; métricas de Log Analytics; `dm_db_resource_stats` con `cpu_limit` |
| `07-experiments/laboratory-001/db-2/informe_factibilidad.md` | Unidades por métrica (Percent/Count/µs/ms); veredicto de viabilidad; "modelado FOCUS del costo" |
| `07-experiments/laboratory-001/db-2/INSTRUCCIONES_REPETICION.md` | Endpoint y api-version reales de Retail Prices y Metrics |
| `06-data/DATA-001-azure-integration-responses.md` | Registro maestro del dataset; limitaciones (costo 0, ventana corta); reglas de transformación |
| `06-data/DATA-DICTIONARY-DATA-001.md` | Unidades, rangos y reglas de validación que sustentan los contratos JSON propuestos |

## Fuentes encontradas pero NO verificadas (excluidas de los hallazgos)

| Fuente candidata | Motivo de exclusión |
|---|---|
| Página "Elastic pool metrics" (enlazada desde [1]) | No se accedió en esta fase; el proyecto opera BD única, no pool. Pendiente si se amplía a pools |
| Precios vigentes por región en portales de terceros | Fuentes secundarias; solo se acepta la Retail Prices API oficial como fuente de precios |
