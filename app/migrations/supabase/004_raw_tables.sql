-- ==========================================================================
-- 004_raw_tables.sql
-- Append-only metric persistency.
--   raw.metric_reading : interval aggregates (avg/min/max/total/count); valid
--                        for trends, NOT for exact percentiles.
--   raw.metric_sample  : individual samples; THE source for exact percentiles.
-- Both are uniquely keyed so collector retries never duplicate rows.
-- ==========================================================================

create table if not exists raw.metric_reading (
    metric_reading_uid uuid primary key default gen_random_uuid(),
    experiment_uid     uuid not null references experiment.experiment_run (experiment_uid),
    workload_run_uid   uuid references experiment.workload_run (workload_run_uid),
    collector_run_uid  uuid not null references ops.collector_run (collector_run_uid),
    resource_uid       uuid not null,
    metric_code        text not null,
    observed_at_utc    timestamptz not null,
    interval_seconds   integer not null,
    value              double precision not null,
    unit               text not null,
    source             text not null,
    source_record_id   text,
    source_payload_json jsonb not null default '{}'::jsonb,
    inserted_at_utc    timestamptz not null default now(),
    quality_status     text not null default 'usable'
        check (quality_status in ('usable','degraded','not_available','invalid')),
    quality_warnings   text[] not null default '{}'
);

-- Idempotency key for retries (ON CONFLICT DO NOTHING).
alter table raw.metric_reading
    add constraint uq_metric_reading_idem
    unique (experiment_uid, resource_uid, metric_code, observed_at_utc, source);

create table if not exists raw.metric_sample (
    metric_sample_uid uuid primary key default gen_random_uuid(),
    experiment_uid    uuid not null references experiment.experiment_run (experiment_uid),
    workload_run_uid  uuid references experiment.workload_run (workload_run_uid),
    resource_uid      uuid not null,
    metric_code       text not null,
    observed_at_utc   timestamptz not null,
    value             double precision not null,
    unit              text not null,
    source            text not null,
    source_sample_id  text,
    inserted_at_utc   timestamptz not null default now()
);

-- Partial unique key: only dedups when the source provides a sample id.
create unique index if not exists uq_metric_sample_idem
    on raw.metric_sample (experiment_uid, resource_uid, metric_code, observed_at_utc, source_sample_id)
    where source_sample_id is not null;