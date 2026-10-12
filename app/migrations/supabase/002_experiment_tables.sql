-- ==========================================================================
-- 002_experiment_tables.sql
-- Authoritative experiment + workload run records.
-- ==========================================================================

create table if not exists experiment.experiment_run (
    experiment_uid  uuid primary key,
    resource_uid    uuid not null,
    started_at_utc  timestamptz not null,
    stopped_at_utc  timestamptz,
    workload_class  text not null,
    random_seed     integer,
    status          text not null check (
                        status in (
                            'CREATED','RUNNING','PAUSING','PAUSED',
                            'STOPPING','STOPPED','FAILED',
                            'STOPPED_WITH_PENDING_BUFFER'
                        )
                    ),
    stop_reason     text,
    config_json     jsonb not null default '{}'::jsonb,
    created_at_utc  timestamptz not null default now(),
    updated_at_utc  timestamptz not null default now(),
    final_flush_status text
);

create table if not exists experiment.workload_run (
    workload_run_uid    uuid primary key,
    experiment_uid      uuid not null references experiment.experiment_run (experiment_uid),
    workload_class      text not null,
    started_at_utc      timestamptz not null,
    finished_at_utc     timestamptz,
    duration_seconds    numeric,
    intensity           numeric not null,
    concurrency         integer not null,
    pattern_params_json jsonb not null default '{}'::jsonb,
    operations_total    bigint not null default 0,
    errors_total        bigint not null default 0,
    status              text not null check (
                            status in ('CREATED','RUNNING','PAUSED','FINISHED','STOPPED','FAILED')
                        ),
    stop_reason         text
);