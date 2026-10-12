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

### `raw` (append-only)
- `metric_reading` — interval aggregates. Columns: `metric_code`,
  `observed_at_utc`, `interval_seconds`, `value`, `unit`, `source`,
  `source_record_id`, `source_payload_json`, `quality_status`,
  `quality_warnings`. Unique:
  `(experiment_uid, resource_uid, metric_code, observed_at_utc, source)`.
- `metric_sample` — individual observations (the exact-percentile source).
  Unique on the same columns plus `source_sample_id`.

### `feat`
- `metric_hourly` — one row per `(experiment, resource, period_start_utc)`.
  Includes `cpu_avg/p50/p95/p99/max`, IO, log, sessions, workers, storage,
  `workload_intensity`, `deadlock_count`, `latency_p95_ms`, plus `sample_count`,
  `missing_fraction`, `quality_status` and `quality_warnings`. Percentiles are
  `NULL` when only aggregates were available.
- `v_training_base` (view) — joins hourly features with the workload class for
  model training.

### Metric catalog

Canonical codes and bounds live in `src/metrics/normalizer.py`
(`METRIC_CATALOG`). Raw provider names are mapped through `NORMALIZATION_MAP`;
unknown names are dropped, never stored.

Exact percentile policy: `exact_sample` when individual samples exist,
`not_available` otherwise. Query Store aggregates are persisted as readings and
are explicitly never used to fabricate p95/p99.

## Correlation

- `experiment_uid` — random UUID, generated once by Terraform.
- `resource_uid` — deterministic UUID derived from the Azure resource id
  (`Config.resource_uid()`), stable across restarts.