-- ==========================================================================
-- 006_views.sql
-- Training view: features at hour h joined with targets at hour h+1.
-- No future information is used inside the feature columns.
-- ==========================================================================

create or replace view feat.v_training_base as
select
    f.*,
    n.cpu_avg            as target_cpu_avg_next1h,
    n.cpu_p95            as target_cpu_p95_next1h,
    n.workload_intensity as target_workload_next1h,
    n.storage_used_bytes as target_storage_next1h
from feat.metric_hourly f
left join feat.metric_hourly n
    on n.experiment_uid = f.experiment_uid
   and n.resource_uid = f.resource_uid
   and n.period_start_utc = f.period_start_utc + interval '1 hour'
where f.period_end_utc = f.period_start_utc + interval '1 hour';

create or replace view ops.v_sync_status as
select
    cr.collector_run_uid,
    cr.experiment_uid,
    cr.source,
    cr.started_at_utc,
    cr.finished_at_utc,
    cr.status,
    cr.rows_observed,
    cr.rows_written,
    cr.error_message
from ops.collector_run cr
order by cr.started_at_utc desc;

create or replace view ops.v_collector_latest as
select distinct on (cr.source)
    cr.source,
    cr.collector_run_uid,
    cr.started_at_utc,
    cr.finished_at_utc,
    cr.status,
    cr.rows_observed,
    cr.rows_written,
    cr.error_message
from ops.collector_run cr
order by cr.source, cr.started_at_utc desc;