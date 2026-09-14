# Informe de factibilidad - Cloud Database Cost Optimizer

> Generado: `2026-09-13T19:34:00.682775+00:00` UTC.

## Alcance del laboratorio

Este laboratorio solo **comprueba si las APIs y fuentes de Azure entregan los datos** necesarios para que la propuesta sea tecnicamente viable. **No implementa** el sistema final de optimizacion: no hay motor de recomendaciones, forecast, baselines, comparacion automatica, autoescalamiento ni cambios sobre recursos. No se modifican recursos productivos ni se almacenan secretos.

## Metodologia

Para cada fuente se ejecuto al menos una consulta real. Se guardo la respuesta original (`respuesta.json`), su estructura (`estructura.json`) y los metadatos de la consulta (`resumen.json`: fecha UTC, estado, parametros sin secretos, HTTP, errores y advertencias). Sobre esas capturas, este informe computa campos, tipos, unidades, cobertura temporal, nulos, conteos y hashes.

## Evaluacion por fuente

### Configuracion SQL (ARM / Azure Resource Manager)  (`arm_sql_resources`)

| # | Campo | Valor |
|---|-------|-------|
| 1 | Nombre de la fuente | Configuracion SQL (ARM / Azure Resource Manager) |
| 2 | Objetivo de la consulta | Configuracion de servidor y base de datos SQL via ARM. |
| 3 | Estado de la consulta | ok (inferido: captura previa sin metadatos HTTP) |
| 4 | Codigo o estado de respuesta | no registrado |
| 5 | Fecha y hora de consulta (UTC) | 2026-09-06T02:26:29.873222+00:00 |
| 6 | Recurso consultado | no registrado |
| 7 | Periodo cubierto | inicio=2026-09-05T22:09:58.490000+00:00, fin=2026-09-05T22:09:58.490000+00:00 (origen: creationDate (database_resource)) |
| 8 | Cantidad de registros | 4 (detalle: {"database_resource": 1, "server_resource": 1, "databases_list": 2}) |
| 9 | Campos disponibles | 81 campos (tipos: booleano, entero, numero, texto) |
| 10 | Unidades disponibles | {"capacidad": "vCores (sku.capacity)", "storage": "bytes (maxSizeBytes, maxLogSizeBytes)", "tls": "version de TLS (minimalTlsVersion)", "autopause": "minutos (autoPauseDelay)"} |
| 11 | Datos faltantes (nulos/vacios) | [{"array": "server_resource.properties.privateEndpointConnections", "veces": 1, "veces_vacio": 1}] |
| 12 | Ejemplo anonimizado | `{"database": {"name": "proyectogrado", "sku": {"name": "GP_S_Gen5", "tier": "GeneralPurpose", "family": "Gen5", "capacity": 2}, "tier": "GP_S_Gen5_2"}, "server": {"name": "sql-cuddly-moray", "version": "12.0"}}` |
| 13 | Utilidad para el proyecto | Configuracion completa del recurso y del servidor: region, estado, SKU/tier/familia/capacidad, serverless, auto-pause, minCapacity, tamanos de storage y log, TLS minimo, acceso de red, redundancia, free limit y listado de bases. |
| 14 | Limitaciones | Es una snapshot puntual: no tiene historico y no trae metricas ni costo. No informa uso en el tiempo. |
| 15 | Recomendacion | **suficiente** - La API entrega datos suficientes para validar experimentalmente la propuesta. |


---

### Costos (Azure Cost Management)  (`azure_cost_management`)

| # | Campo | Valor |
|---|-------|-------|
| 1 | Nombre de la fuente | Costos (Azure Cost Management) |
| 2 | Objetivo de la consulta | Costos registrados de la suscripcion. |
| 3 | Estado de la consulta | ok (inferido: captura previa sin metadatos HTTP) |
| 4 | Codigo o estado de respuesta | no registrado |
| 5 | Fecha y hora de consulta (UTC) | 2026-09-06T02:26:19.098824+00:00 |
| 6 | Recurso consultado | no registrado |
| 7 | Periodo cubierto | inicio=2026-09-01T00:00:00+00:00, fin=2026-09-05T00:00:00+00:00 (origen: UsageDate (rows)) |
| 8 | Cantidad de registros | 11 (detalle: {"columnas": 5}) |
| 9 | Campos disponibles | 5 campos (tipos: numero, texto) |
| 10 | Unidades disponibles | {"Currency": ["USD"]} |
| 11 | Datos faltantes (nulos/vacios) | sin nulos/vacios detectados |
| 12 | Ejemplo anonimizado | `{"columnas": ["PreTaxCost", "UsageDate", "ServiceName", "ResourceGroup", "Currency"], "primera_fila": [0.1600161336, 20260901, "Storage", "pruebas", "USD"]}` |
| 13 | Utilidad para el proyecto | Costo diario por servicio, recurso (ResourceId), grupo de recursos, tipo de cargo (ChargeType) y moneda. Con ResourceId y ChargeType deja de ser solo un total por servicio. |
| 14 | Limitaciones | En el laboratorio gratuito el recurso SQL aparece en 0 (nivel gratuito/capacidad minima): el costo cero no equivale a ausencia de costo economico. Datos con rezago y agregacion diaria; falta historial del mes. No entrega componentes desglosados por meter del recurso. |
| 15 | Recomendacion | **parcialmente suficiente** - La API entrega el campo requerido, pero aun no datos suficientes para el analisis. |


---

### Precios (Azure Retail Prices)  (`azure_retail_pricing`)

| # | Campo | Valor |
|---|-------|-------|
| 1 | Nombre de la fuente | Precios (Azure Retail Prices) |
| 2 | Objetivo de la consulta | Precios de referencia de SQL Database en la region. |
| 3 | Estado de la consulta | ok (inferido: captura previa sin metadatos HTTP) |
| 4 | Codigo o estado de respuesta | no registrado |
| 5 | Fecha y hora de consulta (UTC) | 2026-09-06T02:26:13.362142+00:00 |
| 6 | Recurso consultado | no registrado |
| 7 | Periodo cubierto | inicio=2014-11-01T00:00:00+00:00, fin=2025-10-01T00:00:00+00:00 (origen: effectiveStartDate (Items)) |
| 8 | Cantidad de registros | 343 (detalle: {"paginas": 1, "max_paginas_alcanzado": false}) |
| 9 | Campos disponibles | 25 campos (tipos: booleano, entero, null, numero, texto) |
| 10 | Unidades disponibles | {"unidades_de_cobro": ["1 GB/Month", "1 Hour", "1/Day", "1M"], "moneda": ["USD"]} |
| 11 | Datos faltantes (nulos/vacios) | [{"campo": "NextPageLink", "tipo": "null", "veces_presente": 1, "veces_nulo": 1}] |
| 12 | Ejemplo anonimizado | `{"currencyCode": "USD", "tierMinimumUnits": 0.0, "retailPrice": 14.49104, "unitPrice": 14.49104, "armRegionName": "australiaeast", "location": "AU East", "effectiveStartDate": "2025-06-01T00:00:00Z", "meterId": "2d6ad12e-49dd-4279-9c6b-e0cd904957ab", "meterName": "vCore", "productId": "DZH318Z0BPXL", "skuId": "DZH318Z0BPXL/010Q", "productName": "SQL Database Single/Elastic Pool General Purpose - C...[truncado]` |
| 13 | Utilidad para el proyecto | Catalogo de precios de lista (retail) de SQL Database con SKU, region, capacidad, unidad de cobro, precio, tipo de precio y vigencia. Es la referencia para cotizar candidatos sin tocar recursos. |
| 14 | Limitaciones | Mezcla modalidades, tiers, reservas y componentes en un mismo catalogo; no hay un 'solo precio' por configuracion. Falta precio contractual/efectivo real de la suscripcion. No trae costos historicos. |
| 15 | Recomendacion | **suficiente** - La API entrega datos suficientes para validar experimentalmente la propuesta. |


---

### Metricas (Azure Monitor / Metrics)  (`azure_monitor_metrics`)

| # | Campo | Valor |
|---|-------|-------|
| 1 | Nombre de la fuente | Metricas (Azure Monitor / Metrics) |
| 2 | Objetivo de la consulta | Metricas de rendimiento de la base de datos. |
| 3 | Estado de la consulta | ok (inferido: captura previa sin metadatos HTTP) |
| 4 | Codigo o estado de respuesta | no registrado |
| 5 | Fecha y hora de consulta (UTC) | 2026-09-06T02:26:24.310525+00:00 |
| 6 | Recurso consultado | no registrado |
| 7 | Periodo cubierto | inicio=2026-09-06T02:10:00+00:00, fin=2026-09-06T02:24:00+00:00 (origen: timeStamp (timeseries) + timespan=2026-09-06T02:10:25Z/2026-09-06T02:25:25Z) |
| 8 | Cantidad de registros | 5 (detalle: {"puntos_por_metrica": {"cpu_percent": 15, "storage_percent": 15, "sessions_percent": 15, "workers_percent": 15, "deadlock": 15}, "puntos_totales": 75}) |
| 9 | Campos disponibles | 16 campos (tipos: entero, numero, texto) |
| 10 | Unidades disponibles | {"por_metrica": [{"metrica": "cpu_percent", "unidad": "Percent"}, {"metrica": "storage_percent", "unidad": "Percent"}, {"metrica": "sessions_percent", "unidad": "Percent"}, {"metrica": "workers_percent", "unidad": "Percent"}, {"metrica": "deadlock", "unidad": "Count"}]} |
| 11 | Datos faltantes (nulos/vacios) | [{"array": "value[].timeseries[].metadatavalues", "veces": 1, "veces_vacio": 1}] |
| 12 | Ejemplo anonimizado | `{"nombre": "cpu_percent", "unidad": "Percent", "primer_punto": {"timeStamp": "2026-09-06T02:10:00Z", "average": 26.25, "minimum": 21, "maximum": 31}, "ultimo_timestamp": {"timeStamp": "2026-09-06T02:24:00Z", "average": 28, "minimum": 25, "maximum": 31}}` |
| 13 | Utilidad para el proyecto | CPU, storage, sesiones, workers y deadlocks con promedio, minimo y maximo a 1 min. Basico para caracterizar carga y capacidad. |
| 14 | Limitaciones | Ventana consultada corta (PT15M) y sin historico en la captura; depende de la retencion del monitoreo. No incluye memoria, latencia por consulta ni throughput transaccional. |
| 15 | Recomendacion | **suficiente** - La API entrega datos suficientes para validar experimentalmente la propuesta. |


---

### Logs (Log Analytics / Azure Monitor Logs)  (`azure_monitor_logs`)

| # | Campo | Valor |
|---|-------|-------|
| 1 | Nombre de la fuente | Logs (Log Analytics / Azure Monitor Logs) |
| 2 | Objetivo de la consulta | Logs de monitoreo en Log Analytics. |
| 3 | Estado de la consulta | ok (inferido: captura previa sin metadatos HTTP) |
| 4 | Codigo o estado de respuesta | no registrado |
| 5 | Fecha y hora de consulta (UTC) | 2026-09-06T02:26:25.184950+00:00 |
| 6 | Recurso consultado | no registrado |
| 7 | Periodo cubierto | inicio=2026-09-06T00:10:00+00:00, fin=2026-09-06T00:16:00+00:00 (origen: TimeGenerated (rows)) |
| 8 | Cantidad de registros | 20 (detalle: {"columnas": 41}) |
| 9 | Campos disponibles | 41 campos (tipos: entero, fecha_hora, numero, texto) |
| 10 | Unidades disponibles | {"UnitName": ["Percent"]} |
| 11 | Datos faltantes (nulos/vacios) | [{"campo": "rows[].DurationMs", "tipo": "entero", "veces_presente": 20, "veces_nulo": 20}, {"campo": "rows[].RemoteIPLatitude", "tipo": "numero", "veces_presente": 20, "veces_nulo": 20}, {"campo": "rows[].RemoteIPLongitude", "tipo": "numero", "veces_presente": 20, "veces_nulo": 20}, {"campo": "rows[].Severity", "tipo": "entero", "veces_presente": 20, "veces_nulo": 20}] |
| 12 | Ejemplo anonimizado | `{"columnas": 41, "primera_fila": {"TenantId": "8e247ee6-014d-4f78-866d-7a228f7e71c7", "SourceSystem": "Azure", "TimeGenerated": "2026-09-06T00:10:00Z", "ResourceId": "/SUBSCRIPTIONS/E44B10E0-CDAE-4525-B479-B9AD9454F0D1/RESOURCEGROUPS/RG-PROYECTO-GRADO-FREE/PROVIDERS/MICROSOFT.SQL/SERVERS/SQL-CUDDLY-MORAY/DATABASES/PROYECTOGRADO", "OperationName": "", "OperationVersion": "", "Category": "", "Result...[truncado]` |
| 13 | Utilidad para el proyecto | Serie de logs de AzureMetrics con promedio, minimo, maximo, unidad, granularidad e identificador de recurso (ambito de la consulta). |
| 14 | Limitaciones | La captura devolvio pocas filas con muchas columnas vacias en este laboratorio; requiere el workspace vinculado a la BD. La ventana observada es insuficiente para tendencias estacionales. |
| 15 | Recomendacion | **parcialmente suficiente** - La API entrega el campo requerido, pero aun no datos suficientes para el analisis. |


---

### Query Store + Database Watcher (DMVs)  (`query_store_database_watcher`)

| # | Campo | Valor |
|---|-------|-------|
| 1 | Nombre de la fuente | Query Store + Database Watcher (DMVs) |
| 2 | Objetivo de la consulta | Query Store, DMVs y telemetria interna. |
| 3 | Estado de la consulta | ok (inferido: captura previa sin metadatos HTTP) |
| 4 | Codigo o estado de respuesta | no registrado |
| 5 | Fecha y hora de consulta (UTC) | 2026-09-06T02:26:29.342171+00:00 |
| 6 | Recurso consultado | no registrado |
| 7 | Periodo cubierto | inicio=2026-09-05T22:09:54.703000+00:00, fin=2026-09-06T02:26:29.723000+00:00 (origen: timestamps de Query Store / DMVs) |
| 8 | Cantidad de registros | 127 (detalle: {"query_store_query_text": 10, "query_store_query": 10, "query_store_runtime_stats": 20, "query_store_wait_stats": 20, "dm_exec_query_stats": 10, "database_watcher_dm_db_resource_stats": 50, "database_watcher_sessions": 6, "carga_telemetria_resumen": 1}) |
| 9 | Campos disponibles | 48 campos (tipos: booleano, entero, null, numero, texto) |
| 10 | Unidades disponibles | {"duracion": "microsegundos (avg_duration/worker_time en sys.query_store_*)", "esperas": "milisegundos (avg_query_wait_time_ms)", "recursos": "porcentaje (avg_cpu_percent, avg_data_io_percent, ...)", "conteos": "unidades (execution_count, count_executions, count_compiles)"} |
| 11 | Datos faltantes (nulos/vacios) | [{"campo": "database_watcher_dm_db_resource_stats[].dtu_limit", "tipo": "null", "veces_presente": 1, "veces_nulo": 1}] |
| 12 | Ejemplo anonimizado | `{"query_store_runtime_stats": {"query_id": 37, "count_executions": 1, "avg_duration": 214292.0, "avg_cpu_time": 214259.0, "avg_logical_io_reads": 3.0, "avg_logical_io_writes": 0.0, "first_execution_time": "2026-09-06T02:26:29.557000", "last_execution_time": "2026-09-06T02:26:29.557000"}}` |
| 13 | Utilidad para el proyecto | Texto e identificador de consulta, cantidad de ejecuciones, duracion, CPU, I/O logico, esperas, primera/ultima ejecucion, resource stats de la instancia y sesiones activas. Sirve como telemetria adicional del lab. |
| 14 | Limitaciones | Depende de que Query Store este habilitado y de los permisos sobre la BD; captura TOP N de cada vista. Requiere historial acumulado para tendencias. |
| 15 | Recomendacion | **suficiente** - La API entrega datos suficientes para validar experimentalmente la propuesta. |


---

### Estado de datos de prueba (tabla de telemetria)  (`azure_sql_estado_tabla`)

| # | Campo | Valor |
|---|-------|-------|
| 1 | Nombre de la fuente | Estado de datos de prueba (tabla de telemetria) |
| 2 | Objetivo de la consulta | Estado y volumen de los datos de prueba. |
| 3 | Estado de la consulta | ok (inferido: captura previa sin metadatos HTTP) |
| 4 | Codigo o estado de respuesta | no registrado |
| 5 | Fecha y hora de consulta (UTC) | 2026-09-06T02:26:29.795524+00:00 |
| 6 | Recurso consultado | no registrado |
| 7 | Periodo cubierto | inicio=2026-09-06T02:25:59.729095+00:00, fin=2026-09-06T02:26:00.334028+00:00 (origen: creado_el (filas)) |
| 8 | Cantidad de registros | 239006 (detalle: {"existe": true, "filas_en_muestra": 100}) |
| 9 | Campos disponibles | 10 campos (tipos: booleano, entero, texto) |
| 10 | Unidades disponibles | {"carga": "intensidad simulada (unidades arbitrarias)"} |
| 11 | Datos faltantes (nulos/vacios) | sin nulos/vacios detectados |
| 12 | Ejemplo anonimizado | `{"total_filas": 239006, "muestra": [{"id": 476913, "evento": "evento_7lsSkV", "carga": 572, "descripcion": "IwhGwwcucxzYxo7vZAvPasZwoY07Wnj5oeiU4MXY", "creado_el": "2026-09-06T02:26:00.334028"}]}` |
| 13 | Utilidad para el proyecto | Estado y volumen de los datos de prueba: tabla existente, total de filas, esquema y muestra. Valida la generacion de workload. |
| 14 | Limitaciones | Solo describe la tabla del laboratorio; no es una fuente del sistema de produccion futuro. |
| 15 | Recomendacion | **suficiente** - La API entrega datos suficientes para validar experimentalmente la propuesta. |


---

### Azure Advisor (fuente complementaria)  (`advisor_recommendations`)

| # | Campo | Valor |
|---|-------|-------|
| 1 | Nombre de la fuente | Azure Advisor (fuente complementaria) |
| 2 | Objetivo de la consulta | Recomendaciones de Azure Advisor. |
| 3 | Estado de la consulta | ok (inferido: captura previa sin metadatos HTTP) |
| 4 | Codigo o estado de respuesta | no registrado |
| 5 | Fecha y hora de consulta (UTC) | 2026-09-06T02:26:30.640310+00:00 |
| 6 | Recurso consultado | no registrado |
| 7 | Periodo cubierto | inicio=2026-04-17T00:17:45.300897+00:00, fin=2026-09-06T00:30:47.357963+00:00 (origen: lastUpdated (recomendaciones)) |
| 8 | Cantidad de registros | 9 (detalle: {"categoria_cost": null}) |
| 9 | Campos disponibles | 15 campos (tipos: texto) |
| 10 | Unidades disponibles | {} |
| 11 | Datos faltantes (nulos/vacios) | sin nulos/vacios detectados |
| 12 | Ejemplo anonimizado | `{"properties": {"category": "HighAvailability", "impact": "High", "impactedField": "Microsoft.Subscriptions/subscriptions", "impactedValue": "e44b10e0-cdae-4525-b479-b9ad9454f0d1", "lastUpdated": "2026-05-20T19:34:01.7499612Z", "recommendationTypeId": "242639fd-cd73-4be2-8f55-70478db8d1a5", "shortDescription": {"problem": "Create an Azure Service Health alert", "solution": "Create an Azure Service...[truncado]` |
| 13 | Utilidad para el proyecto | Recomendaciones externas de Azure (categorias, impacto, recurso afectado, problema/solucion). En el lab no devuelve categoria Cost: solo HighAvailability / OperationalExcellence. |
| 14 | Limitaciones | No entrega recomendaciones de rightsizing/costo sin telemetria acumulada; no sustituye al baseline propio de la propuesta. |
| 15 | Recomendacion | **complementaria** - Solo complementa: no es necesaria para la viabilidad de la propuesta. |

---

## Tabla resumen

| Fuente | Consulta exitosa | Datos necesarios disponibles | Cobertura suficiente | Utilidad |
|---|---|---|---|---|
| Configuracion SQL (ARM / Azure Resource Manager) | Si | Completa | Si (snapshot puntual: la fuente no exige historico) | Suficiente |
| Costos (Azure Cost Management) | Si | Parcial | Si (mes en curso) | Parcial |
| Precios (Azure Retail Prices) | Si | Completa | Si (catalogo vigente; historico limitado) | Suficiente |
| Metricas (Azure Monitor / Metrics) | Si | Parcial | No (ventana consultada corta: PT15M; falta historico) | Suficiente |
| Logs (Log Analytics / Azure Monitor Logs) | Si | Parcial | No (20 filas, ventana corta para tendencias) | Parcial |
| Query Store + Database Watcher (DMVs) | Si | Completa | Parcial (historico acumulandose; pocas horas en el lab) | Suficiente |
| Estado de datos de prueba (tabla de telemetria) | Si | Completa | Si (coincide con la ventana experimental) | Suficiente |
| Azure Advisor (fuente complementaria) | Si | Parcial | No (sin categoria Cost en el lab) | Complementaria |

## Fuentes disponibles y faltantes

| Fuente | Consulta exitosa | Con datos | Datos necesarios | Cobertura suficiente | Recomendacion | Faltante predominante |
|---|---|---|---|---|---|---|
| arm_sql_resources | Si | Si | Completa | Si (snapshot puntual: la fuente no exige historico) | suficiente | server_resource.properties.privateEndpointConnections |
| azure_cost_management | Si | Si | Parcial | Si (mes en curso) | parcialmente suficiente | ninguno |
| azure_retail_pricing | Si | Si | Completa | Si (catalogo vigente; historico limitado) | suficiente | NextPageLink |
| azure_monitor_metrics | Si | Si | Parcial | No (ventana consultada corta: PT15M; falta historico) | suficiente | value[].timeseries[].metadatavalues |
| azure_monitor_logs | Si | Si | Parcial | No (20 filas, ventana corta para tendencias) | parcialmente suficiente | rows[].DurationMs, rows[].RemoteIPLatitude, rows[].RemoteIPLongitude |
| query_store_database_watcher | Si | Si | Completa | Parcial (historico acumulandose; pocas horas en el lab) | suficiente | database_watcher_dm_db_resource_stats[].dtu_limit |
| azure_sql_estado_tabla | Si | Si | Completa | Si (coincide con la ventana experimental) | suficiente | ninguno |
| advisor_recommendations | Si | Si | Parcial | No (sin categoria Cost en el lab) | complementaria | ninguno |

## Cobertura temporal

| Fuente | Inicio (UTC) | Fin (UTC) | Rango | Origen |
|---|---|---|---|---|
| Configuracion SQL (ARM / Azure Resource Manager) | 2026-09-05T22:09:58.490000+00:00 | 2026-09-05T22:09:58.490000+00:00 | 0:00:00 | creationDate (database_resource) |
| Costos (Azure Cost Management) | 2026-09-01T00:00:00+00:00 | 2026-09-05T00:00:00+00:00 | 4 days, 0:00:00 | UsageDate (rows) |
| Precios (Azure Retail Prices) | 2014-11-01T00:00:00+00:00 | 2025-10-01T00:00:00+00:00 | 3987 days, 0:00:00 | effectiveStartDate (Items) |
| Metricas (Azure Monitor / Metrics) | 2026-09-06T02:10:00+00:00 | 2026-09-06T02:24:00+00:00 | 0:14:00 | timeStamp (timeseries) + timespan=2026-09-06T02:10:25Z/2026-09-06T02:25:25Z |
| Logs (Log Analytics / Azure Monitor Logs) | 2026-09-06T00:10:00+00:00 | 2026-09-06T00:16:00+00:00 | 0:06:00 | TimeGenerated (rows) |
| Query Store + Database Watcher (DMVs) | 2026-09-05T22:09:54.703000+00:00 | 2026-09-06T02:26:29.723000+00:00 | 4:16:35.020000 | timestamps de Query Store / DMVs |
| Estado de datos de prueba (tabla de telemetria) | 2026-09-06T02:25:59.729095+00:00 | 2026-09-06T02:26:00.334028+00:00 | 0:00:00.604933 | creado_el (filas) |
| Azure Advisor (fuente complementaria) | 2026-04-17T00:17:45.300897+00:00 | 2026-09-06T00:30:47.357963+00:00 | 142 days, 0:13:02.057066 | lastUpdated (recomendaciones) |

## Conclusiones

### Datos que SI pueden obtenerse
Configuracion SQL (ARM / Azure Resource Manager), Precios (Azure Retail Prices), Metricas (Azure Monitor / Metrics), Query Store + Database Watcher (DMVs), Estado de datos de prueba (tabla de telemetria), Azure Advisor (fuente complementaria)

### Datos que se obtienen PARCIALMENTE
Costos (Azure Cost Management), Logs (Log Analytics / Azure Monitor Logs)

### Datos que NO pueden obtenerse (o no en el lab)
N/A.

### Datos suficientes para iniciar el sistema propuesto
Configuracion SQL (ARM / Azure Resource Manager), Precios (Azure Retail Prices), Metricas (Azure Monitor / Metrics), Query Store + Database Watcher (DMVs), Estado de datos de prueba (tabla de telemetria)

### Datos que requieren mayor historico
Metricas, logs y Query Store: con ventanas de 15 min / pocas filas no se observan tendencias estacionales ni picos consolidados; se requiere acumular capturas durante dias y correlacionar con los workloads.

### Datos que requieren permisos adicionales
Cost Management con granularidad por recurso ya esta disponible. Advisor categoria Cost requiere historial y RBAC de lectura; Log Analytics requiere el workspace vinculado y retencion configurada. Si una API falla por AuthorizationFailed/403, revisar el rol sobre el scope.

### Partes de la propuesta que se deben MANTENER
Baseline por configuracion (ARM), cotizacion de candidatos (Retail Prices), modelado FOCUS del costo (Cost Management con resource_id) y caracterizacion de carga con Metricas + Query Store.

### Partes de la propuesta que se deben MODIFICAR
No construir aun motor de recomendacion/forecast: las ventanas observadas son cortas y el costo de la BD gratuita es 0 (nivel gratuito), lo que impide medir un KPI economico real sobre el recurso SQL sin pasar un ciclo de facturacion con capacidad pagada.


## Veredicto de viabilidad

**La propuesta puede continuar tecnicamente de forma incremental.** La plataforma responde en todas las fuentes consultadas, y las de configuracion, precios, metricas y Query Store ya entregan campos suficientes para validar experimentalmente la propuesta en el laboratorio. No obstante, **conectar una API no equivale a validar la propuesta**: costos, logs y Advisor necesitan historial acumulado, permisos verificados y, para el KPI economico, una BD fuera del nivel gratuito durante un ciclo de facturacion.

## Anexos

- Inventario de campos: `data/feasibility/inventario_campos.json`
- Cobertura temporal: `data/feasibility/cobertura_temporal.json`
- Fuentes disponibles/faltantes: `data/feasibility/fuentes_disponibles_faltantes.json`
- Registro de errores: `data/feasibility/errores.json`
- Hash de archivos: `data/feasibility/hashes.json`
- Configuracion sin secretos: `data/feasibility/config_anonimizado.json`
- Instrucciones de repeticion: `data/feasibility/INSTRUCCIONES_REPETICION.md`
