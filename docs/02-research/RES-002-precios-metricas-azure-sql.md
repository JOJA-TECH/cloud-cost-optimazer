# RES-002 — Precios y métricas reales de Azure SQL Database

- **ID:** RES-002
- **Fecha:** 2026-09-28
- **Investigador:** Equipo del proyecto (Jesús, responsable de TASK-006; consolidado con asistencia de IA sobre evidencia del repositorio y fuentes oficiales verificadas el 2026-09-28)
- **Área:** Investigación / precios y métricas de Azure SQL
- **Estado:** Borrador (pendiente de revisión del equipo según `docs/MANUAL-DE-USO.md` §15)
- **Relacionado con:**
  - TASK-006 de MTG-001 ("Investigar precios reales de Azure y métricas de Azure SQL")
  - LAB-001, EXP-001, DATA-001 y `DATA-DICTIONARY-DATA-001.md` (evidencia empírica de las fuentes)
  - DEC-007 (conectores por proveedor), REQ-002 / ADR-002 (persistencia de recolección), REQ-003 (vista de métricas del frontend), REQ-006 (tiempo de respuesta)
  - RES-001 (decisión propuesta DEC-027: normalización FOCUS de costos)

## Pregunta de investigación

1. ¿Cuáles son las fuentes oficiales de precios de Azure aplicables a Azure SQL Database y cómo se estructuran sus datos?
2. ¿Qué métricas expone Azure SQL Database, con qué unidades y significado, y cuáles son relevantes para el frontend y el módulo de ML del proyecto?

## Contexto

TASK-006 asignó a Jesús la investigación de precios reales de Azure y métricas de Azure SQL (MTG-001). El equipo ya ejecutó LAB-001 y EXP-001 (2026-09-06), que verificaron empíricamente las fuentes de precios y métricas sobre un recurso real de laboratorio, con evidencia conservada en DATA-001. Sin embargo, no existía un documento consolidado que: (a) catalogara las métricas disponibles del servicio con unidades y significado, y (b) definiera el contrato de datos normalizado para el frontend y el módulo de ML. Este registro cierra esos vacíos.

## Alcance y método

- **Tipo:** investigación documental sobre fuentes oficiales de Microsoft, contrastada con la evidencia empírica del repositorio (DATA-001).
- **Verificación:** fuentes accedidas el 2026-09-28; el detalle está en `literature-review/RES-002-sources.md`.
- **Limitación:** el catálogo corresponde a la documentación pública vigente; algunos medidores y métricas varían por región, modalidad (vCore/DTU) y tier (Hyperscale, serverless). La lista empírica de DATA-001 es una muestra, no el catálogo completo.

## Hallazgos

### Hallazgo 1 — Fuentes oficiales de precios de Azure para Azure SQL

| Fuente | Qué entrega | Acceso | Estado en LAB-001/EXP-001 |
|---|---|---|---|
| Azure Retail Prices API | Precio de lista (USD) de SKUs/medidores: vCore, eDTU, storage, backup, licencias | GET público `https://prices.azure.com/api/retail/prices` sin autenticación; filtro `serviceName eq 'SQL Database'` | ✅ Verificada: 343 items, 38 productos, paginación completa (`INSTRUCCIONES_REPETICION.md`) |
| Azure Cost Management (Query API) | Costo facturado real de la suscripción (`PreTaxCost`) por día, servicio, grupo de recursos | REST autenticado; scope de suscripción | ✅ Verificada: 11 filas; SQL Database USD 0 por nivel gratuito (limitación registrada) |
| Cost Management exports (esquema FOCUS) | Datos de costo/uso normalizados según FOCUS (1.0–1.2) | Export configurado a storage | ⏳ No probada aún en el laboratorio; propuesta en RES-001/DEC-027 |
| Azure Advisor (categoría Cost) | Recomendaciones de optimización de costo | REST autenticado | ⏳ Consultada: 9 recomendaciones, ninguna comparable de rightsizing SQL |

Notas contrastadas con la evidencia (DATA-001):

- El catálogo Retail Prices **mezcla modalidades** (Consumption vs Reservation), tiers (GP/BC/Hyperscale/Premium), familias (Gen5, DC-series, FSv2, M-series) y componentes (compute, storage, backup). Requiere normalización antes de cotizar candidatos (regla ya registrada en el diccionario de datos).
- El costo facturado de la BD gratuita es **USD 0**: para el KPI económico hay que construir un **costo contrafactual** (precio de lista × uso medido) o capturar un ciclo de facturación con BD pagada (escenario `db-2` pendiente de poblarse).
- Campos clave del catálogo (observados en DATA-001): `retailPrice`, `unitOfMeasure` (`1 Hour`, `1 GB/Month`, `1/Day`), `meterName`, `skuName`, `productName`, `armSkuName`, `isPrimaryMeterRegion`, `type` (Consumption/Reservation), `effectiveStartDate`.

### Hallazgo 2 — Catálogo de métricas de Azure Monitor para Azure SQL Database

Fuente oficial: Microsoft Learn, "Monitor Azure SQL Database with metrics and alerts" (última actualización 2025-02-11). Unidades según la API de Azure Monitor. Las métricas marcadas con ★ ya fueron capturadas empíricamente en DATA-001.

| Métrica (ID de API) | Unidad | Significado | Aplicación en el proyecto |
|---|---|---|---|
| CPU percentage ★ (`cpu_percent`) | Percent | CPU del **workload del usuario** hacia el límite de la BD/pool | Indicador principal de utilización para rightsizing |
| SQL instance CPU percent (`sql_instance_cpu_percent`) | Percent | CPU total (usuario + sistema); **escala no comparable** con `cpu_percent` | Contexto de presión total; ya capturada vía Log Analytics |
| Data IO percentage (`physical_data_read_percent`) | Percent | Consumo de I/O de archivos de datos hacia el límite del workload | Componente de DTU; detección de cuellos de I/O |
| Log IO percentage (`log_write_percent`) | Percent | Throughput de escritura de log de transacciones hacia el límite | Componente de DTU; límite de throughput de escritura |
| Workers percentage ★ (`workers_percent`) | Percent | Consumo de hilos de trabajo (workers) hacia el límite | Detección de saturación de concurrencia |
| DTU percentage (`dtu_consumption_percent`) | Percent | Consumo DTU = máx(cpu, data IO, log IO) | Modalidad DTU; comparable entre tiers |
| CPU used (`cpu_used`) | Count (vCores) | CPU consumida expresada en vCores | Traducción directa a capacidad vCore para cotización de precios |
| DTU used (`dtu_used`) | Count | DTUs usadas | Modalidad DTU |
| **App CPU billed** (`app_cpu_billed`) | Count (**vCore-seconds**) | **Compute facturado en serverless** (CPU+memoria aplicada); base del costo en la modalidad serverless | **Crítica para el KPI económico del proyecto**: conecta uso medido con precio vCore-hora |
| App CPU percentage (`app_cpu_percent`) | Percent | CPU hacia el máximo del paquete de la app (serverless) | Utilización real en serverless |
| App memory percentage (`app_memory_percent`) | Percent | Memoria hacia el máximo del paquete (serverless) | Utilización real en serverless |
| Sessions count (`sessions_count`) | Count | Sesiones de usuario establecidas | Concurrencia |
| Data space used (`storage`) | Bytes/Megabytes | Espacio usado en archivos de datos (BD) | Costo de storage |
| Data space used ★ (`storage_used`, pools) | — | Espacio usado en pools | Pool analysis |
| Data space allocated (`allocated_data_storage`) | Bytes | Espacio asignado (incluye espacio vacío); suele ser mayor que usado | Diferencia asignado vs usado (optimización de storage) |
| Data space used percent ★ (`storage_percent`) | Percent | Espacio usado hacia el límite de datos | Capacidad de almacenamiento |
| Tempdb Percent Log Used (`tempdb_log_used_percent`) | Percent | Log de tempdb hacia su máximo | Presión de tempdb |
| Successful Connections (`connection_successful`) | Count | Conexiones exitosas (dimensión: SslProtocol, driver) | Carga de conexiones |
| Failed Connections: System Errors (`connection_failed`) | Count | Fallos por errores internos/transitorios | Disponibilidad |
| Failed Connections: User Errors (`connection_failed_user_error`) | Count | Fallos por error de usuario (credenciales, firewall) | Diagnóstico |
| Deadlocks ★ (`deadlock`) | Count | Deadlocks en la BD | Salud del workload |
| Availability (`availability`) | Percent (100/0 por minuto) | Disponibilidad de conexión por minuto | SLA; **no soportada en serverless** (devuelve 100%) |
| Replication lag (preview) (`replication_lag_seconds`) | Seconds | Retraso de replicación primary→secondary | Solo escenarios geo-replicación |

Notas oficiales relevantes:

- `dtu_consumption_percent` se deriva como el **máximo** de cpu/data IO/log IO en cada punto.
- La métrica `availability` no es válida en serverless; evitarla como SLA real en el laboratorio actual.
- Agregaciones disponibles por la API: total, count, average, minimum, maximum; granularidad mínima 1 minuto (PT1M).

#### Métricas complementarias fuera de Azure Monitor (ya evidenciadas en DATA-001)

| Fuente | Métrica | Unidad | Significado |
|---|---|---|---|
| DMV `sys.dm_db_resource_stats` (Database watcher) | `avg_cpu_percent`, `avg_data_io_percent`, `avg_log_write_percent`, `avg_memory_usage_percent`, `avg_instance_cpu_percent` | Percent (ventana de 15 s) | Uso de recursos interno a mayor granularidad que Azure Monitor |
| DMV `sys.dm_db_resource_stats` | `cpu_limit` | vCores | Límite de capacidad vigente (2.0 en el laboratorio) |
| Query Store (`sys.query_store_runtime_stats`) | `count_executions`, `avg_duration` (µs), `avg_cpu_time` (µs), `avg_logical_io_reads` (páginas lógicas) | µs / páginas / conteos | Comportamiento por consulta: insumo del módulo de ML |
| Query Store (`sys.query_store_wait_stats`) | `wait_category_desc`, `avg_query_wait_time_ms` | ms / texto | Causas de espera |
| Log Analytics (`AzureMetrics`) | `sqlserver_process_core_percent`, `sqlserver_process_memory_percent`, `sql_instance_cpu_percent` | Percent | Vista de logs de las mismas métricas |

### Hallazgo 3 — Contratos JSON propuestos (frontend y módulo de ML)

Los siguientes son **contratos de salida propuestos** (no datos reales): definen la forma normalizada que el backend debe entregar, derivada de las fuentes verificadas. El nombre de campos sigue las convenciones del diccionario de datos de DATA-001 y la propuesta de normalización FOCUS de RES-001.

#### 3.1 Contrato para el frontend — vista de recurso (REQ-003)

```json
{
  "resourceId": "/subscriptions/<sub>/resourceGroups/rg-proyecto-grado-free/providers/Microsoft.Sql/servers/sql-cuddly-moray/databases/proyectogrado",
  "resourceName": "proyectogrado",
  "provider": "azure",
  "region": "australiaeast",
  "sku": {
    "name": "GP_S_Gen5_2",
    "tier": "GeneralPurpose",
    "family": "Gen5",
    "capacity": 2,
    "unit": "vCores",
    "computeTier": "serverless",
    "minCapacity": 0.5,
    "autoPauseDelayMinutes": 60
  },
  "metrics": [
    {
      "metricId": "cpu_percent",
      "unit": "Percent",
      "interval": "PT1M",
      "aggregation": "average",
      "points": [
        { "time": "2026-09-06T02:10:00Z", "avg": 26.25, "min": 21, "max": 31 }
      ]
    },
    {
      "metricId": "storage_percent",
      "unit": "Percent",
      "interval": "PT1M",
      "aggregation": "average",
      "points": [
        { "time": "2026-09-06T02:10:00Z", "avg": 0.0, "min": 0, "max": 0 }
      ]
    }
  ],
  "cost": {
    "currency": "USD",
    "billedCostMonthToDate": 0.0,
    "effectiveCostEstimate": 12.34,
    "note": "billedCost=0 por free limit; effectiveCost = costo contrafactual (precio de lista x app_cpu_billed)"
  },
  "suggestions": [
    {
      "id": "sug-001",
      "type": "rightsizing",
      "summary": "CPU promedio 27.7% con max 36% en 15 min; sin evidencia para cambio",
      "requiresConfirmation": true,
      "source": "ml-module"
    }
  ],
  "generatedAt": "2026-09-06T02:30:00Z"
}
```

#### 3.2 Contrato para el módulo de ML — serie de entrenamiento costo/uso

```json
{
  "datasetVersion": "DATA-002 v0.1 (propuesto)",
  "resourceId": ".../databases/proyectogrado",
  "granularity": "PT1H",
  "series": [
    {
      "timestamp": "2026-09-06T02:00:00Z",
      "configuration": {
        "skuName": "GP_S_Gen5_2",
        "computeTier": "serverless",
        "minCapacityVcores": 0.5,
        "maxCapacityVcores": 2.0,
        "cpuLimit": 2.0
      },
      "usage": {
        "cpuPercentAvg": 27.73,
        "cpuPercentMax": 36.0,
        "appCpuBilledVcoreSeconds": 18342,
        "workersPercentAvg": 1.0,
        "storagePercentAvg": 0.0,
        "deadlockCount": 0,
        "sessionsCountAvg": null
      },
      "cost": {
        "billedCost": 0.0,
        "listUnitPriceVcoreHour": 0.1458,
        "effectiveCost": 0.743,
        "pricingCategory": "Consumption"
      },
      "workloadLabel": null
    }
  ]
}
```

Reglas de construcción del contrato ML (derivadas de DATA-001 y del diccionario):

- Cada punto une: configuración vigente + uso + costo + etiqueta de workload (cuando exista).
- `app_cpu_billed` (vCore-segundos) × precio de lista vCore-hora ÷ 3600 = costo contrafactual por hora; sustituye al costo facturado cuando el free limit lo distorsiona (caso verificado: USD 0).
- No calcular p95/p99 solo desde promedio/mín/máx agregados (regla del diccionario de datos).
- Fechas siempre UTC ISO 8601; precios separados por región/modalidad/tier/familia/unidad/tipo.

## Comparaciones / contradicciones

- **Precio de lista vs costo facturado:** la Retail Prices API entrega el precio público; Cost Management entrega el costo real con descuentos/beneficios. En el laboratorio difieren radicalmente (USD 0 facturado vs precio de lista vigente). El sistema debe modelar ambos planos (coherente con `BilledCost`/`ListCost` de FOCUS, RES-001).
- **Azure Monitor vs DMVs:** Monitor entrega granularidad 1 min con retención extendida; `sys.dm_db_resource_stats` entrega 15 s pero ventana corta (~1 h). El pipeline debe elegir por caso de uso; hoy solo Monitor está acumulándose.
- **`cpu_percent` vs `sql_instance_cpu_percent`:** escalas distintas (workload usuario vs total); no son intercambiables (advertencia explícita de Microsoft Learn).

## Conclusión

1. Las fuentes de precios están verificadas y documentadas: Retail Prices API (catálogo, 343 items evidenciados) y Cost Management (costo real), con la restricción del costo 0 por free limit.
2. El catálogo de métricas de Azure Monitor para Azure SQL Database queda documentado con unidades y significado (23 métricas), más las métricas complementarias de DMVs y Query Store ya evidenciadas.
3. Existen contratos JSON propuestos para frontend (REQ-003) y módulo ML, construidos exclusivamente con campos ya verificados empíricamente en DATA-001.
4. El criterio "ejemplos de datos o JSON para el frontend y el módulo de ML" se cumple con contratos propuestos; quedará plenamente cerrado cuando el backend genere el primer payload real que los respete (futuro TEST).

## Impacto sobre el proyecto

- **Frontend:** el contrato 3.1 define los datos que REQ-003 debe mostrar (incluye el aviso de costo contrafactual para BD serverless gratuitas).
- **ML:** el contrato 3.2 fija la unidad de entrenamiento (fila horaria config+uso+costo) y resuelve la base del KPI económico vía `app_cpu_billed`.
- **Pipeline de recolección:** incorporar `app_cpu_billed`, `app_cpu_percent`, `app_memory_percent`, `cpu_used` y `physical_data_read_percent`/`log_write_percent` a la próxima captura (no estaban en DATA-001).
- **TASK-007 (modelo de datos):** este registro es insumo directo para diseñar la tabla de costos y métricas en la BD de recolección (REQ-002/ADR-002).

## Decisiones derivadas

- Ninguna obligatoria. Sugerencia técnica no vinculante: incorporar las métricas serverless (`app_cpu_billed`, `app_cpu_percent`, `app_memory_percent`) a la próxima ejecución de LAB (queda como tarea, no como decisión).

## Tareas derivadas

- Poblar el escenario `db-2` (BD con costo) para sustentar el KPI económico con costo facturado real (ya registrada como pendiente en DATA-001).
- Actualizar el script de recolección para incluir las métricas del Hallazgo 2 marcadas como no capturadas.
- Definir el modelo de datos (TASK-007 de MTG-001) alineado con los contratos propuestos y con FOCUS (RES-001/DEC-027).

## Referencias

Fecha de acceso y verificación de todas las referencias: **2026-09-28** (detalle en `literature-review/RES-002-sources.md`).

1. Microsoft Learn — "Monitor Azure SQL Database with metrics and alerts": <https://learn.microsoft.com/en-us/azure/azure-sql/database/monitoring-metrics-alerts>
2. Microsoft Learn — "Azure Retail Prices API" (REST, Cost Management): <https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices>
3. Microsoft Learn — "Cost Management Query Usage API": <https://learn.microsoft.com/en-us/rest/api/cost-management/query/usage>
4. Microsoft Learn — "Monitor the serverless compute tier" (billing de serverless, `app_cpu_billed`): <https://learn.microsoft.com/en-us/azure/azure-sql/database/serverless-tier-overview#monitor-the-serverless-compute-tier>
5. Microsoft Learn — "Serverless compute tier" (autoscale y billing vCore-seconds): <https://learn.microsoft.com/en-us/azure/azure-sql/database/serverless-tier-overview>
6. Microsoft Learn — "Database watcher data collection and datasets": <https://learn.microsoft.com/en-us/azure/azure-sql/database/database-watcher-data?view=azure-sql-mi>
7. Microsoft Learn — "Understand Azure Cost Management data": <https://learn.microsoft.com/en-us/azure/cost-management-billing/costs/understand-cost-mgt-data>
8. Evidencia interna: DATA-001 (`06-data/DATA-001-azure-integration-responses.md`), LAB-001 (`07-experiments/LAB-001-azure-data-integration.md`), `laboratory-001/db-1/DATOS_PROVEEDORES.md`, `laboratory-001/db-2/informe_factibilidad.md`, `laboratory-001/db-2/INSTRUCCIONES_REPETICION.md`, `06-data/DATA-DICTIONARY-DATA-001.md`.
9. Investigación relacionada: RES-001 (`02-research/RES-001-antecedentes-software-similar-estandar-focus.md`).

> Nota sobre el formato bibliográfico: cuando el equipo adopte un gestor bibliográfico (p. ej. Zotero/BibTeX), estas referencias se migrarán al formato oficial.
