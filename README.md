# Cloud Cost Optimizer

Plataforma inteligente de optimización de costos en la nube para monitorear, analizar y recomendar configuraciones de recursos rentables en entornos de nube.

## Documentación

| Documento | Descripción |
|-----------|-------------|
| [`backend/API.md`](backend/API.md) | Referencia de la API HTTP, permisos, auditoría y trazas |
| [`backend/README.md`](backend/README.md) | Estructura del backend Go, módulos, portabilidad y pruebas |
| [`docs/README.md`](docs/README.md) | Índice de documentación |
| [`docs/04-architecture/MODULES.md`](docs/04-architecture/MODULES.md) | Catálogo y límites de los módulos del backend |
| [`docs/05-design/DESIGN-AUDIT-001.md`](docs/05-design/DESIGN-AUDIT-001.md) | Diseño técnico de auditoría y trazabilidad |
| [`docs/05-design/DESIGN-RECOMMENDATION-001.md`](docs/05-design/DESIGN-RECOMMENDATION-001.md) | Diseño técnico de Recommendation |

## Inicio rápido (backend)

Desde `backend/`:

```powershell
go build -o cco-app.exe ./cmd/cco-app/
$env:CCO_DB_DSN = "file::memory:?cache=shared"
$env:CCO_DB_DRIVER = "sqlite"
$env:CCO_JWT_SIGNING_KEY = "clave-secreta-dev"
$env:CCO_HTTP_ADDR = ":8080"
.\cco-app.exe
```

Luego consultar [`backend/API.md`](backend/API.md) para probar los endpoints con `curl` o PowerShell.

Los módulos `data`, `analytics`, `acquisition` y `recommendation` requieren `CCO_DATA_DSN` (PostgreSQL `cco_data`). Sin él, sus endpoints responden `503` y no se inicia el scheduler de adquisición. La auditoría persistente se almacena en `cco_app.audit.events`; `slog` se mantiene para observabilidad operativa.

## Módulos del proyecto (backend)

| Módulo | Paquete | Estado | Qué hace |
|--------|---------|--------|----------|
| **Identidad y Acceso** | `backend/internal/iam` | Implementado | Auth (JWT/Argon2id), OIDC, M2M, refresh tokens, rate limiting, lockout, roles y permisos |
| **Gestión de Nube y Fuentes de Datos** | `backend/internal/cloud` | Implementado | Cuentas cloud, validación de conexión, sincronización de recursos Azure, fuentes de datos (bindings) |
| **Adquisición y Monitoreo de Datos** | `backend/internal/acquisition` | Implementado | Scheduler de ciclos, Azure Monitor, DMVs SQL, Query Store, costos diarios (Azure Cost Management) |
| **Gestión de Datos** | `backend/internal/data` | Implementado | Lectura de métricas/costos/configuración y contratos de escritura para adquisición en `cco_data` |
| **Analítica y Optimización** | `backend/internal/analytics` | Implementado | Snapshots, percentiles, tendencias, patrones, capacidad, costos, DES, datasets y forecasts |
| Integración con ML | `backend/internal/analytics` | Parcial | Contrato `MLPort`; proveedor productivo pendiente |
| **Motor de Recomendaciones** | `backend/internal/recommendation` | Implementado | Generación persistente, estados explícitos y consulta de trazas |
| **Auditoría y Trazabilidad** | `backend/internal/audit` | Implementado | Eventos append-only en `cco_app`, consultas protegidas y correlación |
| Simulación y Laboratorio | — | Pendiente | |
| Reportes | — | Pendiente | |

Las decisiones de Recommendation se guardan en `cco_data` y su auditoría en `cco_app`. Al no existir una transacción distribuida, una falla al registrar la auditoría se informa como `audit_pending`; no se descarta la recomendación ya persistida. El endpoint de aplicación de una recomendación solo registra una transición de estado, no ejecuta cambios cloud.

## Variables de entorno del backend (resumen)

| Variable | Valor por defecto | Descripción |
|----------|------------------|-------------|
| `CCO_DB_DSN` | `file::memory:?cache=shared` | DSN de `cco_app` (SQLite en dev; PostgreSQL DSN en prod) |
| `CCO_DB_DRIVER` | `sqlite` | `sqlite` o `postgres` |
| `CCO_DATA_DSN` | (vacío) | DSN de `cco_data`; sin él, `data`/`analytics`/`acquisition`/`recommendation` responden `503` |
| `CCO_JWT_SIGNING_KEY` | `dev-secret-change-in-production` | Clave para firmar JWT |
| `CCO_HTTP_ADDR` | `:8080` | Dirección y puerto del servidor HTTP |
| `CCO_LOG_FORMAT` | (texto) | `json` para salida de logs en JSON |
| `CCO_ACQ_INTERVAL_SECONDS` | `60` | Intervalo del scheduler de adquisición (segundos) |
| `CCO_ACQ_COST_INTERVAL_SECONDS` | `21600` | Intervalo del ciclo de costos (segundos, 6 h) |
| `CCO_ACQ_SQL_USER` / `CCO_ACQ_SQL_PASSWORD` | — | Credenciales para diagnósticos SQL de Azure |
| `CCO_CLOUD_PROVIDER_MODE` | — | `azure` activa el adaptador real; otro valor usa adaptadores falsos |
| `CCO_ALLOWED_ORIGINS` | `*` | Orígenes CORS permitidos |
| `CCO_CSP` | — | Cabecera `Content-Security-Policy` |
| `CCO_ENV` | — | `prod` exige `CCO_JWT_SIGNING_KEY` |

Lista completa de variables en [`backend/API.md`](backend/API.md), sección 1.
