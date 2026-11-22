# Módulos del backend

El backend es un monolito modular Go. Los módulos de negocio exponen contratos propios y se ensamblan en `backend/cmd/cco-app/main.go`. La base operativa es `cco_app`; la base de adquisición/analítica es `cco_data`. No se realizan consultas SQL entre ambas bases.

## Catálogo

| Módulo | Paquete | Persistencia | Estado y responsabilidad |
|---|---|---|---|
| 1. Identidad y Acceso | `backend/internal/iam` | `cco_app` | Implementado: autenticación, identidad, permisos y middleware |
| 2. Gestión de Nube | `backend/internal/cloud` | `cco_app` | Implementado: cuentas cloud, validación, recursos y bindings |
| 3. Adquisición | `backend/internal/acquisition` | `cco_app` y contratos de `cco_data` | Implementado: adquisición periódica de métricas, configuración y costos |
| 4. Gestión de Datos | `backend/internal/data` | `cco_data` | Implementado: escritura de adquisición y consultas de datos |
| 5. Analytics | `backend/internal/analytics` | `cco_data` | Implementado: métricas agregadas y evidencia analítica |
| 6. Integración con ML | `backend/internal/analytics` (contrato) + `backend/internal/analytics/adapters` + `src/forecasting/` (servidor Python) | `cco_data` (lectura `feat.*`; escritura `ml.*` vía servicio Python) | Contrato `MLPort` (`Forecast`, `ValidateContract`) disponible. Cliente gRPC productivo (`grpcMLClient`) hacia el servicio Python de Module 6 con `CCO_ML_MODE=grpc` (Slices 1-2). El servidor Python persiste pronósticos en `ml.forecast` de forma idempotente y devuelve `forecast_uid` consultable vía `contract.v_forecast`. Persistencia de `ml.training_run`/`ml.model_version` y promoción a `champion` pendiente (Slice 3). |
| 7. Recommendation | `backend/internal/recommendation` | `cco_data` | Implementado: evaluación, persistencia versionada, trazas y transiciones explícitas de estado |
| 8. Auditoría y Trazabilidad | `backend/internal/audit` | `cco_app.audit.events` | Implementado: captura, persistencia append-only, consulta y correlación |
| 9. Jobs (cola duradera y worker) | `backend/internal/jobs` | `cco_app.app.jobs` | Implementado: creación, reclamación, estados, reintentos con backoff+jitter, idempotencia, cancelación, endpoints de consulta y ejecución asíncrona de adquisición (`acquisition:run`), reportes (`report:optimization`), analítica (`analytics:compute`) y recomendaciones pesadas (`recommendation:generate`) |
| Simulación y Laboratorio | — | — | Pendiente |
| Reportes | — | — | Pendiente |

La API actual está documentada en [backend/API.md](../../backend/API.md). El diseño detallado se encuentra en [DESIGN-AUDIT-001](../05-design/DESIGN-AUDIT-001.md) y [DESIGN-RECOMMENDATION-001](../05-design/DESIGN-RECOMMENDATION-001.md).

## Reglas de dependencia

- IAM es transversal y proporciona identidad/autorización; no importa módulos de negocio.
- Recommendation depende de interfaces y adapters; la decisión persiste en `cco_data`, y su evento se delega a `audit.AuditLogger`/`audit.DecisionTraceService`.
- Auditoría depende de contratos genéricos y UUIDs; no importa Recommendation, Analytics, Cloud, GORM ni SDKs de proveedores desde el paquete de dominio.
- El repositorio de auditoría solo lee y escribe `cco_app.audit.events`; el historial de Recommendation combina lecturas separadas de las dos bases en memoria.
- `slog` es observabilidad operativa; `audit.events` es el registro persistente autorizado; `raw.activity_log` conserva actividad del proveedor cloud.
- Ninguna recomendación o transición de estado ejecuta por sí misma cambios sobre recursos cloud.

## Trazabilidad

El middleware propaga `request_id`, `correlation_id` y `operation_id`. Los eventos incluyen actor, organización, acción, recurso, timestamp y metadata, sin secretos ni cuerpos HTTP.

Los eventos auditados hoy cubren IAM, Cloud, Acquisition y Recommendation. Analytics no tiene todavía instrumentación de auditoría específica. Los permisos generales de auditoría no se conceden a `viewer`; las trazas de recomendaciones usan `recommendations:read` y alcance de organización/usuario.

Ver [ADR-014](./decisions/ADR-014-audit-traceability.md), [ADR-006](./decisions/ADR-006-logging-standard.md) y [Architecture](./ARCHITECTURE.md).
