-- ==========================================================================
-- 001_schemas.sql
-- Analytical persistence schemas for the two-database design.
-- Idempotent: safe to run as many times as needed.
-- ==========================================================================

create extension if not exists pgcrypto;

create schema if not exists experiment;
create schema if not exists raw;
create schema if not exists feat;
create schema if not exists ml;
create schema if not exists ops;

comment on schema experiment is 'Experiment and workload run bookkeeping (authoritative copy lives here, Azure SQL mirrors it)';
comment on schema raw is 'Append-only metric readings and samples persisted from the collector';
comment on schema feat is 'Hourly feature store and the training view';
comment on schema ml is 'Reserved for model registry, forecasts and evaluations';
comment on schema ops is 'Collector runs, sync health and schema migrations';