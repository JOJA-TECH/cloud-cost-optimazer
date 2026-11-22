# Sistema de Documentación del Proyecto

Este repositorio contiene la documentación operativa, técnica, experimental y académica del proyecto.

## Principio rector

> Si una actividad produce una decisión, evidencia, cambio, resultado, riesgo o aprendizaje relevante, debe quedar registrada y ser trazable.

## Estructura

- `00-project/`: definición y dirección del proyecto.
- `01-governance/`: reuniones, decisiones, cambios, riesgos y retrospectivas.
- `02-research/`: investigación, fuentes y hallazgos.
- `03-requirements/`: requisitos y trazabilidad.
- `04-architecture/`: arquitectura y ADR.
- `05-design/`: diseño técnico.
- `06-data/`: datasets, diccionarios y linaje.
- `07-experiments/`: laboratorios, experimentos y benchmarks.
- `08-ai/`: modelos, prompts y evaluación de IA.
- `09-testing/`: pruebas y reportes.
- `10-operations/`: despliegue, monitoreo y troubleshooting.
- `11-releases/`: versiones y notas de lanzamiento.
- `registry/`: índices y registros maestros.

## Backend implementado

El backend Go (`backend/`) documenta su API HTTP en [`backend/API.md`](../backend/API.md) y su guía de ejecución, módulos, reglas de portabilidad y pruebas en [`backend/README.md`](../backend/README.md). El catálogo de módulos está en [`04-architecture/MODULES.md`](04-architecture/MODULES.md). Módulos implementados en `backend/internal/`:

- `iam` — Identidad y Acceso (auth JWT/Argon2id, OIDC, M2M, refresh tokens, rate limiting, lockout, roles y permisos)
- `cloud` — Gestión de Nube y Fuentes de Datos (cuentas cloud, validación de conexión, sincronización de recursos, fuentes de datos/bindings)
- `acquisition` — Adquisición y Monitoreo de Datos (scheduler de ciclos, Azure Monitor, diagnósticos SQL, Query Store, costos diarios)
- `data` — Gestión de Datos (lectura de métricas/costos/configuración y contratos de escritura para adquisición en `cco_data`)
- `analytics` — Analítica y Optimización (17 endpoints REST: snapshots, percentiles, tendencias, patrones, capacidad, costos, DES, datasets, forecasts)
- `recommendation` — Motor de Recomendaciones (decisiones versionadas, estados explícitos y trazas)
- `audit` — Auditoría y Trazabilidad (registro append-only en `cco_app.audit.events`, consultas protegidas)
- `jobs` — Jobs duraderos y worker (cola `app.jobs`, reintentos con backoff+jitter, idempotencia, cancelación, endpoints `/api/v1/jobs/*`; encolado asíncrono de adquisición, reportes, analítica y recomendaciones pesadas)
- `platform/database` — persistencia GORM portátil SQLite/PostgreSQL

La integración productiva con ML, Simulación y Laboratorio, y Reportes siguen pendientes. Analytics aún no tiene eventos de auditoría específicos.

## Documentos de arquitectura y diseño

- [Architecture](04-architecture/ARCHITECTURE.md) y [componentes](04-architecture/diagrams/components.mmd)
- [ADR-006 — Logging](04-architecture/decisions/ADR-006-logging-standard.md)
- [ADR-013 — Recommendation](04-architecture/decisions/ADR-013-recommendation-engine.md)
- [ADR-014 — Auditoría y trazabilidad](04-architecture/decisions/ADR-014-audit-traceability.md)
- [DESIGN-AUDIT-001](05-design/DESIGN-AUDIT-001.md)
- [DESIGN-RECOMMENDATION-001](05-design/DESIGN-RECOMMENDATION-001.md)

## Identificadores

`REQ`, `ADR`, `DEC`, `MTG`, `ISS`, `CHG`, `RSK`, `RES`, `LAB`, `EXP`, `BEN`, `DATA`, `MOD`, `TEST`, `REL`, `RESU`.

## Regla de oro

No se borra la historia para que el proyecto "se vea limpio". Los cambios importantes se registran.
