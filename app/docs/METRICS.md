# Metrics

Every stored number is tagged with its `source` and the `catalog_version` that
produced it, so provenance is never lost. There are **four interval sources**
(one reading per capture interval) and **one sample source** (one row per
operation, the only source of exact percentiles).

- Raw capture interval: **60 s** (`CAPTURE_RAW_INTERVAL_SECONDS`).
- Operational aggregation: **5 min**. Feature aggregation: **1 h**.
- Catalog contract: `METRIC_CATALOG_VERSION` in `src/metrics/normalizer.py`.
- Canonical codes, units and bounds live in `METRIC_CATALOG`; raw provider
  names are translated through `NORMALIZATION_MAP`; **anything unknown is
  recorded in `ops.metric_rejection`** (never silently dropped, never stored as
  a metric).

## Sources at a glance

| Source                  | Where it comes from                                    | Table                | Kind     |
| ----------------------- | ------------------------------------------------------ | -------------------- | -------- |
| `azure_monitor`         | ARM metrics API via VM managed identity                | `raw.metric_reading` | interval |
| `sql_diagnostics`       | DMVs in Azure SQL (`sys.dm_db_resource_stats`, `sys.dm_exec_sessions`) | `raw.metric_reading` | interval |
| `query_store_execution` | Query Store (`sys.query_store_runtime_stats`)          | `raw.metric_reading` | interval |
| `workload_self`         | Counters derived from the workload observation spool    | `raw.metric_reading` | interval |
| `workload_observation`  | One JSON line per operation from the generator          | `raw.metric_sample`  | sample   |

## Canonical source policy

Some codes (`cpu_percent`, `data_io_percent`, `log_write_percent`,
`sessions_count`) arrive from more than one source. They are **not averaged
together** — granularity, window and meaning differ. For each window the
aggregator picks one canonical source by priority:

```
azure_monitor > sql_diagnostics > query_store_execution > workload_self > workload_observation
```

The winner per metric is stored in `feat.metric_hourly.canonical_sources`
(JSON: `{metric_code: source}`). Both raw copies still coexist in
`raw.metric_reading`, keyed by `(…, metric_code, observed_at_utc, source)`.

## `azure_monitor` — control-plane / platform metrics

Read with the VM's system-assigned managed identity (needs
`AZURE_SQL_RESOURCE_ID`, or `AZURE_SUBSCRIPTION_ID` + `AZURE_RESOURCE_GROUP` +
`AZURE_LOCATION`). Failures degrade gracefully.

| Raw Azure metric             | `metric_code`        | Unit    | Meaning                                 |
| ---------------------------- | -------------------- | ------- | --------------------------------------- |
| `cpu_percent`                | `cpu_percent`        | percent | % of the database CPU limit in use      |
| `physical_data_read_percent` | `data_io_percent`    | percent | % of the data-file I/O limit in use     |
| `log_write_percent`          | `log_write_percent`  | percent | % of the transaction-log write limit    |
| `storage_percent`            | `storage_percent`    | percent | % of max storage in use                 |
| `storage`                    | `storage_used_bytes` | bytes   | **Data space used** (not allocated)     |
| `workers_percent`            | `workers_percent`    | percent | % of the worker-thread limit in use     |
| `sessions_percent`           | `sessions_percent`   | percent | % of max concurrent sessions in use     |
| `sessions_count`             | `sessions_count`     | count   | Active sessions                         |
| `deadlock`                   | `deadlock_count`     | count   | Deadlocks in the interval               |
| `cpu_limit`                  | `cpu_limit_vcores`   | count   | Control-plane view of the CPU cap       |
| `cpu_used`                   | `cpu_used_vcores`    | count   | vCores in use (billing view)            |
| `app_cpu_billed`             | `app_cpu_billed`     | count   | vCore-seconds billed for the app        |
| `app_cpu_percent`            | `app_cpu_percent`    | percent | Billed CPU as % of the app quota        |
| `connection_successful`      | `connection_successful_count` | count | Successful logins             |
| `connection_failed`          | `connection_failed_count`     | count | Failed logins                 |
| `blocked_by_firewall`        | `blocked_by_firewall_count`   | count | Logins rejected by firewall   |
| `tempdb_log_used_percent`    | `tempdb_log_used_percent`     | percent | TempDB log usage           |
| `tempdb_data_size`           | `tempdb_data_size_bytes`      | bytes | TempDB data size              |
| `sqlserver_process_core_percent`   | `process_core_percent`   | percent | SQL process CPU          |
| `sqlserver_process_memory_percent` | `process_memory_percent` | percent | SQL process memory   |
| `free_amount_remaining`      | `free_vcore_seconds_remaining` | count | Free-tier budget left (900 s grain) |
| `free_amount_consumed`       | `free_vcore_seconds_consumed`  | count | Free-tier budget used (900 s grain) |

> The two `free_amount_*` metrics **only accept a 15-minute grain**. They are
> queried separately (`BUDGET_GRANULARITY_SECONDS = 900`); asking for them in
> the same call as the 60 s metrics returns `HTTP 400 Commonly allowed time
> grains`. Read them as a single point, never as a rate (`SIZING.md` §3.3).

> `dtu_consumption_percent` is intentionally not queried: it does not exist on
> vCore/serverless and Azure rejects the whole batch (HTTP 400) if any requested
> metric name is invalid.

## `sql_diagnostics` — inside the system under test

Read with the contained workload login (needs `VIEW DATABASE STATE`, migration
004). Bounded `TOP (1)` queries.

| Raw column                 | `metric_code`       | Unit    | Meaning                              |
| -------------------------- | ------------------- | ------- | ------------------------------------ |
| `avg_cpu_percent`          | `cpu_percent`       | percent | Average CPU over the last interval   |
| `avg_data_io_percent`      | `data_io_percent`   | percent | Average data I/O over the interval   |
| `avg_log_write_percent`    | `log_write_percent` | percent | Average log write over the interval  |
| `avg_memory_usage_percent` | `memory_percent`    | percent | Average memory usage over the interval |
| `max_worker_percent`       | `workers_percent`   | percent | Worker-thread ceiling reached        |
| `max_session_percent`      | `sessions_percent`  | percent | Session ceiling reached              |
| `avg_instance_cpu_percent` | `instance_cpu_percent` | percent | CPU of the instance hosting the DB |
| `sessions` (count)         | `sessions_count`    | count   | Active user sessions (own SPID excluded) |

`max_worker_percent` and `max_session_percent` reuse the Azure Monitor codes on
purpose: same meaning, same unit, and the aggregator keeps a single canonical
source per window.

## `sql_diagnostics` — capacity, waits, IO (levels 0–3)

The same source also emits the capacity ceilings, the wait breakdown, the file
IO deltas and a slow inventory. Full rationale and the sizing rules live in
**`SIZING.md`**; this is the short list.

| Group | Codes | Cadence |
| ----- | ----- | ------- |
| Caps (level 0) | `cpu_limit_vcores`, `max_dop`, `max_sessions`, `max_memory_percent`, `max_memory_grant_percent`, `log_rate_cap_bytes_per_sec`, `instance_max_log_rate_bytes_per_sec` | 30 min |
| Waits (level 2) | `wait_{cpu,log,lock,io,memory,other}_ms_per_sec`, `wait_critical_ms_per_sec` | 60 s |
| Gauges (level 2) | `page_life_expectancy_sec`, `memory_grants_pending`, `total_server_memory_bytes`, `database_cache_memory_bytes` | 60 s |
| File IO (level 2) | `rows_read_bytes_per_sec`, `rows_write_bytes_per_sec`, `rows_stall_ms_per_sec`, `log_stall_ms_per_sec` | 60 s |
| Congestion (level 2) | `running_requests`, `blocked_requests`, `longest_running_ms` (user sessions only) | 60 s |
| Inventory (level 3) | `log_size_bytes`, `log_vlf_count`, `index_{seeks,scans,lookups,updates}_total`, `tuning_recommendations_count`, `tempdb_used_bytes`, `runnable_tasks`, `work_queue_count` | 30 min |

Rules that make these numbers trustworthy:

- Monotonic counters (waits, file IO) are stored as **per-second deltas**. If a
  counter goes backwards (a pause/resume resets `dm_os_wait_stats`) the cycle is
  dropped instead of emitting a negative or a fake spike.
- Background waits are excluded (`SLEEP_`, `HADR_`, `XE_`, `BROKER_`, `QDS_`,
  `PREEMPTIVE_`, `LAZYWRITER`, …). Unfiltered, `SOS_WORK_DISPATCHER` alone was
  222 s in a 10 s sample.
- `sys.dm_os_performance_counters` is collected as **gauges only**: its
  per-second counters do not behave as rates on Azure SQL (measured: no reaction
  to a 20 k-row insert).
- `dm_io_virtual_file_stats` log *bytes* under-report on Azure SQL; only the
  stall accumulator is kept. Log throughput is read as
  `log_write_percent × log_rate_cap_bytes_per_sec`.

## `query_store_execution` — query latency (aggregated)

Weighted average of the most recent Query Store intervals
(`avg_duration × count_executions`). `source_payload.percentile_method` is
always `not_available`: interval aggregates **must never** become p95/p99.

| Column         | `metric_code`           | Unit         | Meaning                              |
| -------------- | ----------------------- | ------------ | ------------------------------------ |
| `avg_duration` | `query_duration_avg_ms` | milliseconds | Mean execution duration per interval |

## `workload_self` — offered load and outcomes

| `metric_code`               | Unit            | Meaning                                    |
| --------------------------- | --------------- | ------------------------------------------ |
| `workload_intensity`        | ops_per_min     | Operations **offered** per minute           |
| `offered_operations`        | count           | Operations attempted in the window          |
| `successful_operations`     | count           | Operations that succeeded                   |
| `failed_operations`         | count           | Operations that failed                      |
| `error_rate`                | ratio (0–1)     | `failed / offered`                          |
| `throughput_ops_per_second` | ops_per_second  | Successful operations per second            |

## `workload_observation` — per-operation samples

One row **per operation** in `raw.metric_sample` (the exact-percentile source).
Each row carries `source_sample_id` and a payload with `op` (template key) and
`ok`.

| Raw field     | `metric_code`                    | Unit         | Meaning                          |
| ------------- | -------------------------------- | ------------ | -------------------------------- |
| `duration_ms` | `workload_operation_latency_ms`  | milliseconds | Client-side latency of one operation |

> This is **operation** latency (may include several statements, commit,
> serialization and network), not the internal latency of a single query. For
> per-statement latency the workload must emit one sample per statement.

## Catalog (canonical codes)

| `metric_code`                    | Unit           | Bounds | In features |
| -------------------------------- | -------------- | ------ | ----------- |
| `cpu_percent`                    | percent        | 0–100  | yes         |
| `storage_percent`                | percent        | 0–100  | yes         |
| `storage_used_bytes`             | bytes          | ≥ 0    | yes         |
| `storage_capacity_bytes`         | bytes          | ≥ 0    | yes         |
| `data_io_percent`                | percent        | 0–100  | yes         |
| `log_write_percent`              | percent        | 0–100  | yes         |
| `memory_percent`                 | percent        | 0–100  | yes         |
| `sessions_count`                 | count          | ≥ 0    | yes         |
| `sessions_percent`               | percent        | 0–100  | yes         |
| `workers_percent`                | percent        | 0–100  | yes         |
| `deadlock_count`                 | count          | ≥ 0    | yes         |
| `query_duration_avg_ms`          | milliseconds   | ≥ 0    | yes         |
| `workload_operation_latency_ms`  | milliseconds   | ≥ 0    | yes         |
| `workload_intensity`             | ops_per_min    | ≥ 0    | yes         |
| `offered_operations`             | count          | ≥ 0    | yes         |
| `successful_operations`          | count          | ≥ 0    | yes         |
| `failed_operations`              | count          | ≥ 0    | yes         |
| `error_rate`                     | ratio          | 0–1    | yes         |
| `throughput_ops_per_second`      | ops_per_second | ≥ 0    | yes         |
| `dtu_consumption_percent`        | percent        | 0–100  | no          |
| `cpu_limit_vcores`               | count          | 0–64   | no          |
| `cpu_used_vcores`                | count          | 0–64   | no          |
| `max_dop`                        | count          | 0–64   | no          |
| `max_sessions`                   | count          | 0–32767| no          |
| `max_memory_percent`             | percent        | 0–100  | no          |
| `max_memory_grant_percent`       | percent        | 0–100  | no          |
| `instance_cpu_percent`           | percent        | 0–100  | no          |
| `log_rate_cap_bytes_per_sec`     | bytes/s        | ≥ 0    | no          |
| `instance_max_log_rate_bytes_per_sec` | bytes/s    | ≥ 0    | no          |
| `wait_{cpu,log,lock,io,memory,other}_ms_per_sec` | ms/s | ≥ 0 | no |
| `wait_critical_ms_per_sec`       | ms/s           | ≥ 0    | no          |
| `page_life_expectancy_sec`       | seconds        | ≥ 0    | no          |
| `memory_grants_pending`          | count          | ≥ 0    | no          |
| `total_server_memory_bytes`      | bytes          | ≥ 0    | no          |
| `database_cache_memory_bytes`    | bytes          | ≥ 0    | no          |
| `rows_{read,write}_bytes_per_sec`| bytes/s        | ≥ 0    | no          |
| `{rows,log}_stall_ms_per_sec`    | ms/s           | ≥ 0    | no          |
| `running_requests`, `blocked_requests` | count    | ≥ 0    | no          |
| `longest_running_ms`             | milliseconds   | ≥ 0    | no          |
| `log_size_bytes`                 | bytes          | ≥ 0    | no          |
| `log_vlf_count`                  | count          | ≥ 0    | no          |
| `index_{seeks,scans,lookups,updates}_total` | count | ≥ 0   | no          |
| `tuning_recommendations_count`   | count          | ≥ 0    | no          |
| `tempdb_used_bytes`              | bytes          | ≥ 0    | no          |
| `runnable_tasks`, `work_queue_count` | count      | ≥ 0    | no          |
| `app_cpu_billed`                 | count          | ≥ 0    | no          |
| `app_cpu_percent`                | percent        | 0–100  | no          |
| `connection_successful_count`    | count          | ≥ 0    | no          |
| `connection_failed_count`        | count          | ≥ 0    | no          |
| `blocked_by_firewall_count`      | count          | ≥ 0    | no          |
| `tempdb_log_used_percent`        | percent        | 0–100  | no          |
| `tempdb_data_size_bytes`         | bytes          | ≥ 0    | no          |
| `process_core_percent`           | percent        | 0–100  | no          |
| `process_memory_percent`         | percent        | 0–100  | no          |
| `free_vcore_seconds_remaining`   | count          | ≥ 0    | no          |
| `free_vcore_seconds_consumed`    | count          | ≥ 0    | no          |

Codes outside this table are rejected by the quality policy and recorded in
`ops.metric_rejection`; they never reach `raw.metric_reading`.

## Percentiles (explicit methods)

- **CPU** (`cpu_p50/p95/p99`): **empirical quantiles of the 60 s interval
  readings**, method `empirical_interval_quantile`. These are *not* exact
  per-moment quantiles inside the engine.
- **Latency** (`workload_operation_latency_p95_ms`): `exact_sample`, computed
  from individual operation samples.
- When the inputs are missing the method is `not_available` and the value stays
  `NULL` — a number is never invented.

## Coverage and quality

- Per reading: `catalog_version`, `source_granularity_seconds`,
  `coverage_start_utc`, `coverage_end_utc`.
- Per hour: `coverage_json` holds a missing fraction for **every** catalog code
  (not only CPU); `missing_fraction` is kept for CPU for backward compatibility.
- Window status (`quality.window_quality`):
  - `not_available` — a mandatory metric (`cpu_percent`, `data_io_percent`,
    `log_write_percent`, `workload_intensity`) has no readings;
  - `degraded` — mandatory or optional coverage below 70 % (or storage
    inconsistency);
  - `usable` — mandatory metrics covered, optional ones dense.
- `feat.v_training_base.is_trainable` = `quality_status = 'usable'`, so the
  training consumer decides explicitly whether to include degraded windows.

## Hourly features — `feat.metric_hourly`

One row per `(experiment_uid, resource_uid, period_start_utc)` (idempotent):

| Column group | Columns |
| ------------ | ------- |
| CPU          | `cpu_avg`, `cpu_max`, `cpu_p50/p95/p99`, `cpu_p50/p95/p99_method`, `cpu_p95_over_interval_readings` |
| I/O + memory | `data_io_avg`, `log_write_avg`, `memory_avg` |
| Sessions     | `sessions_avg_count` (count), `sessions_utilization_avg_pct` (percent) |
| Workers      | `workers_avg` |
| Storage      | `storage_used_pct`, `storage_used_bytes`, `storage_capacity_bytes`, `storage_growth_bytes` |
| Workload     | `workload_intensity`, `offered_operations`, `successful_operations`, `failed_operations`, `error_rate`, `throughput_ops_per_second`, `deadlock_count` |
| Latency      | `workload_operation_latency_p95_ms`, `latency_percentile_method` |
| Provenance   | `canonical_sources` (jsonb), `catalog_version` |
| Quality      | `sample_count`, `missing_fraction`, `coverage_json`, `quality_status`, `quality_warnings` |

Deprecated (kept, no longer written): `sessions_avg` (mixed count/%),
`latency_p95_ms` (unclear latency semantics).

## Not yet covered (deferred, by design)

- **Full resource configuration snapshot** (`service_tier`, `compute_model`,
  `sku_name`, `vcore_min/max`, `auto_pause_delay`, `backup_retention`, …) and
  **price/cost** (`price_per_vcore_hour`, `actual/estimated_cost_interval`,
  `currency`, `price_catalog_version`). The *hard limits* that bind a query
  (`cpu_limit`, `max_dop`, `max_sessions`, memory caps, log rate) are already
  collected (`SIZING.md` §6); what is missing is the SKU/price contract, which
  needs ARM / Cost-Management reads and a pricing catalog.
- **Anomaly labels** (`incident_uid`, `anomaly_label`, `severity`, …) — no
  ground truth yet; anomaly detection is unsupervised (rolling median/MAD,
  z-score, forecast residual) until labels exist.
- **Network metrics** (`network_latency_ms`, `client_round_trip_ms`,
  egress bytes) — not required for the first CPU/workload/storage forecast.

## Where to look

- Supabase SQL Editor, schemas `raw`, `feat`, `ops` (Table Editor opens on
  `public`; pick the schema explicitly).
- VM: `sudo journalctl -u cco-collector -f`.
- Telegram: `/status`, `/spool`.
