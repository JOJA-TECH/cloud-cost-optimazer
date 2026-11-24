-- =============================================================================
--  Forecasting extension of the cco_data context (idempotent migration).
--
--  This migration is applied on top of the branch baseline
--  (databases/cco_data/cco_data_schema.sql). The baseline file is NOT edited:
--  the difference introduced by this migration is documented in the header of
--  this file and in README.md.
--
--  Contents
--    1. Extra feature columns required by the forecasting contract
--       (memory, sessions count vs. utilization, errors, latency, workload
--        counters, quality/provenance) on feat.feature_snapshot.
--    2. Extended feat.v_training_base (baseline columns + new ones).
--    3. synthetic schema: scenario_run, scenario_metric_hourly, provenance.
--    4. forecasting schema: run registry, dataset manifests, model metadata.
--    5. Indexes used by the extraction queries.
--
--  Re-running the file is safe: every statement is IF NOT EXISTS / OR REPLACE.
-- =============================================================================

-- =============================================================================
-- 1. Feature store: columnas adicionales
-- =============================================================================
--  El baseline define el contrato minimo del feature store. El contrato de
--  forecasting necesita separar conteo de sesiones de su utilizacion, registrar
--  la memoria, la tasa de error, la latencia media y los contadores de carga
--  aplicada, ademas de la calidad y la procedencia de cada ventana.
ALTER TABLE feat.feature_snapshot
    ADD COLUMN IF NOT EXISTS experiment_uid        uuid,
    ADD COLUMN IF NOT EXISTS resource_uid          uuid,
    ADD COLUMN IF NOT EXISTS workload_class        text,
    ADD COLUMN IF NOT EXISTS memory_percent        numeric(8,4),
    ADD COLUMN IF NOT EXISTS sessions_count        numeric(12,4),
    ADD COLUMN IF NOT EXISTS sessions_percent      numeric(8,4),
    ADD COLUMN IF NOT EXISTS error_rate            numeric(10,6),
    ADD COLUMN IF NOT EXISTS latency_avg_ms        numeric(12,2),
    ADD COLUMN IF NOT EXISTS throughput_ops_per_second numeric(12,4),
    ADD COLUMN IF NOT EXISTS offered_operations    bigint,
    ADD COLUMN IF NOT EXISTS successful_operations bigint,
    ADD COLUMN IF NOT EXISTS failed_operations     bigint,
    ADD COLUMN IF NOT EXISTS canonical_sources     jsonb,
    ADD COLUMN IF NOT EXISTS coverage_json         jsonb,
    ADD COLUMN IF NOT EXISTS quality_status        text,
    ADD COLUMN IF NOT EXISTS quality_warnings      text[];

COMMENT ON COLUMN feat.feature_snapshot.sessions_count IS
    'Conteo promedio de sesiones concurrentes (unidad: count). Deliberadamente separado de sessions_percent (utilizacion).';
COMMENT ON COLUMN feat.feature_snapshot.sessions_percent IS
    'Utilizacion promedio de sesiones en porcentaje de la SKU. No es comparable con sessions_count.';
COMMENT ON COLUMN feat.feature_snapshot.memory_percent IS
    'Porcentaje de uso del buffer pool (dm_db_resource_stats).';
COMMENT ON COLUMN feat.feature_snapshot.error_rate IS
    'failed_operations / offered_operations observados en la ventana.';
COMMENT ON COLUMN feat.feature_snapshot.canonical_sources IS
    'Fuente canonica elegida por metric_code segun la prioridad documentada (azure_monitor > sql_diagnostics > query_store_execution > workload_self > workload_observation).';
COMMENT ON COLUMN feat.feature_snapshot.quality_status IS
    'usable | degraded | not_available | invalid. Derivado de la cobertura por metrica.';

-- =============================================================================
-- 1b. Tablas raw: procedencia de cada lectura
-- =============================================================================
--  El baseline modela raw.metric_reading con la clave natural
--  (resource_id, metric_code, timestamp_utc, interval_seconds). En la
--  telemetria real una misma metrica llega desde varias fuentes (Azure
--  Monitor y las DMVs de SQL Server) con el mismo timestamp, por lo que la
--  clave natural no alcanza y una de las lecturas se perderia.
--
--  La migracion anade la columna `source` a la clave natural (una lectura por
--  fuente y timestamp) y conserva la fuente, la calidad y el payload original
--  de cada observacion. La copia local deja de ser una foto incompleta: cada
--  fila conserva su procedencia, y la eleccion de la fuente canonica se hace
--  despues, al construir las features.
ALTER TABLE raw.metric_reading
    ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS quality_status text,
    ADD COLUMN IF NOT EXISTS quality_warnings text[],
    ADD COLUMN IF NOT EXISTS source_payload jsonb;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'raw.metric_reading'::regclass
          AND contype = 'p'
          AND pg_get_constraintdef(oid) LIKE '%source%'
    ) THEN
        ALTER TABLE raw.metric_reading
            DROP CONSTRAINT IF EXISTS metric_reading_pkey;
        ALTER TABLE raw.metric_reading
            ADD CONSTRAINT metric_reading_pkey
            PRIMARY KEY (resource_id, metric_code, timestamp_utc, interval_seconds, source);
    END IF;
END;
$$;

COMMENT ON COLUMN raw.metric_reading.source IS
    'Fuente de la observacion (azure_monitor, sql_diagnostics, query_store_execution, workload_self). Entra en la clave natural: la misma metrica puede llegar de varias fuentes en el mismo timestamp.';
COMMENT ON COLUMN raw.metric_reading.quality_status IS
    'usable | degraded | not_available | invalid, tal como lo reporto el colector.';

ALTER TABLE raw.metric_sample
    ADD COLUMN IF NOT EXISTS quality_status text,
    ADD COLUMN IF NOT EXISTS source_payload jsonb;

-- =============================================================================
-- 2. Vista de entrenamiento extendida
-- =============================================================================
--  Se conserva el contrato del baseline (mismas columnas, mismo orden, mismos
--  tipos) y se anaden al final las columnas nuevas. El JOIN del label sigue
--  siendo strictly point-in-time: l.period_start_utc = f.period_start_utc + 1h.
CREATE OR REPLACE VIEW feat.v_training_base AS
SELECT
    f.resource_id,
    f.period_start_utc,
    f.sku_id,
    f.compute_model,
    f.capacity_vcores,
    f.cpu_avg, f.cpu_p50, f.cpu_p95, f.cpu_p99, f.cpu_max, f.cpu_slope,
    f.data_io_avg, f.data_io_p95, f.log_write_avg, f.log_write_p95,
    f.sessions_avg, f.sessions_p95, f.workers_avg,
    f.storage_used_pct, f.storage_used_bytes, f.storage_growth_bytes,
    f.is_peak_hour, f.day_of_week, f.hour_of_day,
    f.workload_intensity, f.workload_run_id,
    f.deadlock_count, f.latency_p95_ms,
    l.cpu_avg              AS target_cpu_avg_next1h,       -- objetivo de forecasting CPU (8.3)
    l.cpu_p95              AS target_cpu_p95_next1h,
    l.workload_intensity   AS target_workload_next1h,      -- objetivo de intensidad (8.3)
    l.storage_used_bytes   AS target_storage_next1h,       -- objetivo de crecimiento (8.3)
    -- extension forecasting (columnas anadidas, mismo contrato de la vista base)
    f.experiment_uid,
    f.resource_uid,
    f.workload_class,
    f.memory_percent,
    f.sessions_count,
    f.sessions_percent,
    f.error_rate,
    f.latency_avg_ms,
    f.throughput_ops_per_second,
    f.offered_operations,
    f.successful_operations,
    f.failed_operations,
    f.quality_status,
    f.quality_warnings,
    f.canonical_sources,
    f.coverage_json,
    f.period_end_utc,
    l.cpu_max              AS target_cpu_max_next1h,
    l.error_rate           AS target_error_rate_next1h
FROM feat.feature_snapshot f
LEFT JOIN feat.feature_snapshot l
       ON l.resource_id = f.resource_id
      AND l.period_type = '1h'
      AND l.period_start_utc = f.period_start_utc + interval '1 hour'
WHERE f.period_type = '1h';

COMMENT ON VIEW feat.v_training_base IS 'Matriz de entrenamiento: features de la hora h y labels de la hora h+1 (CPU, intensidad, storage). El JOIN con l.period_start = f.period_start + 1h evita fugas de datos (no usa el futuro). Extension forecasting: columnas de memoria, sesiones (conteo y utilizacion separadas), errores, latencia, contadores de carga y calidad/procedencia.';

-- =============================================================================
-- 3. Esquema synthetic -- datos generados, jamas mezclados con los reales
-- =============================================================================
CREATE SCHEMA IF NOT EXISTS synthetic;

COMMENT ON SCHEMA synthetic IS 'Filas generadas por escenarios parametricos. Nunca se mezclan con raw.metric_reading ni con feat.feature_snapshot reales: el origen se declara fila a fila.';

CREATE TABLE IF NOT EXISTS synthetic.scenario_run (
    scenario_run_uid uuid primary key default gen_random_uuid(),
    base_experiment_uid uuid,
    base_resource_uid uuid,
    scenario_id text not null,
    scenario_type text not null,
    generation_seed bigint not null,
    formula text not null,
    parameter_json jsonb not null,
    calibration_model_id text,
    calibration_model_version text,
    started_at_utc timestamptz,
    ended_at_utc timestamptz,
    created_at_utc timestamptz not null default now()
);

COMMENT ON TABLE synthetic.scenario_run IS 'Corrida de generacion de un escenario. Guarda semilla, formula y parametros para reproducir exactamente las filas sinteticas.';

CREATE TABLE IF NOT EXISTS synthetic.metric_hourly (
    synthetic_row_uid uuid primary key default gen_random_uuid(),
    scenario_run_uid uuid not null references synthetic.scenario_run(scenario_run_uid) on delete cascade,
    scenario_id text not null,
    scenario_type text not null,
    period_start_utc timestamptz not null,
    period_end_utc timestamptz not null,
    -- carga aplicada
    workload_intensity numeric(12,4),
    concurrency integer,
    batch_size integer,
    operation_mix jsonb,
    -- respuestas del recurso
    cpu_avg numeric(8,4),
    cpu_max numeric(8,4),
    data_io_avg numeric(8,4),
    log_write_avg numeric(8,4),
    memory_percent numeric(8,4),
    sessions_count numeric(12,4),
    sessions_percent numeric(8,4),
    workers_avg numeric(8,4),
    latency_avg_ms numeric(12,2),
    latency_p95_ms numeric(12,2),
    error_rate numeric(10,6),
    storage_used_bytes bigint,
    storage_growth_bytes numeric(18,2),
    -- procedencia
    row_origin text not null default 'synthetic_scenario',
    generation_seed bigint not null,
    generation_formula text not null,
    noise_model text not null,
    calibration_model_id text,
    calibration_model_version text,
    base_experiment_uid uuid,
    base_resource_uid uuid,
    source_window_start_utc timestamptz,
    source_window_end_utc timestamptz,
    uncertainty_lower numeric(18,4),
    uncertainty_upper numeric(18,4),
    created_at_utc timestamptz not null default now(),
    UNIQUE (scenario_run_uid, period_start_utc)
);

COMMENT ON TABLE synthetic.metric_hourly IS 'Horas sinteticas generadas por un escenario. Filename de referencia: simulated_capacity_response. Cada fila declara su origen, semilla, formula y calibracion usada.';

CREATE INDEX IF NOT EXISTS idx_synthetic_metric_hourly_scenario
    ON synthetic.metric_hourly (scenario_id, period_start_utc);
CREATE INDEX IF NOT EXISTS idx_synthetic_metric_hourly_run
    ON synthetic.metric_hourly (scenario_run_uid);

-- =============================================================================
-- 4. Esquema forecasting -- trazabilidad de corridas y manifiestos
-- =============================================================================
CREATE SCHEMA IF NOT EXISTS forecasting;

COMMENT ON SCHEMA forecasting IS 'Trazabilidad del pipeline de forecasting: corridas, datasets, metricas por escenario y codigos de reproducibilidad.';

CREATE TABLE IF NOT EXISTS forecasting.run_registry (
    run_uid uuid primary key default gen_random_uuid(),
    run_type text not null,
    started_at_utc timestamptz not null default now(),
    finished_at_utc timestamptz,
    status text not null default 'running' check (status in ('running','succeeded','failed')),
    config_json jsonb not null default '{}'::jsonb,
    notes text
);

CREATE TABLE IF NOT EXISTS forecasting.dataset_manifest (
    manifest_uid uuid primary key default gen_random_uuid(),
    dataset_name text not null,
    source_type text not null check (source_type in ('real','synthetic','real+synthetic','real+calibrated')),
    experiment_uid uuid,
    resource_uid uuid,
    period_start_utc timestamptz,
    period_end_utc timestamptz,
    row_count integer not null,
    dataset_hash text not null,
    feature_definition_hash text not null,
    source_schema_hash text not null,
    code_commit text,
    branch text,
    random_seed integer,
    created_at_utc timestamptz not null default now()
);

COMMENT ON TABLE forecasting.dataset_manifest IS 'Manifiesto de cada dataset usado: hash del contenido, hash de la definicion de features, commit y rama. Permite reconstruir el dato exacto de cualquier experimento.';

CREATE TABLE IF NOT EXISTS forecasting.scenario_evaluation (
    evaluation_uid uuid primary key default gen_random_uuid(),
    model_name text not null,
    model_version text not null,
    dataset_label text not null,
    scenario_type text,
    workload_class text,
    intensity_band text,
    source_type text,
    peak_band text,
    mae numeric(18,6),
    rmse numeric(18,6),
    smape numeric(18,6),
    mase numeric(18,6),
    interval_coverage numeric(8,6),
    mean_interval_width numeric(18,6),
    n_points integer,
    created_at_utc timestamptz not null default now()
);

-- =============================================================================
-- 5. Indices de extraccion
-- =============================================================================
CREATE INDEX IF NOT EXISTS idx_feature_snapshot_lookup
    ON feat.feature_snapshot (period_type, period_start_utc);
CREATE INDEX IF NOT EXISTS idx_metric_reading_metric_time
    ON raw.metric_reading (metric_code, timestamp_utc);
CREATE INDEX IF NOT EXISTS idx_metric_sample_metric_time
    ON raw.metric_sample (metric_code, observed_at_utc);
