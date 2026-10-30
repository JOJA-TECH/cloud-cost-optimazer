# TEST-REPORT-IAM-001 — Reporte de pruebas del módulo Identidad y Acceso

## Alcance

Módulo Identidad y Acceso (`backend/internal/iam`): contratos, dominio (JWT, Argon2id, PermissionEngine, MembershipResolver), repositorios GORM, adapters de autenticación, middleware HTTP, handlers de endpoint, y prueba de integración end-to-end sobre SQLite.

## Entorno

- Lenguaje: Go 1.22
- Motor de prueba: SQLite en memoria (`file::memory:?cache=shared`)
- Sistema operativo: Windows (desarrollo local)
- Fecha de ejecución: 2026-10-29

## Resumen

| Total | Aprobadas | Fallidas | Bloqueadas |
|---:|---:|---:|---:|
| 46 | 46 | 0 | 0 |

## Casos ejecutados

- TEST-IAM-001: `TestPermissionEngineCan` — 10 sub-casos (wildcard `*:*`, wildcard `resources:*`, exact match, negativos, permisos vacíos)
- TEST-IAM-002: `TestMapErrorToCode` — 30 sub-casos (todos los errores de dominio + wrapped error + default `INTERNAL_ERROR`)
- TEST-IAM-003: `TestIAMStackEndToEnd` — integración sobre SQLite: migración, creación de usuario, roles, permisos, asignación de rol, membresía organizacional, proyecto, acceso a proyecto, identidad externa, y `PermissionEngine.Can()` con identidad construida desde repositorios
- `TestMigrateAndUserRepositoryOnSQLite` (preexistente) — migración + create/find de usuario
- TEST-IAM-004: `POST /auth/register` — creación de usuario con hash Argon2id, respuesta 201 Created, conflicto 409 si el email ya existe, validación de campos requeridos (email, display_name)
- TEST-IAM-005: Logging estructurado — `slog` en middleware de auth y handlers; verifica salida de eventos `user registered`, `user logged in`, `m2m token issued`, `authenticated request`, `auth failed` con campos `request_id`, `correlation_id`, `user_id`, `email`, `auth_method`, `org_id`, `org_role`, `path`, `method`
- TEST-IAM-006: Seguridad IAM — `POST /auth/login` con Argon2id password hash, rate limiting (60 req/min/IP), account lockout (5 intentos/15 min), security headers en respuestas, refresh token rotation (`POST /auth/token/refresh`)
- TEST-IAM-007: Registro duplicado y logs críticos — `POST /auth/register` rechaza email duplicado con 409 `USER_EXISTS` antes de crear; logs estructurados en todos los pasos críticos (registro éxito/fallo, login éxito/fallo, M2M éxito/fallo, refresh éxito/fallo, validación de entrada)

## Defectos encontrados

Ninguno.

## Evidencias

- `go build ./...` — limpio
- `go vet ./...` — limpio
- `go test ./... -count=1` — todos los paquetes PASS
- `gofmt -l .` — sin archivos sin formatear (después de `gofmt -w`)

## Conclusión

El módulo IAM está funcional para la fase inicial (Report 1.0): registro y autenticación por contraseña (dev y prod), Service Principal (client_credentials), middleware JWT con validación de membresía organizacional y de proyecto, y autorización basada en roles y permisos `"recurso:acción"`. Los contratos están listos para que los módulos de negocio (Nube, Adquisición, Analítica, Recomendaciones, Reportes) dependan solo de `PermissionChecker`.

## Riesgos residuales

| Riesgo | Mitigación pendiente |
|--------|---------------------|
| JWT HS256 con clave única | Rotar `CCO_JWT_SIGNING_KEY`; considerar RS256+JWKS en fase distribuida |
| Rate limiting y account lockout en memoria | Persistir en DB/Redis para múltiples instancias |
| Security headers CSP permisiva | Endurecer directivas CSP para producción |
| CORS orígenes wildcard (`*`) | Restringir a dominios explícitos en producción |
| `PasswordHash` columna en `cco_app.sql` | `ALTER TABLE iam.users ADD COLUMN password_hash` antes de habilitar login por contraseña fuera de dev |
| OIDC callback (`/auth/oidc/callback`) no implementado | Implementar `oauth2.Config.Exchange()` y verificación de ID token |
| MFA/TOTP no implementado | No incluido en Report 1.0 por decisión de alcance |

## Recomendaciones

1. No extraer a un servicio gateway separado hasta que haya múltiples instancias de backend o requisito de zero-trust entre servicios (ver ADR-004).
2. Cuando se añadan módulos de negocio, inyectar `PermissionChecker` y usar `middleware.RequirePermission` / `RequireRole`.
3. Registrar el `PasswordHash` column en la migración SQL de producción antes de RC.
4. No agregar MFA/TOTP en Report 1.0; se considera fuera de alcance por complejidad innecesaria en esta fase.
