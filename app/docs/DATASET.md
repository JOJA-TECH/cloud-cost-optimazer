# Dataset

Two schemas, two purposes. Nothing is duplicated between them except the
correlation ids (`experiment_uid`, `resource_uid`).

## Azure SQL — `loadtest` (system under test)

| Object                        | Purpose                                              |
| ----------------------------- | ---------------------------------------------------- |
| `experiment_context`          | Experiment metadata mirrored from Supabase.          |
| `fact_events`                 | Append-only event rows written by the workload.      |
| `fact_events_payload`         | Payload bodies + sha256, joined by `event_id`.       |
| `workload_control`            | State machine enforced by an `INSTEAD OF UPDATE` trigger, with an audit table. |
| `query_templates`             | Named operations the workload draws from (seeded).   |

The contained workload login has `SELECT/INSERT/UPDATE/DELETE/REFERENCES/EXECUTE`
on `loadtest`, plus `VIEW DATABASE STATE` for the collector's DMV reads, and is
denied `VIEW DEFINITION`.

## Supabase — analytical store

### `experiment`
- `experiment_run` — one row per experiment (status, workload class, seed,
  timing, `final_flush_status`).
- `workload_run` — one row per phase (class, intensity, concurrency, counters).

### `ops`
- `collector_run` — one row per collection cycle (`ok`/`partial`/`error`,
  rows observed/written, error message).
- `schema_migrations` — applied migration hashes.
- `metric_rejection` — unknown provider metrics (raw name, unit, reason,
  catalog version); captured once per experiment/source/name/version instead of
  being silently dropped.

### `raw` (append-only)
- `metric_reading` — interval aggregates. Columns: `metric_code`,
  `observed_at_utc`, `interval_seconds`, `value`, `unit`, `source`,
  `source_record_id`, `source_payload_json`, `quality_status`,
  `quality_warnings`, plus provenance `catalog_version`,
  `source_granularity_seconds`, `coverage_start_utc`, `coverage_end_utc`.
  Unique: `(experiment_uid, resource_uid, metric_code, observed_at_utc, source)`.
- `metric_sample` — individual observations (the exact-percentile source).
  Unique on the same columns plus `source_sample_id`.

### `feat`
- `metric_hourly` — one row per `(experiment, resource, period_start_utc)`.
  CPU (`cpu_avg/max/p50/p95/p99` + methods), IO/log/memory, split sessions
  (`sessions_avg_count`, `sessions_utilization_avg_pct`), workers, storage
  (`storage_used_pct`, `storage_used_bytes`, `storage_capacity_bytes`,
  `storage_growth_bytes`), workload outcomes (`workload_intensity`,
  `offered/successful/failed_operations`, `error_rate`,
  `throughput_ops_per_second`, `deadlock_count`),
  `workload_operation_latency_p95_ms`, plus provenance (`canonical_sources`,
  `catalog_version`) and quality (`sample_count`, `missing_fraction`,
  `coverage_json`, `quality_status`, `quality_warnings`).
- `v_training_base` (view) — hourly features joined with next-hour targets and
  an `is_trainable` flag (`quality_status = 'usable'`).

### Metric catalog

Canonical codes, units and bounds live in `src/metrics/normalizer.py`
(`METRIC_CATALOG`) and are versioned by `METRIC_CATALOG_VERSION`. Raw provider
names are mapped through `NORMALIZATION_MAP`; unknown names are written to
`ops.metric_rejection` and never stored as metrics.

Metrics present in more than one source are **not** averaged together; the
aggregator selects a canonical source by priority (Azure Monitor > SQL DMVs >
Query Store > workload self) and records it in
`feat.metric_hourly.canonical_sources`.

Percentile methods are explicit: CPU percentiles are
`empirical_interval_quantile` (60 s readings, not internal moments); the
workload operation latency p95 is `exact_sample`. Query Store aggregates are
persisted as readings and are never used to fabricate p95/p99.

## Correlation

- `experiment_uid` — random UUID, generated once by Terraform.
- `resource_uid` — deterministic UUID derived from the Azure resource id
  (`Config.resource_uid()`), stable across restarts.