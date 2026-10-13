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
| `sessions` (count)         | `sessions_count`    | count   | Active user sessions (own SPID excluded) |

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

- **Resource configuration snapshot** (`service_tier`, `compute_model`,
  `sku_name`, `vcore_min/max`, `max_storage_bytes`, `auto_pause_delay`, …) and
  **price/cost** (`price_per_vcore_hour`, `actual/estimated_cost_interval`,
  `currency`, `price_catalog_version`). These need ARM / Cost-Management reads
  and a pricing catalog; they are the next iteration for the cost-optimization
  model. Only `storage_capacity_bytes` is available today (from config).
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
