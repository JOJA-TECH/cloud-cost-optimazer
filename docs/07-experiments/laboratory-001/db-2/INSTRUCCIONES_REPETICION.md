# Instrucciones para repetir las consultas

Estas instrucciones permiten repetir el laboratorio de factibilidad. No se
almacenan secretos aqui: los valores sensibles se leen de `python/.env`.

## Entorno consultado (anonimizado)

| Variable | Valor |
|---|---|
| Suscripcion | `e44b10e0-cdae-4525-b479-b9ad9454f0d1` |
| Grupo de recursos | `rg-proyecto-grado-free` |
| Region | `australiaeast` |
| Servidor SQL | `sql-cuddly-moray.database.windows.net` |
| Base de datos | `proyectogrado` |

## 1. Pre-requisitos

- Python 3.14+, ODBC Driver 18, `uv`.
- Infraestructura desplegada (Terraform) y `python/.env` con:
  `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `SQL_SERVER`, `SQL_DB`,
  `SQL_USER`, `SQL_PASSWORD`, `SQL_DB_RESOURCE_ID`, `SQL_SERVER_RESOURCE_ID`,
  `LOG_ANALYTICS_WORKSPACE_ID`, `LOG_ANALYTICS_RESOURCE_ID`.
- Sobre la BD SQL, permisos que alcancen a `sys.query_store_*`,
  `sys.dm_db_resource_stats`, `sys.dm_exec_sessions` y las tablas de workload.
- Sobre la suscripcion, rol que permita lectura de `Microsoft.CostManagement`
  (ej. Cost Management Reader), `Microsoft.Insights` y `Microsoft.Advisor`.

## 2. Ejecutar la captura

```bash
cd python
uv run python run_all.py --duration 5m
```

Opciones utiles:

- `--only azure_monitor_metrics,query_store_database_watcher`
  solo esas fuentes.
- `--duration 24h` acumula historico (cuida la cuota gratuita).
- `--out DIR` cambia el directorio de salida (`CAPTURE_OUT`).

Cada fuente genera en `data/responses/<fuente>/`:
`respuesta.json` (original), `estructura.json` (esquema) y
`resumen.json` (fecha UTC, estado, parametros, HTTP, errores, advertencias).

## 3. Generar los artefactos de factibilidad

```bash
cd python
uv run python feasibility.py --out data/feasibility
```

Si la captura salio a otro directorio:
`uv run python feasibility.py --captures <DIR> --out data/feasibility`.

Salida:

- `inventario_campos.json` - campos, tipos, ejemplos, nulos.
- `cobertura_temporal.json` - periodo cubierto por fuente.
- `fuentes_disponibles_faltantes.json` - matriz disponibilidad.
- `errores.json` - errores/advertencias clasificados por causa.
- `hashes.json` - sha256 de los archivos de respuesta.
- `config_anonimizado.json` - configuracion sin secretos.
- `informe_factibilidad.md` - informe general.
- `INSTRUCCIONES_REPETICION.md` - este documento.

## 4. Consultas por fuente (referencia)

| Fuente | Consulta / API | version |
|---|---|---|
| Configuracion | GET `${SQL_DB_RESOURCE_ID}?api-version=2023-05-01-preview` | ARM |
| Costos | POST `/subscriptions/${SUB}/providers/Microsoft.CostManagement/query` (grouping ServiceName, ResourceId, ResourceGroup, ChargeType) | 2025-03-01 |
| Precios | GET `prices.azure.com/api/retail/prices?$filter=armRegionName eq '<region>' and serviceName eq 'SQL Database'` | 2021-10-01-preview |
| Metricas | GET `${SQL_DB_RESOURCE_ID}/providers/Microsoft.Insights/metrics?timespan=PT15M&interval=PT1M&metricnames=cpu_percent,...` | 2023-10-01 |
| Logs | POST `api.loganalytics.io/v1${SQL_DB_RESOURCE_ID}/query` (AzureMetrics) | v1 |
| Query Store | `sys.query_store_*`, `sys.dm_exec_query_stats`, `sys.dm_db_resource_stats`, `sys.dm_exec_sessions` | SQL |
| Estado tabla | COUNT/TOP 100 sobre `dbo.carga_telemetria` | SQL |
| Advisor | GET `/subscriptions/${SUB}/providers/Microsoft.Advisor/recommendations` (+ filtro `properties/category eq 'Cost'`) | 2020-01-01 |

## 5. Que mirar al repetir

- Verificar que cada `resumen.json` registre `http.codigos` y `estado`.
- Costos: si el recurso SQL aparece en 0, registrar que es nivel gratuito, no
  ausencia de costo.
- Logs: si devuelve 0 filas, validar vinculo del workspace a la BD.
- Query Store: si una consulta falla, revisar permisos/habilitacion.
- No confundir 'la API responde' con 'la API entrega datos suficientes para
  validar la propuesta'.
