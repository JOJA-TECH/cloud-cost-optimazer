-- ==========================================================================
-- 003_ops_tables.sql
-- Collector execution registry (tracability of every sync cycle).
-- ==========================================================================

create table if not exists ops.collector_run (
    collector_run_uid    uuid primary key,
    experiment_uid       uuid not null references experiment.experiment_run (experiment_uid),
    source               text not null,
    started_at_utc       timestamptz not null,
    finished_at_utc      timestamptz,
    status               text not null check (status in ('ok','partial','error','failed_flush')),
    rows_observed        bigint not null default 0,
    rows_written         bigint not null default 0,
    error_message        text,
    request_metadata_json jsonb not null default '{}'::jsonb
);