-- ==========================================================================
-- 007_indexes.sql
-- Query patterns that matter: time + experiment/resource access, idempotency
-- lookups on retry, and sample scans for exact percentiles.
-- ==========================================================================

-- Raw readings: the idempotency unique key already covers
-- (experiment_uid, resource_uid, metric_code, observed_at_utc, source).
-- A time-range helper keeps collector "between" queries fast.
create index if not exists ix_metric_reading_time
    on raw.metric_reading (observed_at_utc);

create index if not exists ix_metric_reading_exp_resource
    on raw.metric_reading (experiment_uid, resource_uid);

-- Samples for percentile computation.
create index if not exists ix_metric_sample_window
    on raw.metric_sample (experiment_uid, resource_uid, metric_code, observed_at_utc);

-- Hourly features join on the next hour window (v_training_base).
create index if not exists ix_metric_hourly_window
    on feat.metric_hourly (experiment_uid, resource_uid, period_start_utc);

-- Collector runs commonly read "latest by source".
create index if not exists ix_collector_run_source_time
    on ops.collector_run (source, started_at_utc desc);

-- Experiment lookups by status for the watchdog / bot.
create index if not exists ix_experiment_run_status
    on experiment.experiment_run (status);

comment on table raw.metric_reading is 'Interval aggregates. NOT valid for exact p95/p99; use raw.metric_sample.';
comment on table raw.metric_sample is 'Individual samples; the only valid source for exact percentiles.';