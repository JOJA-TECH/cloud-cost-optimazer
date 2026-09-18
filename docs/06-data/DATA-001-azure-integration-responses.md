# DATA-001 — Respuestas de integración y carga experimental de Azure SQL

## Identificación

- **Nombre:** Respuestas de integración y carga experimental de Azure SQL
- **Versión:** 1.0-inicial
- **Fecha:** 2026-09-06
- **Responsable:** Equipo del proyecto
- **Origen:** Consultas realizadas sobre un recurso Azure SQL de laboratorio y archivos JSON generados durante la carga de datos
- **Licencia / condiciones de uso:** Datos internos del proyecto. No redistribuir credenciales, identificadores sensibles ni endpoints sin revisión.

## Propósito

El dataset se utiliza para verificar la disponibilidad de fuentes, documentar la estructura inicial de los datos y orientar el diseño del pipeline histórico y del experimento de optimización.

## Descripción

El paquete contiene respuestas de configuración de recursos, costos, precios, métricas, logs, recomendaciones externas, Query Store, Database watcher, estado de una tabla y registros de una carga sintética. No constituye todavía un dataset de benchmark ni un conjunto etiquetado de workloads comparables.

Las capturas se organizan en dos escenarios en `07-experiments/laboratory-001/`:

- `db-1/` — captura de prueba sobre base de datos **gratuita** (nivel free limit) + `DATOS_PROVEEDORES.md`.
- `db-2/` — captura sobre base de datos **con costo** (esquema pagado), prevista para medir el KPI económico + artefactos de factibilidad (informe, instrucciones de repetición y `data/feasibility/`).

> Nota de integridad: en este snapshot, los archivos de respuesta de `db-1` y `db-2` coinciden a nivel de hash (mismos datos capturados, incluido el costo 0 de la BD gratuita). La captura de la BD con costo (escenario `db-2`) debe confirmarse/poblarse en una ejecución sobre el esquema pagado para sustentar el KPI económico.

## Tamaño

| Elemento | Tamaño observado |
|---|---:|
| Paquete comprimido | 12.269.734 bytes |
| Archivos | 35 |
| Categorías principales | 9 |
| Filas persistidas en la tabla de carga | 239.006 |
| Filas de costos | 11 |
| Elementos de precios | 343 |
| Filas de métricas de Azure Monitor | 5 series, 15 puntos por serie |
| Filas de Log Analytics | 20 |
| Observaciones de Database watcher | 50 |

## Variables principales

Las variables se describen en `06-data/DATA-DICTIONARY-DATA-001.md`.

## Recolección / adquisición

Las respuestas fueron obtenidas durante una ejecución de integración en un recurso Azure SQL de laboratorio ubicado en Australia East. La configuración observada correspondió a General Purpose, Gen5 y serverless, con 2 vCores configurados y capacidad mínima de 0,5 vCore. El nivel gratuito estaba activo.

## Preprocesamiento

Las respuestas fueron conservadas en JSON. Para el análisis se revisaron claves, tipos, tamaños, ventanas temporales, cantidades y relaciones entre campos. No se aplicaron imputaciones, eliminación de duplicados ni transformaciones estadísticas sobre el contenido original.

## Calidad

| Aspecto | Evaluación |
|---|---|
| Valores faltantes | Presentes en campos de logs que no aplican a todas las filas |
| Duplicados | No auditados de forma completa |
| Outliers | No evaluados estadísticamente |
| Cobertura temporal | Insuficiente para forecasting y estacionalidad |
| Costos | Costo SQL igual a cero; posible distorsión por nivel gratuito |
| Granularidad | Métricas agregadas por minuto; no suficientes para todos los percentiles |
| Etiquetado experimental | Incompleto; la carga no identifica formalmente todas las clases de workload |

## Sesgos / limitaciones

El dataset representa un único recurso de laboratorio, una región y una ventana temporal corta. La carga es sintética y no representa necesariamente un workload productivo. El catálogo de precios mezcla productos y modalidades. Azure Advisor no generó una recomendación comparable de rightsizing de Azure SQL. El costo facturado no representa necesariamente el costo normal sin beneficios gratuitos.

## Privacidad y seguridad

El dataset contiene identificadores de suscripción, nombres de recursos, FQDN y nombres de usuario de laboratorio. Antes de publicar el dataset se deben anonimizar esos campos y revisar que no existan secretos, tokens, credenciales ni datos personales. El acceso público del servidor debe tratarse como una configuración temporal de laboratorio.

## Transformaciones

Para análisis posteriores se deberán normalizar fechas a UTC, convertir filas tabulares en registros con nombres de columna, separar precio de consumo y reserva, identificar la modalidad serverless o provisioned, y asociar cada observación con una configuración y un identificador de experimento.

## Integridad

- **Hash del archivo de entrada:** `f2398e51f56e297061a360152682e9591cfe1a1922be4e86958755f611ddf472`
- **Formato:** ZIP con archivos JSON
- **Nombre del artefacto:** `responses.zip`

## Uso

Experimentos que utilizan este dataset:

- EXP-001
- LAB-001

## Versionado

Cambios respecto a la versión anterior: primera versión registrada. La siguiente versión debe incluir histórico extendido, etiquetas de workload, configuración por ejecución, métricas de latencia y throughput, errores, SLA y un contrato normalizado de costos y precios.

## Referencias

[1]: https://learn.microsoft.com/en-us/rest/api/monitor/metrics/list "Microsoft Learn: Metrics - List"
[2]: https://learn.microsoft.com/en-us/rest/api/cost-management/query/usage "Microsoft Learn: Cost Management Query Usage API"
[3]: https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices "Microsoft Learn: Azure Retail Prices API"
