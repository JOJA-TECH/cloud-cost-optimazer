# DATA-001 — Dataset Card: Capturas de respuestas de las APIs de Azure

## Identificación

- **Nombre:** Capturas de respuestas de las APIs de Azure (laboratorio de factibilidad LAB-001)
- **Versión:** 1.0
- **Fecha:** 2026-09-06 (captura); análisis 2026-09-13
- **Responsable:** Jesús (datos/ML y laboratorios)
- **Origen:** Consultas directas a las APIs de Azure sobre la suscripción `e44b10e0-cdae-4525-b479-b9ad9454f0d1` (región `australiaeast`, RG `rg-proyecto-grado-free`, BD `proyectogrado`)
- **Licencia / condiciones de uso:** Datos del propio laboratorio académico; sin secretos. El catálogo de precios proviene de la API pública Retail Prices de Azure.

## Propósito

Servir como evidencia primaria de [LAB-001](../07-experiments/LAB-001-factibilidad-apis-azure.md): verificar la viabilidad técnica de la propuesta verificando la disponibilidad, estructura y cobertura de los datos que entregan las APIs de Azure.

## Descripción

Conjunto de respuestas crudas (respuestas originales, estructura/esquema y resumen con metadatos de consulta) de 8 fuentes de Azure, más el resumen de la carga de pruebas generada. Se organiza en dos escenarios de captura:

- `../07-experiments/laboratory-001/db-1/data/responses/` — captura de prueba sobre BD **gratuita** (nivel free limit) + `DATOS_PROVEEDORES.md`.
- `../07-experiments/laboratory-001/db-2/data/responses/` — captura sobre BD **con costo** (esquema pagado) + artefactos de factibilidad (`data/feasibility/`).

> Nota de integridad: en este snapshot, los archivos de respuesta de `db-1` y `db-2` coinciden a nivel de hash (mismos datos capturados, incluido el costo 0 de la BD gratuita). La captura de la BD con costo (escenario `db-2`) debe confirmarse/poblarse en una ejecución sobre el esquema pagado para sustentar el KPI económico.

Fuentes capturadas:

| Fuente | Archivo de respuesta | Contenido |
|---|---|---|
| Configuración SQL (ARM) | `arm_sql_resources/respuesta.json` | Configuración de BD y servidor (4 registros) |
| Costos (Cost Management) | `azure_cost_management/respuesta.json` | Costo mensual por servicio/día (11 filas) |
| Precios (Retail Prices) | `azure_retail_pricing/respuesta.json` | Catálogo de precios SQL Database (343 items) |
| Métricas (Monitor Metrics) | `azure_monitor_metrics/respuesta.json` | Métricas de BD a 1 min (5 métricas, 75 puntos) |
| Logs (Log Analytics) | `azure_monitor_logs/respuesta.json` | Métricas en logs (41 columnas, 20 filas) |
| Query Store + Watcher (DMVs) | `query_store_database_watcher/respuesta.json` | DMVs y Query Store (127 registros en 8 consultas) |
| Estado tabla telemetría | `azure_sql_estado_tabla/respuesta.json` | Estado y muestra de `dbo.carga_telemetria` (239 006 filas) |
| Azure Advisor | `advisor_recommendations/respuesta.json` | Recomendaciones (9 items) |
| Carga generada | `carga_base_de_datos/resumen_carga.json` | Resumen de carga: 3 434 ciclos, 817 417 operaciones |

## Tamaño

- Registros: 343 precios + 11 costos + 75 puntos de métricas + 20 filas de logs + 127 registros Query Store + 239 006 filas de telemetría + 4 registros ARM + 9 recomendaciones + 3 434 ciclos de carga.
- Variables: por fuente (entre 5 y 81 campos).
- Tamaño en disco: ~35 MB por directorio (`carga_base_de_datos/registros.json` ≈ 34.8 MB); capturas completas comprimidas: `responses.zip` ≈ 12.3 MB.

## Variables principales

Ver `data-dictionary` y el inventario de campos: `../07-experiments/laboratory-001/db-2/data/feasibility/inventario_campos.json`.

Campos clave por fuente: `sku`/`capacity`/`maxSizeBytes` (ARM), `PreTaxCost`/`UsageDate`/`ServiceName`/`ResourceId`/`Currency` (Cost Management), `retailPrice`/`unitOfMeasure`/`armSkuName`/`effectiveStartDate` (Retail Prices), `cpu_percent`/`storage_percent`/`sessions_percent`/`workers_percent`/`deadlock` (Métricas), `MetricName`/`Average`/`Minimum`/`Maximum` (Logs), `avg_duration`/`count_executions`/`avg_cpu_percent`/`cpu_limit` (Query Store), `id`/`evento`/`carga`/`creado_el` (telemetría), `category`/`impact`/`impactedValue` (Advisor).

## Recolección / adquisición

- Scripts `python/run_all.py` (captura) y `python/feasibility.py` (análisis), conforme a [INSTRUCCIONES_REPETICION.md](../07-experiments/laboratory-001/db-2/INSTRUCCIONES_REPETICION.md).
- Cada fuente genera `respuesta.json`, `estructura.json` y `resumen.json` con metadatos de la consulta (fecha UTC, estado, parámetros sin secretos, HTTP, errores y advertencias).

## Preprocesamiento

- Anonimización de configuraciones: se excluyen secretos (`sql_password`, `telegram_token`, `telegram_chat_id`, `authorization`).
- Análisis de factibilidad computado sobre las capturas (campos, tipos, unidades, cobertura temporal, nulos, conteos y hashes).

## Calidad

- Valores faltantes: documentados por fuente en el informe de factibilidad (p. ej., `rows[].DurationMs`, `rows[].RemoteIPLatitude/ Longitude` y `rows[].Severity` nulos en Logs; `server_resource.properties.privateEndpointConnections` vacío en ARM; `dtu_limit` nulo en Watcher).
- Duplicados: los archivos de respuesta presentes en `db-1` y `db-2` coinciden a nivel de hash en este snapshot (ver Nota de integridad en Descripción).
- Outliers: el catálogo de precios incluye SKUs de gama alta (hasta 28 787 USD/h) que son valores legítimos de la lista de precios.
- Errores conocidos: ver `../07-experiments/laboratory-001/db-2/data/feasibility/errores.json`.

## Sesgos / limitaciones

- Ventanas de captura cortas (métricas PT15M, logs 20 filas): no permiten tendencias estacionales.
- La captura registrada corresponde a la BD en nivel gratuito (`db-1`): su costo es 0, no equivale a ausencia de costo económico. El escenario de BD con costo (`db-2`) es el previsto para medir el KPI económico real en un ciclo de facturación.
- Advisor no devolvió recomendaciones de categoría `Cost` en el laboratorio.

## Privacidad y seguridad

- No se almacenan secretos ni credenciales en el repositorio.
- Configuración anonimizada: `../07-experiments/laboratory-001/db-2/data/feasibility/config_anonimizado.json`.
- Los `resource_id` y nombres de recurso son infraestructura propia del laboratorio académico.

## Transformaciones

- Sin transformaciones sobre las respuestas crudas: se conserva el JSON original (`respuesta.json`).
- Los artefactos de factibilidad (`informe_factibilidad.md`, inventario de campos, cobertura temporal, hashes) se derivan de los originales mediante `feasibility.py`.

## Integridad

- **Hash del archivo:** respuestas comprimidas: `responses.zip` sha256 `f2398e51f56e297061a360152682e9591cfe1a1922be4e86958755f611ddf472` (12 269 734 bytes). Hashes por archivo en `../07-experiments/laboratory-001/db-2/data/feasibility/hashes.json`.
- **Formato:** JSON (respuestas originales, estructura, resumen, artefactos).

## Uso

Experimentos/modelos que utilizan este dataset:

- LAB-001 — `docs/07-experiments/LAB-001-factibilidad-apis-azure.md`

## Versionado

Cambios respecto a la versión anterior: sin versiones anteriores. Nueva versión 1.0 (captura inicial del 2026-09-06).