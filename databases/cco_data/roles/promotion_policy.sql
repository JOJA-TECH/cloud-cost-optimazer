-- =============================================================================
--  Tabla ml.promotion_policy — umbrales de promoción a champion
-- =============================================================================
--
-- Almacena las reglas de calidad que debe cumplir un modelo candidato para
-- ser promovido a champion. Sin una política vigente, cualquier versión
-- candidata puede promoverse (comportamiento por defecto, no recomendado
-- en producción).
--
-- Aplicar con un rol de administrador de base de datos:
--   psql -d cco_data -f roles/promotion_policy.sql
-- =============================================================================

CREATE TABLE IF NOT EXISTS ml.promotion_policy (
    promotion_policy_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    model_name          text NOT NULL,              -- nombre del modelo (ej. 'cpu_forecast_1h')
    version             integer NOT NULL CHECK (version > 0),
    mae_max             numeric(18,4),              -- MAE máximo aceptable (NULL = sin umbral)
    min_improvement_pct numeric(6,3),               -- mejora mínima vs campeón actual (NULL = sin umbral)
    min_training_rows   integer CHECK (min_training_rows > 0), -- filas mínimas de entrenamiento
    enabled             boolean NOT NULL DEFAULT true,
    created_by          text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (model_name, version)
);

COMMENT ON TABLE ml.promotion_policy IS 'Umbrales de calidad para promover un modelo candidato a champion. Sin política vigente, se permite promoción sin restricciones.';

CREATE INDEX IF NOT EXISTS idx_promotion_policy_model_enabled
    ON ml.promotion_policy (model_name, version DESC)
    WHERE enabled;

-- =============================================================================
--  Función helper: evaluar si un candidato cumple los umbrales de promoción
-- =============================================================================

CREATE OR REPLACE FUNCTION ml.can_promote(
    p_model_name text,
    p_version integer,
    OUT eligible boolean,
    OUT reason text
) RETURNS record AS $$
DECLARE
    v_current_champion_version integer;
    v_current_champion_mae numeric(18,4);
    v_candidate_mae numeric(18,4);
    v_policy_mae_max numeric(18,4);
    v_policy_min_improvement numeric(6,3);
    v_policy_min_rows integer;
    v_candidate_rows integer;
BEGIN
    eligible := false;
    reason := '';

    -- 1. Verificar que exista una política habilitada para este modelo/versión
    SELECT mae_max, min_improvement_pct, min_training_rows
    INTO v_policy_mae_max, v_policy_min_improvement, v_policy_min_rows
    FROM ml.promotion_policy
    WHERE model_name = p_model_name
      AND version = p_version
      AND enabled;

    IF NOT FOUND THEN
        -- Sin política vigente: permitir promoción (comportamiento por defecto)
        eligible := true;
        reason := 'no promotion policy defined';
        RETURN;
    END IF;

    -- 2. Obtener métricas del candidato
    SELECT (mv.metrics_summary->>'mae')::numeric(18,4),
           (tr.notes::jsonb->>'rows')::integer
    INTO v_candidate_mae, v_candidate_rows
    FROM ml.model_version mv
    JOIN ml.training_run tr ON tr.training_run_id = mv.training_run_id
    WHERE mv.model_name = p_model_name
      AND mv.version = p_version;

    IF NOT FOUND THEN
        eligible := false;
        reason := 'candidate model not found';
        RETURN;
    END IF;

    -- 3. Verificar filas mínimas de entrenamiento
    IF v_policy_min_rows IS NOT NULL AND v_candidate_rows < v_policy_min_rows THEN
        eligible := false;
        reason := format('insufficient training rows: %s < %s', v_candidate_rows, v_policy_min_rows);
        RETURN;
    END IF;

    -- 4. Verificar MAE máximo
    IF v_policy_mae_max IS NOT NULL AND v_candidate_mae > v_policy_mae_max THEN
        eligible := false;
        reason := format('mae %%.4f exceeds maximum %%.4f', v_candidate_mae, v_policy_mae_max);
        RETURN;
    END IF;

    -- 5. Verificar mejora sobre el campeón actual (si existe)
    SELECT mv.version, (mv.metrics_summary->>'mae')::numeric(18,4)
    INTO v_current_champion_version, v_current_champion_mae
    FROM ml.model_version mv
    WHERE mv.model_name = p_model_name
      AND mv.stage = 'champion'
    ORDER BY mv.version DESC
    LIMIT 1;

    IF v_current_champion_mae IS NOT NULL AND v_policy_min_improvement IS NOT NULL THEN
        -- Mejora requerida: candidato debe ser mejor que campeón por al menos min_improvement_pct
        IF v_candidate_mae >= v_current_champion_mae * (1 - v_policy_min_improvement / 100.0) THEN
            eligible := false;
            reason := format('no sufficient improvement: candidate mae %%.4f vs champion mae %%.4f (required improvement %s%%)',
                             v_candidate_mae, v_current_champion_mae, v_policy_min_improvement);
            RETURN;
        END IF;
    END IF;

    eligible := true;
    reason := 'eligible for promotion';
    RETURN;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION ml.can_promote(text, integer) IS 'Evalúa si un modelo candidato cumple los umbrales de promoción a champion. Devuelve (eligible, reason).';
