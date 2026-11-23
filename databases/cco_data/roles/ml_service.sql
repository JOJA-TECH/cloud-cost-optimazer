-- =============================================================================
--  Rol app_mlservice — permisos mínimos para el servicio Python ML (Module 6)
-- =============================================================================
--
-- Este rol es usado exclusivamente por el servicio Python de forecasting para
-- leer el feature store y escribir resultados de ML en cco_data. Nunca tiene
-- acceso a cco_app, credenciales de Azure, ni tablas opt.*.
--
-- Aplicar con un rol de migración/administrador de la base de datos cco_data:
--   psql -d cco_data -f roles/ml_service.sql
-- =============================================================================

-- Rol dedicado para el servicio ML (Slice 4a: separación de permisos)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_mlservice') THEN
        CREATE ROLE app_mlservice LOGIN;
    END IF;
END
$$;

-- ---------------------------------------------------------------------------
-- 1. Uso de esquemas (solo lectura de metadatos, no escritura en tablas ajenas)
-- ---------------------------------------------------------------------------
GRANT USAGE ON SCHEMA catalog TO app_mlservice;
GRANT USAGE ON SCHEMA core TO app_mlservice;
GRANT USAGE ON SCHEMA raw TO app_mlservice;
GRANT USAGE ON SCHEMA feat TO app_mlservice;
GRANT USAGE ON SCHEMA ml TO app_mlservice;

-- ---------------------------------------------------------------------------
-- 2. Lectura: tablas de catálogo, configuración, raw y feature store
-- ---------------------------------------------------------------------------
GRANT SELECT ON ALL TABLES IN SCHEMA catalog TO app_mlservice;
GRANT SELECT ON ALL TABLES IN SCHEMA core TO app_mlservice;
GRANT SELECT ON ALL TABLES IN SCHEMA raw TO app_mlservice;
GRANT SELECT ON ALL TABLES IN SCHEMA feat TO app_mlservice;

-- ---------------------------------------------------------------------------
-- 3. Escritura: ÚNICAMENTE tablas ml.* (registro de modelos, datasets,
--    forecasts y evaluaciones). El servicio ML es propietario de estas
--    entidades; ningún otro rol escribe aquí excepto el backend via Jobs.
-- ---------------------------------------------------------------------------
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ml TO app_mlservice;

-- Secuencias de ml.* (necesarias para identity columns en INSERT)
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ml TO app_mlservice;

-- ---------------------------------------------------------------------------
-- 4. Vistas públicas del contrato (lectura de superficie estable)
-- ---------------------------------------------------------------------------
GRANT USAGE ON SCHEMA contract TO app_mlservice;
GRANT SELECT ON contract.v_forecast TO app_mlservice;
GRANT SELECT ON contract.v_resource TO app_mlservice;

-- ---------------------------------------------------------------------------
-- 5. Permisos por defecto para tablas futuras en ml.*
--    (cualquier tabla nueva creada en ml hereda estos permisos)
-- ---------------------------------------------------------------------------
ALTER DEFAULT PRIVILEGES IN SCHEMA ml
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO app_mlservice;
ALTER DEFAULT PRIVILEGES IN SCHEMA ml
    GRANT USAGE, SELECT ON SEQUENCES TO app_mlservice;

-- ---------------------------------------------------------------------------
-- 6. Nota: app_mlservice NO tiene permisos sobre:
--    - cco_app (base de aplicación, identidad, permisos, auditoría)
--    - opt.* (escenarios, candidatos, recomendaciones, decisión de negocio)
--    - raw.* escritura (solo lectura; el collector es otro rol)
--    - core.* escritura (solo lectura; adquisición escribe aquí)
-- =============================================================================
