# ADR-013 — Motor de Recomendaciones (Module 7)

- **Estado:** Aceptado e implementado; validación PostgreSQL end-to-end pendiente
- **Fecha:** 2026-11-14
- **Decisores:** Equipo del proyecto
- **Relacionado con:** ADR-001, ADR-002, ADR-003, ADR-011, ADR-012, ADR-014, MODULES.md (módulo 7), DESIGN-RECOMMENDATION-001

## Contexto

Analytics (Module 5) produce evidencia y el servicio ML (Module 6) produce forecasts. Ninguno debe tomar la decisión final de optimización. Se necesita un módulo independiente que use esa información junto con configuración, costos, catálogo de SKU, precios y políticas para generar recomendaciones reproducibles, explicables y auditables.

El esquema PostgreSQL `cco_data` ya define la mayoría de las entidades `opt.*`; la implementación Go debe usar esas tablas y mantener el acceso a costos, configuración, SKU y precios detrás de los contratos de Data Management.

## Problema

Definir dónde vive la decisión final, cómo se integran sus dependencias sin acoplar el dominio a GORM o a la implementación ML, y cómo se conserva un historial versionado sin ejecutar cambios en la infraestructura.

## Alternativas consideradas

### Alternativa A: agregar las reglas de negocio a Analytics

**Desventajas:** mezcla evidencia con decisión, acopla los cambios de negocio a los cómputos analíticos y contradice la responsabilidad de Module 5.

### Alternativa B: módulo independiente Recommendation Engine (seleccionada)

**Ventajas**
- Mantiene explícita la separación **Analytics = evidencia; ML = predicciones; Recommendation = decisión**.
- Permite sustituir repositorios, proveedores de forecasts y evaluadores de políticas mediante interfaces.
- Produce decisiones trazables con escenarios, candidatos, simulaciones, riesgos y evidencias.
- Las acciones son descriptivas; el motor nunca ejecuta cambios sobre recursos.

## Decisión

Se decide implementar Recommendation Engine como **Module 7**, bajo `backend/internal/recommendation/`.

1. **Límites y contratos**
   - El dominio declara contratos propios para Analytics, forecasts, costos, configuración, catálogo/precios, políticas, persistencia y auditoría.
   - El paquete de dominio no depende de modelos GORM.
   - Los adapters convierten los contratos existentes de `analytics` y `data`.
   - El lector de forecasts tiene una interfaz separada (`MLPredictionReader`), aunque actualmente lo implementa el mismo adapter que lee resultados analíticos.

2. **Flujo de decisión**
   - Valida recurso, ventana, horizonte, política y moneda.
   - Lee la configuración vigente, evidencia analítica, costos observados y forecasts disponibles.
   - Consulta SKU dentro del catálogo admitido y precios vigentes para la región, SKU y moneda solicitados.
   - Simula candidatos, calcula ahorro, riesgo y confianza, evalúa restricciones y elige una alternativa factible.
   - Si la evidencia es insuficiente o no hay candidato aceptable, produce una decisión `no_action` auditable.
   - Evalúa la política antes de publicar la recomendación.

3. **Persistencia e historial**
   - Usa `opt.optimization_policy`, `opt.scenario`, `opt.candidate_configuration`, `opt.scenario_simulation`, `opt.recommendation`, `opt.recommendation_evidence` y `opt.risk_assessment`.
   - Persiste la decisión en una transacción.
   - Una actualización material crea una recomendación versionada enlazada mediante `supersedes_id`; no modifica la decisión histórica.

4. **Integración y disponibilidad**
   - `main.go` inicializa Data, Analytics y Recommendation Engine cuando existe `CCO_DATA_DSN`.
   - Las rutas de Recommendation se registran aun sin ese DSN. Con servicio no configurado, responden `503 RECOMMENDATION_STORAGE_UNAVAILABLE`; la ausencia del DSN no impide el arranque del proceso.
   - Las rutas requieren `recommendations:write`, `recommendations:evaluate` o `recommendations:read`, según la operación.
   - El endpoint de traza obtiene la recomendación de `cco_data` y los eventos de `cco_app` mediante consultas independientes. Las transiciones de estado son explícitas; `/apply` registra el estado y no ejecuta cambios cloud.

5. **Políticas**
   - Las políticas son entidades versionadas y reutilizables.
   - El evaluador considera, entre otras, riesgo máximo, confianza mínima, permiso de scale down/cambio de modelo y disponibilidad de datos de SLA/latencia cuando la política las exige.

## Justificación

La separación permite probar decisiones con lectores y repositorios simulados, conservar el origen de cada decisión y evolucionar Analytics o ML sin exponer sus modelos de almacenamiento al dominio. Mantener las acciones declarativas evita que la generación de recomendaciones aplique cambios cloud fuera de un flujo explícito de aprobación y ejecución.

## Consecuencias

### Positivas

- Los endpoints pueden generar, evaluar sin persistir, consultar recomendaciones y consultar su evidencia.
- La persistencia transaccional conserva el contexto de la decisión y sus vínculos de política, escenario, candidatos y riesgo.
- Los forecasts se consultan mediante una interfaz independiente de la implementación ML.
- La degradación por forecast ausente se hace visible en el resultado; si la política lo requiere, la decisión se degrada a información insuficiente.

### Limitaciones y riesgos conocidos

- La simulación `rule_v1` aproxima utilización de CPU mediante capacidad relativa y no predice latencia o violaciones SLA si no existe un modelo/dato correspondiente. La política puede rechazar esos casos por evidencia no disponible.
- El cálculo de costo propuesto normaliza precios por hora a un mes promedio de 730 horas y precios mensuales directamente. Debe validarse con tarifas, unidades, tiers y reglas comerciales reales del proveedor antes de usarlo como estimación de producción.
- El acceso a PostgreSQL y el esquema actualizado no se han validado mediante una prueba end-to-end en esta entrega.
- La auditoría se registra mediante `audit.AuditLogger` en `cco_app.audit.events` y también se emite a `slog` para observabilidad. La metadata incluye identificadores, score/nivel de riesgo, confianza, ahorro y evidencia. Por la separación de bases no hay transacción ACID entre recomendación y auditoría; ante un fallo posterior al commit, la respuesta conserva la recomendación con `audit_pending`. No existe outbox en esta versión.

## Evidencia

- `backend/internal/recommendation/` — dominio, orquestación, reglas, scoring, handlers, adapters y repositorio.
- `backend/internal/data/interfaces.go` y `backend/internal/data/repositories/data_repository.go` — contratos y consultas de costos/configuración/SKU/precios.
- `backend/cmd/cco-app/main.go` — inicialización condicional y registro de rutas.
- `backend/internal/audit/decision_trace_service.go` — persistencia y reconstrucción de trazas.
- `backend/internal/recommendation/adapters/persistent_audit_sink.go` — integración persistente de decisiones.
- `databases/cco_data/cco_data_schema.sql` — entidades `opt.*` y vista de forecasts.
- `backend/API.md` — referencia de API del módulo.
- `backend/internal/recommendation/recommendation_service_test.go` y `handlers_test.go` — pruebas de servicio y disponibilidad sin DSN.

## Revisión

- **Fecha prevista de revisión:** Al completar la prueba end-to-end con PostgreSQL y validar la simulación/precios contra datos reales.

## Referencias

[1]: docs/04-architecture/MODULES.md — separación de responsabilidades entre módulos.
[2]: docs/04-architecture/decisions/ADR-012-analytics-module.md — módulo que produce evidencia analítica.
[3]: docs/05-design/DESIGN-RECOMMENDATION-001.md — diseño técnico del motor.
[4]: docs/04-architecture/decisions/ADR-014-audit-traceability.md — auditoría persistente y alcance de trazas.
