# Datos capturados de los proveedores de Azure

Documento que describe la **función de cada proveedor**, el **significado de sus
datos** y las **tablas** con la información capturada en
`data/responses/<proveedor>/respuesta.json`.

> Captura del periodo: `2026-09-06` (ventana de carga de ~3.7 h sobre
> `sql-cuddly-moray.database.windows.net`, BD `proyectogrado`, región `australiaeast`).

---

## 1. `azure_retail_pricing` — Precios públicos de Azure

**Función:** consulta la API pública de precios minoristas de Azure
(`Retail Prices API`) filtrando los productos de **SQL Database** en la región
`australiaeast`, con paginación completa.

**Qué significan los datos:** el catálogo de SKUs/medidores (meter) de SQL
Database que Azure publica, con su precio por unidad. Es la referencia de
cuánto cuesta cada configuración antes de aplicarle el mix de la suscripción.

- 343 items de precios distribuidos en **38 productos**.
- Precio por hora, por día o por unidad según `unitOfMeasure`.
- Campos clave: `retailPrice` (precio), `meterName` (qué se mide: vCore,
  eDTUs, storage…), `armSkuName` (la SKU de ARM), `isPrimaryMeterRegion`
  (si esa región es la "primaria" del medidor).

### Tabla A — Productos más ofertados (conteo por producto y rango de precio)

| Producto | N.º precios | Rango precio (USD) |
|--------------------------------------------|----:|--------------------|
| SQL Database Single/Elastic Pool General Purpose - Compute Gen5 | 66 | 0.11 – 2 142.00 |
| SQL Database Single/Elastic Pool Business Critical - Compute Gen5 | 33 | 0.36 – 4 284.00 |
| SQL Database Single/Elastic Pool General Purpose - Compute DC-Series | 29 | 0.44 – 5 220.00 |
| SQL Database Single/Elastic Pool General Purpose - Compute FSv2 Series | 25 | 0.16 – 11.45 |
| SQL Database Single/Elastic Pool Business Critical - Compute DC-Series | 21 | 0.88 – 10 441.00 |
| SQL Database Single Premium | 18 | 12.39 – 571.41 |

### Tabla B — Los 5 precios más altos del catálogo

| Producto | SKU | Precio | Unidad |
|---------------------------------------------------|-------|----------|--------|
| SQL Database Single/Elastic Pool Business Critical - Compute M Series | vCore | 28 787.00 USD | 1 Hour |
| SQL Database Single/Elastic Pool Business Critical - Compute M Series | vCore | 13 861.00 USD | 1 Hour |
| SQL Database Single/Elastic Pool Business Critical - Compute DC-Series | vCore | 10 441.00 USD | 1 Hour |
| SQL Database Single/Elastic Pool Hyperscale - Compute DC-Series | vCore | 6 264.00 USD | 1 Hour |
| SQL Database Single/Elastic Pool General Purpose - Compute DC-Series | vCore | 5 220.00 USD | 1 Hour |

### Tabla C — Ejemplo de items de la respuesta

| productName | skuName | meterName | retailPrice | unitOfMeasure | armSkuName | isPrimaryMeterRegion |
|----------------------------------------------|------------------|-----------|-------------|---------------|---------------------|---:|
| SQL Database Single/Elastic Pool General Purpose - Compute Gen5 | 80 vCore | vCore | 14.49 USD | 1 Hour | SQLDB_GP_Compute_Gen5_80 | false |
| SQL Database Single/Elastic Pool Business Critical - Compute DC-Series | 12 vCore | vCore | 10.60 USD | 1 Hour | 12 vCore | true |
| SQL Database Elastic Pool - Premium RS | 3000 DTU Pack | eDTUs | 149.48 USD | 1/Day | *(vacío)* | false |

---

## 2. `azure_cost_management` — Costo de la suscripción

**Función:** ejecuta una consulta de **Cost Management** sobre la suscripción:
costo bruto mensual agregado por **Servicio** y **Grupo de recursos**,
desglosado por día.

**Qué significan los datos:** cuánto genera cada servicio en
`PreTaxCost` (USD) por fecha (`UsageDate`, formato `AAAAMMDD`). Permite
ver la facturación real del mes actual: Storage, Virtual Network y SQL
Database en los grupos `pruebas` y `rg-proyecto-grado-free`.

| PreTaxCost (USD) | UsageDate | ServiceName | ResourceGroup | Currency |
|-----------------:|----------:|:------------|:--------------|:---------|
| 0.160016 | 2026-09-01 | Storage | pruebas | USD |
| 0.12 | 2026-09-01 | Virtual Network | pruebas | USD |
| 0.160016 | 2026-09-02 | Storage | pruebas | USD |
| 0.12 | 2026-09-02 | Virtual Network | pruebas | USD |
| 0.160016 | 2026-09-03 | Storage | pruebas | USD |
| 0.12 | 2026-09-03 | Virtual Network | pruebas | USD |
| 0.160016 | 2026-09-04 | Storage | pruebas | USD |
| 0.12 | 2026-09-04 | Virtual Network | pruebas | USD |
| 0.0 | 2026-09-05 | SQL Database | rg-proyecto-grado-free | USD |
| 0.140014 | 2026-09-05 | Storage | pruebas | USD |
| 0.105 | 2026-09-05 | Virtual Network | pruebas | USD |

> ✔ La BD gratuita (`SQL Database`) muestra **costo 0** durante la captura.

---

## 3. `azure_monitor_metrics` — Métricas en tiempo real de la BD

**Función:** consulta las **métricas de Azure Monitor** de la BD SQL del
último cuarto de hora (15 min, granularidad `PT1M`).

**Qué significan los datos:** comportamiento de recursos de la BD durante la
ventana de carga. `cost` es el número de puntos de datos devueltos por la API;
`unit` es la unidad de la métrica; `timeseries[].data[]` es la serie temporal
con promedio/mínimo/máximo por minuto.

### Tabla A — Métricas capturadas (promedio del periodo)

| Métrica | Unidad | Descripción | Avg | Max |
|------------------|---------|----------------------------------------------|-----:|----:|
| cpu_percent | Percent | Porcentaje de CPU usada | 27.73 | 36 |
| storage_percent | Percent | Espacio de datos usado (%) | 0.00 | 0 |
| sessions_percent | Percent | Sesiones en uso (%) | 0.00 | 0 |
| workers_percent | Percent | Trabajadores (workers) en uso (%) | 1.00 | 1 |
| deadlock | Count | Deadlocks (bloqueos mutuos) | 0.00 | 0 |

### Tabla B — Serie de CPU por minuto (muestra)

| timeStamp | average | minimum | maximum |
|:----------|--------:|--------:|--------:|
| 2026-09-06T02:10:00Z | 26.25 | 21 | 31 |
| 2026-09-06T02:13:00Z | 28.00 | 26 | 29 |
| 2026-09-06T02:16:00Z | 28.00 | 21 | 35 |
| 2026-09-06T02:18:00Z | 28.75 | 23 | 36 |
| 2026-09-06T02:20:00Z | 24.75 | 19 | 29 |
| 2026-09-06T02:24:00Z | 28.00 | 25 | 31 |

---

## 4. `azure_monitor_logs` — Consulta de logs (Log Analytics)

**Función:** ejecuta una consulta **Kusto** sobre el área de AzureMetrics del
workspace (contexto-recurso de la BD) y devuelve métricas de SQL agregadas a
1 minuto. Contiene **41 columnas y 20 filas**.

**Qué significan los datos:** las métricas de la BD también registradas como
logs en el workspace: `sqlserver_process_core_percent` (CPU del proceso SQL),
`sqlserver_process_memory_percent` (memoria del proceso) y
`sql_instance_cpu_percent` (CPU a nivel de instancia), con sus agregados por
minuto (`Total`, `Average`, `Minimum`, `Maximum`).

### Tabla A — Métricas presentes en los logs

| MetricName | Filas |
|:------------------------------|----:|
| sqlserver_process_core_percent | 7 |
| sqlserver_process_memory_percent | 7 |
| sql_instance_cpu_percent | 6 |

### Tabla B — Muestra de filas devueltas

| TimeGenerated | MetricName | Average | Minimum | Maximum | UnitName | ResourceGroup |
|:--------------|:------------------------------|--------:|--------:|--------:|:---------|:--------------|
| 2026-09-06T00:10:00Z | sqlserver_process_core_percent | 10.5 | 10.0 | 11.0 | Percent | RG-PROYECTO-GRADO-FREE |
| 2026-09-06T00:11:00Z | sqlserver_process_core_percent | 10.625 | 9.5 | 11.5 | Percent | RG-PROYECTO-GRADO-FREE |
| 2026-09-06T00:12:00Z | sqlserver_process_core_percent | 9.625 | 9.0 | 10.5 | Percent | RG-PROYECTO-GRADO-FREE |

> Nota: estos logs corresponden al `AzureMetrics` del workspace; las columnas
> restantes (IP, indicadores, etc.) están vacías para esta captura.

---

## 5. `azure_sql_estado_tabla` — Estado de la tabla de telemetría

**Función:** consulta el estado de `dbo.carga_telemetria` (tabla principal de
la carga) tras la ventana: si existe, cuántas filas tiene, su esquema y una
muestra de 100 filas.

**Qué significan los datos:** el resultado/a huella de la carga en la BD.
`total_filas` = registros acumulados por los ciclos de carga; `columnas` es el
esquema; `filas` es la muestra capturada.

### Tabla A — Resumen de la tabla

| tabla | existe | total_filas | filas capturadas |
|---------------------|:------:|------------:|-----------------:|
| dbo.carga_telemetria | true | 239 006 | 100 |

### Tabla B — Esquema (columnas)

| columna | tipo |
|:--------|:-----|
| id | bigint |
| evento | nvarchar |
| carga | int |
| descripcion | nvarchar |
| creado_el | datetime2 |

### Tabla C — Muestra de filas

| id | evento | carga | descripcion | creado_el |
|------:|:-----------|------:|:------------------------------------------|:--------------------------|
| 476913 | evento_7lsSkV | 572 | IwhGwwcucxzYxo7vZAvPasZwoY07Wnj5oeiU4MXY | 2026-09-06T02:26:00.334028 |
| … | … | … | … | … |

---

## 6. `query_store_database_watcher` — DMVs y Query Store

**Función:** captura 8 consultas de administración a DMVs y **Query Store**
de la BD: textos de consulta, estadísticas de ejecución, esperas (waits),
recursos de la instancia, sesiones y resumen de la tabla de carga.
JSON extenso → se muestran **3 tablas**.

**Qué significan los datos:** rendimiento real de las consultas dentro de la
BD: cuántas veces se ejecutó cada query (`count_executions`), cuánto tardó
(`avg_duration` en µs), I/O lógico (`avg_logical_io_reads`), por qué esperó
(`wait_category_desc`), el uso de CPU/disco/log/memoria de la instancia
(`database_watcher_dm_db_resource_stats`, con `cpu_limit` = 2 vCore de la BD
gratuita) y las sesiones activas.

### Tabla A — Consultas más ejecutadas (Query Store runtime stats)

| query_id | count_executions | avg_duration | avg_logical_io_reads |
|---------:|-----------------:|-------------:|---------------------:|
| 10 | 626 | 0.04 ms | 3.2 |
| 23 | 323 | 13.9 ms | 273 |
| 22 | 323 | 0.2 ms | 0 |
| 11 | 125 | 0.8 ms | 152 |
| 14 | 93 | 46.4 ms | 9 058 |

### Tabla B — Uso de recursos de la instancia (dm_db_resource_stats, muestra)

| end_time | avg_cpu_percent | avg_data_io_percent | avg_log_write_percent | avg_memory_usage_percent | avg_instance_cpu_percent | cpu_limit |
|:-------------------------|----------------:|--------------------:|----------------------:|-------------------------:|-------------------------:|----------:|
| 2026-09-06T02:26:07Z | 29.61 | 16.02 | 0.37 | 0.17 | 14.46 | 2.0 |
| 2026-09-06T02:25:52Z | 28.27 | 15.97 | 0.38 | 0.17 | 15.40 | 2.0 |
| 2026-09-06T02:25:37Z | 31.51 | 0.00 | 0.40 | 0.17 | 11.39 | 2.0 |
| 2026-09-06T02:25:22Z | 22.50 | 0.00 | 0.26 | 0.17 | 11.80 | 2.0 |

### Tabla C — Sesiones activas en la BD

| session_id | status | login_name | host_name | program_name |
|-----------:|:-------|:-----------|:----------|:-------------|
| 52 | running | dbadmin | vm-extractor | python3.14 |
| 87 | sleeping | DB000009\WF-… | DB000009 | DmvCollector |
| 78 | sleeping | DB000009\WF-… | DB000009 | AutomaticTuningAgent |
| 73 | running | NT AUTHORITY\SYSTEM | DB000009 | TdService |

> Extra: `carga_telemetria_resumen` → `total_filas = 239 006`,
> `suma_carga = 180 181 054`, `carga_promedio = 753`.

---

## 7. `arm_sql_resources` — Recursos SQL vía ARM

**Función:** consulta **Azure Resource Manager** para obtener la configuración
de la BD `proyectogrado`, el servidor `sql-cuddly-moray` y el listado de bases
del servidor.

**Qué significan los datos:** la definición real de los recursos desplegados
por Terraform: SKU (`GP_S_Gen5_2` = 2 vCore Gen5), modo `serverless` con
auto-pausa de 60 s, `useFreeLimit=true` (BD gratuita), `minCapacity=0.5`,
tamaño máximo de 32 GB y versión SQL 12.0.

### Tabla A — Base de datos `proyectogrado`

| Propiedad | Valor |
|:--------------------------|:-------------------------------------------|
| name | proyectogrado |
| sku | GP_S_Gen5_2 (GeneralPurpose, Gen5, 2 vCore) |
| kind | v12.0,user,vcore,serverless,freelimit |
| maxSizeBytes | 34 359 738 368 (32 GB) |
| status | Online |
| autoPauseDelay (min) | 60 |
| minCapacity | 0.5 |
| useFreeLimit | true |
| freeLimitExhaustionBehavior | AutoPause |
| zoneRedundant | false |

### Tabla B — Servidor `sql-cuddly-moray`

| Propiedad | Valor |
|:--------------------------|:--------------------------------------------|
| name | sql-cuddly-moray |
| version | 12.0 |
| state | Ready |
| fullyQualifiedDomainName | sql-cuddly-moray.database.windows.net |
| minimalTlsVersion | 1.2 |
| publicNetworkAccess | Enabled |
| administratorLogin | dbadmin |

### Tabla C — Bases de datos del servidor

| name | sku.name | tier | capacity | status | useFreeLimit |
|:-------|:---------|:---------------|--------:|:-------|:-------------|
| master | GP_SYSTEM | System | 4 | Online | — |
| proyectogrado | GP_S_Gen5 | GeneralPurpose | 2 | Online | true |

---

## 8. `advisor_recommendations` — Recomendaciones de Azure Advisor

**Función:** consulta las **recomendaciones de Azure Advisor** de la
suscripción.

**Qué significan los datos:** sugerencias de optimización (alta disponibilidad,
excelencia operacional) con su `category`, `impact` (nivel de impacto),
`impactedField`/`impactedValue` (recurso afectado) y `lastUpdated`. Incluyen
tanto recursos propios del proyecto (`vnet-extractor`) como de otras VMs del
RG `pruebas` (`streaming`, `vnet-centralus`).

| Categoría | Impacto | Recurso afectado | Problema / solución |
|-----------------------|:-------|:------------------------------|:--------------------------------------------|
| HighAvailability | Alta | Suscripción completa | Crear una alerta de Azure Service Health |
| HighAvailability | Alta | VM `streaming` | Revisar y migrar cargas de VM |
| HighAvailability | Media | VM `streaming` | Migrar a Virtual Machine Scale Sets Flex |
| HighAvailability | Alta | VM `streaming` | Migrar a serie D o mejor |
| HighAvailability | Media | VNet `streaming-vnet` | Usar NAT gateway para salida |
| HighAvailability | Media | VNet `vnet-centralus` | Usar NAT gateway para salida |
| HighAvailability | Media | VNet `vnet-extractor` | Usar NAT gateway para salida |
| OperationalExcellence | Media | VM `streaming` | Habilitar VM Insights |
| HighAvailability | Media | VM `streaming` | Alinear ubicación recurso/grupo |

---

## 9. `carga_base_de_datos` — Resumen de la carga generada

**Función:** almacena el **resumen de la Fase 1**: operaciones acumuladas y la
serie `carga_por_ciclo` con la intensidad de cada uno de los 3 434 ciclos
ejecutados en la ventana de ~3.67 h.

**Qué significan los datos:** el volumen de trabajo generado durante la ventana
(inserts, updates, deletes, selects, creaciones de tabla y consultas a DMVs),
la duración real, y cómo varió la intensidad ciclo a ciclo (modelo de ondas +
picos + pausas).

### Tabla A — Operaciones totales de la ventana

| Operación | Cantidad |
|:-----------------------|----------:|
| create_table | 6 868 |
| insert | 615 911 |
| update | 93 541 |
| delete | 72 953 |
| select | 17 842 |
| query_dmv | 10 302 |
| **Total** | **817 417** |
| ciclos | 3 434 |
| duración | 13 199 s (~3.67 h) |

### Tabla B — Estadísticas de intensidad por ciclo

| Métrica | Valor |
|:-------------------------|------:|
| intensidad mínima | 0.05 |
| intensidad máxima | 4.00 |
| intensidad promedio | 1.39 |
| operaciones en un ciclo (min) | 29 |
| operaciones en un ciclo (max) | 669 |

### Tabla C — Primeros ciclos de la serie `carga_por_ciclo`

| ciclo | progreso | intensidad | operaciones |
|------:|---------:|-----------:|------------:|
| 1 | 0.0 | 1.03 | 179 |
| 2 | 0.0 | 0.18 | 39 |
| 3 | 0.0 | 1.29 | 221 |
| 4 | 0.0 | 1.34 | 229 |
| 5 | 0.0 | 1.03 | 180 |

> Nota: `registros.json` (34 836 674 bytes, solo presente si la ejecución fue
> detenida con el comando `terminar` del bot) guarda la totalidad de filas de
> `dbo.carga_telemetria` en el momento de la detención.

---

## Resumen general

| Proveedor | Archivo | Contenido | Tablas |
|-----------|---------|-----------|--------|
| azure_retail_pricing | respuesta.json | Catálogo de precios SQL Database (343) | 3 |
| azure_cost_management | respuesta.json | Costo mensual por servicio/día (11 filas) | 1 |
| azure_monitor_metrics | respuesta.json | Métricas de la BD a 1 min (5 métricas) | 2 |
| azure_monitor_logs | respuesta.json | Métricas en Log Analytics (41 cols, 20 filas) | 2 |
| azure_sql_estado_tabla | respuesta.json | Estado + muestra de carga_telemetria | 3 |
| query_store_database_watcher | respuesta.json | DMVs y Query Store (8 consultas) | 3 |
| arm_sql_resources | respuesta.json | Configuración SQL vía ARM | 3 |
| advisor_recommendations | respuesta.json | Recomendaciones de Advisor (9) | 1 |
| carga_base_de_datos | resumen_carga.json | Resumen de la carga (3 434 ciclos) | 3 |