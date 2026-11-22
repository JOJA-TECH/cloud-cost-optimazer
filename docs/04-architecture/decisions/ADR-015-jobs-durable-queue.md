# ADR-015 — Módulo de jobs duraderos y worker (Fase 3 del roadmap)

- **Estado:** Aceptada e implementada; validación PostgreSQL end-to-end pendiente
- **Fecha:** 2026-11-22
- **Decisores:** Equipo del proyecto
- **Relacionado con:** ADR-002, ADR-003, ADR-004, ADR-010, REQ-004

## Contexto

El scheduler de adquisición y el ciclo de costos ya se ejecutan en goroutines dentro del proceso HTTP con contexto cancelable y shutdown ordenado (ADR-010), pero no existe una cola duradera de trabajos con estados, reintentos, cancelación e idempotencia. Las tareas lentas (sincronización Azure, adquisición de costos, forecasting, generación de recomendaciones pesadas, reportes) compiten con las solicitudes API y no hay un modelo de `202 Accepted` con `job_id` para consultar estado de forma asíncrona.

La auditoría de `develop` (commit `cfbc97a`) marca como PARCIAL la fase 3 (jobs): falta tabla durable, repository de jobs, worker pool, estados, reintentos, backoff, idempotencia, cancelación y reclamación transaccional.

## Decisión

1. Se agrega el módulo `backend/internal/jobs/` con tabla duradera `app.jobs` en `cco_app` (no en `cco_data`, para no acoplar la cola a la base analítica y evitar consultas cruzadas entre bases). PK uuid generada en Go (SQLite no requiere `pgcrypto`).
2. Estados: `queued`, `running`, `completed`, `failed`, `cancelled`. Campos: `type`, `organization_id`, `project_id`, `attempts`, `max_attempts`, `payload` (JSON), `idempotency_key` (único), `scheduled_at`, `started_at`, `finished_at`, `last_error`, `duration_ms`, `created_at`, `updated_at`.
3. `Claim` reclama un job con `FOR UPDATE SKIP LOCKED` en PostgreSQL (evita doble procesamiento entre instancias); en SQLite devuelve `ErrNoJobsAvailable` (dev local).
4. `Enqueue` respeta `idempotency_key`: si ya existe un job no terminal con esa clave, lo devuelve sin duplicar.
5. El worker ejecuta `HandlerFunc` registrado por tipo, marca `completed` o `failed`, y reprograma con backoff exponencial con jitter (`base * 2^(attempts-1) + [0, base)`) hasta `max_attempts`, tras lo cual marca `failed`.
6. El worker arranca con `signal.NotifyContext` y se detiene con el shutdown ordenado del servidor HTTP (`srv.Shutdown`).
7. Endpoints: `POST/GET/DELETE /api/v1/jobs` y `GET /api/v1/jobs` (list), protegidos por permisos `jobs:read` / `jobs:write`.
8. Endpoint de ejemplo de uso: `POST /api/v1/jobs/acquisition` devuelve `202 Accepted` con `job_id` y encola un job de tipo `acquisition:run` que ejecuta `acquisition.Service.RunCycle` para la organización del caller (opcionalmente un `resource_uid`). El path síncrono existente `/api/v1/acquisitions/run` se mantiene sin cambios.
9. No hay microservicios, ni Kafka, ni RabbitMQ, ni Valkey como única fuente de verdad de la cola; Valkey sigue usándose solo para rate limiting y lockout (ADR-008).
10. La aplicación de una recomendación sigue siendo informativa (REQ-004): ningún job ejecuta cambios automáticos sobre recursos cloud.

## Alternativas consideradas

### Cola en memoria (mapa)

Rechazada: no sobrevive reinicios, no da idempotencia ni estado consultable por la API.

### Valkey/Redis como única fuente de verdad de la cola

Rechazada: Valkey es para caché, rate limiting y lockout; no debe ser la única fuente de verdad de adquisición (hallazgo F de la auditoría).

### Microservicio de worker separado

Rechazada para esta fase: el repo es un monolito modular; el worker corre en el mismo proceso con responsabilidad de ejecución distinta. Para dividir en dos procesos basta arrancar solo el worker.

## Consecuencias

### Positivas

- Tareas lentas no bloquean la API HTTP; el cliente recibe `202` y consulta `GET /jobs/{id}`.
- Reintentos con backoff y jitter, cancelación y estados persistentes.
- Idempotencia por `idempotency_key` y reclamación transaccional (`FOR UPDATE SKIP LOCKED`).
- Reutiliza `platform/database` (GORM portable SQLite/PostgreSQL) y los contratos de IAM existentes.

### Riesgos y limitaciones

- En dev con SQLite, `Claim` devuelve `ErrNoJobsAvailable`; el worker solo consume en PostgreSQL. El path síncrono de adquisición sigue disponible en dev.
- Requiere validación con `go test -race` y PostgreSQL real antes de Release 1.0.
- Sin outbox distribuido entre `cco_app` y `cco_data`; un job de adquisición persiste en `cco_data` a través de los contratos de `data`.

## Evidencia

- `backend/internal/jobs/` (models, interfaces, repository, service, worker, handlers).
- `backend/cmd/cco-app/main.go`: `jobs.NewRepository`, `jobs.NewService`, `jobs.NewWorker`, `RegisterRoutes`, `SeedJobsPermissions`, handler `POST /api/v1/jobs/acquisition`.
- `backend/internal/iam/jobs_permissions_seed.go`: permisos `jobs:read`, `jobs:cancel`, `acquisitions:run`, `acquisitions:read`.
- `backend/internal/platform/database/migrate.go`: `migrateJobsTable`.
- Pruebas: `backend/internal/jobs/repository_test.go`.
