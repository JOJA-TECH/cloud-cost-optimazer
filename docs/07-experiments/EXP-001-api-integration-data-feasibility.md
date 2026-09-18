# EXP-001 — Prueba de integración y factibilidad de datos para Azure SQL Cost Optimizer

- **Tipo:** Experimento exploratorio de integración y calidad de datos
- **Fecha de ejecución:** 2026-09-06
- **Responsable:** Equipo del proyecto
- **Estado:** Ejecutado; requiere repetición para validación histórica
- **Hipótesis:** Las fuentes de Azure seleccionadas permiten construir la base de datos de configuración, métricas, costos, precios y telemetría necesaria para una futura recomendación de capacidad.
- **Relacionado con:** DATA-001, LAB-001, DEC-020, RSK-002, RSK-003

## Objetivo

Comprobar la conectividad, la estructura de respuesta y la disponibilidad inicial de las fuentes necesarias para el proyecto Cloud Database Cost Optimizer. El experimento también busca identificar limitaciones que afecten el diseño del dataset, el modelo de costos y la validación posterior.

## Diseño experimental

Se ejecutaron consultas de integración sobre un recurso Azure SQL Database de laboratorio. Las respuestas se conservaron en un paquete de evidencia y se analizaron por fuente. La evaluación se centró en existencia de datos, estructura, cobertura temporal, granularidad, posibilidad de relacionamiento y utilidad para los objetivos de Release 1.0.

Este experimento **no** compara configuraciones candidatas y **no** demuestra todavía ahorro real ni superioridad de un modelo de forecasting.

## Variables

### Variables observadas

| Grupo | Variables observadas |
|---|---|
| Configuración | Región, modalidad, tier, familia, capacidad, auto-pause, almacenamiento y estado |
| Costos | Costo antes de impuestos, fecha de uso, servicio, grupo de recursos y moneda |
| Precios | Producto, SKU, medidor, unidad, región, precio, tipo de precio y fecha de vigencia |
| Métricas | CPU, almacenamiento, sesiones, workers y deadlocks |
| Logs | Nombre de métrica, promedio, mínimo, máximo, unidad y granularidad temporal |
| Consultas | Ejecuciones, duración, CPU, lecturas, escrituras y esperas |
| Dataset de carga | Registros, eventos, carga numérica, descripción y fecha de creación |

### Variables de control

El recurso, la región, el periodo de consulta, el grupo de recursos y el entorno de laboratorio fueron los elementos de control de esta ejecución. La ventana de observación no fue suficientemente larga para controlar estacionalidad semanal.

## Dataset

El experimento usa el paquete `responses.zip`, registrado como **DATA-001**. El paquete contiene respuestas JSON de configuración, costos, precios, métricas, logs, recomendaciones, Query Store, Database watcher y una carga sintética persistida en Azure SQL.

## Baseline

No se ejecutó un baseline de rightsizing en esta fase. Azure Advisor se consultó únicamente como referencia externa. Sus resultados no fueron comparables con una recomendación específica de capacidad para Azure SQL Database.

## Configuración

| Elemento | Valor observado |
|---|---|
| Servicio | Azure SQL Database |
| Región | Australia East |
| Modalidad observada | General Purpose, Gen5, serverless |
| Capacidad configurada | 2 vCores |
| Capacidad mínima | 0,5 vCore |
| Pausa automática | 60 minutos |
| Nivel gratuito | Activado; comportamiento de agotamiento: auto-pause |
| Estado | Online |
| Acceso de red | Público habilitado durante el laboratorio |
| TLS mínimo | 1.2 |

## Procedimiento

1. Consultar la configuración del servidor, la base de datos y la lista de bases existentes.
2. Consultar costos agregados por fecha, servicio, grupo de recursos y moneda.
3. Consultar precios regionales de Azure SQL.
4. Consultar métricas de Azure Monitor en intervalos de un minuto.
5. Consultar registros de métricas mediante Log Analytics.
6. Consultar datos de Query Store, estadísticas DMV, esperas, sesiones y Database watcher.
7. Verificar la tabla de carga y su cantidad de filas.
8. Analizar cobertura, granularidad, errores y campos disponibles.
9. Registrar las limitaciones que deben corregirse antes del experimento comparativo.

## Métricas de evaluación

| Métrica | Resultado observado | Interpretación |
|---|---:|---|
| Fuentes principales consultadas | 7 grupos | Cobertura inicial satisfactoria |
| Elementos de precios recuperados | 343 | Disponible; requiere filtrado por escenario |
| Métricas de Azure Monitor | 5 | Disponible en una ventana de 15 minutos |
| Filas de Log Analytics | 20 | Disponible en una ventana aproximada de 7 minutos |
| Filas de carga persistidas | 239.006 | Evidencia de ingestión; no equivale a dataset experimental etiquetado |
| Filas de costos | 11 | Histórico corto |
| Costo registrado de SQL Database | USD 0 | No utilizable como único ground truth por nivel gratuito |
| Recomendaciones de Advisor | 9 | No comparables como baseline de rightsizing SQL |
| Observaciones de Database watcher | 50 | Útiles como prueba de acceso; insuficientes como histórico |

## Resultados

La integración fue exitosa para configuración, precios, métricas, logs, Query Store, Database watcher, Advisor y estado de tabla. Se verificó que la fuente de configuración permite identificar el recurso y su modalidad. La fuente de precios contiene candidatos suficientes para construir un catálogo, pero mezcla modalidades, tiers, familias, reservas y componentes de cobro.

La CPU observada por Azure Monitor tuvo un promedio aproximado de 27,73 % durante la ventana consultada y máximos de hasta 36 %. El almacenamiento y las sesiones aparecieron en cero en esa respuesta; workers se mantuvieron en 1 y no se observaron deadlocks. Estos valores son útiles para validar el pipeline, pero no permiten calcular comportamiento histórico ni forecasting confiable.

Cost Management devolvió costos de Storage, Virtual Network y SQL Database. La fila de SQL Database presentó un costo de USD 0, por lo cual se debe separar costo facturado, precio de lista y costo contrafactual.

## Análisis

El experimento confirma la factibilidad técnica de la arquitectura de adquisición. No confirma todavía la validez de la hipótesis de optimización. La principal brecha no es la ausencia de APIs, sino la falta de suficiente duración, granularidad, etiquetado experimental y costo económico no distorsionado por beneficios gratuitos.

La carga sintética de 239.006 filas demuestra que la persistencia funciona. Sin embargo, sus registros no incluyen por sí solos una etiqueta formal de escenario, una configuración activa, una medición de latencia de cliente, throughput, errores, SLA ni una división temporal de entrenamiento y prueba.

## Conclusión

La hipótesis de integración se acepta. Las fuentes principales pueden formar la base del sistema. La hipótesis académica de ahorro con preservación de rendimiento queda **pendiente de validación**.

El siguiente experimento debe recolectar histórico durante una ventana definida, etiquetar workloads y comparar al menos dos configuraciones bajo condiciones controladas. El ahorro debe calcularse inicialmente mediante costo contrafactual basado en precios de lista cuando el costo facturado esté afectado por el nivel gratuito.

## Limitaciones

La ventana de métricas es corta. El histórico de costos es corto y presenta costo cero para SQL Database. No se ejecutaron configuraciones candidatas. No se obtuvo un baseline comparable de Azure Advisor. No se calcularon p95 o p99 de latencia a partir de observaciones de cliente. El acceso público estuvo habilitado en el laboratorio y requiere hardening.

## Artefactos y hashes

| Artefacto | Descripción | SHA-256 |
|---|---|---|
| `responses.zip` | Respuestas JSON y carga experimental | `f2398e51f56e297061a360152682e9591cfe1a1922be4e86958755f611ddf472` |
| `plan_cloud_database_cost_optimizer(1).pdf` | Plan usado como línea base | `432a0be5d3681f952694dbccff35840395375097b982a5d94aedb53a0901b8e8` |

## Reproducibilidad

Para repetir el experimento se deben consultar las mismas familias de fuentes sobre un recurso de laboratorio equivalente, conservar las respuestas sin modificar, registrar la fecha y la ventana temporal de cada consulta, y calcular el hash del paquete resultante. La repetición debe documentar también el estado del nivel gratuito, la región, el tier, la capacidad y la configuración de auto-pause.

## Documentos relacionados

- `07-experiments/LAB-001-azure-data-integration.md`
- `06-data/DATA-001-azure-integration-responses.md`
- `06-data/DATA-DICTIONARY-DATA-001.md`
- `07-experiments/results/EXP-001-results.md`
- `registry/EXPERIMENT-INDEX.md`
- `registry/DATA-REGISTRY.md`
- `registry/DECISION-LOG.md`
- `01-governance/RISK-REGISTER.md`

## Referencias

[1]: https://learn.microsoft.com/en-us/azure/azure-sql/database/monitoring-metrics-alerts "Microsoft Learn: Monitor Azure SQL Database with metrics and alerts"
[2]: https://learn.microsoft.com/en-us/rest/api/cost-management/query/usage "Microsoft Learn: Cost Management Query Usage API"
[3]: https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices "Microsoft Learn: Azure Retail Prices API"
[4]: https://learn.microsoft.com/en-us/azure/azure-sql/database/database-watcher-data?view=azure-sql-mi "Microsoft Learn: Database watcher data collection and datasets"
