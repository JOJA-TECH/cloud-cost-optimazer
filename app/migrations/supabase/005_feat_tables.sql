-- ==========================================================================
-- 005_feat_tables.sql
-- Hourly feature store. One row per (experiment, resource, period_start).
-- Percentiles are only written when an exact method produced them
-- (see raw.metric_sample); otherwise NULL and a warning is recorded.
-- ==========================================================================

create table if not exists feat.metric_hourly (
    feature_row_uid     uuid primary key default gen_random_uuid(),
    experiment_uid      uuid not null references experiment.experiment_run (experiment_uid),
    resource_uid        uuid not null,
    period_start_utc    timestamptz not null,
    period_end_utc      timestamptz not null,
    workload_class      text not null,
    cpu_avg             double precision,
    cpu_p50             double precision,
    cpu_p95             double precision,
    cpu_p99             double precision,
    cpu_max             double precision,
    data_io_avg         double precision,
    log_write_avg       double precision,
    sessions_avg        double precision,
    workers_avg         double precision,
    storage_used_pct    double precision,
    storage_used_bytes  bigint,
    storage_growth_bytes bigint,
    workload_intensity  double precision,
    deadlock_count      bigint not null default 0,
    latency_p95_ms      double precision,
    sample_count        bigint not null default 0,
    missing_fraction    double precision not null default 0,
    quality_status      text not null default 'usable'
        check (quality_status in ('usable','degraded','not_available','invalid')),
    quality_warnings    text[] not null default '{}',
    computed_at_utc     timestamptz not null default now(),
    check (period_end_utc = period_start_utc + interval '1 hour')
);

alter table feat.metric_hourly
    add constraint uq_metric_hourly_idem
    unique (experiment_uid, resource_uid, period_start_utc);