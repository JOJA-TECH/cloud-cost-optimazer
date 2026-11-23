# Cloud Cost Optimizer

Plataforma inteligente de optimización de costos en la nube para monitorear, analizar y recomendar configuraciones de recursos rentables en entornos de nube.

## ¿Qué es este proyecto?

Cloud Cost Optimizer es una plataforma de análisis y optimización de costos para bases de datos Azure SQL Database. Su objetivo es ayudar a equipos de FinOps, administradores de bases de datos e ingenieros de nube a determinar si sus recursos están correctamente dimensionados, analizando el comportamiento histórico de la carga de trabajo (percentiles, picos, tendencias de crecimiento) junto con los costos reales, para generar recomendaciones de capacidad basadas en evidencia.

El **forecasting y el motor de recomendaciones son el núcleo innovador** del proyecto: convierten telemetría cruda en evidencia analítica, simulan escenarios de capacidad, calculan ahorro, riesgo y confianza, y generan recomendaciones versionadas y auditables. El sistema **nunca ejecuta cambios automáticos** sobre recursos productivos — todas las recomendaciones son informativas y requieren aprobación humana.

## Qué está implementado (Release 1.0)

El backend es un **monolito modular en Go** con los siguientes módulos ya implementados:

| Módulo | Qué hace | Estado |
|--------|----------|--------|
| **Identidad y Acceso** | Autenticación (JWT/Argon2id), OIDC, M2M, refresh tokens, roles y permisos | Implementado |
| **Gestión de Nube y Fuentes de Datos** | Cuentas cloud, validación de conexión, sincronización de recursos Azure | Implementado |
| **Adquisición y Monitoreo de Datos** | Scheduler de ciclos, Azure Monitor, DMVs SQL, Query Store, costos diarios | Implementado |
| **Gestión de Datos** | Lectura de métricas/costos/configuración y contratos de escritura | Implementado |
| **Analítica y Optimización** | Snapshots, percentiles, tendencias, patrones, capacidad, costos, DES, datasets, forecasts | Implementado |
| **Integración con ML/LLM/IA** | `MLPort` (forecast vía gRPC al servicio Python), `LLMPort` (explicación determinista), `MLManagement` (entrenamiento); punto único de conexión a capacidades de IA en `backend/internal/ai` | Implementado |
| **Motor de Recomendaciones** | Generación, persistencia versionada, trazas y transiciones explícitas | Implementado |
| **Auditoría y Trazabilidad** | Eventos append-only, consultas protegidas y correlación | Implementado |
| **Jobs (cola duradera y worker)** | Cola duradera, reintentos con backoff+jitter, idempotencia, cancelación | Implementado |
| **Reportes** | Generación y exportación de reportes (JSON/CSV/HTML) | Implementado |
| Simulación y Laboratorio | Escenarios controlados y datos sintéticos | Pendiente |

## Documentación

| Documento | Descripción |
|-----------|-------------|
| [`docs/README.md`](docs/README.md) | Índice completo de toda la documentación del proyecto |
| [`backend/README.md`](backend/README.md) | Estructura del backend Go, módulos, portabilidad y pruebas |
| [`backend/API.md`](backend/API.md) | Referencia completa de la API HTTP, permisos, auditoría y trazas |
| [`docs/04-architecture/ARCHITECTURE.md`](docs/04-architecture/ARCHITECTURE.md) | Documento de arquitectura (C4: contexto, contenedores, componentes, despliegue) |
| [`docs/04-architecture/MODULES.md`](docs/04-architecture/MODULES.md) | Catálogo y límites de los módulos del backend |
| [`docs/04-architecture/decisions/`](docs/04-architecture/decisions/) | Decisiones de arquitectura (ADR-001 a ADR-015) |
| [`docs/05-design/`](docs/05-design/) | Diseño técnico detallado de cada módulo |
| [`docs/MANUAL-DE-USO.md`](docs/MANUAL-DE-USO.md) | Manual de convenciones y reglas de documentación del proyecto |

## Inicio rápido (backend)

Desde la carpeta `backend/`:

```powershell
# Compilar
go build -o cco-app.exe ./cmd/cco-app/

# Ejecutar en desarrollo (SQLite en memoria)
$env:CCO_DB_DSN = "file::memory:?cache=shared"
$env:CCO_DB_DRIVER = "sqlite"
$env:CCO_JWT_SIGNING_KEY = "<cambiar-en-produccion>"
$env:CCO_HTTP_ADDR = ":8080"
.\cco-app.exe
```

Luego consulta [`backend/API.md`](backend/API.md) para probar los endpoints con `curl` o PowerShell.

Los módulos `data`, `analytics`, `acquisition` y `recommendation` requieren `CCO_DATA_DSN` (una base PostgreSQL `cco_data`). Sin esa variable, sus endpoints responden `503` y no se inicia el scheduler de adquisición. La auditoría persistente se almacena en `cco_app.audit.events`; `slog` se mantiene para observabilidad operativa.

## Estructura del repositorio

```
cloud-cost-optimazer/
├── backend/                 # Backend Go (monolito modular)
├── databases/               # Esquemas SQL (cco_app y cco_data)
├── docs/                    # Documentación del proyecto (12 carpetas numeradas)
├── frontend/                # Frontend Next.js (datos mayormente mock en dashboard)
├── README.md                # Este archivo
├── MODULES.md               # Planteamiento primario de módulos
├── Proyecto_De_Ingenieria.md # Documento académico del proyecto de grado
└── LICENSE
```

## Licencia

Este proyecto se distribuye bajo la licencia [MIT](LICENSE).
