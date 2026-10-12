# Architecture

## Roles

- **Azure SQL (system under test)** — the workload runs here. It holds the
  `loadtest` schema (experiment context, fact tables, query templates) plus
  `loadtest.workload_control`, the operational state machine. It is the only
  place the load is generated, and the workload login cannot touch anything
  outside `loadtest`.
- **Supabase PostgreSQL (analytical persistence)** — long-lived storage for
  readings, samples, hourly features and the collector registry. It never talks
  to Azure SQL.
- **Collector VM** — the only component with both connection strings. It runs
  the load generator, the metric collector, the Telegram bot and the supervisor.

## Data flow

```
workload.runner ──ops──▶ Azure SQL (loadtest)
       │                     │
       │ per-op observation  │ DMV / Query Store / Azure Monitor
       ▼                     ▼
   observe spool  ───▶  metrics.collector ──▶ sync.writer ──▶ Supabase raw.*
                                                 │  (on failure)
                                                 ▼
                                             local spool
                                                 │ retry
                                                 ▼
                                        Supabase raw.* (idempotent)

   metrics.aggregator ──5 min──▶ feat.metric_hourly
```

## Metric quality and percentiles

- Every reading passes `metrics.quality.evaluate_reading`, which classifies it
  as `usable`, `degraded`, `not_available` or `invalid` against the catalog
  bounds in `metrics.normalizer.METRIC_CATALOG`. Values far outside bounds are
  `invalid` and excluded from features; slightly outside are `degraded`.
- **Exact percentiles require individual samples.** Platform metrics and DMVs
  only expose interval aggregates (avg/min/max), so their percentile method is
  `not_available` and no value is stored. Exact p50/p95/p99 for CPU come from
  `raw.metric_sample`, and latency p95 from workload self-observations. When no
  samples exist the column stays `NULL` and a warning is recorded — a number is
  never invented.

## Idempotency and back-pressure

- `raw.metric_reading` has a unique key on
  `(experiment_uid, resource_uid, metric_code, observed_at_utc, source)`;
  `raw.metric_sample` dedups on the same columns plus `source_sample_id`.
  Inserts use `ON CONFLICT DO NOTHING`, so retries never duplicate data.
- If Supabase is unreachable, `sync.writer` serializes the batch into a
  checksummed local spool file and the cycle is marked `partial`. The next
  successful cycle drains the spool first. The spool has a hard size cap; when
  it is exceeded the condition is reported (never silently dropped).

## Workload state machine

`loadtest.workload_control` is authoritative and is enforced by an
`INSTEAD OF UPDATE` trigger. `workload.controller.WorkloadController` mirrors
each transition to `experiment.experiment_run` on Supabase:

```
CREATED ─▶ RUNNING ─▶ PAUSING ─▶ PAUSED
              │            │         │
              └────────────┴─────────┴──▶ STOPPING ─▶ STOPPED
                                   (any) ─▶ FAILED
```

`PAUSING → RUNNING` (resume) and `PAUSING → PAUSED` (ack) are allowed; a stop
from `PAUSING` is done by resuming then stopping. See `docs/OPERATIONS.md`.

## Safe shutdown

`src.shutdown.safe_stop` is the only path to a terminal record: request
`STOPPING`, confirm `STOPPED`, flush the spool within
`FINAL_FLUSH_TIMEOUT_SECONDS`. If the spool cannot be emptied the experiment is
recorded as `STOPPED_WITH_PENDING_BUFFER` and the data stays on disk.
`--supervise` forces this stop once the experiment exceeds `AUTO_STOP_AFTER_HOURS`.