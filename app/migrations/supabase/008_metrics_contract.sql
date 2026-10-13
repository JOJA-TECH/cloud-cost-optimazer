-- ==========================================================================
-- 008_metrics_contract.sql
-- Metric contract v2:
--   * per-reading provenance: catalog version, real source granularity and
--     coverage window;
--   * ops.metric_rejection: unknown provider metrics are recorded, not lost;
--   * feat.metric_hourly: canonical source map, labelled percentile methods,
--     separated session count/utilization, storage capacity, workload outcome
--     counters and per-metric coverage.
-- Additive and idempotent.
-- ==========================================================================

alter table raw.metric_reading
    add column if not exists catalog_version text,
    add column if not exists source_granularity_seconds integer,
    add column if not exists coverage_start_utc timestamptz,
    add column if not exists coverage_end_utc timestamptz;

create table if not exists ops.metric_rejection (
    rejection_uid     uuid primary key default gen_random_uuid(),
    experiment_uid    uuid references experiment.experiment_run (experiment_uid),
    source            text not null,
    raw_metric_name   text not null,
    raw_unit          text,
    observed_at_utc   timestamptz,
    payload_json      jsonb not null default '{}'::jsonb,
    reason            text not null,
    catalog_version   text,
    inserted_at_utc   timestamptz not null default now()
);

-- One row per (experiment, source, raw name, catalog version): a provider
-- rename is captured once instead of on every collection cycle.
create unique index if not exists uq_metric_rejection
    on ops.metric_rejection (experiment_uid, source, raw_metric_name, catalog_version);

alter table feat.metric_hourly
    add column if not exists cpu_p50_method text,
    add column if not exists cpu_p95_method text,
    add column if not exists cpu_p99_method text,
    add column if not exists cpu_p95_over_interval_readings double precision,
    add column if not exists memory_avg double precision,
    add column if not exists sessions_avg_count double precision,
    add column if not exists sessions_utilization_avg_pct double precision,
    add column if not exists storage_capacity_bytes bigint,
    add column if not exists offered_operations bigint,
    add column if not exists successful_operations bigint,
    add column if not exists failed_operations bigint,
    add column if not exists error_rate double precision,
    add column if not exists throughput_ops_per_second double precision,
    add column if not exists workload_operation_latency_p95_ms double precision,
    add column if not exists latency_percentile_method text,
    add column if not exists coverage_json jsonb not null default '{}'::jsonb,
    add column if not exists canonical_sources jsonb not null default '{}'::jsonb,
    add column if not exists catalog_version text;

-- Old ambiguous columns are kept for backward compatibility but no longer
-- written: sessions_avg mixed count with percentage, latency_p95_ms did not
-- distinguish client operation latency from query latency.
comment on column feat.metric_hourly.sessions_avg is
    'DEPRECATED: use sessions_avg_count / sessions_utilization_avg_pct';
comment on column feat.metric_hourly.latency_p95_ms is
    'DEPRECATED: use workload_operation_latency_p95_ms';

-- Training view: features at hour h joined with targets at hour h+1, plus an
-- explicit flag so consumers can choose whether to include non-usable windows.
-- Dropped and recreated because `f.*` gained columns (CREATE OR REPLACE cannot
-- reorder view columns).
drop view if exists feat.v_training_base;
create view feat.v_training_base as
select
    f.*,
    (f.quality_status = 'usable')         as is_trainable,
    n.cpu_avg                             as target_cpu_avg_next1h,
    n.cpu_p95                             as target_cpu_p95_next1h,
    n.workload_intensity                  as target_workload_next1h,
    n.storage_used_bytes                  as target_storage_next1h
from feat.metric_hourly f
left join feat.metric_hourly n
    on n.experiment_uid = f.experiment_uid
   and n.resource_uid = f.resource_uid
   and n.period_start_utc = f.period_start_utc + interval '1 hour'
where f.period_end_utc = f.period_start_utc + interval '1 hour';

