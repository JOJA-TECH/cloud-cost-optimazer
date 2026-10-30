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

## Módulos del proyecto

- **Identidad y Acceso** (`backend/internal/iam`) — autenticación, autorización, roles y permisos (implementado)
- Gestión de Nube y Fuentes de Datos (pendiente)
- Adquisición y Monitoreo de Datos (pendiente)
- Gestión de Datos (pendiente)
- Analítica y Optimización (pendiente)
- Integración con ML (pendiente)
- Motor de Recomendaciones (pendiente)
- Auditoría y Trazabilidad (pendiente)
- Simulación y Laboratorio (pendiente)
- Reportes (pendiente)
