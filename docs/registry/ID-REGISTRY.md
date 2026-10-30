# ID Registry

| ID | Tipo | Título | Fecha | Responsable | Estado | Archivo |
|---|---|---|---|---|---|---|
| MTG-001 | Meeting | Kickoff del proyecto | 2026-09-01 | Omar | Registrada | 01-governance/meetings/MTG-001.md |
| MTG-002 | Meeting | Reunión 2 del proyecto de grado | 2026-09-12 | Equipo del proyecto | Registrada | 01-governance/MTG-002-reunion-2-proyecto-ingenieria.md |
| REQ-001 | Requirement | Requisito inicial del proyecto | 2026-09-12 | Equipo del proyecto | Borrador | 03-requirements/REQUIREMENT.md |
| REQ-002 | Requirement | Separación de persistencia operativa y de inteligencia artificial | 2026-09-12 | Equipo del proyecto | Propuesto | 03-requirements/REQ-002-two-database-persistence.md |
| REQ-003 | Requirement | Consulta de recursos, métricas y sugerencias | 2026-09-12 | Equipo del proyecto | Propuesto | 03-requirements/REQ-003-frontend-resource-metrics.md |
| REQ-004 | Requirement | Recomendaciones sin aplicación automática | 2026-09-12 | Equipo del proyecto | Propuesto | 03-requirements/REQ-004-recommendation-without-automatic-changes.md |
| REQ-005 | Requirement | Entrega beta del primer avance académico | 2026-09-12 | Equipo del proyecto | Propuesto | 03-requirements/REQ-005-beta-academic-delivery.md |
| REQ-006 | Requirement | Tiempo preliminar de respuesta del análisis | 2026-09-12 | Equipo del proyecto | Propuesto | 03-requirements/REQ-006-analysis-response-time.md |
| REQ-007 | Requirement | Despliegue local inicial con PostgreSQL | 2026-09-12 | Equipo del proyecto | Propuesto | 03-requirements/REQ-007-local-postgresql-initial-deployment.md |
| ADR-002 | ADR | Separación inicial de bases de datos PostgreSQL | 2026-09-12 | Equipo del proyecto | Aceptada | 01-governance/ADR-002-two-postgresql-databases.md |
| EXP-001 | Experimento | Prueba de integración y factibilidad de datos para Azure SQL Cost Optimizer | 2026-09-12 | Equipo del proyecto | Ejecutado | 07-experiments/EXP-001-api-integration-data-feasibility.md |
| LAB-001 | Laboratorio | Integración inicial y verificación de fuentes para Cloud Database Cost Optimizer | 2026-09-06 | Equipo del proyecto | Ejecutado; requiere histórico extendido | 07-experiments/LAB-001-azure-data-integration.md |
| DATA-001 | Dataset | Respuestas de integración y carga experimental de Azure SQL | 2026-09-06 | Equipo del proyecto | Registrado | 06-data/DATA-001-azure-integration-responses.md |
| DEC-020 | Decision | Continuar el proyecto con modificaciones en histórico, workloads, costos y métricas | 2026-09-12 | Equipo del proyecto | Adoptada | registry/DECISION-LOG.md |
| DEC-021 | Decision | Entrega beta al profesor | 2026-09-12 | Equipo del proyecto | Adoptada | registry/DECISION-LOG.md |
| DEC-022 | Decision | Separación de persistencia | 2026-09-12 | Equipo del proyecto | Adoptada | registry/DECISION-LOG.md |
| DEC-023 | Decision | PostgreSQL local inicial | 2026-09-12 | Equipo del proyecto | Adoptada | registry/DECISION-LOG.md |
| DEC-024 | Decision | Recomendaciones sin cambios automáticos | 2026-09-12 | Equipo del proyecto | Adoptada | registry/DECISION-LOG.md |
| DEC-025 | Decision | Prioridad del primer avance académico | 2026-09-12 | Equipo del proyecto | Adoptada | registry/DECISION-LOG.md |
| DEC-026 | Decision | Sprint dominical a las 10:00 a. m. | 2026-09-12 | Equipo del proyecto | Adoptada | registry/DECISION-LOG.md |
| RES-001 | Research | Antecedentes de software similar y estudio del estándar FOCUS | 2026-09-28 | Equipo del proyecto | Borrador | 02-research/RES-001-antecedentes-software-similar-estandar-focus.md |
| RES-002 | Research | Precios y métricas reales de Azure SQL Database | 2026-09-28 | Jesús / Equipo del proyecto | Borrador | 02-research/RES-002-precios-metricas-azure-sql.md |
| DEC-027 | Decision | Adoptar FOCUS como formato de normalización de datos de costo/uso en la capa de ingesta | 2026-09-28 | Equipo del proyecto | Propuesta | registry/DECISION-LOG.md |
| ADR-003 | ADR | Persistencia GORM portable para cco_app | 2026-10-26 | Equipo del proyecto | Propuesto | 04-architecture/decisions/ADR-003-gorm-portable-persistence.md |
| ADR-004 | ADR | Arquitectura de Autenticación y Autorización del módulo IAM | 2026-10-29 | Equipo del proyecto | Aceptada | 04-architecture/decisions/ADR-004-iam-authentication.md |
| ADR-005 | ADR | Estructura de pruebas del backend Go | 2026-10-29 | Equipo del proyecto | Aceptada | 04-architecture/decisions/ADR-005-test-directory-structure.md |
| TEST-IAM-001 | Test | PermissionEngine Can() wildcard y exact-match | 2026-10-29 | Equipo del proyecto | Aprobado | backend/internal/iam/permission_engine_test.go |
| TEST-IAM-002 | Test | MapErrorToCode para todos los errores de dominio | 2026-10-29 | Equipo del proyecto | Aprobado | backend/internal/iam/errors_test.go |
| TEST-IAM-003 | Test | Integración IAM end-to-end sobre SQLite (repos, membresía, proyectos, identidades externas) | 2026-10-29 | Equipo del proyecto | Aprobado | backend/test/iam_integration_test.go |
| TEST-IAM-004 | Test | Endpoint POST /auth/register: creación de usuario con Argon2id, 201 Created, 409 si email duplicado, validación de campos requeridos | 2026-10-29 | Equipo del proyecto | Aprobado | backend/internal/iam/handlers.go |
| TEST-IAM-005 | Test | Logging estructurado: slog en middleware y handlers, eventos de auth con request_id/correlation_id | 2026-10-30 | Equipo del proyecto | Aprobado | backend/internal/iam/middleware/auth_middleware.go, backend/internal/iam/handlers.go |
| TEST-IAM-006 | Test | Seguridad IAM: Argon2id password hash, rate limiting, account lockout, security headers, refresh token rotation | 2026-10-30 | Equipo del proyecto | Aprobado | backend/internal/iam/auth.go, backend/internal/iam/handlers.go, backend/internal/iam/middleware/security.go |
| TEST-IAM-007 | Test | Registro duplicado y logs críticos: email duplicado → 409 USER_EXISTS; logs en todos los pasos críticos de auth | 2026-10-30 | Equipo del proyecto | Aprobado | backend/internal/iam/handlers.go |
| ADR-006 | ADR | Estándar de logging del backend con log/slog | 2026-10-30 | Equipo del proyecto | Aceptada | 04-architecture/decisions/ADR-006-logging-standard.md |
| ADR-007 | ADR | Diseño de seguridad del módulo IAM (Argon2id, rate limiting, account lockout, security headers, refresh token rotation) | 2026-10-30 | Equipo del proyecto | Aceptada | 04-architecture/decisions/ADR-007-security-design.md |
