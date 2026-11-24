# DESIGN-RECOMMENDATION-001 — Diseño técnico del Motor de Recomendaciones

## Componente

`backend/internal/recommendation/` — módulo Go que implementa Recommendation Engine (Module 7). Convierte configuración, evidencia analítica, costos observados, forecasts, precios y políticas en una decisión versionada y explicable. El módulo declara acciones; no las ejecuta.

Componentes principales:

| Archivo | Responsabilidad |
|---------|-----------------|
| `interfaces.go` | Contratos de dominio para el motor, Analytics, forecasts, costos, configuración, SKU/precios, políticas, repositorio y auditoría |
| `dto.go` | Tipos de entrada/salida y entidades del dominio: Recommendation, Scenario, Candidate, Simulation, Evidence, Risk, Confidence y OptimizationPolicy |
| `errors.go` | Errores de dominio para validación, política, configuración, evidencia, dependencias y persistencia |
| `service.go` | Orquestación de evaluación, generación, versionado, publicación y auditoría |
| `rules.go` | Selección de candidatos, simulación inicial, acciones, oportunidad y rationale basado en evidencia |
| `scoring.go` | Estimación de ahorro, puntuación de riesgo y confianza |
| `policies.go` | Evaluación de límites y restricciones de política |
| `handlers.go` | Rutas HTTP protegidas bajo `/api/v1/recommendations` |
| `adapters/analytics_reader.go` | Conversión de resultados de `analytics.AnalyticsResultRepository` y acceso a forecasts a tipos del dominio |
| `adapters/data_readers.go` | Adaptación de costos, configuración, precios, catálogo SKU y contexto de región/proveedor |
| `adapters/policy_evaluator.go` | Adapter del evaluador de políticas |
| `adapters/audit_sink.go` | Emisión operativa de eventos de decisión con `slog` |
| `adapters/persistent_audit_sink.go` | Conversión de decisiones a `audit.AuditEvent` y delegación en `audit.AuditLogger` |
| `adapters/audit_trace_reader.go` | Consulta de eventos de recomendación, correlación y operación en `cco_app` |
| `repositories/recommendation_repository.go` | Lectura y escritura transaccional de políticas y entidades `opt.*` |

## Responsabilidad

La responsabilidad se divide explícitamente:

- **Data Management:** acceso contractual a costos, configuración, SKU y precios.
- **Analytics (Module 5):** evidencia histórica, percentiles, tendencias, patrones, capacidad, costos y DES.
- **ML (Module 6):** forecasts, consultados por interfaz y no implementados dentro de Recommendation.
- **Recommendation (Module 7):** oportunidad, candidatos, simulación, política, ahorro, riesgo, confianza, selección, explicación y decisión.

La recomendación es una propuesta no ejecutable. La aceptación/rechazo humano y cualquier aplicación posterior pertenecen a flujos separados.

## Dirección de dependencias

```text
acquisition
    ↓
  data ────────────┐
    ↓              │ costos, configuración, catálogo y precios
 analytics ────────┤
    │              │
    └── MLPort ────┤
                   ↓
          recommendation
                   ↓
               opt.*
                   ↓
             REST API
```

- Recommendation depende de interfaces propias y de UUID/tiempo; no importa los modelos GORM de los repositorios.
- Los adapters de infraestructura conocen los paquetes `analytics` y `data` y traducen sus DTOs.
- El adapter de auditoría conoce los contratos genéricos de `audit`; `audit` no depende de Recommendation.
- `main.go` comparte la conexión `CCO_DATA_DSN` entre Data, Analytics y Recommendation.
- La auditoría usa la conexión de `CCO_DB_DSN` (`cco_app`), separada de `cco_data`.
- La ausencia de `CCO_DATA_DSN` deja `recommendationSvc` nulo y las rutas registradas; los handlers responden `503 RECOMMENDATION_STORAGE_UNAVAILABLE`.

## Entidades y persistencia

| Entidad de dominio | Persistencia |
|--------------------|--------------|
| `OptimizationPolicy` | `opt.optimization_policy` |
| `Scenario` | `opt.scenario` |
| `CandidateConfiguration` | `opt.candidate_configuration`, con SKU referenciado a `catalog.sql_sku` |
| `ScenarioSimulation` | `opt.scenario_simulation`, incluyendo `forecast_uid`, `model_version_id`, factibilidad y restricciones violadas |
| `Recommendation` | `opt.recommendation`, incluido tipo de acción, ahorro, confianza, estado y relación de supersesión |
| `RecommendationEvidence` | `opt.recommendation_evidence` |
| `RiskAssessment` | `opt.risk_assessment` |

`PersistDecision` escribe escenario, candidatos, simulaciones, recomendación, evidencias y evaluación de riesgo dentro de una transacción. Los identificadores internos de PostgreSQL no forman parte del contrato público; el dominio y la API usan UUIDs.

## Contratos principales

El dominio expone:

- `AnalyticsReader` para snapshots, percentiles, tendencias, patrones, capacidad, costo analítico y DES.
- `MLPredictionReader` para consultar forecasts de un recurso, métrica y ventana.
- `CostReader`, `ConfigurationReader`, `SKUReader`, `PriceReader` y `ResourceContextReader`.
- `PolicyEvaluator` y `OptimizationPolicyRepository`.
- `RecommendationRepository` para persistencia, lectura, listado y consulta de la última recomendación.
- `AuditSink` para registrar decisiones sin acoplar el dominio a un destino concreto.

La implementación actual usa `adapters.AnalyticsReader` tanto para `AnalyticsReader` como para `MLPredictionReader`; la interfaz de ML permanece separada y permite reemplazar el adapter cuando exista un origen ML dedicado.

## Flujo de generación

`Service.generate` realiza los siguientes pasos:

1. Valida UUID del recurso/política, ventana temporal, horizonte entre 1 y 365 días y moneda de tres letras en mayúsculas.
2. Adquiere un lock en memoria por recurso para evitar generaciones concurrentes dentro del proceso.
3. Obtiene una política existente y habilitada y la última configuración observada; sin SKU/configuración utilizable falla con `RECOMMENDATION_CONFIGURATION_UNAVAILABLE`.
4. Si `force_refresh` es falso, reutiliza la recomendación más reciente solo si coinciden política, ventana, horizonte y moneda. Si cambian los parámetros, genera una nueva versión con `supersedes_id`.
5. Lee snapshots, percentiles de CPU/I/O/sesiones, tendencias, patrón, capacidad, costos analíticos, DES y forecasts.
6. Lee costos diarios observados y normaliza la suma a costo mensual promedio; no mezcla monedas. Consulta contexto regional/proveedor y SKU elegibles desde Data.
7. Genera candidatos de catálogo: SKU actual, SKU adyacente inferior/superior en el mismo tier/familia/modelo y, si está permitido, una opción serverless elegible.
8. Consulta precios vigentes por candidato, calcula costo mensual propuesto y descarta el candidato si falta precio compatible.
9. Simula utilización/costo, calcula ahorro, riesgo y confianza, evalúa la política y aplica reglas iniciales de oportunidad.
10. Selecciona entre candidatos aceptables por costo, y en empate por riesgo y confianza.
11. Si los datos mínimos faltan o no hay candidato aceptable, genera una decisión `no_action`; la salida distingue `insufficient_information` de un `no_action` por ausencia de alternativa aceptable.
12. Persiste la decisión completa en `cco_data.opt` y, solo si esa transacción termina correctamente, registra el evento mediante `AuditSink` en `cco_app.audit.events` y `slog`.

La evaluación preliminar (`EvaluateRecommendation`) usa el mismo flujo de cálculo pero no persiste.

## Reglas y scoring iniciales

### Oportunidad

- **Scale down:** exige percentiles p95/p99, análisis de capacidad, headroom superior al 40%, CPU p95 menor que 40%, CPU p99 menor que 60%, tendencia disponible y no creciente; además, la utilización simulada debe quedar bajo los umbrales configurados en la regla.
- **Scale up:** considera percentiles altos, headroom bajo o forecasts de CPU por encima del umbral.
- **Cambio a serverless:** solo si la política permite cambio de modelo y el patrón/carga satisface las reglas iniciales de baja intensidad e intermitencia.
- **No action:** ahorro no positivo, falta de evidencia, riesgo/confianza fuera de política, simulación no factible o ausencia de candidato aprobado.

Estos umbrales son reglas iniciales (`rule_v1`), no un modelo predictivo de capacidad.

### Costos y ahorro

- Costo actual: suma de costos observados `core.cost_daily` en la ventana, normalizada a un mes promedio; si no hay filas, se usa el análisis de costos disponible como fallback.
- Costo propuesto: precio de catálogo vigente para moneda, región, tipo de precio y SKU; una tarifa por hora se multiplica por 730, una mensual se usa directamente.
- `monthly_amount = current_monthly_cost - proposed_monthly_cost`.
- `monthly_percentage = monthly_amount / current_monthly_cost` cuando el costo actual es positivo.
- Se persisten por separado las bases `actual_cost_daily` y `price_catalog`.

### Riesgo

`calculateRisk` genera un score entre 0 y 1 con los componentes de uso pico, variabilidad, tendencia, headroom, incertidumbre del forecast y distancia de capacidad del candidato. La versión actual usa pesos `rule_v1` (0.27, 0.18, 0.18, 0.20, 0.10 y 0.07, respectivamente) y asigna niveles `low`, `medium`, `high` o `critical`.

### Confianza

`calculateConfidence` conserva los componentes y usa:

```text
0.30 * data_quality_score
+ 0.25 * evidence_completeness_score
+ 0.20 * forecast_quality_score
+ 0.15 * simulation_feasibility_score
+ 0.10 * policy_match_score
```

La ausencia de forecast reduce el componente de forecast; si la política exige forecast, no se emite una acción de optimización como si la predicción estuviera disponible.

### Política

`EvaluatePolicy` valida habilitación, riesgo máximo, confianza mínima, permiso para reducir/cambiar modelo y requisitos de factibilidad. Cuando la política exige límites de SLA o latencia, evidencia de esos atributos no disponible implica rechazo del candidato, en lugar de inferirla a partir de CPU.

## Evidencia y rationale

El rationale se construye a partir de evidencia computada y asociada a la ventana: CPU p95/p99, headroom, costo mensual proyectado, ahorro, horas pico, forecast y restricciones. Los errores parciales de ML se conservan como evidencia y afectan el indicador `degraded`. No se deben generar valores de evidencia narrativos que no existan en las fuentes.

## API y autorización

| Método y ruta | Permiso | Efecto |
|---------------|---------|--------|
| `POST /api/v1/recommendations` | `recommendations:write` | Genera y persiste una recomendación |
| `POST /api/v1/recommendations/evaluate` | `recommendations:evaluate` | Evalúa sin persistir |
| `GET /api/v1/recommendations` | `recommendations:read` | Lista recomendaciones |
| `GET /api/v1/recommendations/{uid}` | `recommendations:read` | Lee una recomendación |
| `GET /api/v1/recommendations/{uid}/evidence` | `recommendations:read` | Lee la evidencia |
| `POST /api/v1/recommendations/{uid}/evaluate` | `recommendations:evaluate` | Reevalúa con datos actuales sin mutar el historial |
| `GET /api/v1/recommendations/{uid}/trace` | `recommendations:read` | Combina la recomendación de `cco_data` con sus eventos de `cco_app`, dentro del alcance del usuario |
| `POST /api/v1/recommendations/{uid}/accept` | `recommendations:write` | Acepta una recomendación propuesta o bajo revisión |
| `POST /api/v1/recommendations/{uid}/reject` | `recommendations:write` | Rechaza una recomendación propuesta o bajo revisión |
| `POST /api/v1/recommendations/{uid}/dismiss` | `recommendations:write` | Descarta una recomendación propuesta o bajo revisión |
| `POST /api/v1/recommendations/{uid}/apply` | `recommendations:write` | Registra como aplicada una recomendación aceptada; no modifica recursos cloud |

Los detalles de request/response y errores HTTP se mantienen en `backend/API.md`.

Las transiciones `accepted`, `rejected` y `dismissed` solo se permiten desde `proposed` o `under_review`; `applied` solo se permite desde `accepted`. La actualización de estado se persiste en `cco_data` antes de auditarla. Si falla la auditoría, la respuesta identifica `audit_pending` sin revertir el estado ya confirmado.

## Auditoría

`PersistentAuditSink` y `DecisionTraceService` adaptan el contrato estable `AuditSink` a `audit.AuditLogger`. El servicio de auditoría persiste en `cco_app.audit.events` y emite el mismo evento a `slog`; `SlogAuditSink` se conserva como adapter operativo alternativo. El evento identifica la recomendación como recurso y conserva `resource_uid`, `scenario_uid`, política/versión, algoritmo, decisión, candidato seleccionado, resultados, `risk_score`, `risk_level`, `confidence_score`, ahorro estimado, cantidad de evidencia, duración y el conteo de errores en metadata (no el texto crudo de errores). Request, correlation y operation IDs se obtienen del contexto de trazabilidad y se guardan en sus columnas UUID.

La auditoría se intenta después del commit en `cco_data`; las dos bases no participan en una transacción distribuida. Si falla la persistencia de auditoría, el sink emite un error estructurado con `recommendation_uid`, `correlation_id` y `operation_id`, y la respuesta conserva la recomendación con `audit_status: "audit_pending"` sin alterar `generation_state`. Esta primera versión no reintenta automáticamente: el estado pendiente requiere reconciliación operativa. Un outbox transaccional en `cco_data` queda como mejora futura.

`GET /api/v1/recommendations/{uid}/trace` consulta primero los datos de recomendación en `cco_data` y luego obtiene eventos en `cco_app` mediante el adapter de trazabilidad. Los historiales se combinan en memoria por `recommendation_uid`, `correlation_id` y `operation_id`; no se emite SQL entre bases.

Las rutas de consulta de auditoría aplican el alcance `all`, `organization` o `own` en el handler antes de responder. El middleware transversal registra acciones IAM, cloud y adquisición con actor y trace context, conserva método/estado HTTP y referencia genérica del recurso, y nunca inspecciona el body. Recommendation registra sus decisiones y transiciones mediante el sink persistente. Los valores con nombres de campos sensibles se rechazan durante la validación de metadata.

## Pruebas y estado de validación

Pruebas unitarias del servicio cubren aprobación/rechazo de políticas, flujo de generación con stubs, configuración ausente, forecast obligatorio ausente y versionado/reutilización según parámetros. Pruebas de handlers verifican el `503` cuando no existe servicio.

Pendiente para completar la validación del diseño:

- Prueba de integración `backend/test/recommendation_integration_test.go` que cree recurso, configuración, costos, evidencia, forecast y política, genere la recomendación, consulte la API y verifique una segunda versión.
- Pruebas directas del repositorio y de `ListPrices` con datos de catálogo; SQLite/CGO y PostgreSQL deben correrse donde estén disponibles.
- Aplicación/validación del esquema `databases/cco_data/cco_data_schema.sql` en PostgreSQL.
- Validación de precios por proveedor, unidad, tiers y políticas de facturación reales.

## Referencias

- `docs/04-architecture/decisions/ADR-013-recommendation-engine.md`
- `docs/04-architecture/decisions/ADR-011-data-module.md`
- `docs/04-architecture/decisions/ADR-012-analytics-module.md`
- `docs/04-architecture/decisions/ADR-014-audit-traceability.md`
- `docs/05-design/DESIGN-AUDIT-001.md`
- `docs/04-architecture/MODULES.md`
- `databases/cco_data/cco_data_schema.sql`
- `backend/API.md`
