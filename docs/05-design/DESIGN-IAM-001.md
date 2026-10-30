# DESIGN-IAM-001 — Diseño técnico del módulo Identidad y Acceso (IAM)

## Componente

`backend/internal/iam/` — paquete Go que implementa el módulo Identidad y Acceso del backend monolito modular.

Contiene:

| Archivo | Responsabilidad |
|---------|-----------------|
| `interfaces.go` | Contratos públicos (interfaces y DTOs) |
| `identity_context.go` | `IdentityContext` y claves de contexto (`ctxKey`) |
| `errors.go` | Errores de dominio, `ErrorWithCode`, `MapErrorToCode` |
| `auth.go` | JWT (`TokenManager`), Argon2id, `PermissionEngine`, `MembershipResolver` |
| `user.go` | `UserRepository` adaptador GORM contra `cco_app` |
| `handlers.go` | Handlers HTTP públicos (`/auth/*`, `/healthz`, `/`) |
| `middleware/auth_middleware.go` | Middleware de autenticación y autorización |
| `repositories/role_repository.go` | Roles del sistema (`platform_admin`, `db_admin`, `analyst`, `viewer`) |
| `repositories/permission_repository.go` | Códigos de permiso (`permission_code`) |
| `repositories/user_role_repository.go` | Asignaciones usuario ↔ rol |
| `repositories/external_identity_repository.go` | Identidades federadas (OIDC subject mapping) |
| `repositories/membership_repository.go` | Miembros de organización y sus roles |
| `repositories/project_repository.go` | Proyectos y acceso de usuario a proyectos |
| `adapters/oidc_adapter.go` | Proveedor OIDC (Entra ID / Keycloak / Google) |
| `adapters/password_adapter.go` | Autenticación por contraseña (dev/local) |
| `adapters/service_principal_adapter.go` | Service Principal (`client_credentials`) |
| `adapters/managed_identity_adapter.go` | Azure Managed Identity |

## Responsabilidad

Gestionar la identidad de los usuarios y controlar el acceso a las capacidades del sistema mediante autenticación, autorización, roles y permisos. Es un módulo transversal: los demás módulos de negocio (Nube, Adquisición, Analítica, Recomendaciones, Reportes, Auditoría) dependen de él únicamente a través de la interfaz `PermissionChecker` y del `IdentityContext` inyectado en `context.Context`.

Casos de uso cubiertos en Report 1.0:

- Validar credenciales (contraseña) o identidad delegada.
- Validar tokens JWT y construir el contexto de identidad de una solicitud.
- Consultar roles y permisos efectivos de un usuario.
- Comprobar si un actor puede ejecutar una operación sobre un recurso (`Can(resource, action)`).
- Registrar usuarios, roles, permisos, membresías organizacionales y proyectos.
- Vincular identidades externas (OIDC subject) a usuarios locales.
- Exponer endpoints HTTP públicos de autenticación (`/auth/register`, `/auth/login`, `/auth/m2m/token`) y protegidos mediante middleware.

Entidades propias del modelo:

| Entidad | Descripción |
|---------|-------------|
| `User` | Usuario del sistema (email, display_name, status, password_hash dev-only) |
| `Role` | Rol de sistema (`platform_admin`, `db_admin`, `analyst`, `viewer`, `collector`, `ml_inference`) |
| `Permission` | Código de permiso (`resources:read`, `metrics:write`, etc.) |
| `UserRole` | Asignación usuario ↔ rol de sistema |
| `ExternalIdentity` | Mapeo usuario ↔ proveedor externo (OIDC subject) |
| `Membership` | Usuario como miembro de una organización (owner/admin/member/viewer) |
| `Project` | Proyecto dentro de una organización |

## Interfaces

Todas las interfaces públicas viven en `internal/iam/interfaces.go`.

### `AuthenticationProvider`

```go
type AuthenticationProvider interface {
    Authenticate(ctx context.Context, cred Credentials) (IdentityContext, error)
}
```

Implementaciones:
- `adapters.PasswordAdapter` — email + contraseña (Argon2id), solo dev/local.
- `adapters.OIDCAdapter` — Entra ID / Keycloak / Google (Authorization Code + PKCE). Existe como adapter pero no está conectado al flujo de autenticación en Report 1.0 por limitaciones de tiempo.
- `adapters.ServicePrincipalAdapter` — `client_credentials` para colectores y ML.
- `adapters.ManagedIdentityAdapter` — Azure Managed Identity (firma RS256 pendiente, `shortcut` marcado).

### `TokenValidator`

```go
type TokenValidator interface {
    Validate(ctx context.Context, token string) (IdentityContext, error)
}
```

Implementado por `auth.TokenManager` (JWT HS256 autofirmados) y delegado en los adapters OIDC / SP / MI.

### `PermissionChecker`

```go
type PermissionChecker interface {
    Can(ctx context.Context, identity IdentityContext, resource, action string) (bool, error)
}
```

Implementado por `auth.PermissionEngine`. Soporta wildcards: `*:*`, `resource:*`, `resource:action`.

### `AuthorizationService`

```go
type AuthorizationService interface {
    ResolvePermissions(ctx, userID, orgID, projectID) ([]string, error)
    ResolveOrgRole(ctx, userID, orgID) (string, error)
    ResolveProjectRole(ctx, userID, projectID) (string, error)
}
```

Implementado por `auth.MembershipResolver`.

### `IdentityRepository` (composite)

Agrupa todos los repositorios de persistencia:

- `UserRepository` — `CreateUser`, `FindUserByID`, `FindUserByEmail`, `FindPasswordHash`, `UpdateUser`, `DeleteUser`, `ListUsers`
- `RoleRepository` — `FindRoleByCode`, `FindRoleByID`, `ListRoles`
- `PermissionRepository` — `FindPermissionByCode`, `FindPermissionByID`, `ListPermissions`, `ListPermissionsByRole`
- `UserRoleRepository` — `AssignRole`, `RevokeRole`, `ListUserRolesByUser`, `ListUserRolesByRole`
- `ExternalIdentityRepository` — `LinkExternalIdentity`, `FindExternalIdentityByProviderSubject`, `FindExternalIdentitiesByUser`, `UpdateExternalIdentityLastSeen`
- `MembershipRepository` — `AddMember`, `UpdateMember`, `RemoveMember`, `ListMembershipsByOrg`, `ListMembershipsByUser`, `FindMembership`
- `ProjectRepository` — `FindProjectByID`, `ListProjectsByOrg`, `CheckUserProjectAccess`

## Entradas

### HTTP (endpoints públicos)

| Endpoint | Método | Body | Descripción |
|----------|--------|------|-------------|
| `/` | GET | — | Información de la API |
| `/healthz` | GET | — | Health check |
| `/auth/register` | POST | `RegisterRequest` (email, display_name, password?) | Registrar usuario |
| `/auth/login` | POST | `LoginRequest` (email, password) | Login con contraseña |
| `/auth/oidc/login` | GET | — | Redirect a proveedor OIDC (no disponible en Report 1.0) |
| `/auth/oidc/callback` | GET | query `code`, `state` | Callback OIDC (no disponible en Report 1.0) |
| `/auth/m2m/token` | POST | `M2MTokenRequest` (client_id, client_secret, scope?) | Token máquina a máquina |
| `/auth/token/refresh` | POST | — | Refresh token (pendiente) |

### HTTP (endpoints protegidos)

| Endpoint | Método | Body | Middleware requerido |
|----------|--------|------|----------------------|
| `/api/v1/users` | GET | — | `AuthMiddleware` + `RequireRole("platform_admin")` |
| `/api/v1/can` | POST | — | `AuthMiddleware` + `RequirePermission("resources", "read")` |

### Credentials (dominio)

```go
type Credentials struct {
    Type         string // "password" | "client_credentials" | "managed_identity"
    Email        string
    Password     string
    IDToken      string
    ClientID     string
    ClientSecret string
    Scope        string
    MIToken      string
    MIClientID   string
}
```

## Salidas

### `IdentityContext` (contexto de solicitud)

```go
type IdentityContext struct {
    UserID        uuid.UUID
    Email         string
    DisplayName   string
    AuthMethod    string // "oauthc" | "password" | "service_principal" | "managed_identity"
    SystemRoles   []string
    OrgID         uuid.UUID
    OrgRole       string // owner | admin | member | viewer
    ProjectID     *uuid.UUID
    ProjectRole   *string
    Permissions   []string
    RequestID     string
    CorrelationID string
    TokenExpiresAt int64 // unix ms
}
```

Viaja en `context.Context` tras el middleware de autenticación. Los handlers de negocio lo extraen con `iam.IdentityContextFromContext(r.Context())`.

### Respuestas HTTP

Todas las respuestas de error usan formato:
```json
{"code": "CÓDIGO_DEL_ERROR", "message": "descripción"}
```

### Tokens

- **Access token**: JWT HS256, TTL 15 min, contiene claims `sub`, `email`, `name`, `amr`, `roles`, `org`, `org_r`, `prj`, `prj_r`, `perm`, `rid`, `cid`, `iat`, `exp`, `iss`, `aud`.
- **Refresh token**: string opaca base64url, TTL 7 días (storage pendiente, `shortcut` marcado).

## Dependencias

### Internas (mismo módulo)

Todas las dependencias entre archivos de `internal/iam/` son internas al paquete, excepto `platform/database` para los modelos GORM.

### Externas (go.mod)

| Paquete | Uso |
|---------|-----|
| `github.com/google/uuid` | Identificadores UUID |
| `github.com/golang-jwt/jwt/v5` | Firmado y validación de JWT HS256 |
| `golang.org/x/crypto/argon2` | Hash de contraseñas (Argon2id) |
| `github.com/coreos/go-oidc/v3` | Descubrimiento y verificación de proveedores OIDC |
| `golang.org/x/oauth2` | Flujo Authorization Code OIDC |
| `gorm.io/gorm` + `gorm.io/driver/postgres` + `gorm.io/driver/sqlite` | Persistencia portable |
| `gorm.io/datatypes` | Campos JSON (usados por otros modelos de `platform/database`) |

### Estándar de Go

- `context`, `crypto/rand`, `encoding/base64`, `encoding/json`, `errors`, `fmt`, `net/http`, `strconv`, `strings`, `time`

No se añaden frameworks web (Chi, Gin, Echo, etc.). Se usa `net/http` + `ServeMux` de la biblioteca estándar.

## Datos

### Base de datos operativa `cco_app`

El módulo IAM persiste en los schemas `iam` y `organization` de `cco_app` (PostgreSQL) o tablas sin calificar (SQLite).

#### Tablas utilizadas

| Schema | Tabla | Entidad | Notas |
|--------|-------|---------|-------|
| `iam` | `users` | User | `password_hash` es columna adicional solo para dev (requiere `ALTER TABLE` en prod) |
| `iam` | `roles` | Role | Roles de sistema precargados |
| `iam` | `permissions` | Permission | Códigos de permiso |
| `iam` | `role_permissions` | RolePermission | Relación rol ↔ permiso |
| `iam` | `user_roles` | UserRole | Asignación usuario ↔ rol |
| `iam` | `external_identities` | ExternalIdentity | Mapeo OIDC subject |
| `organization` | `organizations` | Organization | Referenciada por membresías |
| `organization` | `members` | Membership | Usuario ↔ organización con rol |
| `organization` | `projects` | Project | Proyectos por organización |

### Flujo de datos de autenticación

1. Cliente envía credenciales a `/auth/login` o `/auth/m2m/token`.
2. Adapter correspondiente valida las credenciales (Argon2id hash, OIDC ID token, client secret, MI token).
3. Se resuelve el usuario (o se crea si no existe).
4. Se resuelve la organización y el proyecto activos (mediante membresía).
5. `MembershipResolver` compila los permisos efectivos (system roles + org role + project role).
6. `TokenManager` genera un JWT HS256 firmado con los claims de identidad.
7. Middleware `AuthMiddleware` valida el JWT en requests posteriores, resuelve org/project desde headers `X-Org-ID` / `X-Project-ID` si se envían, y inyecta `IdentityContext` en `context.Context`.
8. Handlers de negocio llaman a `PermissionChecker.Can(ctx, ic, "recurso", "acción")`.

## Algoritmos / lógica

### PasswordEngine (Argon2id)

1. `HashPassword(password)` → `argon2.IDKey` con parámetros fijos (64 MB, 3 iter, 4 threads).
2. `VerifyPassword(hash, password)` → `argon2.IDKey` + `subtle.ConstantTimeCompare`.

### TokenManager (JWT HS256)

1. `GenerateAccessToken(ic)` → construye `jwt.MapClaims` con identidad, firma con `SigningMethodHS256` usando `SigningKey`.
2. `GenerateRefreshToken()` → 32 bytes aleatorios codificados en base64url.
3. `ValidateAccessToken(token)` → `jwt.Parse` con validación de `issuer`, `audience`, método de firma y expiración.

### PermissionEngine

1. `EffectivePermissions(ic)` → une permisos de `SystemRoles` (mapa estático `rolePermissions`) + `OrgRole` (mapa estático `membershipRolePermissions`) + `ProjectRole` si está definido.
2. `Can(ctx, ic, resource, action)` → verifica si existe `resource:action`, `resource:*` o `*:*` en el conjunto de permisos efectivos.

### MembershipResolver

1. `ResolveOrgRole(ctx, userID, orgID)` → consulta `members` para obtener `membership_role` si `status = 'active'`.
2. `ResolveProjectRole(ctx, userID, projectID)` → valida acceso al proyecto vía membresía de su organización, hereda el rol de org.
3. `ResolvePermissions(ctx, userID, orgID, projectID)` → obtiene roles de sistema del usuario, resuelve org/project role, construye `IdentityContext` temporal y delega en `PermissionEngine.EffectivePermissions`.

## Errores y excepciones

### Errores de dominio (`internal/iam/errors.go`)

Todos implementan `error` y se mapean a códigos de API mediante `MapErrorToCode`.

| Error | Código HTTP | Código API |
|-------|-------------|------------|
| `ErrInvalidCredentials` | 401 | `INVALID_CREDENTIALS` |
| `ErrUserNotFound` | 404 | `USER_NOT_FOUND` |
| `ErrUserExists` | 409 | `USER_EXISTS` |
| `ErrUserInactive` | 401 | `USER_INACTIVE` |
| `ErrEmailNotVerified` | 401 | `EMAIL_NOT_VERIFIED` |
| `ErrTokenExpired` | 401 | `TOKEN_EXPIRED` |
| `ErrTokenInvalid` | 401 | `TOKEN_INVALID` |
| `ErrTokenMalformed` | 401 | `TOKEN_MALFORMED` |
| `ErrUnauthorized` | 401 | `UNAUTHORIZED` |
| `ErrForbidden` | 403 | `FORBIDDEN` |
| `ErrInsufficientScope` | 403 | `INSUFFICIENT_SCOPE` |
| `ErrOrgNotFound` | 404 | `ORG_NOT_FOUND` |
| `ErrOrgInactive` | 403 | `ORG_INACTIVE` |
| `ErrProjectNotFound` | 404 | `PROJECT_NOT_FOUND` |
| `ErrProjectInactive` | 403 | `PROJECT_INACTIVE` |
| `ErrNotMember` | 403 | `NOT_MEMBER` |
| `ErrNotProjectMember` | 403 | `NOT_PROJECT_MEMBER` |
| `ErrRoleNotFound` | 404 | `ROLE_NOT_FOUND` |
| `ErrPermissionNotFound` | 404 | `PERMISSION_NOT_FOUND` |
| `ErrInvalidRole` | 400 | `INVALID_ROLE` |
| `ErrInvalidMembershipRole` | 400 | `INVALID_MEMBERSHIP_ROLE` |
| `ErrCannotRemoveOwner` | 403 | `CANNOT_REMOVE_OWNER` |
| `ErrLastOwner` | 403 | `LAST_OWNER` |
| `ErrProviderNotSupported` | 400 | `PROVIDER_NOT_SUPPORTED` |
| `ErrExternalIdentityExists` | 409 | `EXTERNAL_IDENTITY_EXISTS` |
| `ErrInvalidToken` | 401 | `INVALID_TOKEN` |
| `ErrMIValidationFailed` | 401 | `MI_VALIDATION_FAILED` |
| `ErrSPValidationFailed` | 401 | `SP_VALIDATION_FAILED` |

### `ErrorWithCode`

```go
type ErrorWithCode struct {
    Code    string
    Message string
    Err     error
}
```

Permite envolver errores con un código legible por máquina para respuestas HTTP.

## Seguridad

### Autenticación

- **Contraseñas:** Hash Argon2id con parámetros fijos (64 MB, 3 iter, 4 threads, 32 bytes clave, 16 bytes sal). Es el único método de autenticación de usuarios en Report 1.0, tanto en desarrollo como en producción.
- **JWT:** HS256 con clave única en variable de entorno `CCO_JWT_SIGNING_KEY`. TTL de access token: 15 minutos.
- **OIDC:** Adapter existe (`go-oidc/v3`) pero no está conectado al flujo de autenticación en Report 1.0 por limitaciones de tiempo.
- **Service Principal:** Flujo `client_credentials` para colectores y servicio ML.
- **Managed Identity:** Validación de tokens de Azure IMDS (firma RS256 pendiente, `shortcut` marcado).

### Autorización

- Modelo basado en roles (RBAC) con wildcards de permiso.
- `PermissionChecker` es la única puerta de entrada para los módulos de negocio.
- Middleware valida org/project mediante membresía activa antes de inyectar `IdentityContext`.

### Headers de trazabilidad (RNF-13)

- `X-Request-ID`: generado automáticamente si no se envía.
- `X-Correlation-ID`: para lineage de recomendaciones.
- `X-Org-ID` / `X-Project-ID`: validados contra membresía antes de usarse.

### Secretos

- Ningún secreto se almacena en texto plano en logs ni en tablas analíticas (RNF-15).
- `credential_reference` en `cloud.accounts` apunta a Azure Key Vault / Managed Identity (no implementado en Report 1.0).

## Rendimiento

| Operación | Complejidad | Notas |
|-----------|-------------|-------|
| `PermissionEngine.Can` | O(n) donde n = permisos efectivos | Conjunto pequeño (<50 permisos típicos) |
| `MembershipResolver.ResolvePermissions` | O(1) consultas DB | Una consulta por roles, una por membresía, una por proyecto |
| JWT validation | O(1) | Verificación de firma HMAC |
| Password hash | O(costo Argon2id) | Costo por defecto, adecuado para login ocasional |

No hay consultas a base de datos en el path caliente de cada request más allá de la resolución de membresía (cacheable en futuro). El endpoint `GET /` es O(1) y estático.

## Decisiones relacionadas

| ID | Decisión | Documento |
|----|----------|-----------|
| ADR-004 | Arquitectura de AuthN/AuthZ en monolito Go (sin gateway separado) | `docs/04-architecture/decisions/ADR-004-iam-authentication.md` |
| ADR-003 | Persistencia GORM portable para `cco_app` | `docs/04-architecture/decisions/ADR-003-gorm-portable-persistence.md` |
| ADR-002 | Separación de bases de datos `cco_app` y `cco_data` | `docs/04-architecture/decisions/ADR-002-two-postgresql-databases.md` |
| DEC-005 | Backend como API Gateway en Go | `docs/registry/DECISION-LOG.md` |
| DEC-011 | Autenticación con proveedor externo | `docs/registry/DECISION-LOG.md` |
| DEC-027 | Autenticación y autorización dentro del monolito Go | `docs/registry/DECISION-LOG.md` |

## Pruebas relacionadas

| ID | Prueba | Archivo | Estado |
|----|--------|---------|--------|
| TEST-IAM-001 | `PermissionEngine.Can()` wildcard y exact-match | `backend/internal/iam/permission_engine_test.go` | Aprobado |
| TEST-IAM-002 | `MapErrorToCode` para todos los errores de dominio | `backend/internal/iam/errors_test.go` | Aprobado |
| TEST-IAM-003 | Integración IAM end-to-end sobre SQLite | `backend/test/iam_integration_test.go` | Aprobado |
| TEST-IAM-004 | Endpoint `POST /auth/register` | `backend/internal/iam/handlers.go` | Aprobado |
| TEST-IAM-007 | Registro duplicado y logs críticos: email duplicado → 409 `USER_EXISTS`, logs en pasos críticos | `backend/internal/iam/handlers.go` | Aprobado |
| — | `TestMigrateAndUserRepositoryOnSQLite` (preexistente) | `backend/internal/platform/database/database_test.go` | Aprobado |

Ejecución: `go test ./... -count=1` desde `backend/`.

## Shortcuts marcados (techo conocido y camino de upgrade)

| Shortcut | Código | Techo | Camino de upgrade |
|----------|--------|-------|-------------------|
| JWT HS256 simétrico | `auth.go` | Una clave en env; rotación coordinada en múltiples instancias | RS256 + JWKS |
| PermissionEngine mapa estático | `auth.go` | Permisos no se leen de `iam.role_permissions` | Cargar desde DB con caché |
| Project access = org membership | `project_repository.go` | Sin tabla `project_members` | Añadir tabla cuando per-project ACLs sea requisito |
| Refresh token storage | `handlers.go` | `POST /auth/token/refresh` devuelve 501 | Almacenar refresh tokens hasheados en DB con expiración |
| OIDC callback | `handlers.go` | Devuelve 501 "pendiente de implementación" | Implementar `oauth2.Config.Exchange()` |
| MI token sin verificación RS256 | `managed_identity_adapter.go` | Acepta estructura JWT sin verificar firma | go-jose o Azure SDK contra JWKS de Microsoft |
| CSRF state estático en OIDC | `handlers.go` | `"dev-state"` fijo | State por request en cookie de sesión segura |
| Password solo dev | `password_adapter.go` | Producción debe forzar OIDC | Passkeys/WebAuthn, MFA TOTP |
| Password es el único método | `password_adapter.go` | Únicamente password disponible en Report 1.0 (dev y prod) | OIDC se conectará cuando haya tiempo; el adapter ya existe |
| `PasswordHash` columna | `models.go` | No existe en `cco_app.sql` de producción | `ALTER TABLE iam.users ADD COLUMN password_hash` |
| Email único en registro | `handlers.go` | `FindUserByEmail` antes de `CreateUser`; race condition posible en concurrencia alta | Unique constraint en DB como fuente de verdad final |
| Rate limiting | `middleware/security.go` | Almacenamiento en memoria (`sync.Map`); se reinicia al reiniciar | Redis-backed limiter para múltiples instancias |
| Account lockout | `auth.go` (`LoginLockout`) | Almacenamiento en memoria; bloqueo temporal de 15 min | Persistir en DB/Redis con coordinación distribuida |
| Security headers | `middleware/security.go` | CSP permisiva para desarrollo | Endurecer directivas CSP para producción |
| CORS orígenes | `middleware/security.go` | `Access-Control-Allow-Origin: *` | Restringir a dominios explícitos en producción |
| Refresh token storage | `auth.go` (`TokenManager.refreshTokens`) | `sync.Map` en memoria; rotación básica | Persistir a DB/Redis con detección de reuso |

## Notas de implementación

- El módulo no depende de frameworks web externos; usa `net/http` de la biblioteca estándar.
- Los modelos GORM viven en `internal/platform/database/models.go`; los DTOs de IAM no contienen etiquetas de GORM (separación de infraestructura y dominio).
- El middleware `AuthMiddleware` permite rutas anónimas (`/healthz`, `/`, `/auth/*`) mediante `AllowAnonymous`.
- Los repositorios usan `database.TableName(db, schema, table)` para calificar schemas en PostgreSQL y dejarlos sin calificar en SQLite.
- `IdentityContext` se inyecta en `context.Context` y viaja a través de toda la pila de handlers para trazabilidad y autorización.

## Observabilidad y logging

El módulo IAM emite logs estructurados a `stdout` mediante `log/slog` (estándar de Go 1.22), configurado una sola vez en `cmd/cco-app/main.go`.

| Aspecto | Descripción |
|---------|-------------|
| **Salida** | `stdout` (interfaz nativa hacia colectores externos: Loki, ELK, Fluentd, CloudWatch) |
| **Formato** | Texto por defecto; JSON si `CCO_LOG_FORMAT=json` |
| **Niveles** | `Info` (éxitos), `Warn` (fallos de autenticación/autorización) |
| **Campos obligatorios** | `request_id`, `correlation_id`, `user_id`, `email`, `auth_method`, `org_id`, `org_role`, `project_id`, `path`, `method` |

### Eventos registrados

| Evento | Contexto | Datos clave |
|--------|----------|-------------|
| `user registered` | POST /auth/register | `user_id`, `email`, `display_name`, `status` |
| `user logged in` | POST /auth/login | `user_id`, `email`, `auth_method`, `org_id`, `org_role` |
| `m2m token issued` | POST /auth/m2m/token | `user_id`, `auth_method`, `client_id` |
| `authenticated request` | Cualquier request autenticado | `request_id`, `correlation_id`, `user_id`, `email`, `auth_method`, `org_id`, `org_role`, `project_id`, `path`, `method` |
| `auth failed` | Fallos de authz | `path`, `remote`, `error`, `code`, opcionalmente `user_id` |

### Ejemplo de salida (texto)

```
time=2026-10-30T10:15:12Z level=INFO msg="authenticated request" request_id=... correlation_id=... user_id=... email=ana@example.com auth_method=password org_id=... org_role=viewer path=/api/v1/can method=POST
```

### Ejemplo de salida (JSON, `CCO_LOG_FORMAT=json`)

```json
{"time":"2026-10-30T10:15:12Z","level":"INFO","msg":"authenticated request","request_id":"...","correlation_id":"...","user_id":"...","email":"ana@example.com","auth_method":"password","org_id":"...","org_role":"viewer","path":"/api/v1/can","method":"POST"}
```

### shortcuit

```go
// shortcut: un solo handler global; upgrade: handler por módulo con niveles
// diferentes si el volumen de logs lo justifica.
```

Este shortcut está marcado en `cmd/cco-app/main.go` donde se inicializa `slog`.

### Véase también

- `docs/04-architecture/decisions/ADR-006-logging-standard.md` — decisión de arquitectura.
- `backend/API.md` — sección "Observabilidad y logging" con ejemplos de uso.

## Seguridad del módulo IAM

### Resumen de medidas implementadas

| Medida | Implementación | Estado |
|--------|---------------|--------|
| Hash de contraseñas | `auth.go` — `HashPassword` / `VerifyPassword` con Argon2id | ✅ Activo |
| Firma JWT | `auth.go` — `TokenManager` con HS256 | ✅ Activo |
| Refresh token con rotación | `auth.go` — `TokenManager.refreshTokens`, `handlers.go` — `TokenRefreshHandler` | ✅ Activo |
| Rate limiting | `middleware/security.go` — `RateLimitMiddleware` | ✅ Activo |
| Account lockout | `auth.go` — `LoginLockout`, `handlers.go` — `LoginHandler` | ✅ Activo |
| Security headers | `middleware/security.go` — `SecurityHeaders` | ✅ Activo |
| CORS | `middleware/security.go` — `CORS` | ✅ Activo |
| Trazabilidad | `identity_context.go` — `IdentityContext`, `auth_middleware.go` | ✅ Activo |

### Hash de contraseñas: Argon2id

Las contraseñas se hashean con **Argon2id** (ganador del Password Hashing Competition 2015), usando `golang.org/x/crypto/argon2`. Es memory-hard, resistente a ataques con GPU y ASIC.

**Parámetros:**
- Memoria: 64 MB
- Iteraciones: 3
- Paralelismo: 4 hilos
- Longitud de clave: 32 bytes
- Sal: 16 bytes aleatorios por contraseña
- Comparación: `crypto/subtle.ConstantTimeCompare` (previene timing attacks)

**Formato almacenado:** `base64(salt) + "." + base64(hash)`

**shortcut:** Parámetros fijos en código; upgrade: hacer configurables vía variables de entorno.

### ¿Qué es más seguro que el actual?

| Componente | Actual (Report 1.0) | Más seguro (upgrade) | Por qué |
|------------|---------------------|----------------------|---------|
| Hash de password | Argon2id (64 MB, 3 iter, 4 threads) | Argon2id con memoria >64 MB, más iteraciones, o **scrypt** | Mayor memoria e iteraciones aumentan el costo para atacantes con GPU/ASIC |
| Firma JWT | HS256 (clave simétrica) | **RS256** (RSA) o **EdDSA/Ed25519** | Firma asimétrica: clave privada solo firma, pública verifica; rotación de claves sin compartir secretos |
| Refresh tokens | `sync.Map` en memoria | **DB / Redis** con rotación y detección de reuso | Persistencia y detección de uso simultáneo (posible robo) |

### Rate limiting

- **Límite:** 60 requests por minuto por IP.
- **Ventana:** Deslizante de 60 segundos.
- **Respuesta:** `429 Too Many Requests`.
- **Detección de IP:** `X-Forwarded-For` o `RemoteAddr`.

**shortcut:** Memoria (`sync.Map`); se reinicia al reiniciar. Upgrade: Redis-backed limiter.

### Account lockout

- **Umbral:** 5 intentos fallidos de login por email.
- **Bloqueo:** 15 minutos tras alcanzar el umbral.
- **Reinicio:** Contador se resetea en login exitoso.
- **Respuesta durante bloqueo:** `429 Too Many Requests`.

**shortcut:** Memoria; no sobrevive reinicios. Upgrade: DB/Redis con coordinación distribuida.

### Security headers (todas las respuestas)

| Header | Valor | Propósito |
|--------|-------|-----------|
| `X-Content-Type-Options` | `nosniff` | Evita MIME sniffing |
| `X-Frame-Options` | `DENY` | Evita clickjacking |
| `Referrer-Policy` | `no-referrer` | No envía información de referencia |
| `Content-Security-Policy` | `default-src 'self'; ...` | Restringe fuentes de contenido |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | Fuerza HTTPS |

**shortcut:** CSP permisiva para dev; upgrade: endurecer CSP en producción.

### CORS

- Orígenes: `*` (todos en dev)
- Métodos: GET, POST, OPTIONS, PUT, DELETE
- Headers: Authorization, Content-Type, X-Request-ID, X-Correlation-ID, X-Org-ID, X-Project-ID

**shortcut:** Permite todos los orígenes; upgrade: restringir a dominios explícitos en producción.

### Véase también

- `docs/04-architecture/decisions/ADR-007-security-design.md` — decisión de arquitectura de seguridad.
- `backend/API.md` — sección "Seguridad de la API" con ejemplos de uso.
