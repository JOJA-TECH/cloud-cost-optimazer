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

El backend Go (`backend/`) documenta su API HTTP en `API.md` y su guía de ejecución, reglas de portabilidad y pruebas en `backend/README.md`. Módulos ya implementados en `backend/internal/`:

- `iam` — Identidad y Acceso (auth JWT/Argon2id, OIDC, M2M, refresh tokens, rate limiting, lockout, roles y permisos)
- `cloud` — Gestión de Nube y Fuentes de Datos (cuentas cloud, validación de conexión, sincronización de recursos, fuentes de datos/bindings)
- `acquisition` — Adquisición y Monitoreo de Datos (scheduler de ciclos, Azure Monitor, diagnósticos SQL, Query Store, costos diarios)
- `data` — Gestión de Datos (API de solo lectura sobre `cco_data`: métricas, costos, snapshots de configuración, cobertura)
- `analytics` — Analítica y Optimización (17 endpoints REST: snapshots, percentiles, tendencias, patrones, capacidad, costos, DES, datasets, forecasts)
- `platform/database` — persistencia GORM portátil SQLite/PostgreSQL

Pendientes de implementar: Integración con ML, Motor de Recomendaciones, Auditoría y Trazabilidad, Simulación y Laboratorio, Reportes.

## Identificadores

`REQ`, `ADR`, `DEC`, `MTG`, `ISS`, `CHG`, `RSK`, `RES`, `LAB`, `EXP`, `BEN`, `DATA`, `MOD`, `TEST`, `REL`, `RESU`.

## Regla de oro

No se borra la historia para que el proyecto "se vea limpio". Los cambios importantes se registran.
