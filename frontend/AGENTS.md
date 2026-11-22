# Cloud Cost Optimizer — Frontend (Next.js)

Stack: Next.js 16.3.3, React 19, Tailwind 4, pnpm 12.3.4.

## Estructura real del proyecto (sin carpeta `src/`)

Todo el código vive en la raíz de `frontend/`:

| Ruta | Contenido |
|------|-----------|
| `frontend/app/` | App Router de Next.js (pages, layouts) |
| `frontend/components/` | Componentes React (shadcn/ui base, dashboard, charts, landing, auth) |
| `frontend/lib/` | Utilidades y clientes de API (`api.ts`, `api-services.ts`, `auth.ts`, `mock-data.ts`, `utils.ts`) |
| `frontend/public/` | Assets estáticos (solo `placeholder-user.jpg`) |

## Scripts disponibles (desde `frontend/`)

| Comando | Descripción |
|---------|-------------|
| `pnpm dev` | Servidor de desarrollo con Turbopack |
| `pnpm build` | Build de producción (`typescript.ignoreBuildErrors: true`) |
| `pnpm start` | Servidor de producción |

> No existen scripts `lint` ni `test` en `package.json`.

## Variables de entorno

Definir en `frontend/.env.local` (no commitear):

- `NEXT_PUBLIC_API_URL` — URL del backend Go (default `http://localhost:8080`, se usa en `lib/api.ts`)

> `NEXT_PUBLIC_APP_NAME` documentada en versiones anteriores **no se usa en el código**.

## Qué consume realmente el backend Go

La capa de red vive en `frontend/lib/api.ts` (`apiFetch` con `fetch()` + Bearer token; redirige a `/login` en 401) y `frontend/lib/auth.ts` (`login` → `POST /auth/login`, `register` → `POST /auth/register`).

**Endpoints que sí llaman al backend (con fallback silencioso a `mock-data.ts` si fallan):**
- `POST /auth/login`, `POST /auth/register` (auth consistente)
- `GET /api/v1/cloud-accounts`, `GET /api/v1/cloud-accounts/{id}/resources`
- `GET /api/v1/data/metrics`, `GET /api/v1/data/costs`, `GET /api/v1/data/config-snapshots`
- `GET /api/v1/recommendations`, `GET /api/v1/recommendations/{uid}`

**Datos estáticos (mock, siempre):** KPIs, rendimiento, eventos de confiabilidad, escenarios, forecast, series de carga, clasificación de workload, top queries, categorías de espera, calidad de datos, historial.

> Riesgo: la mayoría de vistas del dashboard funcionan con `mock-data.ts` aunque el backend esté caído. El token de auth solo vive en memoria (se pierde al recargar).

## Notas para agentes

- No usar el bloque auto-generado de `next dev` como documentación real; este archivo lo reemplaza.
- La API del backend se documenta en `backend/API.md`.
- El frontend consume la API HTTP del backend mediante `fetch` manual; no hay cliente generado ni carpeta `hooks/`.
- El alias `@/hooks` de `components.json` apunta a una carpeta inexistente.
- `next.config.mjs` tiene `typescript.ignoreBuildErrors: true`.