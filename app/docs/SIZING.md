# Límites, SLO y dimensionamiento

Este documento responde una sola pregunta: **con los números que ya guardamos,
¿cómo se decide si una base de datos está mal dimensionada y a qué SKU debería
moverse?** Sin precios (no hay catálogo de precios en el dataset), solo
capacidad, demanda y techos.

Complementa `METRICS.md` (qué se guarda) y `DATASET.md` (dónde se guarda). Aquí
está el *por qué* de cada métrica y las trampas que invalidan un cálculo hecho a
la ligera.

Todo lo que sigue se verificó contra `loadtest-free` el **2026-11-14**, con el
login de workload `cco_workload`, que **no es administrador**: `VIEW DATABASE
STATE` alcanza para todas las DMV citadas.

---

## 1. El SLO real, no el del portal

`sys.dm_database_service_objectives` y `sys.dm_user_db_resource_governance`
dicen cuál es el techo. Los valores vigentes:

| Qué | Valor | De dónde sale | Para qué sirve |
|---|---|---|---|
| Edición / SLO | `GeneralPurpose` / `GP_S_Gen5_1` | `dm_database_service_objectives` | identifica el SKU lógico |
| `slo_name` interno | `SQLDB_GP_S_GEN5_ON_GEN8AH_1_INTERNAL_GPGEN8HH_128IAD` | governance | hardware real detrás del SKU |
| **`cpu_limit`** | **1 vCore** | governance | **denominador de `cpu_percent`** |
| `min_cpu` / `max_cpu` / `cap_cpu` | 0 / 32 / 32 | governance | techo del *pool*, **no** de tu BD |
| `min_cores` / `max_dop` | 2 / **1** | governance | sin paralelismo: no hay CXPACKET útil |
| `max_memory` / `max_memory_grant` | 100 % / **25 %** | governance | grant máximo ≈ 25 % de la memoria de la BD |
| `max_db_memory` | 1 882 133 KB ≈ 1,8 GB | governance | memoria real asignada a la BD |
| `max_sessions` | 30 000 | governance | techo de sesiones concurrentes |
| `primary_max_log_rate` | **4 718 592 B/s ≈ 4,5 MB/s** | governance | **techo de escritura de log de esta BD** |
| `instance_max_log_rate` | 9 437 184 B/s | governance | techo del *servidor*, comparte con `DBADashDB` |
| `pool_max_log_rate` | 6 291 456 B/s | governance | techo del pool |
| `volume_pfs_iops` / `volume_local_iops` | 875 / 5 000 | governance | IOPS reales detrás de `data_io_percent` |
| `max_db_max_size_in_mb` | 4 194 304 ≈ 4 TB | governance | techo del SKU |
| Tamaño máximo configurado | 34 359 738 368 B = **32 GiB** | ARM (`maxSizeBytes`) | denominador de `storage_percent` |
| `autoPauseDelay` | **60 min** | ARM | cuánto tarda en apagarse sola |
| `min_capacity` / `max_capacity` | 0,5 / 1,0 vCore | ARM + `terraform.tfvars` | rango de autoscale |

> **Deriva real:** `terraform/terraform.tfvars` declara
> `auto_pause_delay_minutes = 15`, pero Azure reporta `autoPauseDelay = 60`.
> El valor que manda es el del recurso; el de Terraform solo describe la
> intención. Cualquier cálculo de "minutos despierto al mes" debe leer ARM.

---

## 2. De porcentaje a capacidad: vCore-segundos

Todos los porcentajes de Azure SQL son **relativos a un techo**, y el techo
cambia con el SKU. Guardar solo el porcentaje hace imposible comparar dos SKUs o
sumar consumo. La conversión es una línea:

```
vcore_seconds = (avg_cpu_percent / 100) × cpu_limit × interval_seconds
```

Ejemplo con datos reales del tick de las 08:00 UTC del 2026-11-13 (89 muestras
de 15 s de `sys.dm_db_resource_stats`, media 0,1438 %):

```
0,1438 / 100 × 1 vCore × 15 s × 89 = 1,92 vCore-s
```

Ese tick entero costó **menos de 2 vCore-segundos**. La cuota gratuita es de
**100 000 vCore-s/mes**, o sea **27,8 horas de 1 vCore al 100 %**.

Con los datos guardados hasta hoy (708 lecturas de `cpu_percent` entre el 11 y
el 14 de noviembre):

| Métrica | Media | Máximo | Muestras |
|---|---|---|---|
| `cpu_percent` | 0,1028 % | **2,01 %** | 708 |
| `data_io_percent` | 0,0105 % | 1,14 % | 708 |
| `log_write_percent` | 0,0027 % | 0,05 % | 708 |
| `memory_percent` | 0,2012 % | 0,40 % | 588 |
| `workers_percent` | 0,3958 % | 2,00 % | 120 |
| `sessions_percent` | 0,0000 % | 0,00 % | 120 |
| `sessions_count` | 2,49 | 11 | 200 |

Si el pico medido (2,01 %) se mantuviera **todo el mes**:

```
2,01 / 100 × 1 vCore × 2 592 000 s = 52 099 vCore-s  →  52 % de la cuota
```

O sea: el pico de trabajo real nunca fue el problema. **El problema es el tiempo
despierto.** `app_cpu_billed` (lo que Azure factura de verdad) marca ≈ **41 por
minuto** (≈ 612 por cada 15 minutos, ≈ 0,68 vCore de media, medido el 14-nov con
la BD casi vacía). A ese ritmo, la cuota mensual se agota en **~41 horas de BD
despierta**, aunque no entre una sola consulta.

De ahí la regla de oro para el optimizador:

> **La palanca de coste no es el SKU, es despertar menos.** Un SKU más grande
> solo tiene sentido si el pico *mientras hay trabajo* supera el techo.

---

## 3. Las cuatro trampas

### 3.1 `max_cpu` / `cap_cpu = 32` no es tu límite

`32` es el techo del *pool* que comparten todas las bases del servidor. El
límite de esta base es `cpu_limit = 1`. Usar 32 como denominador divide el uso
por 32 y hace que todo parezca ocioso, siempre. **Solo `cpu_limit` sirve como
denominador**, y por eso se guarda como métrica (`cpu_limit_vcores`).

### 3.2 `app_cpu_billed` no es `avg_cpu_percent`

| | `avg_cpu_percent` (DMV) | `app_cpu_billed` (Azure Monitor) |
|---|---|---|
| Qué mide | % del `cpu_limit` usado por la carga | vCore-segundos facturados del *app* |
| Con la BD vacía | ~0,0 % | ≈ 600 por 15 min |
| Sirve para | dimensionar la demanda | estimar el gasto y el tiempo despierto |

Son señales distintas y no se pueden convertir una en otra. El modelo debe
consumir ambas: la primera para decidir SKU, la segunda para decidir si conviene
apagar.

### 3.3 `free_amount_*` no es fiable entre granularidades

`free_amount_remaining` y `free_amount_consumed` **solo aceptan grano de 15
minutos**; pedirlas en el mismo `query_resource` que las métricas de 60 s
devuelve `HTTP 400` (`Commonly allowed time grains`). Además cambian de valor
según el grano pedido y `free_amount_consumed` no se comporta como un contador
monótono (verificado: avanzó ~9 170 en una ventana de 15 min, incompatible con
`app_cpu_billed`). Por eso:

- se guardan con una consulta aparte (`BUDGET_GRANULARITY_SECONDS = 900`);
- **se leen como un punto único** (`free_vcore_seconds_remaining`), nunca como
  una tasa ni como base para derivar la cuota;
- la cuota se toma del contrato del free tier (100 000 vCore-s/mes), no del DMV.

Comprobación del 14-nov: `free_vcore_seconds_remaining = 73 118` y
`free_vcore_seconds_consumed = 26 882` suman exactamente 100 000. Son un par de
puntos coherentes; lo que no es coherente es pedirles una tasa.

### 3.4 El porcentaje sin denominador no dice nada

`memory_percent = 0,4 %` puede ser 0,4 % de 1,8 GB o de 32 GB. Por eso todo
porcentaje que entra en una decisión de sizing viaja con su techo:
`cpu_percent` con `cpu_limit_vcores`, `log_write_percent` con
`log_rate_cap_bytes_per_sec`, `memory_percent` con `max_memory_percent`,
`data_io_percent` con `volume_pfs_iops`, `storage_percent` con
`storage_capacity_bytes`.

---

## 4. Los seis techos y su firma en las esperas

Azure SQL no dice "estás throttled". Lo dice la espera. Esta es la tabla de
correspondencia que usa el modelo para atribuir el cuello de botella:

| # | Techo | Métrica que lo mide | Firma en esperas | Qué hacer |
|---|---|---|---|---|
| 1 | CPU (`cpu_limit`) | `cpu_percent` → 100 | `SOS_SCHEDULER_YIELD` alto | subir vCores o bajar concurrencia |
| 2 | Workers | `workers_percent` → 100 | `THREADPOOL` | subir vCores / reducir conexiones |
| 3 | Escritura de log (4,5 MB/s) | `log_write_percent` → 100 | `WRITELOG`, `LOG_RATE_GOVERNOR` | lotes más pequeños, menos commits |
| 4 | IOPS de datos | `data_io_percent` → 100 | `PAGEIOLATCH_*`, `IO_QUEUE_LIMIT`, `IO_RETRY` | índices, menos scans |
| 5 | Memoria / grant (25 %) | `memory_percent`, `memory_grants_pending` > 0 | `RESOURCE_SEMAPHORE`, `RESOURCE_SEMAPHORE_QUERY_COMPILE` | subir vCores o reescribir la query |
| 6 | TempDB / almacenamiento | `tempdb_*`, `storage_percent` → 100 | error 1105, 40544/40545 | limpiar datos / subir tamaño |

`wait_critical_ms_per_sec` suma **solo** las esperas de la columna "firma"
(`LOG_RATE_GOVERNOR`, `IO_QUEUE_LIMIT`, `IO_RETRY`, `RESMGR_THROTTLED`,
`RESOURCE_SEMAPHORE*`, `THREADPOOL`, `SE_REPL_*`). Es el indicador directo de
"el plataforma me está frenando".

**Cuidado con las esperas benignas.** En una muestra real de 10 s, la espera más
grande en bruto fue `SOS_WORK_DISPATCHER` con 222 903 ms — puro ruido de
background. Sumar `sys.dm_os_wait_stats` sin filtrar hace que el 99 % del total
sea irrelevante. El colector excluye los prefijos `SLEEP_`, `HADR_`, `XE_`,
`BROKER_`, `SQLTRACE_`, `QDS_`, `PREEMPTIVE_`, `LAZYWRITER`, … y reparte el resto
en seis clases: `cpu`, `log`, `lock`, `io`, `memory`, `other`.

---

## 5. La regla de dimensionamiento (sin precios)

Para cada ventana de 5 minutos con trabajo:

```text
demanda_cpu_vcore = (percentil_95(cpu_percent) / 100) × cpu_limit
demanda_io        = percentil_95(data_io_percent)
demanda_mem       = percentil_95(memory_percent)
demanda_log       = percentil_95(log_write_percent)

vcores_necesarios = techo_a_sku( demanda_cpu_vcore × (1 + headroom) )
```

Pasos, en orden. El primero que dispare decide:

1. **¿Hay throttling real?** `wait_critical_ms_per_sec > 0` de forma sostenida
   durante el pico. Si no, el SKU **no** está chico por CPU.
2. **¿Cuál es el recurso saturado?** el de mayor porcentaje en el p95:
   - `cpu_percent` ≥ 80 → subir vCores por CPU;
   - `log_write_percent` ≥ 80 → subir vCores (el log rate escala con vCores) o
     reducir commits;
   - `data_io_percent` ≥ 80 → subir vCores o índice/consulta;
   - `memory_percent` ≥ 80 o `memory_grants_pending > 0` → subir vCores;
   - `storage_percent` ≥ 80 → **no** se arregla con vCores: es tamaño.
3. **¿El coste es estar despierto?** si `app_cpu_billed` alto y
   `cpu_percent` bajo → la recomendación es *pause policy / bajar frecuencia de
   ticks*, no un SKU distinto. Esta es la salida correcta para `loadtest-free`
   hoy.
4. **Headroom**: 30 % por defecto (p95 ya ignora el pico instantáneo). Si el
   p95 supera el 70 % del techo, el SKU actual está justo: subir un escalón.
5. **Nunca bajar de `min_capacity`** ni recomendar un SKU que el free tier no
   soporte (máximo 4 vCores en esta suscripción; free tier = 1 vCore).

Con los datos medidos, el resultado para `loadtest-free` es: **cero
recomendaciones de SKU**. Ningún recurso superó el 2,01 % del techo. El único
hallazgo de coste es el suelo de tiempo despierto.

---

## 6. Qué se guarda ahora (niveles 0–3)

Todo entra en `raw.metric_reading` con `source`, `unit`, `interval_seconds`,
`coverage_*` y `catalog_version` (`2026.11.1`). No hubo migración: `metric_code`
es `text` y el catálogo vive en `app/src/metrics/normalizer.py`.

### Nivel 0 — identidad y contrato

| `metric_code` | Unidad | Fuente | Cadencia |
|---|---|---|---|
| `cpu_limit_vcores` | count | `sql_diagnostics` | 30 min |
| `max_dop` | count | `sql_diagnostics` | 30 min |
| `max_sessions` | count | `sql_diagnostics` | 30 min |
| `max_memory_percent` | percent | `sql_diagnostics` | 30 min |
| `max_memory_grant_percent` | percent | `sql_diagnostics` | 30 min |
| `log_rate_cap_bytes_per_sec` | bytes/s | `sql_diagnostics` | 30 min |
| `instance_max_log_rate_bytes_per_sec` | bytes/s | `sql_diagnostics` | 30 min |
| `storage_capacity_bytes` | bytes | `azure_monitor` | 60 s |

### Nivel 1 — utilización contra su techo

| `metric_code` | Unidad | Fuente |
|---|---|---|
| `cpu_percent`, `data_io_percent`, `log_write_percent`, `memory_percent` | percent | `sql_diagnostics` (`dm_db_resource_stats`) |
| `workers_percent`, `sessions_percent` | percent | ambas fuentes (mismo código; el agregador elige una) |
| `instance_cpu_percent` | percent | `sql_diagnostics` |
| `app_cpu_billed`, `app_cpu_percent`, `cpu_used_vcores` | count/percent | `azure_monitor` |
| `free_vcore_seconds_remaining`, `free_vcore_seconds_consumed` | count | `azure_monitor` (grano 900 s) |
| `connection_successful_count`, `connection_failed_count`, `blocked_by_firewall_count` | count | `azure_monitor` |
| `tempdb_log_used_percent`, `tempdb_data_size_bytes` | percent/bytes | `azure_monitor` |
| `process_core_percent`, `process_memory_percent` | percent | `azure_monitor` |

### Nivel 2 — presión, espera y congestión

| `metric_code` | Unidad | Origen |
|---|---|---|
| `wait_cpu_ms_per_sec`, `wait_log_…`, `wait_lock_…`, `wait_io_…`, `wait_memory_…`, `wait_other_…` | ms/s | delta de `dm_os_wait_stats` |
| `wait_critical_ms_per_sec` | ms/s | solo esperas de throttle |
| `rows_read_bytes_per_sec`, `rows_write_bytes_per_sec` | bytes/s | delta de `dm_io_virtual_file_stats` |
| `rows_stall_ms_per_sec`, `log_stall_ms_per_sec` | ms/s | delta del mismo DMV |
| `running_requests`, `blocked_requests`, `longest_running_ms` | count/ms | `dm_exec_requests` (solo `is_user_process = 1`) |
| `page_life_expectancy_sec`, `memory_grants_pending` | s/count | gauges de `dm_os_performance_counters` |
| `total_server_memory_bytes`, `database_cache_memory_bytes` | bytes | gauges del mismo DMV |

### Nivel 3 — inventario y forma de la carga

| `metric_code` | Unidad | Qué revela |
|---|---|---|
| `log_size_bytes`, `log_vlf_count` | bytes/count | fragmentación del log, riesgo de `WRITELOG` |
| `index_seeks_total`, `index_scans_total`, `index_lookups_total`, `index_updates_total` | count | índices que sobran o faltan |
| `tuning_recommendations_count` | count | recomendaciones del motor |
| `tempdb_used_bytes` | bytes | presión de TempDB |
| `runnable_tasks`, `work_queue_count` | count | cola de CPU por scheduler |
| `query_duration_avg_ms` | ms | latencia interna (Query Store) |
| `workload_operation_latency_p95_ms` | ms | latencia de cliente (muestras exactas) |

Todo lo que viene de un contador monótono se guarda **como tasa por segundo**
(`/segundo`), no como total. Así una pausa/reanudación no inventa un pico: si el
contador retrocede, el ciclo se descarta en vez de emitir un negativo o un salto
falso (`sql_extras.rates`).

---

## 7. Cómo lo usa el modelo

**Features** (por ventana de 5 min y por hora):
`cpu_percent_p95`, `cpu_percent_max`, `cpu_limit_vcores`, `mem_pct_p95`,
`log_write_pct_p95`, `data_io_pct_p95`, `workers_pct_p95`,
`wait_critical_ms_per_sec`, las seis clases de espera, `running/blocked`,
stalls de IO, `storage_percent`, `tempdb_log_used_percent`.

**Etiquetas** (lo que hay que predecir):

| Etiqueta | Definición | Uso |
|---|---|---|
| `is_throttled` | `wait_critical_ms_per_sec > 0` sostenido en la ventana | clasificación |
| `bottleneck` | clase de espera dominante (cpu/log/lock/io/memory/other) | clasificación multi-clase |
| `headroom_vcores` | `cpu_limit × (1 − cpu_p95)` | regresión |
| `needs_bigger_sku` | `cpu_p95 ≥ 0,8` **o** log/io ≥ 0,8 **o** grants pendientes | binaria |
| `awake_cost_share` | `app_cpu_billed` normalizado por minutos despierto | regresión (palanca real) |

`max_dop = 1` significa que este dataset **no puede aprender** efectos de
paralelismo. Está bien: es una restricción del SKU, se documenta y se excluye.

**Censura:** si la BD se pausa, no hay lectura, y eso no es "0 % de uso". El
`quality_status` y `missing_fraction` ya distinguen "sin dato" de "dato bajito";
el modelo debe usar `is_trainable` y no imputar ceros en las pausas.

---

## 8. Límites honestos de este dataset

| Límite | Consecuencia |
|---|---|
| 1 tick de 5 min cada 8 h | ~15 ventanas/día, ~45 en los 3 días que quedan |
| BD pausada entre ticks | huecos largos; `missing_fraction` > 50 % en la hora del tick |
| `max_dop = 1` | sin señal de paralelismo |
| `cpu_percent` máx. 2,01 % | no hay ejemplos de saturación; el modelo aprende "no throttled" |
| Un solo SKU (`GP_S_Gen5_1`) | no hay contraste de SKUs; el efecto de subir vCores no es observable |
| `sys.dm_os_wait_stats` se reinicia al reanudar | el primer ciclo tras un resume se descarta (por diseño de `rates`) |
| Contadores "por segundo" de `dm_os_performance_counters` | **no** son tasas reales en Azure SQL (verificado: no se movieron insertando 20 k filas) → no se guardan |
| `dm_io_virtual_file_stats` para el log | subreporta los bytes escritos; solo el acumulador de stall es fiable |
| `free_amount_*` | inconsistente entre granularidades; solo como punto |

Con 4–5 ticks por perfil y sin saturación, el dataset sirve para **demostrar el
pipeline y la metodología**, no para entrenar un recomendador con precisión
estadística. Cualquier afirmación de precisión del modelo con este volumen sería
falsa.

---

## 9. Validación

### Contra una herramienta independiente (DBA Dash)

Ventana `2026-11-13 07:45Z → 08:30Z`, ambos leyendo `sys.dm_db_resource_stats`:

| métrica | puntos | media nuestra | media DBA Dash | dif. abs. media | máx. | Pearson |
|---|---|---|---|---|---|---|
| `cpu_percent` | 89 | 0,1438 | 0,1438 | 0,0000 | 0,0000 | 1,0000 |
| `data_io_percent` | 89 | 0,0055 | 0,0055 | 0,0000 | 0,0000 | 1,0000 |
| `log_write_percent` | 89 | 0,0039 | 0,0039 | 0,0000 | 0,0000 | 1,0000 |
| `memory_percent` | 89 | 0,3380 | 0,3380 | 0,0000 | 0,0000 | 1,0000 |

Hallazgo del cruce: nuestro escritor **trunca `observed_at_utc` al bucket de
15 s** y DBA Dash guarda el `end_time` crudo. Cualquier comparación debe truncar
ambos lados o el join pierde puntos. Detalle en
`experiments/dbadash/report.md` (local, no versionado).

### Contra la base real (14-nov)

- `collector --once` en la VM (identidad administrada + login de workload):
  **1 272 lecturas de intervalo, 0 rechazadas, `catalog_version = 2026.11.1`**.
  Llegaron a `raw.metric_reading` los 22 códigos nuevos de `sql_extras`, las 3
  columnas añadidas a `dm_db_resource_stats`, los 11 códigos nuevos de Azure
  Monitor y las 2 métricas de cuota con `interval_seconds = 900`.
- Las 22 métricas de `sql_extras` se leyeron con el login de workload, sin
  errores de permisos.
- Los deltas se validaron con una carga controlada de 12 500 filas: esperas,
  stalls y bytes de lectura pasaron de 0 a valores no nulos; `running_requests`
  pasó de 0 a 2.
- Las esperas de background se aislaron: `HADR_WINFAB_REPLICA_MGR` aparecía como
  request "running" durante 30 min con la base vacía; se corrigió el filtro a
  `is_user_process = 1`.
- Los contadores "por segundo" se descartaron tras medir que no reaccionan a la
  carga (ver §8).
- La cuota del free tier cuadra consigo misma: `free_amount_remaining = 73 118`
  y `free_amount_consumed = 26 882` suman exactamente 100 000. Se guardan como
  dos puntos, no como una serie.

### Lo que **no** está validado

- Los deltas de esperas e IO requieren **al menos dos ciclos** en el mismo
  proceso; el primer ciclo tras un arranque solo establece la línea base. Se
  validan en el tick de las 08:00 UTC, no con un `--once` aislado.
- El efecto de una recomendación de SKU: con un solo SKU y sin saturación no hay
  contraste posible (`SIZING.md` §8).

---

## 10. Dónde está el código

| Archivo | Qué contiene |
|---|---|
| `app/src/metrics/sql_extras.py` | esperas por clase, gauges, IO, congestión, inventario lento y techos |
| `app/src/metrics/sql_diagnostics.py` | `dm_db_resource_stats` ampliado (+worker, +session, +instance CPU) |
| `app/src/metrics/normalizer.py` | catálogo y mapeo; versión `2026.11.1` |
| `app/src/metrics/azure_monitor.py` | métricas de plataforma, facturación y cuota (900 s) |
| `app/src/metrics/collector.py` | encadena las fuentes por ciclo |
| `app/tests/test_sql_extras.py` | exclusión de esperas benignas, clases, reset de contadores, cobertura del catálogo |

Reproducir la lectura de techos:

```sql
SELECT cpu_limit, max_dop, max_sessions, max_memory, max_memory_grant,
       primary_max_log_rate, instance_max_log_rate
FROM sys.dm_user_db_resource_governance;
```
