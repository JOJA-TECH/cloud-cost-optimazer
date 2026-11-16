# DESIGN-AUDIT-001 — Diseño técnico de Auditoría y Trazabilidad

## Estado

Implementado como módulo transversal. La validación end-to-end con PostgreSQL y dos bases separadas queda pendiente.

## Objetivo y alcance

El módulo `backend/internal/audit/` ofrece registro persistente, consulta y trazabilidad reutilizables para el monolito modular. `cco_app.audit.events` es el registro de auditoría de la aplicación; `log/slog` es el destino de observabilidad operativa. `cco_data.raw.activity_log` conserva actividad recolectada de proveedores cloud y no sustituye la auditoría de la aplicación.

El dominio de auditoría no depende de GORM, Recommendation, Analytics, Cloud, Azure SDK, Terraform ni del frontend. La implementación GORM vive en `audit/repositories`; la integración con `slog` y con Recommendation vive en adapters.

## Componentes

| Componente | Responsabilidad |
|---|---|
| `dto.go` | `AuditEvent`, actor, recurso, contexto de correlación, filtro y `DecisionTrace` |
| `interfaces.go` | Contratos de registro, consulta, contexto, persistencia y trazas de decisión |
| `trace_context.go` | Normaliza y propaga trazabilidad e identidad IAM en `context.Context` |
| `middleware.go` | Genera/propaga headers, captura el resultado HTTP y emite eventos de los flujos instrumentados |
| `service.go` | Valida, enriquece, persiste y registra eventos |
| `decision_trace_service.go` | Persiste y reconstruye trazas de recomendación desde eventos genéricos |
| `handlers.go` | Endpoints de consulta y autorización por alcance |
| `repositories/` | Repositorio y mapeo GORM de `audit.events` |
| `adapters/slog_logger.go` | Registro operativo estructurado y fallos de persistencia |

`cmd/cco-app/main.go` construye un `audit.Service` con el repositorio de `cco_app`, registra las rutas protegidas y lo comparte con el sink persistente y el lector de trazas.

## Entidades y contexto

`AuditEvent` contiene organización, actor, acción, referencia genérica del recurso, timestamp, contexto de trazabilidad y metadata. Los tipos de actor admitidos son `user`, `service_principal`, `managed_identity`, `system` y `anonymous`. Los eventos de sistema usan `actor_type = system` y no asignan `user_id`.

El middleware transversal genera UUID para `X-Request-ID` y `X-Operation-ID`; si no se recibe `X-Correlation-ID`, reutiliza el request ID. Los UUID inválidos se sustituyen por UUID nuevos y el valor inválido se registra de forma segura. Los tres valores se devuelven en los headers de respuesta. IAM conserva la identidad como fuente de actor, organización y método de autenticación; auditoría no crea un sistema de identidad paralelo.

## Persistencia

La tabla `audit.events` de `cco_app` conserva `organization_id`, `user_id`, `actor_type`, `action`, `entity_type`, `entity_id`, `occurred_at`, `request_id`, `correlation_id`, `operation_id`, `event_version`, `ip_address`, `user_agent` y `metadata`.

El dump SQL define las restricciones, políticas RLS, índices y triggers del esquema. La migración GORM incremental incorpora las columnas y controles requeridos por la aplicación:

- Restricción para impedir acciones vacías, tipos de actor admitidos y `metadata` JSONB.
- Políticas RLS para inserción y lectura por usuario/organización, además de acceso de plataforma.
- Triggers append-only que rechazan `UPDATE`, `DELETE` y `TRUNCATE`.
- Índices de correlación, operación y recomendación; el dump SQL también incluye índices por entidad, organización, usuario y acción.

Los valores de Recommendation que no tienen columna dedicada —por ejemplo `recommendation_uid`, `scenario_uid`, política/versión, algoritmo, decisión, riesgo, confianza, ahorro y referencias de evidencia— se guardan en metadata JSON. `DecisionTraceService` representa la decisión como un `AuditEvent` del recurso `recommendation`, asigna `decision_trace_id` y reconstruye trazas desde ese metadata; no crea una dependencia de auditoría hacia el esquema `cco_data`.

## Servicio y fallos

`Audit.Service.Record` normaliza la acción y metadata, completa `occurred_at`, enriquece actor/organización/request/correlation desde IAM cuando existe y persiste mediante `AuditRepository`. Después escribe el evento persistido en `slog`. La metadata se rechaza si no es JSON o contiene nombres de campos sensibles como tokens, contraseñas, credenciales o cadenas de conexión.

Recommendation persiste primero la decisión en `cco_data.opt`; solo después intenta registrar la traza en `cco_app.audit.events`. No existe una transacción ACID entre ambas bases. Si falla la auditoría, el sistema conserva y devuelve la decisión con `audit_status: "audit_pending"` y escribe un error estructurado; en esta versión no hay outbox ni reintento automático.

Las transiciones `accepted`, `rejected` y `dismissed` parten de `proposed` o `under_review`; `applied` solo parte de `accepted`. Estas operaciones registran el estado indicado y el actor; el endpoint `apply` no modifica recursos cloud ni ejecuta infraestructura.

## Consultas y autorización

Endpoints de auditoría:

- `GET /api/v1/audit/events`
- `GET /api/v1/audit/events/{event_id}`
- `GET /api/v1/audit/correlations/{correlation_id}`
- `GET /api/v1/audit/resources/{entity_type}/{entity_id}/history`

La consulta de trazas se expone en `GET /api/v1/recommendations/{uid}/trace`. Primero lee los datos de recomendación desde `cco_data`; luego consulta los eventos en `cco_app`, filtrados por organización o usuario. Los historiales por correlación y operación también se consultan en `cco_app`. Las consultas se combinan en memoria; no hay SQL entre bases.

El permiso general es `audit:read`, con alcances `audit:read:own` y `audit:read:organization`. `platform_admin` puede consultar todos los eventos; `auditor`, `owner` y `admin` quedan restringidos a la organización activa; `viewer` no tiene acceso general. La traza de recomendación requiere `recommendations:read` y conserva el alcance de organización/usuario. Las comprobaciones de permiso y el filtro de alcance se aplican antes de devolver los datos.

Los límites de listado son 50 por defecto, 200 máximo y offset no negativo. El orden es determinista por `occurred_at DESC, event_id DESC`; las trazas se presentan cronológicamente.

## Eventos instrumentados

El middleware y los servicios registran los flujos implementados:

- IAM: registro, login exitoso/fallido, refresh y denegación de autorización.
- Cloud: registro/validación de cuenta y comienzo/resultado de sincronización.
- Acquisition: comienzo/resultado de ciclo.
- Recommendation: evaluación, generación, lectura, reevaluación, aceptación, rechazo, descarte y registro de aplicación.

Los eventos de aceptación/rechazo/descarte/aplicación solo se emiten cuando se invoca el endpoint de transición. No se registran secretos, credenciales ni cuerpos HTTP. Analytics no tiene todavía instrumentación específica de eventos de auditoría; Reportes aún no es un módulo implementado.

## Pruebas y ejecución

Hay pruebas focalizadas para validación, contexto, sink persistente, trazas y consultas. La matriz obligatoria completa de autorización y la integración end-to-end de Recommendation quedan para una fase de validación posterior. Las pruebas SQLite requieren CGO; la prueba PostgreSQL optativa requiere `CCO_TEST_POSTGRES_DSN` apuntando a una base de pruebas dedicada con `audit.events` migrada. La prueba end-to-end de Recommendation necesita conexiones de prueba independientes para `cco_app` y `cco_data`.

La prueba de arquitectura del paquete `audit` debe verificar que el dominio no importe GORM, Recommendation, Analytics, Cloud, Azure, Terraform ni frontend.

## Referencias

- [API del backend](../../backend/API.md)
- [ADR-014 — Auditoría y trazabilidad persistente](../04-architecture/decisions/ADR-014-audit-traceability.md)
- [ADR-006 — Logging del backend](../04-architecture/decisions/ADR-006-logging-standard.md)
- [Diseño de Recommendation](./DESIGN-RECOMMENDATION-001.md)
- [Módulos del backend](../04-architecture/MODULES.md)
