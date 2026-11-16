# ADR-014 — Auditoría y trazabilidad persistente

- **Estado:** Aceptada e implementada; validación PostgreSQL end-to-end pendiente
- **Fecha:** 2026-11-15
- **Decisores:** Equipo del proyecto
- **Relacionado con:** ADR-002, ADR-003, ADR-004, ADR-006, ADR-007, ADR-013, REQ-004

## Contexto

El backend ya propaga identidad IAM y correlation IDs, pero los logs operativos no son un registro de auditoría consultable ni sujeto a autorización. Se requiere reconstruir quién realizó una acción, sobre qué recurso, cuándo y bajo qué operación, incluyendo la explicación de decisiones de Recommendation.

El sistema mantiene dos bases separadas: `cco_app` para la aplicación y `cco_data` para recolección y decisiones analíticas. No hay transacción ACID que abarque ambas.

## Decisión

1. Se mantiene `audit.events` en `cco_app` como registro persistente, append-only y consultable de acciones de la aplicación.
2. `log/slog` sigue siendo el destino de observabilidad y diagnóstico operativo. `raw.activity_log` sigue representando actividad del proveedor cloud y no sustituye los eventos de auditoría de la aplicación.
3. El módulo `backend/internal/audit` define entidades y puertos; GORM queda en `audit/repositories`, y logging/adaptación técnica en `audit/adapters`.
4. El middleware transversal crea o normaliza UUIDs para request, correlation y operation, los propaga por contexto y responde con `X-Request-ID`, `X-Correlation-ID` y `X-Operation-ID`. IAM es la fuente de actor y organización.
5. `audit.events` incluye `correlation_id`, `operation_id`, `actor_type` y `event_version`, con índices, RLS, restricciones y protección contra modificaciones/eliminaciones.
6. Recommendation registra un `DecisionTrace` como evento genérico con identificadores y medidas en metadata. La lectura obtiene el registro de recomendación desde `cco_data` y los eventos desde `cco_app` por consultas separadas, sin SQL cruzado.
7. El acceso a consultas está protegido por `audit:read`, `audit:read:own`, `audit:read:organization` y `recommendations:read` para trazas, aplicando alcance de organización/usuario.
8. La consistencia entre bases es síncrona y eventual: se confirma primero la decisión de `cco_data`, luego se persiste la auditoría en `cco_app`. Si el segundo paso falla, se conserva la recomendación y se informa `audit_pending`. Un outbox queda como evolución posterior.
9. Aceptar, rechazar, descartar y marcar como aplicado son transiciones explícitas autorizadas y auditadas. Marcar `applied` no ejecuta cambios en cloud; REQ-004 sigue prohibiendo aplicación automática.

## Alternativas consideradas

### Solo logs estructurados

Rechazada como mecanismo único: los logs sirven para operación, pero no garantizan un historial persistente consultable con filtros y autorización de aplicación.

### Usar `raw.activity_log`

Rechazada: la actividad del proveedor no necesariamente conoce el usuario, organización, permisos ni correlation ID de la aplicación.

### Tabla de traza de decisión acoplada a Recommendation

No se adopta en esta fase. Los datos de Recommendation ya se conservan en `cco_data`; `audit.events` mantiene referencias genéricas y metadata, y el módulo de auditoría no importa el dominio Recommendation.

### Outbox transaccional

Se reserva para una fase futura. Es la estrategia robusta para entrega/reintentos, pero requiere una tabla y worker en `cco_data`; la primera versión expone el estado pendiente y no oculta el fallo.

## Consecuencias

### Positivas

- Historial auditable y consultable separado de logs operativos y actividad del proveedor.
- Correlación transversal y trazas de Recommendation sin dependencias GORM en el dominio.
- Restricción append-only, RLS y permisos de consulta.
- Fallos de auditoría visibles sin descartar decisiones ya persistidas.

### Riesgos y limitaciones

- Sin outbox, una falla posterior a la transacción de Recommendation puede dejar la auditoría pendiente hasta reconciliación manual.
- Los permisos de base, RLS y consultas por alcance deben validarse con PostgreSQL configurado para pruebas.
- La auditoría específica de Analytics aún no está instrumentada; el módulo de Reportes no existe.

## Evidencia

- `backend/internal/audit/` — dominio, servicio, middleware, handlers y repositorio.
- `backend/internal/recommendation/adapters/persistent_audit_sink.go` — adaptación de decisiones.
- `backend/internal/platform/database/migrate.go` y `databases/cco_app/cco_app.sql` — esquema, RLS, índices y append-only.
- `backend/cmd/cco-app/main.go` — wiring de servicios y rutas.
- `backend/API.md` — endpoints y permisos.
- `docs/05-design/DESIGN-AUDIT-001.md` — diseño técnico del módulo.

## Revisión

- **Fecha prevista:** Al completar pruebas de integración con PostgreSQL para `cco_app` y `cco_data`.
- **Condición de revisión:** Requisitos de entrega garantizada, reintentos automáticos o volúmenes que justifiquen el outbox.
