-- =============================================================================
--  Cloud Database Cost Optimizer
--  Base de datos: cco_data (bounded context analítico)
--  Esquema de datos PostgreSQL (PostgreSQL 14+) — "Data contract" (Fase 3, Sem 1)
--  -----------------------------------------------------------------------------
--  Propósito del modelo: soportar el entrenamiento del modelo de forecasting y
--  el pipeline completo del optimizador de capacidad, con trazabilidad
--  end-to-end: telemetría cruda -> features -> datasets -> modelos -> forecasts
--  -> escenarios -> recomendaciones (con riesgo y evidencia).
--
--  Organización en 6 schemas (= etapas del pipeline del plan, sección 8.1):
--    catalog : dominios y catálogos compartidos (proveedor, métricas, SKUs, workloads)
--    core    : datos maestros y hechos canónicos (recursos, configuración,
--              precios de lista, costo real FOCUS-lite, workloads experimentales)
--    raw     : espejo append-only de telemetría cruda de los proveedores
--    feat    : feature store (engineering) para ML
--    ml      : experiment tracking, model registry, forecasting y evaluación
--    opt     : optimización (escenarios, candidatos, simulación, recomendación,
--              riesgo, evidencia, efficiency score)
--
--  Convenciones:
--    * Identificadores en inglés; comentarios y documentación en español.
--    * Toda tabla tiene claves naturales + surrogate keys donde aplica y
--      columnas de auditoría (collected_at / created_at).
--    * raw es append-only y particionado por rango mensual (retención barata).
--    * Evitar UPDATE sobre series temporales: dedup por clave natural con
--      INSERT ... ON CONFLICT.
--    * JSONB solo para payloads volátiles del proveedor (procedencia) y
--      parámetros (hiperparámetros, constraints); lo analítico va tipado.
--
--  Base de datos del proyecto: cco_data
--
--  Ejecutar sobre una base de datos nueva:
--      createdb cco_data
--      psql -d cco_data -f esquema_postgres_cloud_cost_optimizer.sql
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS catalog;
CREATE SCHEMA IF NOT EXISTS core;
CREATE SCHEMA IF NOT EXISTS raw;
CREATE SCHEMA IF NOT EXISTS feat;
CREATE SCHEMA IF NOT EXISTS ml;
CREATE SCHEMA IF NOT EXISTS opt;
CREATE SCHEMA IF NOT EXISTS contract;

COMMENT ON SCHEMA catalog IS 'Dominios compartidos: proveedores cloud, regiones, catalogo de metricas, clases de workload y catalogo de SKUs Azure SQL (espacio de candidatos).';
COMMENT ON SCHEMA core    IS 'Datos maestros y hechos canonicos: recursos registrados, historial de configuracion (snapshots ARM), catalogo de precios de lista, costo real diario (modelo FOCUS-lite) y workloads experimentales.';
COMMENT ON SCHEMA raw     IS 'Telemetria cruda de Azure en append-only, particionada por mes. Espejo fiel de las APIs; nunca se modifica, solo se archiva.';
COMMENT ON SCHEMA feat    IS 'Feature store: definicion formal de features y snapshots periodicos (15m/1h/1d) listos para entrenamiento.';
COMMENT ON SCHEMA ml      IS 'Seguimiento de experimentos, datasets versionados, model registry, forecasts y evaluacion (MAE/RMSE/MAPE/cobertura).';
COMMENT ON SCHEMA opt     IS 'Motor de optimizacion: escenarios, candidatos, simulacion, recomendaciones con versionado, evidencia, riesgo y Database Efficiency Score.';
COMMENT ON SCHEMA contract IS 'Contrato externo read-only entre cco_data y cco_app: versiones, vistas públicas y UUIDs estables; no contiene claves foráneas cross-database.';

-- =============================================================================
-- 1. CATALOG — dominios compartidos
-- =============================================================================

-- 1.1 Proveedores cloud (abstraccion multi-cloud, plan seccion 13.5)
CREATE TABLE catalog.cloud_provider (
    provider_code  text PRIMARY KEY,             -- 'azure', futuro 'aws', 'gcp'
    name           text NOT NULL,
    enabled        boolean NOT NULL DEFAULT true,
    created_at     timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE catalog.cloud_provider IS 'Catalogo de proveedores cloud. Azure es la implementacion de referencia; las FKs a provider_code preparan la extension multi-cloud sin migrar el modelo.';

INSERT INTO catalog.cloud_provider (provider_code, name) VALUES
    ('azure', 'Microsoft Azure')
ON CONFLICT (provider_code) DO NOTHING;

-- 1.2 Regiones (normalizadas por proveedor; armRegionName de Azure)
CREATE TABLE catalog.region (
    provider_code  text NOT NULL REFERENCES catalog.cloud_provider(provider_code),
    region_code    text NOT NULL,                -- 'australiaeast'
    display_name   text,                         -- 'AU East'
    PRIMARY KEY (provider_code, region_code)
);

COMMENT ON TABLE catalog.region IS 'Regiones normalizadas por proveedor. El codigo de region de Azure (armRegionName) se conserva tal cual para cruzar con precios y recursos.';

INSERT INTO catalog.region (provider_code, region_code, display_name) VALUES
    ('azure', 'australiaeast', 'AU East')
ON CONFLICT (provider_code, region_code) DO NOTHING;

-- 1.3 Catalogo de metricas (contrato de ingesta; evita metricas "implicitas")
CREATE TABLE catalog.metric_catalog (
    metric_code            text PRIMARY KEY,     -- 'cpu_percent'
    display_name           text NOT NULL,        -- 'CPU percentage'
    unit                   text NOT NULL,        -- 'Percent', 'Count', 'Bytes'
    source                 text NOT NULL
        CHECK (source IN ('azure_monitor','database_watcher','query_store','derived')),
    collection_granularity text NOT NULL DEFAULT 'PT1M',
    is_key_feature         boolean NOT NULL DEFAULT false,   -- entra al feature store
    description            text
);

COMMENT ON TABLE catalog.metric_catalog IS 'Registro formal de metricas ingestables: nombre canonico interno, unidad, origen y granularidad. El collector solo ingesta metricas declaradas aqui (contract-first).';

INSERT INTO catalog.metric_catalog (metric_code, display_name, unit, source, is_key_feature, description) VALUES
    ('cpu_percent',        'CPU percentage',           'Percent', 'azure_monitor',     true,  'Porcentaje de CPU de la base de datos (limite de la SKU). Fuente principal del forecasting.'),
    ('storage_percent',    'Data space used percent',  'Percent', 'azure_monitor',     true,  'Porcentaje de almacenamiento usado respecto al maximo.'),
    ('storage_used_bytes', 'Data space used',          'Bytes',   'azure_monitor',     true,  'Almacenamiento usado en bytes (metrica storage de Azure Monitor).'),
    ('sessions_percent',   'Sessions percent',         'Percent', 'azure_monitor',     true,  'Porcentaje de sesiones concurrentes respecto al limite.'),
    ('workers_percent',    'Workers percent',          'Percent', 'azure_monitor',     true,  'Porcentaje de workers respecto al limite.'),
    ('deadlock',           'Deadlocks',                'Count',   'azure_monitor',     true,  'Numero de deadlocks por intervalo (senal de calidad).'),
    ('data_io_percent',    'Data IO percentage',       'Percent', 'database_watcher',  true,  'Porcentaje de I/O de datos (dm_db_resource_stats). Detecta cuellos no visibles en CPU.'),
    ('log_write_percent',  'Log write percentage',     'Percent', 'database_watcher',  true,  'Porcentaje de escritura de log.'),
    ('memory_usage_percent','Memory usage percentage', 'Percent', 'database_watcher',  true,  'Porcentaje de uso de memoria del buffer pool.'),
    ('instance_cpu_percent','Instance CPU percentage', 'Percent', 'database_watcher',  false, 'CPU a nivel de instancia SQL (contexto de vecinos).'),
    ('query_duration_ms',  'Query duration',           'Milliseconds', 'query_store',      false, 'Duración de una ejecución individual; solo es válida para percentiles si la fuente entrega observaciones individuales.')
ON CONFLICT (metric_code) DO NOTHING;

-- 1.4 Clases de workload del dataset experimental (plan Cuadro 3)
CREATE TABLE catalog.workload_class (
    class_code        text PRIMARY KEY,          -- 'low_stable'
    name              text NOT NULL,
    description       text NOT NULL,
    experimental_goal text NOT NULL,             -- objetivo en la validacion
    sort_order        smallint NOT NULL DEFAULT 0
);

COMMENT ON TABLE catalog.workload_class IS 'Clases de workload controlado del dataset experimental. Toda corrida de carga debe declarar su clase: es la variable experimental que permite construir ground truth.';

INSERT INTO catalog.workload_class (class_code, name, description, experimental_goal, sort_order) VALUES
    ('low_stable',    'Low stable',     'Baja utilizacion y baja variabilidad',              'Identificar candidatos de reduccion',            1),
    ('high_stable',   'High stable',    'Utilizacion alta pero estable',                     'Validar ausencia de falsas reducciones',          2),
    ('periodic',      'Periodic',       'Picos predecibles y periodicos',                    'Evaluar percentiles y temporalidad',              3),
    ('bursty',        'Bursty',         'Picos irregulares de alta intensidad',              'Evaluar riesgo',                                  4),
    ('increasing',    'Increasing',     'Tendencia sostenida de crecimiento',                'Validar forecasting',                             5),
    ('cpu_intensive', 'CPU intensive',  'Cuello de botella en CPU',                          'Estudiar headroom',                               6),
    ('io_intensive',  'I/O intensive',  'Presion de I/O con CPU baja',                       'Mostrar que CPU sola no basta',                   7),
    ('mixed',         'Mixed',          'Senales combinadas CPU/IO/concurrencia',            'Evaluar caso realista',                           8)
ON CONFLICT (class_code) DO NOTHING;

-- 1.5 Catalogo de SKUs Azure SQL = espacio finito de candidatos C1..Cn (ec. 12)
CREATE TABLE catalog.sql_sku (
    sku_id              integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sku_name            text NOT NULL UNIQUE,     -- 'GP_S_Gen5_2' (service objective)
    arm_sku_name        text,                     -- 'SQLDB_GP_Compute_Gen5_2' (retail prices)
    tier                text NOT NULL
        CHECK (tier IN ('GeneralPurpose','BusinessCritical','Hyperscale')),
    family              text,                     -- 'Gen5'
    compute_model       text NOT NULL
        CHECK (compute_model IN ('provisioned','serverless')),
    capacity_vcores     numeric(4,1) NOT NULL,
    min_capacity_vcores numeric(4,1),             -- serverless: 0.5
    max_size_gb         integer,
    in_scope_r1         boolean NOT NULL DEFAULT true,  -- alcance Release 1.0 (GP vCore + serverless)
    notes               text,
    created_at          timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE catalog.sql_sku IS 'Catalogo de SKUs Azure SQL en alcance Release 1.0 (General Purpose vCore provisioned + serverless). El optimizador solo puede recomendar configuraciones presentes aqui: dominio finito y auditable.';

INSERT INTO catalog.sql_sku (sku_name, arm_sku_name, tier, family, compute_model, capacity_vcores, min_capacity_vcores, max_size_gb, notes) VALUES
    ('GP_S_Gen5_2', 'SQLDB_GP_Compute_Gen5_2',  'GeneralPurpose', 'Gen5', 'serverless',   2.0, 0.5, 32, 'SKU observada en el laboratorio (kind v12.0,user,vcore,serverless,freelimit; autoPauseDelay 60 min)'),
    ('GP_Gen5_2',   'SQLDB_GP_Compute_Gen5_2',  'GeneralPurpose', 'Gen5', 'provisioned',  2.0, NULL, 100, NULL),
    ('GP_Gen5_4',   'SQLDB_GP_Compute_Gen5_4',  'GeneralPurpose', 'Gen5', 'provisioned',  4.0, NULL, 100, NULL),
    ('GP_Gen5_6',   'SQLDB_GP_Compute_Gen5_6',  'GeneralPurpose', 'Gen5', 'provisioned',  6.0, NULL, 100, NULL),
    ('GP_Gen5_8',   'SQLDB_GP_Compute_Gen5_8',  'GeneralPurpose', 'Gen5', 'provisioned',  8.0, NULL, 100, NULL)
ON CONFLICT (sku_name) DO NOTHING;

-- =============================================================================
-- 2. CORE — datos maestros y hechos canónicos
-- =============================================================================

-- 2.1 Recurso registrado (Release 1.0: bases de datos Azure SQL)
CREATE TABLE core.resource (
    resource_id       integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource_uid      uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE, -- ID estable entre cco_app y cco_data
    provider_code     text NOT NULL REFERENCES catalog.cloud_provider(provider_code),
    subscription_id   text NOT NULL,
    resource_group    text NOT NULL,
    server_name       text NOT NULL,
    database_name     text NOT NULL,
    azure_resource_id text NOT NULL UNIQUE,      -- ruta ARM completa
    region_code       text NOT NULL,
    kind              text,                      -- 'v12.0,user,vcore,serverless,freelimit'
    lifecycle_status  text NOT NULL DEFAULT 'active'
        CHECK (lifecycle_status IN ('active','paused','deleted','archived')),
    registered_at     timestamptz NOT NULL DEFAULT now(),
    deactivated_at    timestamptz,
    UNIQUE (provider_code, subscription_id, resource_group, server_name, database_name)
);

COMMENT ON TABLE core.resource IS 'Registro de recursos Azure SQL monitoreados (requisito 12.1.1). La identidad natural es la ruta ARM; el surrogate resource_id es la FK universal del resto del modelo.';
COMMENT ON COLUMN core.resource.resource_uid IS 'UUID público e inmutable del contrato cco_data.v1. cco_app.cloud.resource_bindings.data_resource_uid debe almacenar este valor; nunca resource_id.';

ALTER TABLE core.resource
    ADD CONSTRAINT fk_resource_region
    FOREIGN KEY (provider_code, region_code)
    REFERENCES catalog.region(provider_code, region_code);

-- 2.2 Historial de configuración (snapshots ARM, append-only)
CREATE TABLE core.resource_config_snapshot (
    config_snapshot_id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource_id              integer NOT NULL REFERENCES core.resource(resource_id),
    sku_id                   integer REFERENCES catalog.sql_sku(sku_id),
    observed_at_utc          timestamptz NOT NULL,
    collector_run_id         bigint,             -- FK diferida a raw.collector_run
    sku_name                 text,               -- currentServiceObjectiveName
    tier                     text,
    family                   text,
    capacity_vcores          numeric(4,1),
    min_capacity_vcores      numeric(4,1),
    compute_model            text
        CHECK (compute_model IN ('provisioned','serverless')),
    max_size_bytes           bigint,
    auto_pause_delay_minutes integer,             -- serverless: auto-pause
    status                   text,               -- 'Online'
    free_limit               boolean,            -- useFreeLimit
    source_payload           jsonb,              -- cuerpo ARM verbatim (procedencia)
    UNIQUE (resource_id, observed_at_utc)
);

COMMENT ON TABLE core.resource_config_snapshot IS 'Historial append-only de la configuracion observada por ARM. Snapshot en lugar de UPDATE: permite reconstruir la capacidad vigente en cualquier instante (point-in-time), requisito para features y analisis retroactivos correctos.';

CREATE INDEX idx_config_snapshot_resource_time
    ON core.resource_config_snapshot (resource_id, observed_at_utc DESC);

-- 2.3 Catalogo de precios de lista (Azure Retail Prices API — plan 6.1)
CREATE TABLE core.price_catalog (
    price_id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider_code      text NOT NULL REFERENCES catalog.cloud_provider(provider_code),
    region_code        text NOT NULL,
    meter_id           text NOT NULL,            -- meterId de la API
    meter_name         text NOT NULL,            -- 'vCore'
    product_name       text NOT NULL,
    sku_name           text,                     -- skuName ('2 vCore')
    arm_sku_name       text,                     -- SQLDB_GP_Compute_Gen5_2
    service_name       text NOT NULL,            -- 'SQL Database'
    price_type         text NOT NULL
        CHECK (price_type IN ('Consumption','Reservation','DevTestConsumption')),
    unit_of_measure    text NOT NULL,            -- '1 Hour', '1 GB/Month'
    unit_price         numeric(18,6) NOT NULL,   -- precio de lista (a)
    currency           char(3) NOT NULL DEFAULT 'USD',
    tier_minimum_units numeric(18,4) NOT NULL DEFAULT 0,
    effective_start    date NOT NULL,
    effective_end      date,
    fetched_at         timestamptz NOT NULL DEFAULT now(),
    source_payload     jsonb,
    UNIQUE (meter_id, price_type, effective_start)
);

COMMENT ON TABLE core.price_catalog IS 'Precios de lista (retail) con vigencia efectiva, usados para SIMULAR escenarios (plan 6.1 y 8.3). No confundir con costo facturado (core.cost_daily): a) lista, b) efectivo, c) facturado se modelan por separado.';

ALTER TABLE core.price_catalog
    ADD CONSTRAINT fk_price_catalog_region
    FOREIGN KEY (provider_code, region_code)
    REFERENCES catalog.region(provider_code, region_code);

-- Precios reales extraidos del laboratorio (australiaeast, USD):
INSERT INTO core.price_catalog (provider_code, region_code, meter_id, meter_name, product_name,
    sku_name, arm_sku_name, service_name, price_type, unit_of_measure, unit_price, currency,
    effective_start, source_payload) VALUES
    ('azure','australiaeast','2d6ad12e-49dd-4279-9c6b-e0cd904957ab','vCore',
     'SQL Database Single/Elastic Pool General Purpose - Compute Gen5','2 vCore',
     'SQLDB_GP_Compute_Gen5_2','SQL Database','Consumption','1 Hour',0.362276,'USD','2025-06-01',NULL),
    ('azure','australiaeast','2d6ad12e-49dd-4279-9c6b-e0cd904957ab','vCore',
     'SQL Database Single/Elastic Pool General Purpose - Compute Gen5','4 vCore',
     'SQLDB_GP_Compute_Gen5_4','SQL Database','Consumption','1 Hour',0.724552,'USD','2025-06-01',NULL),
    ('azure','australiaeast','2d6ad12e-49dd-4279-9c6b-e0cd904957ab','vCore',
     'SQL Database Single/Elastic Pool General Purpose - Compute Gen5','6 vCore',
     'SQLDB_GP_Compute_Gen5_6','SQL Database','Consumption','1 Hour',1.086828,'USD','2025-06-01',NULL),
    ('azure','australiaeast','2d6ad12e-49dd-4279-9c6b-e0cd904957ab','vCore',
     'SQL Database Single/Elastic Pool General Purpose - Compute Gen5','8 vCore',
     'SQLDB_GP_Compute_Gen5_8','SQL Database','Consumption','1 Hour',1.449104,'USD','2025-06-01',NULL),
    ('azure','australiaeast','875f798e-2ab2-4db8-b5f3-9aa24c247230','General Purpose Data Stored',
     'SQL Database Single/Elastic Pool General Purpose - Storage','General Purpose',
     NULL,'SQL Database','Consumption','1 GB/Month',0.138000,'USD','2025-06-01',NULL)
ON CONFLICT (meter_id, price_type, effective_start) DO NOTHING;

-- 2.4 Costo real diario (modelo canónico inspirado en FOCUS 1.2 — plan sección 14)
CREATE TABLE core.cost_daily (
    cost_daily_id   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource_id     integer REFERENCES core.resource(resource_id),  -- NULL: costo sin mapeo a BD (Storage, VNet)
    provider_code   text NOT NULL REFERENCES catalog.cloud_provider(provider_code),
    scope_type      text NOT NULL
        CHECK (scope_type IN ('subscription','resource_group','resource')),
    scope_name      text NOT NULL,             -- 'rg-proyecto-grado-free'
    service_name    text NOT NULL,             -- 'SQL Database','Storage','Virtual Network'
    region_code     text,
    usage_date      date NOT NULL,
    currency        char(3) NOT NULL,
    source_charge_id text,                       -- identificador idempotente de Cost Management
    meter_id        text,                        -- meter del cargo cuando exista
    pre_tax_cost    numeric(18,6) NOT NULL,    -- costo efectivo/facturado real (b/c de 6.1)
    usage_quantity  numeric(18,6),             -- consumed_quantity (FOCUS)
    usage_unit      text,                      -- consumed_unit
    list_unit_price numeric(18,6),             -- list_unit_price del dia (FOCUS)
    pricing_model   text
        CHECK (pricing_model IN ('consumption','reservation','spot','unknown')),
    charge_category text
        CHECK (charge_category IN ('usage','purchase','tax','adjustment','credit')),
    source          text NOT NULL DEFAULT 'azure_cost_management',
    collected_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.cost_daily IS 'Hecho de costo diario canonico (FOCUS-lite): provider, servicio, region, fecha, cantidad consumida, precio de lista y costo efectivo. Alimenta KPIs economicos (10.1) y la validacion de simulaciones contra la realidad.';

CREATE INDEX idx_cost_daily_resource ON core.cost_daily (resource_id, usage_date);
CREATE INDEX idx_cost_daily_scope    ON core.cost_daily (scope_type, scope_name, usage_date);

ALTER TABLE core.cost_daily
    ADD CONSTRAINT fk_cost_daily_region
    FOREIGN KEY (provider_code, region_code)
    REFERENCES catalog.region(provider_code, region_code);

-- Unicidad por cargo real: permite varios cargos del mismo servicio y fecha.
CREATE UNIQUE INDEX ux_cost_daily_source_charge
    ON core.cost_daily (provider_code, source_charge_id)
    WHERE source_charge_id IS NOT NULL;

-- 2.5 Corrida de workload experimental (dataset propio — plan sección 7)
CREATE TABLE core.workload_run (
    workload_run_id    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource_id        integer NOT NULL REFERENCES core.resource(resource_id),
    class_code         text NOT NULL REFERENCES catalog.workload_class(class_code),
    config_snapshot_id bigint,                  -- FK diferida; configuracion bajo la que corrio
    started_at_utc     timestamptz NOT NULL,
    finished_at_utc    timestamptz,
    duration_s         numeric(12,2),
    pattern_params     jsonb,                   -- parametros del generador (periodos, fases, ratios)
    notes              text
);

COMMENT ON TABLE core.workload_run IS 'Sesion de carga controlada sobre un recurso, con su clase declarada. Vincula la intensidad aplicada con la telemetria observada: es la base del ground truth experimental.';

CREATE INDEX idx_workload_run_resource ON core.workload_run (resource_id, started_at_utc);

-- 2.6 Ciclos de carga (variable experimental: intensidad aplicada)
CREATE TABLE core.workload_cycle (
    workload_cycle_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workload_run_id   bigint NOT NULL REFERENCES core.workload_run(workload_run_id),
    cycle_number      integer NOT NULL,
    started_at_utc    timestamptz,
    intensity         numeric(10,4),            -- factor de intensidad objetivo
    operations        integer,                  -- operaciones del ciclo
    progress          numeric(5,4),
    inserts           integer,
    updates           integer,
    deletes           integer,
    selects           integer,
    UNIQUE (workload_run_id, cycle_number)
);

COMMENT ON TABLE core.workload_cycle IS 'Ciclos del generador de carga con su intensidad (p.ej. 3434 ciclos en ~3.7 h en el laboratorio). Permite etiquetar picos y construir la feature workload_intensity desde la carga aplicada, no solo observada.';

CREATE INDEX idx_workload_cycle_run ON core.workload_cycle (workload_run_id, cycle_number);

-- =============================================================================
-- 3. RAW — telemetría cruda append-only (espejo fiel de APIs)
-- =============================================================================

-- 3.0 Auditoría de ingesta (trazabilidad de TODA lectura al proveedor)
CREATE TABLE raw.collector_run (
    collector_run_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source           text NOT NULL,   -- 'azure_monitor_metrics','arm_sql_resources','retail_prices','cost_management','query_store','advisor','activity_log'
    resource_id      integer REFERENCES core.resource(resource_id),
    scope            text,            -- subscription / resource group consultado
    started_at_utc   timestamptz NOT NULL,
    finished_at_utc  timestamptz,
    status           text NOT NULL DEFAULT 'ok'
        CHECK (status IN ('ok','partial','error')),
    rows_ingested    integer,
    error_message    text,
    request_payload  jsonb,           -- parametros de la consulta (timespan, interval, metricas)
    response_payload jsonb            -- cuerpo verbatim (opcional; apagar en producción por tamaño)
);

COMMENT ON TABLE raw.collector_run IS 'Auditoria de cada corrida del collector: que se consulto, cuando, con que parametros y cuantas filas entro. Es el primer eslabon de la trazabilidad y permite re-procesar sin re-descargar.';

-- 3.1 Lecturas de Azure Monitor (serie temporal PT1M, particionada por mes)
CREATE TABLE raw.metric_reading (
    resource_id      integer NOT NULL REFERENCES core.resource(resource_id),
    metric_code      text NOT NULL REFERENCES catalog.metric_catalog(metric_code),
    timestamp_utc    timestamptz NOT NULL,
    interval_seconds integer NOT NULL DEFAULT 60,
    avg_value        numeric(18,4),
    min_value        numeric(18,4),
    max_value        numeric(18,4),
    total_value      numeric(18,4),
    count_value      integer,
    collected_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_id, metric_code, timestamp_utc, interval_seconds)
) PARTITION BY RANGE (timestamp_utc);

COMMENT ON TABLE raw.metric_reading IS 'Lecturas de metricas de Azure Monitor (avg/min/max por minuto). Particionada mensualmente: las particiones viejas se desprenden (DETACH) para archivar sin borrar fila a fila.';

CREATE TABLE raw.metric_reading_p2026_09 PARTITION OF raw.metric_reading
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE raw.metric_reading_p2026_10 PARTITION OF raw.metric_reading
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE raw.metric_reading_p2026_11 PARTITION OF raw.metric_reading
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE raw.metric_reading_default PARTITION OF raw.metric_reading DEFAULT;

CREATE INDEX idx_metric_reading_brin ON raw.metric_reading USING brin (timestamp_utc);

-- 3.1b Observaciones individuales para estadísticas de distribución
CREATE TABLE raw.metric_sample (
    sample_id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource_id        integer NOT NULL REFERENCES core.resource(resource_id),
    metric_code        text NOT NULL REFERENCES catalog.metric_catalog(metric_code),
    observed_at_utc    timestamptz NOT NULL,
    value              numeric(18,6) NOT NULL,
    source             text NOT NULL CHECK (source IN ('azure_monitor_sample','database_watcher_sample','query_store_execution','workload_observation')),
    source_sample_id   text,
    collected_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (value >= 0),
    UNIQUE (resource_id, metric_code, observed_at_utc, source, source_sample_id)
);
COMMENT ON TABLE raw.metric_sample IS 'Observaciones individuales o muestras de distribución. Es la única fuente permitida para calcular percentiles exactos. raw.metric_reading solo contiene agregados avg/min/max y no puede producir p95/p99 válidos.';
COMMENT ON COLUMN raw.metric_sample.source_sample_id IS 'Identificador de la observación en la fuente. Debe informarse cuando exista para garantizar idempotencia; NULL solo aplica a una fuente que no entregue ID.';
CREATE INDEX idx_metric_sample_window ON raw.metric_sample (resource_id, metric_code, observed_at_utc);

-- 3.2 dm_db_resource_stats (Database watcher, serie temporal)
CREATE TABLE raw.db_resource_stats (
    resource_id              integer NOT NULL REFERENCES core.resource(resource_id),
    end_time_utc             timestamptz NOT NULL,
    avg_cpu_percent          numeric(6,2),
    avg_data_io_percent      numeric(6,2),
    avg_log_write_percent    numeric(6,2),
    avg_memory_usage_percent numeric(6,2),
    avg_instance_cpu_percent numeric(6,2),
    dtu_limit                numeric(10,2),
    cpu_limit                numeric(10,2),
    collected_at             timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_id, end_time_utc)
) PARTITION BY RANGE (end_time_utc);

COMMENT ON TABLE raw.db_resource_stats IS 'Telemetria interna de Azure SQL (dm_db_resource_stats via Database watcher): CPU, data IO, log write y memoria en %, mas el limite de la SKU en el momento. Complemento clave para features de I/O (plan 7.2).';

CREATE TABLE raw.db_resource_stats_p2026_09 PARTITION OF raw.db_resource_stats
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE raw.db_resource_stats_p2026_10 PARTITION OF raw.db_resource_stats
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE raw.db_resource_stats_default PARTITION OF raw.db_resource_stats DEFAULT;

CREATE INDEX idx_db_resource_stats_brin ON raw.db_resource_stats USING brin (end_time_utc);

-- 3.3 Query Store: textos y queries (snapshot por corrida del collector)
CREATE TABLE raw.query_store_query (
    resource_id        integer NOT NULL REFERENCES core.resource(resource_id),
    query_id           bigint NOT NULL,
    query_text_id      bigint,
    query_hash         text,
    query_sql_text     text,
    is_internal_query  boolean,
    snapshot_at_utc    timestamptz NOT NULL,
    collected_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_id, query_id, snapshot_at_utc)
);

COMMENT ON TABLE raw.query_store_query IS 'Catalogo de queries vistas por el Query Store, capturado por snapshot. El texto permite agrupar por query_hash y detectar las consultas dominantes del workload.';

-- 3.4 Query Store: estadísticas de runtime (duración, CPU, I/O por query)
CREATE TABLE raw.query_store_runtime_stats (
    resource_id            integer NOT NULL REFERENCES core.resource(resource_id),
    query_id               bigint NOT NULL,
    interval_start_utc     timestamptz NOT NULL,   -- first_execution_time
    interval_end_utc       timestamptz NOT NULL,   -- last_execution_time
    count_executions       bigint,
    avg_duration_us        numeric(18,2),          -- unidades nativas (microsegundos)
    avg_cpu_time_us        numeric(18,2),
    avg_logical_io_reads   numeric(18,2),
    avg_logical_io_writes  numeric(18,2),
    snapshot_at_utc        timestamptz NOT NULL,
    collected_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_id, query_id, interval_start_utc)
);

COMMENT ON TABLE raw.query_store_runtime_stats IS 'Runtime stats por query: insumo para features de calidad (latencia p95 por consulta) y para explicar cuellos cuando la CPU agregada no basta.';

CREATE INDEX idx_qs_runtime_snapshot ON raw.query_store_runtime_stats (resource_id, snapshot_at_utc);

-- 3.5 Query Store: wait stats por categoría
CREATE TABLE raw.query_store_wait_stats (
    resource_id     integer NOT NULL REFERENCES core.resource(resource_id),
    query_id        bigint NOT NULL,
    wait_category   text NOT NULL,               -- 'Other Disk IO', 'CPU', 'Lock', ...
    avg_wait_ms     numeric(18,3),
    total_wait_ms   numeric(18,3),
    snapshot_at_utc timestamptz NOT NULL,
    collected_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_id, query_id, wait_category, snapshot_at_utc)
);

COMMENT ON TABLE raw.query_store_wait_stats IS 'Esperas por categoria: senal de cuellos de I/O, locks o CPU que un promedio de utilizacion oculta (plan 7.1: caso I/O intensive).';

-- 3.6 Sesiones activas (instantánea de concurrencia)
CREATE TABLE raw.db_session_snapshot (
    resource_id             integer NOT NULL REFERENCES core.resource(resource_id),
    snapshot_at_utc         timestamptz NOT NULL,
    session_id              integer NOT NULL,
    status                  text,
    login_name              text,
    host_name               text,
    program_name            text,
    last_request_start_utc  timestamptz,
    collected_at            timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_id, snapshot_at_utc, session_id)
);

COMMENT ON TABLE raw.db_session_snapshot IS 'Instantanea de sesiones (sys.dm_exec_sessions): concurrencia real por login/host/programa, insumo del grupo de features de concurrencia.';

-- 3.7 Activity Log (auditoría de operaciones del plano de control)
CREATE TABLE raw.activity_log (
    event_id            text NOT NULL,           -- hash sintetico del collector
    resource_id         integer REFERENCES core.resource(resource_id),
    time_generated_utc  timestamptz NOT NULL,
    resource_arm_id     text,
    operation_name      text,
    category            text,
    result_type         text,
    result_signature    text,
    caller_ip           text,
    correlation_id      text,
    duration_ms         bigint,
    properties          jsonb,
    collected_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, time_generated_utc)
) PARTITION BY RANGE (time_generated_utc);

COMMENT ON TABLE raw.activity_log IS 'Bitacora de operaciones (quien cambio que y cuando). Correlaciona eventos de control (p.ej. un escalado) con saltos en la telemetria.';

CREATE TABLE raw.activity_log_p2026_09 PARTITION OF raw.activity_log
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE raw.activity_log_p2026_10 PARTITION OF raw.activity_log
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE raw.activity_log_default PARTITION OF raw.activity_log DEFAULT;

CREATE INDEX idx_activity_log_brin ON raw.activity_log USING brin (time_generated_utc);

-- 3.8 Recomendaciones de Azure Advisor (baseline externo — plan 16.2)
CREATE TABLE raw.advisor_recommendation (
    advisor_rec_id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider_code           text NOT NULL DEFAULT 'azure',
    recommendation_type_id  text,
    category                text,                -- 'Cost','HighAvailability',...
    impact                  text,                -- 'High','Medium','Low'
    impacted_field          text,
    impacted_value          text,
    resource_arm_id         text,
    problem                 text,
    solution                text,
    last_updated_utc        timestamptz,
    extended_properties     jsonb,
    source_payload          jsonb,
    collected_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (recommendation_type_id, impacted_value, last_updated_utc)
);

COMMENT ON TABLE raw.advisor_recommendation IS 'Snapshot de recomendaciones de Azure Advisor. Se usa como referencia externa/baseline (nunca como ground truth), comparada en opt.advisor_comparison.';

-- =============================================================================
-- 4. FEAT — feature store
-- =============================================================================

-- 4.1 Diccionario de features (explicabilidad: "no hacer del ML una caja negra")
CREATE TABLE feat.feature_definition (
    feature_code  text PRIMARY KEY,             -- 'cpu_p95_1h'
    feature_group text NOT NULL
        CHECK (feature_group IN ('capacity','cpu','io','concurrency','temporal','storage','cost','quality')),
    display_name  text NOT NULL,
    description   text NOT NULL,
    unit          text,
    formula       text,                          -- definición reproducible
    source_tables text,                          -- de dónde sale
    period_types  text NOT NULL DEFAULT '1h,1d',
    created_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE feat.feature_definition IS 'Data dictionary formal de cada feature: grupo (Cuadro 4 del plan), formula y tablas fuente. Cualquier persona debe poder auditar como se calculo una feature sin leer el codigo del pipeline.';

INSERT INTO feat.feature_definition (feature_code, feature_group, display_name, description, unit, formula, source_tables) VALUES
    ('cpu_avg','cpu','CPU promedio','Promedio de cpu_percent en la ventana','%','AVG(avg_value) WHERE metric_code=cpu_percent','raw.metric_reading'),
    ('cpu_p50','cpu','CPU mediana','Percentil 50 exacto solo sobre observaciones individuales; NULL si no hay muestras','%','percentile_statistic(metric_code=cpu_percent, percentile=0.50, method=exact_sample)','raw.metric_sample'),
    ('cpu_p95','cpu','CPU p95','Percentil 95 exacto solo sobre observaciones individuales; NULL si no hay muestras','%','percentile_statistic(metric_code=cpu_percent, percentile=0.95, method=exact_sample)','raw.metric_sample'),
    ('cpu_p99','cpu','CPU p99','Percentil 99 exacto solo sobre observaciones individuales; NULL si no hay muestras','%','percentile_statistic(metric_code=cpu_percent, percentile=0.99, method=exact_sample)','raw.metric_sample'),
    ('cpu_max','cpu','CPU maxima','Maximo de cpu_percent en la ventana','%','MAX(max_value)','raw.metric_reading'),
    ('cpu_slope','cpu','Tendencia CPU','Pendiente de la regresion lineal de cpu_avg (por hora)','%/h','slope(cpu_avg) sobre la ventana','feat.feature_snapshot'),
    ('data_io_avg','io','Data IO promedio','Promedio de data_io_percent','%','AVG(avg_data_io_percent)','raw.db_resource_stats'),
    ('data_io_p95','io','Data IO p95','Percentil 95 exacto solo sobre observaciones individuales; NULL si la fuente solo entrega promedios','%','percentile_statistic(metric_code=data_io_percent, percentile=0.95, method=exact_sample)','raw.metric_sample'),
    ('log_write_avg','io','Log write promedio','Promedio de log_write_percent','%','AVG(avg_log_write_percent)','raw.db_resource_stats'),
    ('log_write_p95','io','Log write p95','Percentil 95 exacto solo sobre observaciones individuales; NULL si la fuente solo entrega promedios','%','percentile_statistic(metric_code=log_write_percent, percentile=0.95, method=exact_sample)','raw.metric_sample'),
    ('sessions_avg','concurrency','Sesiones promedio','Promedio de sessions_percent','%','AVG(avg_value)','raw.metric_reading'),
    ('sessions_p95','concurrency','Sesiones p95','Percentil 95 exacto solo sobre observaciones individuales; NULL si no hay muestras','%','percentile_statistic(metric_code=sessions_percent, percentile=0.95, method=exact_sample)','raw.metric_sample'),
    ('workers_avg','concurrency','Workers promedio','Promedio de workers_percent','%','AVG(avg_value)','raw.metric_reading'),
    ('storage_used_pct','storage','Almacenamiento usado','Porcentaje usado respecto al maximo','%','AVG(avg_value) storage_percent','raw.metric_reading'),
    ('storage_used_bytes','storage','Almacenamiento usado (bytes)','Bytes usados de datos','bytes','AVG(storage_used_bytes)','raw.metric_reading'),
    ('storage_growth_bytes','storage','Crecimiento de storage','Delta de bytes vs ventana anterior','bytes','delta(storage_used_bytes)','feat.feature_snapshot'),
    ('is_peak_hour','temporal','Hora pico','Indica si la hora pertenece a la ventana pico historica','bool','fraccional de horas > percentil','feat.feature_snapshot'),
    ('day_of_week','temporal','Dia de la semana','0-6 (lunes=0) para estacionalidad semanal','','EXTRACT(dow)','feat.feature_snapshot'),
    ('hour_of_day','temporal','Hora del dia','0-23 para estacionalidad diaria','','EXTRACT(hour)','feat.feature_snapshot'),
    ('workload_intensity','temporal','Intensidad de workload','Operaciones por minuto aplicadas por el generador','ops/min','SUM(operations)/minutos','core.workload_cycle'),
    ('deadlock_count','quality','Deadlocks','Conteo de deadlocks en la ventana','count','SUM(total_value)','raw.metric_reading'),
    ('latency_p95_ms','quality','Latencia p95 de queries','Percentil 95 exacto de duraciones individuales; no se deriva de avg_duration_us','ms','percentile_statistic(metric_code=query_duration_ms, percentile=0.95, method=exact_sample)','raw.metric_sample')
ON CONFLICT (feature_code) DO NOTHING;

-- 4.2 Estadísticos de distribución y su calidad/procedencia
CREATE TABLE feat.percentile_statistic (
    resource_id           integer NOT NULL REFERENCES core.resource(resource_id),
    period_type           text NOT NULL CHECK (period_type IN ('15m','1h','1d')),
    period_start_utc      timestamptz NOT NULL,
    metric_code           text NOT NULL REFERENCES catalog.metric_catalog(metric_code),
    percentile             numeric(5,4) NOT NULL CHECK (percentile BETWEEN 0 AND 1),
    value                 numeric(18,6),
    method                text NOT NULL CHECK (method IN ('exact_sample','weighted_histogram','not_available')),
    observation_count     integer NOT NULL DEFAULT 0 CHECK (observation_count >= 0),
    source_granularity    text NOT NULL,
    computed_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_id, period_type, period_start_utc, metric_code, percentile),
    CHECK ((method = 'not_available' AND value IS NULL) OR (method <> 'not_available' AND value IS NOT NULL AND observation_count > 0))
);
COMMENT ON TABLE feat.percentile_statistic IS 'Contrato estadístico de percentiles. exact_sample requiere raw.metric_sample; weighted_histogram requiere un histograma con pesos; not_available es la salida obligatoria cuando solo existen avg/min/max. Nunca se etiqueta como p95/p99 un percentil calculado desde agregados simples.';
COMMENT ON COLUMN feat.percentile_statistic.source_granularity IS 'Descripción de la resolución de la fuente, por ejemplo execution, 1m_sample o histogram. No confundir con avg/min/max de un intervalo.';
CREATE INDEX idx_percentile_statistic_lookup ON feat.percentile_statistic (resource_id, metric_code, period_start_utc DESC);

-- 4.3 Snapshot de features (tabla ancha, particionada por mes)
CREATE TABLE feat.feature_snapshot (
    resource_id          integer NOT NULL REFERENCES core.resource(resource_id),
    period_type          text NOT NULL
        CHECK (period_type IN ('15m','1h','1d')),
    period_start_utc     timestamptz NOT NULL,
    period_end_utc       timestamptz NOT NULL,
    -- contexto de capacidad vigente en la ventana (point-in-time, denormalizado)
    sku_id               integer REFERENCES catalog.sql_sku(sku_id),
    config_snapshot_id   bigint,                 -- FK diferida a core.resource_config_snapshot
    compute_model        text,
    capacity_vcores      numeric(4,1),
    -- CPU
    cpu_avg              numeric(8,4),
    cpu_p50              numeric(8,4),
    cpu_p95              numeric(8,4),
    cpu_p99              numeric(8,4),
    cpu_max              numeric(8,4),
    cpu_slope            numeric(10,6),          -- %/hora
    -- I/O
    data_io_avg          numeric(8,4),
    data_io_p95          numeric(8,4),
    log_write_avg        numeric(8,4),
    log_write_p95        numeric(8,4),
    -- concurrencia
    sessions_avg         numeric(10,4),
    sessions_p95         numeric(10,4),
    workers_avg          numeric(10,4),
    -- almacenamiento
    storage_used_pct     numeric(8,4),
    storage_used_bytes   bigint,
    storage_growth_bytes numeric(18,2),          -- vs ventana anterior (period_type 1d)
    -- temporalidad y carga
    is_peak_hour         boolean,
    day_of_week          smallint,
    hour_of_day          smallint,
    workload_intensity   numeric(12,4),          -- ops/min aplicados (experimental)
    workload_run_id      bigint,                 -- si la ventana cae dentro de un run controlado
    -- calidad
    deadlock_count       integer,
    latency_p95_ms       numeric(12,2),
    computed_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (resource_id, period_type, period_start_utc)
) PARTITION BY RANGE (period_start_utc);

COMMENT ON TABLE feat.feature_snapshot IS 'Feature store fisico: una fila por recurso, tipo de periodo e inicio de ventana. Tabla ANCHA (no EAV): las columnas son el contrato de entrenamiento y el planner trabaja mejor. El contexto de SKU va denormalizado para joins point-in-time correctos.';

CREATE TABLE feat.feature_snapshot_p2026_09 PARTITION OF feat.feature_snapshot
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE feat.feature_snapshot_p2026_10 PARTITION OF feat.feature_snapshot
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE feat.feature_snapshot_p2026_11 PARTITION OF feat.feature_snapshot
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE feat.feature_snapshot_default PARTITION OF feat.feature_snapshot DEFAULT;

CREATE INDEX idx_feature_snapshot_brin ON feat.feature_snapshot USING brin (period_start_utc);

-- 4.4 Vista base de entrenamiento (features + labels t+1h, point-in-time correcta)
CREATE VIEW feat.v_training_base AS
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
    l.storage_used_bytes   AS target_storage_next1h        -- objetivo de crecimiento (8.3)
FROM feat.feature_snapshot f
LEFT JOIN feat.feature_snapshot l
       ON l.resource_id = f.resource_id
      AND l.period_type = '1h'
      AND l.period_start_utc = f.period_start_utc + interval '1 hour'
WHERE f.period_type = '1h';

COMMENT ON VIEW feat.v_training_base IS 'Matriz de entrenamiento lista para exportar: features de la hora h y labels de la hora h+1 (CPU, intensidad, storage). El JOIN con l.period_start = f.period_start + 1h evita fugas de datos (no usa el futuro).';

-- =============================================================================
-- 5. ML — experiment tracking, model registry, forecasting y evaluación
-- =============================================================================

-- 5.1 Experimento (marco de investigación: EXP-01, EXP-02, ...)
CREATE TABLE ml.experiment (
    experiment_id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code          text NOT NULL UNIQUE,          -- 'EXP-01'
    name          text NOT NULL,
    objective     text,
    kind          text NOT NULL
        CHECK (kind IN ('forecast','right_sizing','risk','score')),
    config        jsonb,
    started_at    timestamptz,
    finished_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE ml.experiment IS 'Experimento de investigacion (plan 16): agrupa corridas de entrenamiento y pruebas bajo un objetivo y una configuracion reproducible.';

-- 5.2 Dataset de entrenamiento versionado (lineage)
CREATE TABLE ml.training_dataset (
    dataset_id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name             text NOT NULL,              -- 'cpu_hourly_v1'
    version          integer NOT NULL,
    resource_scope   text NOT NULL DEFAULT 'all',
    period_start_utc timestamptz NOT NULL,
    period_end_utc   timestamptz NOT NULL,
    period_type      text NOT NULL DEFAULT '1h',
    feature_codes    text[] NOT NULL,            -- lista explicita: sin caja negra
    target_column    text NOT NULL,              -- 'cpu_avg' | 'workload_intensity' | 'storage_used_bytes'
    row_count        integer,
    split_strategy   jsonb,                      -- {"type":"temporal","train":0.7,"valid":0.15,"test":0.15}
    dataset_hash     text,                       -- hash del contenido exportado (reproducibilidad)
    created_by       text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (name, version)
);

COMMENT ON TABLE ml.training_dataset IS 'Version inmutable de un dataset de entrenamiento: ventana temporal, features incluidas, target y hash. Permite reconstruir exactamente el dato con el que se entreno cualquier modelo.';

-- 5.3 Corrida de entrenamiento
CREATE TABLE ml.training_run (
    training_run_id  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    experiment_id    integer REFERENCES ml.experiment(experiment_id),
    dataset_id       bigint NOT NULL REFERENCES ml.training_dataset(dataset_id),
    algorithm        text NOT NULL,
    -- baselines explicables primero (plan 8.2): 'naive_seasonal','arima','ets',
    -- 'random_forest','xgboost','rule_baseline'
    algorithm_params jsonb,
    status           text NOT NULL DEFAULT 'running'
        CHECK (status IN ('running','succeeded','failed')),
    started_at       timestamptz NOT NULL DEFAULT now(),
    finished_at      timestamptz,
    trained_by       text,                       -- servicio python-ml
    artifact_uri     text,                       -- ruta del modelo serializado (blob/local)
    notes            text
);

COMMENT ON TABLE ml.training_run IS 'Cada corrida de entrenamiento: dataset exacto, algoritmo, hiperparametros y artefacto resultante. Es la capa de reproducibilidad del pipeline.';

-- 5.4 Model registry versionado
CREATE TABLE ml.model_version (
    model_version_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    training_run_id  bigint NOT NULL REFERENCES ml.training_run(training_run_id),
    model_name       text NOT NULL,              -- 'cpu_forecast_1h'
    version          integer NOT NULL,
    framework        text,                       -- 'statsforecast','scikit-learn','xgboost'
    stage            text NOT NULL DEFAULT 'candidate'
        CHECK (stage IN ('candidate','champion','archived','retired')),
    params           jsonb,
    metrics_summary  jsonb,                      -- {"mae":..,"rmse":..,"mape":..}
    promoted_at      timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (model_name, version)
);

COMMENT ON TABLE ml.model_version IS 'Registro de modelos con ciclo de vida (candidate/champion/archived). Solo el champion alimenta forecasts productivos; la promocion queda fechada y trazable.';

-- 5.5 Forecasts emitidos (con intervalo predictivo)
CREATE TABLE ml.forecast (
    forecast_id      bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    forecast_uid     uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE, -- ID estable de integración
    model_version_id bigint NOT NULL REFERENCES ml.model_version(model_version_id),
    resource_id      integer NOT NULL REFERENCES core.resource(resource_id),
    target_metric    text NOT NULL
        CHECK (target_metric IN ('cpu_avg','workload_intensity','storage_used_bytes')),
    horizon_steps    integer NOT NULL,           -- 1..H
    step_unit        text NOT NULL DEFAULT '1h'
        CHECK (step_unit IN ('15m','1h','1d')),
    period_start_utc timestamptz NOT NULL,       -- periodo futuro pronosticado
    y_pred           numeric(18,4) NOT NULL,
    y_lower          numeric(18,4),              -- intervalo predictivo (KPI cobertura 10.4)
    y_upper          numeric(18,4),
    alpha            numeric(5,4),
    generated_at     timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE ml.forecast IS 'Prediccion por periodo futuro de las variables que el optimizador consume (CPU, intensidad, storage). El costo NUNCA se pronostica directamente: se calcula con core.price_catalog (plan 8.3).';

CREATE INDEX idx_forecast_lookup ON ml.forecast (resource_id, target_metric, period_start_utc);

-- 5.6 Evaluación de forecasting (KPIs 10.4)
CREATE TABLE ml.forecast_evaluation (
    evaluation_id     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    model_version_id  bigint NOT NULL REFERENCES ml.model_version(model_version_id),
    resource_id       integer NOT NULL REFERENCES core.resource(resource_id),
    target_metric     text NOT NULL,
    window_start_utc  timestamptz NOT NULL,
    window_end_utc    timestamptz NOT NULL,
    horizon_steps     integer NOT NULL,
    n_points          integer,
    mae               numeric(18,4),
    rmse              numeric(18,4),
    mape              numeric(10,4),
    smape             numeric(10,4),
    interval_coverage numeric(6,4),              -- fraccion de puntos dentro de [y_lower, y_upper]
    pinball_loss      numeric(18,6),
    evaluated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (model_version_id, resource_id, target_metric, window_start_utc, horizon_steps)
);

COMMENT ON TABLE ml.forecast_evaluation IS 'Metricas de evaluacion temporal (MAE, RMSE, MAPE y cobertura del intervalo predictivo) por modelo, recurso y ventana: el forecasting se valida con datos, no con impresiones.';

-- 5.7 Vista: leaderboard de modelos
CREATE VIEW ml.v_model_leaderboard AS
SELECT
    mv.model_name,
    mv.version,
    mv.stage,
    tr.algorithm,
    mv.metrics_summary->>'mae'  AS mae,
    mv.metrics_summary->>'rmse' AS rmse,
    mv.metrics_summary->>'mape' AS mape,
    mv.created_at
FROM ml.model_version mv
JOIN ml.training_run tr ON tr.training_run_id = mv.training_run_id
ORDER BY mv.model_name, mv.created_at DESC;

COMMENT ON VIEW ml.v_model_leaderboard IS 'Comparacion rapida de modelos por target: insumo para elegir champion y para la memoria de la tesis (baselines vs propuesta).';

-- =============================================================================
-- 6. OPT — optimización, recomendación y riesgo
-- =============================================================================

-- 6.1 Escenario de optimización (petición del motor / usuario)
CREATE TABLE opt.scenario (
    scenario_id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    scenario_uid      uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE, -- ID estable de integración
    resource_id       integer NOT NULL REFERENCES core.resource(resource_id),
    created_by        text NOT NULL DEFAULT 'system',
    horizon_days      integer NOT NULL DEFAULT 30,
    objective_weights jsonb NOT NULL DEFAULT '{"cost": 1.0}',   -- min C(c), ec. 1
    constraints       jsonb NOT NULL,
    -- ejemplo: {"p95_latency_increase_pct_max": 10, "sla_violations_max": 0}
    status            text NOT NULL DEFAULT 'open'
        CHECK (status IN ('open','completed','cancelled')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    completed_at      timestamptz
);

COMMENT ON TABLE opt.scenario IS 'Una consulta de optimizacion: recurso, horizonte, pesos del objetivo y restricciones explicitas (p95 latency <= +10%, sin violaciones SLA). Todo lo que ocurre despues queda colgado de aqui.';

-- 6.2 Configuración candidata (generador de candidatos)
CREATE TABLE opt.candidate_configuration (
    candidate_id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    scenario_id            bigint NOT NULL REFERENCES opt.scenario(scenario_id),
    sku_id                 integer REFERENCES catalog.sql_sku(sku_id),
    compute_model          text
        CHECK (compute_model IN ('provisioned','serverless')),
    capacity_vcores        numeric(4,1),
    generated_by           text NOT NULL,
    -- 'rule_percentile','forecast_based','advisor_mapping','manual'
    estimated_monthly_cost numeric(18,2),
    price_basis            text,                -- 'retail_consumption','reservation_1yr',...
    created_at             timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE opt.candidate_configuration IS 'Candidatos C1..Cn generados para un escenario (ec. 12). Siempre referencian catalog.sql_sku: el espacio de busqueda es finito y auditable.';

-- 6.3 Simulación del escenario (evaluación ANTES de recomendar)
CREATE TABLE opt.scenario_simulation (
    simulation_id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    candidate_id                 bigint NOT NULL REFERENCES opt.candidate_configuration(candidate_id),
    model_version_id             bigint REFERENCES ml.model_version(model_version_id),
    forecast_period_start_utc    timestamptz,
    forecast_period_end_utc      timestamptz,
    predicted_cpu_p95            numeric(8,4),
    predicted_cpu_p99            numeric(8,4),
    predicted_workload_intensity numeric(12,4),
    predicted_latency_p95_ms     numeric(12,2),
    predicted_sla_violations     integer,
    predicted_monthly_cost       numeric(18,2),
    estimated_savings_pct        numeric(6,3),
    feasible                     boolean,
    violated_constraints         text[],
    simulated_at                 timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE opt.scenario_simulation IS 'Resultado de simular cada candidato con los forecasts (lineage a model_version): costo proyectado con price_catalog, riesgo de rendimiento y factibilidad contra las restricciones del escenario.';

-- 6.4 Prueba experimental real (ground truth — plan 16.1)
CREATE TABLE opt.experiment_trial (
    trial_id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    experiment_id               integer REFERENCES ml.experiment(experiment_id),
    workload_run_id             bigint NOT NULL REFERENCES core.workload_run(workload_run_id),
    resource_id                 integer NOT NULL REFERENCES core.resource(resource_id),
    sku_id                      integer REFERENCES catalog.sql_sku(sku_id),
    config_snapshot_id          bigint,           -- FK diferida
    started_at_utc              timestamptz NOT NULL,
    finished_at_utc             timestamptz,
    duration_s                  numeric(12,2),
    measured_monthly_cost       numeric(18,2),
    measured_cpu_avg            numeric(8,4),
    measured_cpu_p95            numeric(8,4),
    measured_cpu_p99            numeric(8,4),
    measured_latency_p95_ms     numeric(12,2),
    measured_throughput_ops_min numeric(12,2),
    sla_violations              integer NOT NULL DEFAULT 0,
    is_ground_truth             boolean NOT NULL DEFAULT false,
    notes                       text
);

COMMENT ON TABLE opt.experiment_trial IS 'Medicion REAL de un workload ejecutado contra una configuracion (W -> {C1..Cn}). Es el ground truth del proyecto: valida simulaciones, calcula factibilidad (KPI 10.3) y el False Optimization Rate.';

-- 6.5 Recomendación versionada y trazable (Cuadro 5 del plan)
CREATE TABLE opt.recommendation (
    recommendation_id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recommendation_uid        uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE, -- ID estable entre cco_data y cco_app
    scenario_id               bigint NOT NULL REFERENCES opt.scenario(scenario_id),
    chosen_candidate_id       bigint REFERENCES opt.candidate_configuration(candidate_id),
    version                   integer NOT NULL,
    supersedes_id             bigint REFERENCES opt.recommendation(recommendation_id),
    current_sku_name          text NOT NULL,
    current_compute_model     text,
    current_capacity_vcores   numeric(4,1),
    proposed_sku_name         text NOT NULL,
    proposed_compute_model    text,
    proposed_capacity_vcores  numeric(4,1),
    estimated_savings_pct     numeric(6,3) NOT NULL,
    estimated_monthly_savings numeric(18,2),
    confidence                numeric(5,4) CHECK (confidence BETWEEN 0 AND 1),
    status                    text NOT NULL DEFAULT 'proposed'
        CHECK (status IN ('proposed','under_review','accepted','rejected','applied','dismissed')),
    rationale                 text NOT NULL,       -- racional legible (Cuadro 5)
    generated_by              text NOT NULL,       -- 'optimizer_v1'
    created_at                timestamptz NOT NULL DEFAULT now(),
    decided_at                timestamptz,
    decided_by                text,                -- human-in-the-loop (plan 19)
    applied_at                timestamptz,
    UNIQUE (scenario_id, version)
);

COMMENT ON TABLE opt.recommendation IS 'Recomendacion final versionada: que se propone, cuanto ahorra, con que confianza y quien decidio. Nunca se actualiza el contenido: se publica version n+1 con supersedes_id (trazabilidad 18.7).';

-- 6.6 Evidencia estructurada de la recomendación (fuente del LLM — plan 15)
CREATE TABLE opt.recommendation_evidence (
    evidence_id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recommendation_id  bigint NOT NULL REFERENCES opt.recommendation(recommendation_id),
    evidence_type      text NOT NULL,
    -- 'cpu_p95','cpu_p99','data_io_p95','trend','peak_hours','storage_growth','advisor_match',...
    metric_code        text REFERENCES catalog.metric_catalog(metric_code),
    value              numeric(18,6),
    unit               text,
    window_start_utc   timestamptz,
    window_end_utc     timestamptz,
    context            jsonb,        -- refs a feature_snapshot, model_version, sim, etc.
    created_at         timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE opt.recommendation_evidence IS 'Evidencia cuantitativa que soporta la recomendacion (CPU p95/p99, I/O p95, tendencia, horas pico). El LLM SOLO narra esta evidencia: no genera datos ni configuraciones.';

-- 6.7 Evaluación de riesgo reproducible (ec. 3)
CREATE TABLE opt.risk_assessment (
    risk_id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recommendation_id  bigint NOT NULL REFERENCES opt.recommendation(recommendation_id),
    model_version      text NOT NULL DEFAULT 'rule_v1',   -- identidad del modelo de riesgo
    peak_usage         numeric(8,4),
    variability        numeric(8,4),
    trend              numeric(10,6),
    headroom           numeric(8,4),
    risk_score         numeric(6,4) NOT NULL,
    risk_level         text NOT NULL
        CHECK (risk_level IN ('low','medium','high','critical')),
    factors            jsonb NOT NULL,     -- desglose reproducible del puntaje
    assessed_at        timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE opt.risk_assessment IS 'Risk = f(PeakUsage, Variability, Trend, Headroom). Los insumos y el desglose se persisten: el score es reproducible y auditable; si manana hay un modelo ML de riesgo, cambia model_version sin tocar el resto.';

-- 6.8 Database Efficiency Score versionado (ec. 4-5)
CREATE TABLE opt.efficiency_score (
    score_id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    resource_id        integer NOT NULL REFERENCES core.resource(resource_id),
    period_type        text NOT NULL CHECK (period_type IN ('1h','1d','30d')),
    period_start_utc   timestamptz NOT NULL,
    capacity_e         numeric(8,4),
    cost_e             numeric(8,4),
    performance_e      numeric(8,4),
    weight_capacity    numeric(5,4) NOT NULL,
    weight_cost        numeric(5,4) NOT NULL,
    weight_performance numeric(5,4) NOT NULL,
    weights_version    text NOT NULL,             -- p.ej. 'w_2026_09_equal'
    des_score          numeric(8,4) NOT NULL,
    computed_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (resource_id, period_type, period_start_utc, weights_version),
    CHECK (weight_capacity + weight_cost + weight_performance BETWEEN 0.999 AND 1.001)
);

COMMENT ON TABLE opt.efficiency_score IS 'DES = wc*E_capacity + we*E_cost + wp*E_performance con pesos que suman 1. Los pesos se versionan para el analisis de sensibilidad (plan 22); por eso el UNIQUE incluye weights_version.';

-- 6.9 Comparación contra Azure Advisor (baseline externo — plan 16.2)
CREATE TABLE opt.advisor_comparison (
    advisor_comparison_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    scenario_id           bigint REFERENCES opt.scenario(scenario_id),
    recommendation_id     bigint REFERENCES opt.recommendation(recommendation_id),
    advisor_rec_id        bigint REFERENCES raw.advisor_recommendation(advisor_rec_id),
    agreement             text
        CHECK (agreement IN ('agree','disagree','partial','not_comparable')),
    notes                 text,
    created_at            timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE opt.advisor_comparison IS 'Mapeo entre recomendaciones propias y de Azure Advisor. Referencia externa, no ground truth universal (plan 16.2).';

-- 6.10 Vista: última recomendación vigente por recurso
CREATE VIEW opt.v_recommendation_latest AS
SELECT DISTINCT ON (s.resource_id)
    s.resource_id,
    rec.recommendation_id,
    rec.scenario_id,
    rec.version,
    rec.current_sku_name,
    rec.proposed_sku_name,
    rec.proposed_compute_model,
    rec.proposed_capacity_vcores,
    rec.estimated_savings_pct,
    rec.confidence,
    rec.status,
    rec.created_at
FROM opt.recommendation rec
JOIN opt.scenario s ON s.scenario_id = rec.scenario_id
WHERE rec.status NOT IN ('dismissed','rejected')
ORDER BY s.resource_id, rec.created_at DESC;

COMMENT ON VIEW opt.v_recommendation_latest IS 'Recomendacion mas reciente no descartada por recurso: el dashboard consume esta vista sin reglas de negocio en el frontend.';

-- =============================================================================
-- 7. Integridad diferida (dependencias cruzadas core <-> raw <-> feat/opt)
-- =============================================================================

ALTER TABLE core.resource_config_snapshot
    ADD CONSTRAINT fk_config_snapshot_collector
    FOREIGN KEY (collector_run_id) REFERENCES raw.collector_run(collector_run_id);

ALTER TABLE core.workload_run
    ADD CONSTRAINT fk_workload_run_config
    FOREIGN KEY (config_snapshot_id) REFERENCES core.resource_config_snapshot(config_snapshot_id);

ALTER TABLE feat.feature_snapshot
    ADD CONSTRAINT fk_feature_snapshot_config
    FOREIGN KEY (config_snapshot_id) REFERENCES core.resource_config_snapshot(config_snapshot_id);

ALTER TABLE opt.experiment_trial
    ADD CONSTRAINT fk_trial_config
    FOREIGN KEY (config_snapshot_id) REFERENCES core.resource_config_snapshot(config_snapshot_id);

-- =============================================================================
-- 8. CONTRATO EXTERNO CON cco_app (read-only, sin FK cross-database)
-- =============================================================================
CREATE TABLE contract.contract_version (
    contract_name    text PRIMARY KEY,
    contract_version text NOT NULL,
    producer         text NOT NULL DEFAULT 'cco_data',
    consumer         text NOT NULL DEFAULT 'cco_app',
    status           text NOT NULL DEFAULT 'active' CHECK (status IN ('draft','active','deprecated')),
    published_at     timestamptz NOT NULL DEFAULT now(),
    notes            text
);
COMMENT ON TABLE contract.contract_version IS 'Registro de versiones del contrato público. Las vistas contract.v_* son la superficie estable; las tablas internas y sus surrogate keys no forman parte del contrato.';
INSERT INTO contract.contract_version (contract_name, contract_version, notes) VALUES
    ('cco_data.external', 'v1', 'UUIDs públicos, vistas read-only y semántica de percentiles; sin FK ni joins cross-database.')
ON CONFLICT (contract_name) DO NOTHING;

CREATE FUNCTION contract.prevent_public_uid_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.resource_uid IS DISTINCT FROM OLD.resource_uid THEN
        RAISE EXCEPTION 'resource_uid is immutable once published in cco_data.external.v1';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_resource_uid_immutable
BEFORE UPDATE OF resource_uid ON core.resource
FOR EACH ROW EXECUTE FUNCTION contract.prevent_public_uid_change();

CREATE FUNCTION contract.prevent_forecast_uid_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.forecast_uid IS DISTINCT FROM OLD.forecast_uid THEN
        RAISE EXCEPTION 'forecast_uid is immutable once published in cco_data.external.v1';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_forecast_uid_immutable
BEFORE UPDATE OF forecast_uid ON ml.forecast
FOR EACH ROW EXECUTE FUNCTION contract.prevent_forecast_uid_change();

CREATE FUNCTION contract.prevent_scenario_uid_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.scenario_uid IS DISTINCT FROM OLD.scenario_uid THEN
        RAISE EXCEPTION 'scenario_uid is immutable once published in cco_data.external.v1';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_scenario_uid_immutable
BEFORE UPDATE OF scenario_uid ON opt.scenario
FOR EACH ROW EXECUTE FUNCTION contract.prevent_scenario_uid_change();

CREATE FUNCTION contract.prevent_recommendation_uid_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.recommendation_uid IS DISTINCT FROM OLD.recommendation_uid THEN
        RAISE EXCEPTION 'recommendation_uid is immutable once published in cco_data.external.v1';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER trg_recommendation_uid_immutable
BEFORE UPDATE OF recommendation_uid ON opt.recommendation
FOR EACH ROW EXECUTE FUNCTION contract.prevent_recommendation_uid_change();

CREATE VIEW contract.v_resource AS
SELECT resource_uid, provider_code, subscription_id, resource_group, server_name,
       database_name, azure_resource_id, region_code, kind, lifecycle_status,
       registered_at, deactivated_at
FROM core.resource;

CREATE VIEW contract.v_forecast AS
SELECT f.forecast_uid, r.resource_uid, f.target_metric, f.horizon_steps,
       f.step_unit, f.period_start_utc, f.y_pred, f.y_lower, f.y_upper,
       f.alpha, f.generated_at
FROM ml.forecast f
JOIN core.resource r ON r.resource_id = f.resource_id;

CREATE VIEW contract.v_scenario AS
SELECT s.scenario_uid, r.resource_uid, s.horizon_days, s.objective_weights,
       s.constraints, s.status, s.created_by, s.created_at, s.completed_at
FROM opt.scenario s
JOIN core.resource r ON r.resource_id = s.resource_id;

CREATE VIEW contract.v_percentile_statistic AS
SELECT r.resource_uid, ps.period_type, ps.period_start_utc,
       ps.metric_code, ps.percentile, ps.value, ps.method,
       ps.observation_count, ps.source_granularity, ps.computed_at
FROM feat.percentile_statistic ps
JOIN core.resource r ON r.resource_id = ps.resource_id;

CREATE VIEW contract.v_recommendation AS
SELECT rec.recommendation_uid, s.scenario_uid, r.resource_uid,
       rec.version, rec.current_sku_name, rec.current_compute_model,
       rec.current_capacity_vcores, rec.proposed_sku_name,
       rec.proposed_compute_model, rec.proposed_capacity_vcores,
       rec.estimated_savings_pct, rec.estimated_monthly_savings,
       rec.confidence, rec.status, rec.rationale, rec.generated_by,
       rec.created_at, rec.decided_at, rec.decided_by, rec.applied_at
FROM opt.recommendation rec
JOIN opt.scenario s ON s.scenario_id = rec.scenario_id
JOIN core.resource r ON r.resource_id = s.resource_id;

COMMENT ON VIEW contract.v_resource IS 'cco_app.cloud.resource_bindings.data_resource_uid = resource_uid. No resource_id is exposed.';
COMMENT ON VIEW contract.v_forecast IS 'Read-only public forecast surface. Consumers use forecast_uid and resource_uid.';
COMMENT ON VIEW contract.v_scenario IS 'Read-only public optimization scenario surface. Consumers use scenario_uid and resource_uid.';
COMMENT ON VIEW contract.v_percentile_statistic IS 'Read-only statistical quality surface. Consumers must inspect method before treating a percentile as exact.';
COMMENT ON VIEW contract.v_recommendation IS 'Read-only public recommendation surface. cco_app stores recommendation_uid in interactions; it never updates cco_data directly.';

-- =============================================================================
-- 8. Operación: roles, retención y particiones
-- =============================================================================

-- Crea las particiones mensuales de las series de alto volumen. Ejecutar
-- anticipadamente (por ejemplo, para los próximos 3 meses) con un rol migrator.
CREATE OR REPLACE FUNCTION raw.ensure_monthly_partitions(p_month date)
RETURNS void
LANGUAGE plpgsql
SECURITY INVOKER
AS $$
DECLARE
    v_start date := date_trunc('month', p_month)::date;
    v_end   date := (date_trunc('month', p_month) + interval '1 month')::date;
    v_suffix text := to_char(v_start, 'YYYY_MM');
BEGIN
    EXECUTE format('CREATE TABLE IF NOT EXISTS raw.metric_reading_p%s PARTITION OF raw.metric_reading FOR VALUES FROM (%L) TO (%L)', v_suffix, v_start, v_end);
    EXECUTE format('CREATE TABLE IF NOT EXISTS raw.db_resource_stats_p%s PARTITION OF raw.db_resource_stats FOR VALUES FROM (%L) TO (%L)', v_suffix, v_start, v_end);
    EXECUTE format('CREATE TABLE IF NOT EXISTS raw.activity_log_p%s PARTITION OF raw.activity_log FOR VALUES FROM (%L) TO (%L)', v_suffix, v_start, v_end);
    EXECUTE format('CREATE TABLE IF NOT EXISTS feat.feature_snapshot_p%s PARTITION OF feat.feature_snapshot FOR VALUES FROM (%L) TO (%L)', v_suffix, v_start, v_end);
END;
$$;

COMMENT ON FUNCTION raw.ensure_monthly_partitions(date) IS 'Crea las particiones mensuales de raw y feat. Ejecutar antes de cada periodo y comprobar que DEFAULT no contenga filas del rango.';

-- Monitoreo de filas que cayeron en DEFAULT por falta de partición mensual.
CREATE VIEW raw.v_default_partition_health AS
SELECT 'metric_reading'::text AS parent_table, count(*) AS row_count FROM raw.metric_reading_default
UNION ALL
SELECT 'db_resource_stats', count(*) FROM raw.db_resource_stats_default
UNION ALL
SELECT 'activity_log', count(*) FROM raw.activity_log_default
UNION ALL
SELECT 'feature_snapshot', count(*) FROM feat.feature_snapshot_default;

COMMENT ON VIEW raw.v_default_partition_health IS 'Debe permanecer en cero; filas en DEFAULT indican que falta crear una partición mensual o que hay datos fuera de planificación.';

-- Plantillas de roles, retención y permisos para completar según el entorno.

-- Roles por schema (separación conceptual Y física de permisos):
--   CREATE ROLE app_backend LOGIN;      -- Go: core, raw (collector), opt (lectura+escritura)
--   CREATE ROLE app_mlservice LOGIN;    -- Python: feat, ml (lectura+escritura), raw (lectura)
--   CREATE ROLE app_frontend LOGIN;     -- vistas de opt y core, solo lectura
--   GRANT USAGE ON SCHEMA catalog, core, raw TO app_backend;
--   GRANT SELECT, INSERT ON ALL TABLES IN SCHEMA raw, core TO app_backend;
--   GRANT USAGE ON SCHEMA feat, ml TO app_mlservice;
--   GRANT SELECT ON ALL TABLES IN SCHEMA raw, core, catalog TO app_mlservice;
--   GRANT SELECT ON opt.v_recommendation_latest TO app_frontend;
--   GRANT USAGE ON SCHEMA contract TO cco_data_backend, cco_data_readonly;
--   GRANT SELECT ON contract.v_resource, contract.v_forecast, contract.v_scenario,
--       contract.v_percentile_statistic, contract.v_recommendation
--       TO cco_data_backend, cco_data_readonly;

-- Retención de crudo (control de costos del laboratorio — plan 19):
--   -- Archivar un mes viejo sin borrar fila a fila:
--   ALTER TABLE raw.metric_reading DETACH PARTITION raw.metric_reading_p2026_09;
--   -- Exportar a almacenamiento barato y luego:
--   DROP TABLE raw.metric_reading_p2026_09;
--   -- Las particiones DEFAULT evitan fallas de ingesta cuando falta crear el mes.

-- Crear particiones del mes siguiente (cron mensual):
--   CREATE TABLE raw.metric_reading_p2026_12 PARTITION OF raw.metric_reading
--       FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');

-- Dedup de ingesta re-ejecutable (idempotencia del collector):
--   INSERT INTO raw.metric_reading (...) VALUES (...)
--   ON CONFLICT (resource_id, metric_code, timestamp_utc, interval_seconds) DO NOTHING;

-- Identificadores de integración: nunca usar las claves identity internas
-- Las surrogate keys (resource_id, scenario_id, forecast_id, recommendation_id) permanecen internas.
-- El contrato externo publica solo UUIDs estables y vistas contract.v_*.

-- Índices de FK y consultas operativas frecuentes.
CREATE INDEX idx_config_snapshot_collector
    ON core.resource_config_snapshot (collector_run_id);
CREATE INDEX idx_cost_daily_resource_date_desc
    ON core.cost_daily (resource_id, usage_date DESC);
CREATE INDEX idx_resource_region_status
    ON core.resource (region_code, lifecycle_status);
CREATE INDEX idx_forecast_model
    ON ml.forecast (model_version_id);
CREATE INDEX idx_scenario_resource_created
    ON opt.scenario (resource_id, created_at DESC);
CREATE INDEX idx_candidate_scenario
    ON opt.candidate_configuration (scenario_id);
CREATE INDEX idx_simulation_candidate
    ON opt.scenario_simulation (candidate_id);
CREATE INDEX idx_recommendation_scenario
    ON opt.recommendation (scenario_id);
CREATE INDEX idx_recommendation_evidence_recommendation
    ON opt.recommendation_evidence (recommendation_id);
CREATE INDEX idx_risk_recommendation
    ON opt.risk_assessment (recommendation_id);

-- Idempotencia lógica de forecasts y simulaciones.
ALTER TABLE ml.forecast
    ADD CONSTRAINT ux_forecast_logical
    UNIQUE (model_version_id, resource_id, target_metric, horizon_steps, step_unit, period_start_utc);

CREATE UNIQUE INDEX ux_scenario_simulation_logical
    ON opt.scenario_simulation (candidate_id, model_version_id, forecast_period_start_utc, forecast_period_end_utc);

-- Protección append-only: aplicar después de crear los roles del entorno.
-- El collector no debe recibir UPDATE, DELETE ni TRUNCATE sobre raw.
-- REVOKE UPDATE, DELETE, TRUNCATE ON ALL TABLES IN SCHEMA raw FROM cco_data_collector;
-- GRANT INSERT, SELECT ON ALL TABLES IN SCHEMA raw TO cco_data_collector;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA raw, core, ml, opt TO cco_data_collector;
-- El rol cco_data_archive/migrator debe ser el único con permisos de retención.

-- Identificadores de integración: nunca usar las claves identity internas
-- Las surrogate keys (resource_id, scenario_id, forecast_id, recommendation_id) permanecen internas.
-- El contrato externo publica solo UUIDs estables y vistas contract.v_*.
