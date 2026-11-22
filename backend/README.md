# Backend — Cloud Cost Optimizer

Este módulo es el backend de aplicación (API HTTP en Go) de `cco_app`. Expone autenticación, gestión de nube, adquisición y consulta de datos, analítica, recomendaciones y auditoría. Es intencionalmente un **monolito modular**: un solo binario Go, paquetes separados por dominio y contratos Go nativos entre módulos.

## Estructura

```text
backend/
├── cmd/cco-app/
│   └── main.go             # entry point, DI container, router, middlewares
├── go.mod / go.sum
├── internal/
│   ├── iam/                # Module 1 — Identidad y Acceso (implementado)
│   ├── cloud/              # Module 2 — Gestión de Nube y Fuentes de Datos (implementado)
│   ├── acquisition/        # Module 3 — Adquisición y Monitoreo de Datos (implementado)
│   ├── data/               # Module 4 — Gestión de Datos (implementado)
│   ├── analytics/          # Module 5 — Analítica y Optimización (implementado)
│   ├── recommendation/     # Module 7 — Motor de Recomendaciones (implementado)
│   ├── recommendation/     # Module 7 — Motor de Recomendaciones (implementado)
│   ├── audit/              # Module 8 — Auditoría y Trazabilidad (implementado)
│   ├── jobs/               # Module 9 — Jobs duraderos y worker (implementado, Fase 3)
│   └── platform/           # infraestructura transversal
│       ├── database/       # GORM portable (SQLite/PostgreSQL), migración
│       └── valkey/         # cliente Valkey compartido (rate limiting, lockout)
└── test/                   # pruebas de integración cross-package
    └── iam_integration_test.go
```

Cada paquete interno contiene `handlers.go`, `service.go`, `interfaces.go`, `errors.go` y su carpeta `repositories/`/`adapters/` cuando corresponde. Ver `API.md` para la referencia HTTP completa y `docs/04-architecture/` para los ADR y diagramas.
El catálogo de módulos y sus límites está en [`docs/04-architecture/MODULES.md`](../docs/04-architecture/MODULES.md).

## Módulos de negocio (estado de implementación)

| Módulo | Paquete | Estado | Responsabilidad |
|--------|---------|--------|-----------------|
| Identidad y Acceso | `internal/iam` | Implementado | Auth (JWT/Argon2id), OIDC, M2M, refresh tokens, roles, permisos, rate limiting, lockout |
| Gestión de Nube y Fuentes de Datos | `internal/cloud` | Implementado | Cuentas cloud, validación de conexión, sincronización de recursos, bindings |
| Adquisición y Monitoreo de Datos | `internal/acquisition` | Implementado | Ciclos de adquisición, Azure Monitor, DMVs SQL, Query Store, costos diarios |
| Gestión de Datos | `internal/data` | Implementado | Lectura de métricas/costos/configuración y contratos de escritura en `cco_data` |
| Analítica y Optimización | `internal/analytics` | Implementado | Snapshots, percentiles, tendencias, patrones, capacidad, costos, DES, datasets, forecasts |
| Integración con ML | `internal/analytics` (contrato) | Parcial | Contrato `MLPort`; proveedor productivo pendiente |
| Motor de Recomendaciones | `internal/recommendation` | Implementado | Generación, persistencia versionada, trazas y transiciones explícitas |
| Auditoría y Trazabilidad | `internal/audit` | Implementado | Eventos append-only, consultas protegidas y propagación de trazas |
| Jobs (cola duradera y worker) | `internal/jobs` | Implementado | Tabla `app.jobs`, reclamación con `FOR UPDATE SKIP LOCKED`, reintentos con backoff+jitter, idempotencia, cancelación; endpoints `/api/v1/jobs/*` y encolado asíncrono de adquisición, reportes, analítica y recomendaciones pesadas |
| Simulación y Laboratorio | — | Pendiente | Escenarios controlados y datos sintéticos |
| Reportes (generación y exportación) | `internal/reporting` | Implementado | `app.report_requests`, `app.report_artifacts`, endpoints `/api/v1/jobs/report` (202 Accepted), formatos JSON/CSV/HTML, almacenamiento local con límites de tamaño y retención |

## Principio de dependencia entre módulos

```
iam (transversal) ◄── cloud ◄── acquisition ──► data ──► analytics ──┐
                                                │       forecasts    │
                                                └────────────────────┤
                                                                     ▼
                                                          recommendation
                                                                     │
                                                               cco_data.opt
iam ──► cloud ──► acquisition ──► data ──► analytics ──► recommendation ──► audit
 └────────────────────────────────────────────────────────────────────────► audit
```

- `iam` no depende de ningún módulo de negocio; todos los demás dependen de `iam` para autenticación y autorización.
- `cloud` depende de `iam` y de sus propios adaptadores de proveedor.
- `acquisition` depende de `cloud` (credenciales), `iam` y escribe en `cco_data` mediante los contratos de `data` (no toca `cco_data` directamente).
- `data` expone contratos de lectura/escritura sobre `cco_data` y aplica los permisos de IAM correspondientes.
- `analytics` depende de `data` (lectura), `iam` y `MLPort` (Module 6, futuro).
- `recommendation` usa contratos de Data/Analytics y persiste decisiones en `cco_data`; integra auditoría mediante el contrato genérico de `audit`.
- El dominio de `audit` no depende de Recommendation, Analytics, Cloud ni GORM. Su repositorio GORM y adapter de `slog` están separados del dominio.
- Los repositorios de infraestructura pueden usar `platform/database`; las entidades y servicios de dominio dependen de interfaces y DTOs.

## Auditoría y trazabilidad

`audit.events` en `cco_app` es el registro persistente de acciones de la aplicación; `slog` se usa para operación y diagnóstico, y `cco_data.raw.activity_log` conserva actividad observada del proveedor cloud. El middleware propaga `X-Request-ID`, `X-Correlation-ID` y `X-Operation-ID`; las consultas de auditoría y de trazas están protegidas por permisos y alcance de organización/usuario.

La decisión de Recommendation se confirma primero en `cco_data` y luego se audita en `cco_app`. Si falla el segundo paso, la respuesta conserva la recomendación e indica `audit_pending`. No hay outbox ni transacción distribuida en esta versión. Los endpoints de estado, incluido `/apply`, registran transiciones; no realizan cambios en infraestructura.

## Variables de entorno

| Variable | Valor por defecto | Descripción |
|----------|------------------|-------------|
| `CCO_DB_DSN` | `file::memory:?cache=shared` | DSN de `cco_app` (SQLite en dev; PostgreSQL DSN en prod) |
| `CCO_DB_DRIVER` | `sqlite` | `sqlite` o `postgres` |
| `CCO_DATA_DSN` | (vacío) | DSN de `cco_data` (PostgreSQL). Sin él, `data`/`analytics`/`acquisition`/`recommendation` responden `503` y el scheduler no arranca |
| `CCO_JWT_SIGNING_KEY` | `dev-secret-change-in-production` | Clave para firmar JWT HS256 |
| `CCO_HTTP_ADDR` | `:8080` | Dirección y puerto del servidor HTTP |
| `CCO_LOG_FORMAT` | (texto) | `json` para salida de logs en JSON (colector externo) |
| `CCO_OIDC_ISSUER_URL` | (vacío) | Issuer OIDC (Entra ID / Keycloak / Google) |
| `CCO_OIDC_CLIENT_ID` | (vacío) | Client ID de OIDC |
| `CCO_OIDC_CLIENT_SECRET` | (vacío) | Client secret de OIDC |
| `CCO_OIDC_REDIRECT_URL` | (vacío) | URL de redirect de OIDC |
| `CCO_COOKIE_DOMAIN` | (vacío) | Dominio para cookies de auth |
| `CCO_COOKIE_SECURE` | `false` | Cookies solo sobre HTTPS |
| `CCO_ALLOWED_ORIGINS` | `*` | Orígenes CORS permitidos |
| `CCO_CSP` | — | Cabecera `Content-Security-Policy` |
| `CCO_ENV` | — | `prod` exige `CCO_JWT_SIGNING_KEY` |
| `CCO_VALKEY_ADDR` | — | Dirección de Valkey (vacío = modo dev, fallback en memoria) |
| `CCO_VALKEY_PASSWORD` | — | Contraseña de Valkey |
| `CCO_VALKEY_DB` | — | Número de base de datos de Valkey |
| `CCO_CLOUD_PROVIDER_MODE` | — | `azure` activa adaptadores reales; otro valor usa adaptadores falsos |
| `CCO_ACQ_INTERVAL_SECONDS` | `60` | Intervalo del scheduler de adquisición (segundos) |
| `CCO_ACQ_COST_INTERVAL_SECONDS` | `21600` | Intervalo del ciclo de costos (segundos, 6 h) |
| `CCO_ACQ_MONITOR_DELAY_SECONDS` | `90` | Retraso de ventana de métricas (segundos) |
| `CCO_ACQ_MONITOR_LOOKBACK_SECONDS` | `300` | Ventana de lookback de métricas (segundos) |
| `CCO_ACQ_COST_LOOKBACK_DAYS` | `3` | Lookback de costos (días) |
| `CCO_ACQ_SQL_USER` | — | Credencial SQL para DMVs/Query Store |
| `CCO_ACQ_SQL_PASSWORD` | — | Contraseña SQL para DMVs/Query Store |

Lista completa en [`API.md`](API.md), sección 1.

## Compilar y ejecutar

Desde `backend/`:

```powershell
# Compilar
go build -o cco-app.exe ./cmd/cco-app/

# Ejecutar (desarrollo, SQLite en memoria)
$env:CCO_DB_DSN = "file::memory:?cache=shared"
$env:CCO_DB_DRIVER = "sqlite"
$env:CCO_JWT_SIGNING_KEY = "<cambiar-en-produccion>"
$env:CCO_HTTP_ADDR = ":8080"
.\cco-app.exe
```

Salida esperada:
```
cco-app listening on :8080 (driver=sqlite)
```

## Cómo se conectan los módulos (DI en `cmd/cco-app/main.go`)

1. Se abre `cco_app` (SQLite o PostgreSQL) y se ejecuta `database.Migrate`.
2. Se construye `tokenMgr` (JWT), `lockout` (Valkey o memoria) y el repositorio compuesto de IAM.
3. Se construyen los repositorios de `cloud` (cuentas, recursos, bindings) y se elige el adaptador de proveedor (fake por defecto, Azure si `CCO_CLOUD_PROVIDER_MODE=azure`).
4. Si `CCO_DATA_DSN` está configurado, se abre `cco_data` (PostgreSQL) y se construyen los servicios de `data`, `analytics` y `acquisition`; de lo contrario esos endpoints responden `503`.
5. Recommendation Engine se inicializa con repositorios y adapters de Data/Analytics cuando `cco_data` está disponible. Sus rutas se registran también sin DSN y responden `503 RECOMMENDATION_STORAGE_UNAVAILABLE`.
6. Se registran las rutas de cada módulo sobre el `*http.ServeMux` y se envuelven con CORS, security headers y rate limiting.
7. El scheduler de adquisición (y el de costos) arranca en goroutines separadas cuando `cco_data` está disponible.

### Recommendation Engine (Module 7)

Rutas protegidas bajo `/api/v1/recommendations`:

| Operación | Permiso | Comportamiento |
|-----------|---------|----------------|
| `POST /api/v1/recommendations` | `recommendations:write` | Genera y persiste una decisión versionada |
| `POST /api/v1/recommendations/evaluate` | `recommendations:evaluate` | Evalúa candidatos y política sin persistir |
| `GET /api/v1/recommendations` | `recommendations:read` | Lista recomendaciones |
| `GET /api/v1/recommendations/{uid}` | `recommendations:read` | Consulta una recomendación |
| `GET /api/v1/recommendations/{uid}/evidence` | `recommendations:read` | Consulta la evidencia asociada |
| `POST /api/v1/recommendations/{uid}/evaluate` | `recommendations:evaluate` | Reevalúa con datos recientes sin mutar el historial |

El módulo genera candidatos desde el catálogo, estima precios de lista vigentes por región/moneda, simula costo y utilización, evalúa restricciones y calcula riesgo y confianza reproducibles. Persiste política, escenario, candidatos, simulaciones, recomendación, evidencias y evaluación de riesgo; `no_action` también queda registrado cuando la información es insuficiente o no hay una opción permitida. `recommendations:write` y `recommendations:evaluate` están incluidos en las semillas de permisos/roles.

El esquema de tablas `opt.*` está en `databases/cco_data/cco_data_schema.sql`. La referencia HTTP está en `API.md`; decisiones y detalles técnicos están en `docs/04-architecture/decisions/ADR-013-recommendation-engine.md` y `docs/05-design/DESIGN-RECOMMENDATION-001.md`.

## Reglas de portabilidad

- El código de aplicación depende de interfaces de repositorio y DTOs, nunca de modelos GORM.
- SQLite usa nombres de tabla sin esquema; PostgreSQL califica con los cinco esquemas (`iam`, `organization`, `cloud`, `app`, `audit`). La decisión está aislada en `database.TableName`.
- Los datos JSON usan `gorm.io/datatypes`, no tipos JSON exclusivos de PostgreSQL en código de aplicación.
- Los UUID se generan en Go en el límite del repositorio, así que SQLite no necesita `pgcrypto`.
- La creación de esquema específica de PostgreSQL está aislada en `Migrate`; el dominio y los repositorios se mantienen neutrales al dialecto.
- `audit.events` reside en `cco_app`; el esquema `cco_data` no contiene una copia de auditoría de la aplicación. La consulta de trazas combina lecturas independientes y no cruza SQL entre bases.
- El dump PostgreSQL existente sigue siendo la referencia de producción para RLS, check constraints, índices, triggers, extensiones y claves foráneas. La migración GORM es para bootstrap local/testing y cambios incrementales propiedad de la aplicación; no debe reemplazar silenciosamente políticas de seguridad.

## Ejecutar pruebas

```bash
go test ./...
```

El objetivo de migración inicial es SQLite para pruebas rápidas e isoladas. PostgreSQL se selecciona con `database.DriverPostgres` y un DSN normal. Los mismos modelos y repositorios se usan en ambos casos.

## Testes de integración

Las pruebas SQLite de Data y Recommendation requieren CGO habilitado (`go-sqlite3`). La validación end-to-end contra PostgreSQL para Recommendation sigue pendiente; la existencia de las pruebas de repositorio no implica que ese entorno se haya validado.
