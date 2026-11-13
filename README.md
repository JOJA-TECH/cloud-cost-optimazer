# Cloud Cost Optimizer

Plataforma inteligente de optimización de costos en la nube para monitorear, analizar y recomendar configuraciones de recursos rentables en entornos de nube.

## Documentación

| Documento | Descripción |
|-----------|-------------|
| [`API.md`](API.md) | Referencia completa de la API HTTP del backend (endpoints `/auth/*`, `/healthz`, `/api/v1/*`) |
| `backend/README.md` | Estructura del backend Go, reglas de portabilidad y cómo ejecutar pruebas |
| `docs/` | Documentación de arquitectura, requisitos, ADRs, experimentos y pruebas |

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

Luego consultar `API.md` para probar los endpoints con `curl` o PowerShell.

Los módulos `data`, `analytics` y `acquisition` escriben y leen la base de datos de telemetría `cco_data`; requieren `CCO_DATA_DSN` (DSN de PostgreSQL) configurado. Sin él responden `503` y el scheduler de adquisición no arranca.

## Módulos del proyecto (backend)

| Módulo | Paquete | Estado | Qué hace |
|--------|---------|--------|----------|
| **Identidad y Acceso** | `backend/internal/iam` | Implementado | Auth (JWT/Argon2id), OIDC, M2M, refresh tokens, rate limiting, lockout, roles y permisos |
| **Gestión de Nube y Fuentes de Datos** | `backend/internal/cloud` | Implementado | Cuentas cloud, validación de conexión, sincronización de recursos Azure, fuentes de datos (bindings) |
| **Adquisición y Monitoreo de Datos** | `backend/internal/acquisition` | Implementado | Scheduler de ciclos, Azure Monitor, DMVs SQL, Query Store, costos diarios (Azure Cost Management) |
| **Gestión de Datos** | `backend/internal/data` | Implementado | API de solo lectura sobre `cco_data`: métricas, costos, snapshots de configuración, cobertura |
| **Analítica y Optimización** | `backend/internal/analytics` | Implementado | 17 endpoints REST: snapshots, percentiles, tendencias, patrones, capacidad, costos, DES, datasets, forecasts |
| Integración con ML | — | Pendiente | Contrato `MLPort` definido; cliente fake en desarrollo |
| Motor de Recomendaciones | — | Pendiente | |
| Auditoría y Trazabilidad | — | Pendiente | Trazabilidad transversal vía `request_id` / `correlation_id` en los logs |
| Simulación y Laboratorio | — | Pendiente | |
| Reportes | — | Pendiente | |

## Variables de entorno del backend (resumen)

| Variable | Valor por defecto | Descripción |
|----------|------------------|-------------|
| `CCO_DB_DSN` | `file::memory:?cache=shared` | DSN de `cco_app` (SQLite en dev; PostgreSQL DSN en prod) |
| `CCO_DB_DRIVER` | `sqlite` | `sqlite` o `postgres` |
| `CCO_DATA_DSN` | (vacío) | DSN de `cco_data`; sin él, `data`/`analytics`/`acquisition` responden `503` |
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

Lista completa de variables en `API.md`, sección 1.
