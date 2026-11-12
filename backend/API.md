# API Reference — Cloud Cost Optimizer (backend)

Documentación de la API HTTP expuesta por `backend/cmd/cco-app/main.go`.

Base URL (desarrollo local): `http://localhost:8080`

---

## Tabla de contenidos

1. [Variables de entorno](#variables-de-entorno)
2. [Compilar y ejecutar](#compilar-y-ejecutar)
3. [Endpoints públicos](#endpoints-públicos)
   - [GET /](#get-)
   - [GET /healthz](#get-healthz)
   - [POST /auth/register](#post-authregister)
   - [POST /auth/login](#post-authlogin)
   - [GET /auth/oidc/login](#get-authocidlogin)
   - [GET /auth/oidc/callback](#get-authocidcallback)
   - [POST /auth/m2m/token](#post-authm2mtoken)
   - [POST /auth/token/refresh](#post-authrefreshtoken)
4. [Endpoints protegidos](#endpoints-protegidos)
   - [GET /api/v1/users](#get-apiv1users)
   - [POST /api/v1/can](#post-apiv1can)
5. [Observabilidad y logging](#observabilidad-y-logging)
6. [Seguridad de la API](#seguridad-de-la-api)
7. [Códigos de error](#códigos-de-error)
8. [Headers de trazabilidad](#headers-de-trazabilidad)
9. [Ejemplos completos (PowerShell)](#ejemplos-completos-powershell)
10. [Endpoints pendientes (futuro)](#endpoints-pendientes-futuro)
11. [Estructura del repositorio](#estructura-del-repositorio)

---

## 1. Variables de entorno

| Variable | Valor por defecto | Descripción |
|----------|------------------|-------------|
| `CCO_DB_DSN` | `file::memory:?cache=shared` | Cadena de conexión. SQLite en memoria para dev; PostgreSQL DSN para prod. |
| `CCO_DB_DRIVER` | `sqlite` | `sqlite` o `postgres` |
| `CCO_JWT_SIGNING_KEY` | `dev-secret-change-in-production` | Clave para firmar JWT. **Cámbiala en producción.** |
| `CCO_HTTP_ADDR` | `:8080` | Dirección y puerto del servidor HTTP |
| `CCO_OIDC_ISSUER_URL` | (vacío) | Issuer de Entra ID / Keycloak / Google. Si está vacío, OIDC queda deshabilitado. |
| `CCO_OIDC_CLIENT_ID` | (vacío) | Client ID de OIDC |
| `CCO_OIDC_CLIENT_SECRET` | (vacío) | Client secret de OIDC |
| `CCO_OIDC_REDIRECT_URL` | (vacío) | URL de redirect de OIDC |
| `CCO_COOKIE_DOMAIN` | (vacío) | Dominio para cookies de auth |
| `CCO_COOKIE_SECURE` | `false` | Cookies solo sobre HTTPS (`true` en producción) |

---

## 2. Compilar y ejecutar

Desde la carpeta `backend/`:

```powershell
# Compilar
go build -o cco-app.exe ./cmd/cco-app/

# Ejecutar (desarrollo, SQLite en memoria)
$env:CCO_DB_DSN = "file::memory:?cache=shared"
$env:CCO_DB_DRIVER = "sqlite"
$env:CCO_JWT_SIGNING_KEY = "clave-secreta-dev"
$env:CCO_HTTP_ADDR = ":8080"
.\cco-app.exe
```

Salida esperada en consola:
```
cco-app listening on :8080 (driver=sqlite)
```

> **Nota:** Con SQLite en memoria (`file::memory:?cache=shared`), los datos se borran al cerrar el ejecutable. Para persistir entre reinicios usa un archivo: `CCO_DB_DSN=file:./cco_app.db?cache=shared` o conéctate a PostgreSQL con un DSN real.

---

## 3. Endpoints públicos

### GET /

Información básica de la API. Endpoint raíz, no requiere autenticación.

**Request:**
```powershell
curl http://localhost:8080/
```

**Response (200 OK):**
```json
{
  "name": "Cloud Cost Optimizer",
  "version": "1.0.0",
  "endpoints": [
    {"method": "GET", "path": "/", "description": "API information"},
    {"method": "GET", "path": "/healthz", "description": "Health check"},
    {"method": "POST", "path": "/auth/register", "description": "Register a new user"},
    ...
  ],
  "documentation": "See API.md"
}
```

---

### GET /healthz

Liveness / readiness check. No requiere autenticación.

**Request:**
```powershell
curl http://localhost:8080/healthz
```

**Response (200 OK):**
```json
{"status":"ok"}
```

---

### POST /auth/register

Crea un nuevo usuario. Endpoint público (sin autenticación). Diseñado para crear el primer usuario en desarrollo.

**Request body:**
```json
{
  "email": "ana@example.com",
  "display_name": "Ana Pérez",
  "password": "Password123"
}
```

| Campo | Tipo | Obligatorio | Descripción |
|-------|------|-------------|-------------|
| `email` | string | Sí | Correo electrónico del usuario |
| `display_name` | string | Sí | Nombre para mostrar |
| `password` | string | Sí | Contraseña en texto plano. Se hashea con Argon2id. Es el único método de autenticación disponible en Report 1.0. |

**Response (201 Created):**
```json
{
  "user_id": "uuid-generado",
  "email": "ana@example.com",
  "display_name": "Ana Pérez",
  "status": "active",
  "email_verified": false,
  "created_at": 1730123456789
}
```

**Errores posibles:**
- `400 Bad Request` — `email` o `display_name` faltantes
- `409 Conflict` — `USER_EXISTS` (el email ya está registrado)

**Ejemplo PowerShell:**
```powershell
$body = @{
    email       = "ana@example.com"
    display_name= "Ana Pérez"
    password    = "Password123"
} | ConvertTo-Json

curl -X POST http://localhost:8080/auth/register -H "Content-Type: application/json" -d $body
```

---

### POST /auth/login

Autenticación con email + contraseña (modo dev/local). Devuelve un par de tokens JWT.

**Request body:**
```json
{
  "email": "ana@example.com",
  "password": "Password123"
}
```

**Response (200 OK):**
```json
{
  "access_token":  "jwt-...",
  "refresh_token": "...",
  "expires_at":    1730123456789,
  "user_id":       "uuid-generado",
  "email":         "ana@example.com",
  "org_id":        "00000000-0000-0000-0000-000000000000",
  "org_role":      "viewer"
}
```

**Errores posibles:**
- `400 Bad Request` — cuerpo inválido
- `401 Unauthorized` — credenciales inválidas / usuario inactivo

**Ejemplo PowerShell:**
```powershell
$body = @{
    email    = "ana@example.com"
    password = "Password123"
} | ConvertTo-Json

curl -X POST http://localhost:8080/auth/login -H "Content-Type: application/json" -d $body
```

---

### GET /auth/oidc/login

Redirige al proveedor OIDC (Entra ID / Keycloak / Google).

**Estado:** No disponible en Report 1.0. El adapter existe en código pero no está conectado al flujo de autenticación por limitaciones de tiempo. Se activará en una versión futura.

---

### GET /auth/oidc/callback

Endpoint de devolución de llamada de OIDC tras la autenticación federada.

**Estado:** No disponible en Report 1.0. Devuelve `501 Not Implemented`.

---

### POST /auth/m2m/token

Autenticación máquina a máquina mediante Service Principal (flujo `client_credentials`). Usado por colectores, servicio ML y otros llamadores sin usuario humano.

**Request body:**
```json
{
  "client_id": "cco-collector",
  "client_secret": "secret",
  "scope": "metrics:write config:read"
}
```

| Campo | Tipo | Obligatorio | Descripción |
|-------|------|-------------|-------------|
| `client_id` | string | Sí | Identificador del cliente de servicio |
| `client_secret` | string | Sí | Secreto del cliente |
| `scope` | string | No | Alcance solicitado (ej. `metrics:write config:read`) |

**Response (200 OK):**
```json
{
  "access_token":  "jwt-...",
  "refresh_token": "...",
  "expires_at":    1730123456789,
  "auth_method":   "service_principal",
  "user_id":       "uuid-generado"
}
```

**Ejemplo PowerShell:**
```powershell
$body = @{
    client_id     = "cco-collector"
    client_secret = "secret"
    scope         = "metrics:write config:read"
} | ConvertTo-Json

curl -X POST http://localhost:8080/auth/m2m/token -H "Content-Type: application/json" -d $body
```

---

### POST /auth/token/refresh

Renueva el access token usando un refresh token. Implementa rotación: revoca el refresh token usado y emite un par nuevo.

**Request body:**
```json
{
  "refresh_token": "refresh-token-valor"
}
```

| Campo | Tipo | Obligatorio | Descripción |
|-------|------|-------------|-------------|
| `refresh_token` | string | Sí | Refresh token previo obtenido en login o refresh anterior |

**Response (200 OK):**
```json
{
  "access_token":  "jwt-...",
  "refresh_token": "nuevo-refresh-token",
  "expires_at":    1730123456789,
  "user_id":       "uuid-generado"
}
```

**Errores posibles:**
- `400 Bad Request` — cuerpo inválido o refresh_token faltante
- `401 Unauthorized` — refresh token inválido o expirado
- `404 Not Found` — usuario asociado al token no existe

**Ejemplo PowerShell:**
```powershell
$body = @{
    refresh_token = "refresh-token-valor"
} | ConvertTo-Json

curl -X POST http://localhost:8080/auth/token/refresh -H "Content-Type: application/json" -d $body
```

---

## 4. Endpoints protegidos

Todos requieren header `Authorization: Bearer <access_token>`.

### GET /api/v1/users

Lista información del usuario autenticado. Requiere rol `platform_admin`.

**Request:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
curl -X GET http://localhost:8080/api/v1/users -H "Authorization: Bearer $token"
```

**Response (200 OK):**
```json
{
  "message":     "authenticated as ana@example.com (org_role=viewer)",
  "user_id":     "uuid-generado",
  "org_id":      "uuid-organización",
  "org_role":    "viewer",
  "permissions": ["resources:read", "dashboards:read"]
}
```

**Errores posibles:**
- `401 Unauthorized` — token ausente, inválido o expirado
- `403 Forbidden` — el usuario no tiene rol `platform_admin`

---

### POST /api/v1/can

Verifica si el usuario autenticado tiene un permiso sobre un recurso. Endpoint de ejemplo para demostrar el middleware de autorización.

**Request:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
curl -X POST http://localhost:8080/api/v1/can -H "Authorization: Bearer $token" -H "Content-Type: application/json" -d '{}'
```

**Response (200 OK):**
```json
{
  "allowed":  true,
  "user_id":  "uuid-generado",
  "resource": "resources",
  "action":   "read"
}
```

**Errores posibles:**
- `401 Unauthorized` — token ausente, inválido o expirado
- `403 Forbidden` — el usuario no tiene el permiso solicitado

---

## 6. Observabilidad y logging

El backend emite logs estructurados a `stdout` mediante `log/slog` (estándar de Go 1.22). No hay dependencias externas ni un servicio de logs separado: stdout es la interfaz hacia cualquier colector (Loki, ELK, Fluentd, CloudWatch).

### Variable de entorno

| Variable | Valor por defecto | Descripción |
|----------|------------------|-------------|
| `CCO_LOG_FORMAT` | (vacío = texto) | `json` para salida JSON; cualquier otro valor produce texto legible |

### Ejemplo de salida (texto)

```
time=2026-10-30T10:15:05Z level=INFO msg="user registered" user_id=... email=ana@example.com display_name="Ana Pérez" status=active
time=2026-10-30T10:15:12Z level=INFO msg="authenticated request" request_id=... correlation_id=... user_id=... email=ana@example.com auth_method=password org_id=... org_role=viewer path=/api/v1/can method=POST
time=2026-10-30T10:15:15Z level=WARN msg="auth failed: token validation error" path=/api/v1/users remote=... error=... code=TOKEN_EXPIRED
```

### Ejemplo de salida (JSON, `CCO_LOG_FORMAT=json`)

```json
{"time":"2026-10-30T10:15:12Z","level":"INFO","msg":"authenticated request","request_id":"...","correlation_id":"...","user_id":"...","email":"ana@example.com","auth_method":"password","org_id":"...","org_role":"viewer","path":"/api/v1/can","method":"POST"}
```

### Campos obligatorios en logs de autenticación

| Campo | Descripción |
|-------|-------------|
| `request_id` | Identificador de solicitud (generado si el cliente no envía `X-Request-ID`) |
| `correlation_id` | Identificador de correlación para lineage (por defecto igual a `request_id`) |
| `user_id` | UUID del usuario autenticado |
| `email` | Correo del usuario |
| `auth_method` | `password` | Método de autenticación utilizado (solo `password` en Report 1.0) |
| `org_id` | UUID de la organización activa |
| `org_role` | `owner`, `admin`, `member` o `viewer` |
| `project_id` | UUID del proyecto activo (si aplica) |
| `path` | Ruta HTTP solicitada |
| `method` | Método HTTP |

### Eventos registrados

- `user registered` — POST /auth/register exitoso
- `user logged in` — POST /auth/login exitoso
- `m2m token issued` — POST /auth/m2m/token exitoso
- `authenticated request` — cada request autenticado que pasa el middleware
- `auth failed` — fallos de autenticación (credenciales inválidas, usuario inactivo, cuenta bloqueada, formato incorrecto)

### Uso con colector externo

```powershell
# Producción: salida JSON hacia un colector
$env:CCO_LOG_FORMAT = "json"
.\cco-app.exe | tee cco-app.log
# o redirigir a: fluentd, loki, filebeat, cloudwatch, etc.
```

**shortcut:** un solo handler global para toda la aplicación; upgrade: handler por módulo con niveles diferentes si el volumen de logs lo justifica (ver ADR-006).

---

## 6. Seguridad de la API

### Resumen de medidas de seguridad

| Medida | Estado | Descripción |
|--------|--------|-------------|
| Hash de contraseñas | ✅ Argon2id | `golang.org/x/crypto/argon2`, memory-hard, resistente a GPU/ASIC |
| Firma JWT | ✅ HS256 | Clave simétrica, TTL 15 min access / 7 días refresh |
| Refresh token rotation | ✅ Implementado | Revoca el refresh token usado y emite uno nuevo |
| Rate limiting | ✅ 60 req/min por IP | Ventana deslizante, respuesta 429 al exceder |
| Account lockout | ✅ 5 intentos / 15 min | Bloqueo temporal por email tras fallos repetidos |
| Security headers | ✅ Implementado | `nosniff`, `DENY`, `no-referrer`, CSP, HSTS |
| CORS | ✅ Implementado | Orígenes, métodos y headers controlados |
| Trazabilidad | ✅ Implementado | `request_id`, `correlation_id` en logs y contexto |

### Hash de contraseñas: Argon2id

Las contraseñas se hashean con **Argon2id** (ganador del Password Hashing Competition 2015), usando `golang.org/x/crypto/argon2`. Es memory-hard, lo que lo hace resistente a ataques con GPU y ASIC.

**Parámetros utilizados:**
- Memoria: 64 MB
- Iteraciones: 3
- Paralelismo: 4 hilos
- Longitud de clave: 32 bytes
- Longitud de sal: 16 bytes (aleatoria por contraseña)

**Formato del hash almacenado:** `base64(salt) + "." + base64(hash)`

**shortcut:** Parámetros fijos en código; upgrade: hacer configurables vía variables de entorno y aumentar memoria/iteraciones según evolucione el hardware.

### ¿Qué es más seguro que el actual?

| Componente | Actual (Report 1.0) | Más seguro (upgrade) | Por qué |
|------------|---------------------|----------------------|---------|
| Hash de password | Argon2id (64 MB, 3 iter, 4 threads) | Argon2id con memoria >64 MB, más iteraciones, o **scrypt** | Mayor memoria e iteraciones aumentan el costo para atacantes con GPU/ASIC. Scrypt es también memory-hard y estándar IETF RFC 7914. |
| Firma JWT | HS256 (clave simétrica) | **RS256** (RSA) o **EdDSA/Ed25519** | Firma asimétrica: la clave privada solo firma, la pública verifica. Permite rotación de claves sin compartir secretos entre instancias. |
| Almacenamiento de refresh tokens | `sync.Map` en memoria | **DB / Redis** con rotación y detección de reuso | Persistencia y detección de uso simultáneo de un mismo refresh token (posible robo). |

### Rate limiting

- **Límite:** 60 requests por minuto por IP.
- **Ventana:** Deslizante de 60 segundos.
- **Respuesta:** `429 Too Many Requests` con cuerpo JSON `{"code":"RATE_LIMITED","message":"too many requests, please try again later"}`.
- **Detección de IP:** Usa `X-Forwarded-For` si está presente, si no `RemoteAddr`.

**shortcut:** Almacenamiento en memoria (`sync.Map`); se reinicia al reiniciar la app. Upgrade: Redis-backed limiter para múltiples instancias.

### Account lockout (bloqueo de cuenta)

- **Umbral:** 5 intentos fallidos de login por email.
- **Bloqueo:** 15 minutos tras alcanzar el umbral.
- **Reinicio:** El contador se resetea en login exitoso.
- **Respuesta durante bloqueo:** `429 Too Many Requests` con mensaje indicando bloqueo temporal.

**shortcut:** Almacenamiento en memoria; no sobrevive reinicios. Upgrade: persistir en DB/Redis con coordinación distribuida.

### Security headers aplicados a todas las respuestas

| Header | Valor | Propósito |
|--------|-------|-----------|
| `X-Content-Type-Options` | `nosniff` | Evita que el navegador infiera el MIME type |
| `X-Frame-Options` | `DENY` | Evita clickjacking (no se puede incrustar en iframes) |
| `Referrer-Policy` | `no-referrer` | No envía información de referencia a otros sitios |
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'` | Restringe fuentes de contenido cargable |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | Fuerza HTTPS en navegadores que lo soporten |

**shortcut:** CSP permisiva para desarrollo; upgrade: endurecer directivas CSP para producción.

### CORS

- `Access-Control-Allow-Origin: *` (todos los orígenes en dev)
- Métodos permitidos: GET, POST, OPTIONS, PUT, DELETE
- Headers permitidos: Authorization, Content-Type, X-Request-ID, X-Correlation-ID, X-Org-ID, X-Project-ID

**shortcut:** Permite todos los orígenes; upgrade: restringir a dominios explícitos en producción.

---

## 7. Códigos de error

Todos los errores devuelven JSON con formato:
```json
{
  "code":    "CÓDIGO_DEL_ERROR",
  "message": "descripción legible"
}
```

| Código | HTTP | Significado |
|--------|------|-------------|
| `INVALID_CREDENTIALS` | 401 | Credenciales inválidas |
| `USER_NOT_FOUND` | 404 | Usuario no encontrado |
| `USER_EXISTS` | 409 | El email ya está registrado |
| `USER_INACTIVE` | 401 | Usuario inactivo |
| `EMAIL_NOT_VERIFIED` | 401 | Email no verificado |
| `TOKEN_EXPIRED` | 401 | Token expirado |
| `TOKEN_INVALID` | 401 | Token inválido |
| `TOKEN_MALFORMED` | 401 | Token malformado |
| `UNAUTHORIZED` | 401 | No autenticado |
| `FORBIDDEN` | 403 | Sin permiso suficiente |
| `INSUFFICIENT_SCOPE` | 403 | Alcance insuficiente |
| `ORG_NOT_FOUND` | 404 | Organización no encontrada |
| `PROJECT_NOT_FOUND` | 404 | Proyecto no encontrado |
| `NOT_MEMBER` | 403 | Usuario no es miembro de la organización |
| `ROLE_NOT_FOUND` | 404 | Rol no encontrado |
| `INVALID_TOKEN` | 401 | Token inválido |
| `NOT_IMPLEMENTED` | 501 | Funcionalidad pendiente |
| `INTERNAL_ERROR` | 500 | Error interno |
| `ACCOUNT_NOT_FOUND` | 404 | Cuenta cloud no encontrada |
| `ACCOUNT_EXISTS` | 409 | Ya existe una cuenta para ese proveedor y suscripción |
| `RESOURCE_NOT_FOUND` | 404 | Recurso cloud no encontrado |
| `DATA_SOURCE_NOT_FOUND` | 404 | Fuente de datos (binding) no encontrada |
| `DATA_SOURCE_EXISTS` | 409 | Ya existe una fuente de datos para ese proyecto y recurso |
| `INVALID_REQUEST` | 400 | Cuerpo de la solicitud inválido |
| `CREDENTIALS_UNAVAILABLE` | 400 | La credencial de referencia no se resuelve (falta variable de entorno) |
| `CONNECTION_FAILED` | 422 | La conexión con el proveedor falló (detalle sin el secreto) |
| `PROVIDER_UNAVAILABLE` | 503 | El adaptador del proveedor no está disponible en este modo |

---

## 8. Headers de trazabilidad

El middleware inyecta y respeta los siguientes headers para trazabilidad transversal (RNF-13):

| Header | Dirección | Descripción |
|--------|-----------|-------------|
| `Authorization` | Entrada | `Bearer <access_token>` |
| `X-Request-ID` | Entrada/Salida | ID de solicitud. Si no se envía, se genera uno automáticamente |
| `X-Correlation-ID` | Entrada/Salida | ID de correlación para lineage de recomendaciones. Por defecto igual a `X-Request-ID` |
| `X-Org-ID` | Entrada | ID de organización activa (validado contra membresía) |
| `X-Project-ID` | Entrada | ID de proyecto activo (validado contra acceso) |

---

## 9. Ejemplos completos (PowerShell)

### Flujo completo: registrar → login → verificar permiso

```powershell
# 1. Registrar usuario
$regBody = @{
    email       = "ana@example.com"
    display_name= "Ana Pérez"
    password    = "Password123"
} | ConvertTo-Json

$regResp = curl -X POST http://localhost:8080/auth/register -H "Content-Type: application/json" -d $regBody
Write-Host "Registro: $($regResp)"

# 2. Login y extraer token
$loginBody = @{
    email    = "ana@example.com"
    password = "Password123"
} | ConvertTo-Json

$loginResp = curl -X POST http://localhost:8080/auth/login -H "Content-Type: application/json" -d $loginBody | ConvertFrom-Json
$token = $loginResp.access_token
Write-Host "Token: $token"

# 3. Verificar permiso
curl -X POST http://localhost:8080/api/v1/can -H "Authorization: Bearer $token" -H "Content-Type: application/json" -d '{}'

# 4. Health check
curl http://localhost:8080/healthz
```

### Flujo máquina a máquina (collector / ML service)

```powershell
$m2mBody = @{
    client_id     = "cco-collector"
    client_secret = "secret"
    scope         = "metrics:write config:read"
} | ConvertTo-Json

curl -X POST http://localhost:8080/auth/m2m/token -H "Content-Type: application/json" -d $m2mBody
```

---

## 10. Endpoints pendientes (futuro)

Estos endpoints existen en el código pero devuelven `501 Not Implemented` hasta completar su implementación:

- `GET /auth/oidc/callback` — intercambio de código de autorización OIDC

Ver `docs/04-architecture/decisions/ADR-004-iam-authentication.md` para detalles de los shortcuts y caminos de upgrade.

---

## 11. Estructura del repositorio

```
cloud-cost-optimazer/
├── API.md                          # ← este documento
├── README.md
├── LICENSE
├── backend/
│   ├── cmd/cco-app/main.go         # entry point, DI, router
│   ├── go.mod / go.sum
│   └── internal/
│       ├── iam/                    # módulo Identidad y Acceso
│       │   ├── interfaces.go       # contratos (AuthenticationProvider, TokenValidator, PermissionChecker...)
│       │   ├── auth.go             # JWT, Argon2id, PermissionEngine, MembershipResolver
│       │   ├── handlers.go         # handlers HTTP (/auth/*, /healthz)
│       │   ├── middleware/         # auth middleware, RequirePermission, RequireRole
│       │   ├── repositories/       # adaptadores GORM (users, roles, permissions, memberships, projects...)
│       │   ├── adapters/           # OIDC, password, Service Principal, Managed Identity
│       │   ├── identity_context.go # IdentityContext + context keys
│       │   ├── errors.go           # domain errors + error codes
│       │   └── *_test.go           # pruebas unitarias
│       └── platform/database/      # GORM, drivers, migración SQLite/Postgres
├── test/
│   └── iam_integration_test.go     # prueba de integración end-to-end
├── docs/                           # documentación de arquitectura, requisitos, ADRs, tests
└── databases/                      # DDL de cco_app y cco_data
```

---

## 12. Endpoints del módulo Cloud (Gestión de Nube y Fuentes de Datos)

Todos los endpoints de esta sección requieren header `Authorization: Bearer <access_token>` y pasan por `AuthMiddleware` + `RequirePermission(recurso, acción)`. Están definidos en `backend/internal/cloud/handlers.go` y resueltos contra `internal/cloud/interfaces.go`.

### POST /api/v1/cloud-accounts

Registra una cuenta cloud. Estado inicial: `pending`. Requiere permiso `cloud_accounts:write`.

**Request body:**
```json
{
  "display_name": "Suscripción Contoso Producción",
  "provider_code": "azure",
  "subscription_id": "/subscriptions/11111111-2222-3333-4444-555555555555",
  "tenant_id": "tenant-id-guid",
  "credential_reference": "env:CCO_AZURE_CLIENT_SECRET"
}
```

| Campo | Tipo | Obligatorio | Descripción |
|-------|------|-------------|-------------|
| `display_name` | string | Sí | Nombre visible de la cuenta (1-160 caracteres) |
| `provider_code` | string | Sí | `azure` | `aws` | `gcp` (Release 1.0 solo `azure`) |
| `subscription_id` | string | Sí | Identificador de suscripción del proveedor |
| `tenant_id` | string | No | ID de inquilino (requerido para `azure`) |
| `credential_reference` | string | Sí | Referencia al secreto: `env:VAR` o nombre bare de variable `VAR`; el valor se lee de `os.Getenv`, nunca se persiste |

**Response (201 Created):**
```json
{
  "cloud_account_id": "uuid-generado",
  "organization_id": "uuid-organización",
  "provider_code": "azure",
  "display_name": "Suscripción Contoso Producción",
  "subscription_id": "/subscriptions/...",
  "tenant_id": "tenant-id-guid",
  "status": "pending",
  "created_at": 1730123456789
}
```

**Errores posibles:**
- `400 Bad Request` — `INVALID_REQUEST` (campos faltantes o inválidos)
- `409 Conflict` — `ACCOUNT_EXISTS` (ya existe cuenta para ese `provider_code` + `subscription_id`)
- `401/403` — authz insuficiente

**Ejemplo PowerShell:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
$body = @{
    display_name        = "Suscripción Contoso Producción"
    provider_code       = "azure"
    subscription_id     = "/subscriptions/11111111-2222-3333-4444-555555555555"
    tenant_id           = "tenant-id-guid"
    credential_reference= "env:CCO_AZURE_CLIENT_SECRET"
} | ConvertTo-Json

curl -X POST http://localhost:8080/api/v1/cloud-accounts -H "Authorization: Bearer $token" -H "Content-Type: application/json" -d $body
```

---

### GET /api/v1/cloud-accounts

Lista las cuentas cloud de la organización del actor. Requiere permiso `cloud_accounts:read`.

**Request:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
curl -X GET http://localhost:8080/api/v1/cloud-accounts -H "Authorization: Bearer $token"
```

**Response (200 OK):**
```json
[
  {
    "cloud_account_id": "uuid-generado",
    "display_name": "Suscripción Contoso Producción",
    "provider_code": "azure",
    "subscription_id": "/subscriptions/...",
    "status": "active",
    "created_at": 1730123456789
  }
]
```

**Errores posibles:**
- `401 Unauthorized` — token ausente, inválido o expirado
- `403 Forbidden` — sin permiso `cloud_accounts:read`

---

### GET /api/v1/cloud-accounts/{id}

Obtiene una cuenta cloud por su ID. Requiere permiso `cloud_accounts:read`.

**Request:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
$id = "uuid-de-la-cuenta"
curl -X GET "http://localhost:8080/api/v1/cloud-accounts/$id" -H "Authorization: Bearer $token"
```

**Response (200 OK):**
```json
{
  "cloud_account_id": "uuid-de-la-cuenta",
  "organization_id": "uuid-organización",
  "provider_code": "azure",
  "display_name": "Suscripción Contoso Producción",
  "subscription_id": "/subscriptions/...",
  "tenant_id": "tenant-id-guid",
  "status": "active",
  "client_id": "client-id-de-azure",
  "last_connection_error": null,
  "created_at": 1730123456789,
  "updated_at": 1730123456789
}
```

**Errores posibles:**
- `404 Not Found` — `ACCOUNT_NOT_FOUND`
- `401/403` — authz insuficiente

---

### POST /api/v1/cloud-accounts/{id}/validate

Valida la conexión de la cuenta resuelvelo el secreto desde la variable de entorno indicada en `credential_reference`. Al éxito pasa a `active`; al fallo, a `error` y registra `last_connection_error` (sin el secreto). Requiere permiso `cloud_accounts:write`.

**Request:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
$id = "uuid-de-la-cuenta"
curl -X POST "http://localhost:8080/api/v1/cloud-accounts/$id/validate" -H "Authorization: Bearer $token"
```

**Response (200 OK):**
```json
{
  "cloud_account_id": "uuid-de-la-cuenta",
  "status": "active",
  "valid": true
}
```

**Response (422 Unprocessable Entity):**
```json
{
  "code": "CONNECTION_FAILED",
  "message": "no se pudo validar la conexión con Azure: autorización denied"
}
```

**Errores posibles:**
- `404 Not Found` — `ACCOUNT_NOT_FOUND`
- `400 Bad Request` — `CREDENTIALS_UNAVAILABLE` (falta la variable de entorno)
- `422 Unprocessable Entity` — `CONNECTION_FAILED`
- `503 Service Unavailable` — `PROVIDER_UNAVAILABLE` (modo `azure` no activado)
- `401/403` — authz insuficiente

**Ejemplo PowerShell:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
$id = "uuid-de-la-cuenta"
curl -X POST "http://localhost:8080/api/v1/cloud-accounts/$id/validate" -H "Authorization: Bearer $token"
```

---

### POST /api/v1/cloud-accounts/{id}/sync

Sincroniza (on-demand) las bases de datos SQL de Azure de la suscripción. Upsert normalizado por `azure_resource_id` en `cloud.resources`. Requiere permiso `cloud_accounts:write`.

**Request:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
$id = "uuid-de-la-cuenta"
curl -X POST "http://localhost:8080/api/v1/cloud-accounts/$id/sync" -H "Authorization: Bearer $token"
```

**Response (200 OK):**
```json
{
  "cloud_account_id": "uuid-de-la-cuenta",
  "synced": 2,
  "resources": [
    {
      "resource_uid": "uuid-recurso-1",
      "azure_resource_id": "/subscriptions/.../providers/Microsoft.Sql/servers/srv-db/databases/ventas",
      "server_name": "srv-db",
      "database_name": "ventas",
      "resource_group": "rg-produccion",
      "region_code": "eastus",
      "kind": "Microsoft.Sql/servers/databases",
      "lifecycle_status": "active",
      "last_seen_at": 1730123456789
    },
    {
      "resource_uid": "uuid-recurso-2",
      "azure_resource_id": "/subscriptions/.../providers/Microsoft.Sql/servers/srv-db/databases/inventario",
      "server_name": "srv-db",
      "database_name": "inventario",
      "resource_group": "rg-produccion",
      "region_code": "eastus",
      "kind": "Microsoft.Sql/servers/databases",
      "lifecycle_status": "active",
      "last_seen_at": 1730123456789
    }
  ]
}
```

**Errores posibles:**
- `404 Not Found` — `ACCOUNT_NOT_FOUND` o cuenta no `active`
- `422 Unprocessable Entity` — `CONNECTION_FAILED`
- `503 Service Unavailable` — `PROVIDER_UNAVAILABLE`
- `401/403` — authz insuficiente

---

### GET /api/v1/cloud-accounts/{id}/resources

Lista los recursos normalizados de una cuenta. Requiere permiso `resources:read`.

**Request:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
$id = "uuid-de-la-cuenta"
curl -X GET "http://localhost:8080/api/v1/cloud-accounts/$id/resources" -H "Authorization: Bearer $token"
```

**Response (200 OK):** (ver cuerpo de ejemplo en `POST .../sync`, campo `resources`)

**Errores posibles:**
- `404 Not Found` — `ACCOUNT_NOT_FOUND`
- `401/403` — authz insuficiente

---

### POST /api/v1/resource-bindings

Registra una fuente de datos (binding) asociando un proyecto con un recurso cloud (`data_resource_uid` = `resource_uid` de `cloud.resources`). Marca `is_monitored=true`. Requiere permiso `data_sources:write`.

**Request body:**
```json
{
  "project_id": "uuid-proyecto",
  "cloud_account_id": "uuid-cuenta",
  "data_resource_uid": "uuid-recurso",
  "display_name": "BD Ventas - Produccion"
}
```

| Campo | Tipo | Obligatorio | Descripción |
|-------|------|-------------|-------------|
| `project_id` | uuid | Sí | Proyecto al que se asocia la fuente de datos |
| `cloud_account_id` | uuid | Sí | Cuenta cloud dueña del recurso |
| `data_resource_uid` | uuid | Sí | `resource_uid` del recurso en `cloud.resources` |
| `display_name` | string | No | Nombre visible de la fuente de datos |

**Response (201 Created):**
```json
{
  "resource_binding_id": "uuid-binding",
  "project_id": "uuid-proyecto",
  "cloud_account_id": "uuid-cuenta",
  "data_resource_uid": "uuid-recurso",
  "display_name": "BD Ventas - Produccion",
  "is_monitored": true,
  "created_at": 1730123456789
}
```

**Errores posibles:**
- `400 Bad Request` — `INVALID_REQUEST`
- `404 Not Found` — `ACCOUNT_NOT_FOUND` o `RESOURCE_NOT_FOUND`
- `409 Conflict` — `DATA_SOURCE_EXISTS` (ya existe binding para ese proyecto/recurso o cuenta/recurso)
- `401/403` — authz insuficiente

**Ejemplo PowerShell:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
$body = @{
    project_id         = "uuid-proyecto"
    cloud_account_id   = "uuid-cuenta"
    data_resource_uid  = "uuid-recurso"
    display_name       = "BD Ventas - Produccion"
} | ConvertTo-Json

curl -X POST http://localhost:8080/api/v1/resource-bindings -H "Authorization: Bearer $token" -H "Content-Type: application/json" -d $body
```

---

### GET /api/v1/resource-bindings

Lista las fuentes de datos (bindings). Requiere permiso `data_sources:read`.

**Request:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
curl -X GET http://localhost:8080/api/v1/resource-bindings -H "Authorization: Bearer $token"
```

**Response (200 OK):** (lista de objetos como el de `POST /api/v1/resource-bindings`)

**Errores posibles:**
- `401/403` — authz insuficiente

---

### PATCH /api/v1/resource-bindings/{id}

Desactiva una fuente de datos (`is_monitored=false`). Requiere permiso `data_sources:write`.

**Request body:**
```json
{
  "is_monitored": false
}
```

| Campo | Tipo | Obligatorio | Descripción |
|-------|------|-------------|-------------|
| `is_monitored` | bool | Sí | `false` para desactivar el monitoreo de la fuente de datos |

**Response (200 OK):**
```json
{
  "resource_binding_id": "uuid-binding",
  "is_monitored": false,
  "updated_at": 1730123456789
}
```

**Errores posibles:**
- `404 Not Found` — `DATA_SOURCE_NOT_FOUND`
- `400 Bad Request` — `INVALID_REQUEST`
- `401/403` — authz insuficiente

**Ejemplo PowerShell:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
$id = "uuid-binding"
$body = @{ is_monitored = $false } | ConvertTo-Json

curl -X PATCH "http://localhost:8080/api/v1/resource-bindings/$id" -H "Authorization: Bearer $token" -H "Content-Type: application/json" -d $body
```

---

## 13. Endpoints del módulo Adquisición y Monitoreo (Adquisición y Monitoreo de Datos)

Todos los endpoints requieren `Authorization: Bearer <access_token>` y pasan por `AuthMiddleware` + `RequirePermission(recurso, acción)`. Están definidos en `backend/internal/acquisition/handlers.go`. Los ciclos escriben en `cco_data` (`raw.*`); si `CCO_DATA_DSN` no está configurado, los endpoints responden `503 STORAGE_UNAVAILABLE` y el scheduler no arranca.

El scheduler interno corre cada `CCO_ACQ_INTERVAL_SECONDS` (default 60s) y adquiere para todos los bindings con `is_monitored=true`: métricas de Azure Monitor (PT1M), `sys.dm_db_resource_stats` + sesiones (DMVs) y Query Store top-20. Las DMVs/Query Store requieren `CCO_ACQ_SQL_USER` / `CCO_ACQ_SQL_PASSWORD` (login con `VIEW DATABASE STATE`); sin ellas solo corre Azure Monitor. Ver `docs/05-design/DESIGN-AMDA-001.md`.

### POST /api/v1/acquisitions/run

Ejecuta un ciclo de adquisición ahora, síncrono, para los recursos monitoreados de la organización del actor (o uno solo). Requiere permiso `acquisitions:run`. Si ya hay un ciclo en curso responde `422 CYCLE_IN_PROGRESS`.

**Request body (opcional):**
```json
{
  "resource_uid": "uuid-recurso"
}
```

**Response (200 OK):**
```json
{
  "triggered": 2,
  "runs": [
    {
      "run_id": 41,
      "resource_uid": "uuid-recurso",
      "source": "azure_monitor_metrics",
      "status": "ok",
      "rows_ingested": 50
    },
    {
      "run_id": 42,
      "resource_uid": "uuid-recurso",
      "source": "sql_diagnostics",
      "status": "error",
      "rows_ingested": 0,
      "detail": "sql diagnostics: login failed"
    }
  ]
}
```

**Errores posibles:**
- `400 Bad Request` — `INVALID_REQUEST` (body malformado o `resource_uid` inválido)
- `404 Not Found` — `RESOURCE_NOT_FOUND` (el recurso no está monitoreado en esta organización)
- `422 Unprocessable Entity` — `CYCLE_IN_PROGRESS`
- `503 Service Unavailable` — `STORAGE_UNAVAILABLE` (falta `CCO_DATA_DSN`)
- `401/403` — authz insuficiente

**Ejemplo PowerShell:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
curl -X POST http://localhost:8080/api/v1/acquisitions/run -H "Authorization: Bearer $token" -H "Content-Type: application/json" -d '{}'
```

---

### GET /api/v1/acquisitions

Lista corridas (`raw.collector_run`) de los recursos monitoreados de la organización. Requiere permiso `acquisitions:read`.

**Query params:** `resource_uid`, `source` (`azure_monitor_metrics`|`sql_diagnostics`|`query_store`), `status` (`ok`|`partial`|`error`), `limit` (default 50, máx 500).

**Request:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
curl -X GET "http://localhost:8080/api/v1/acquisitions?source=azure_monitor_metrics&status=error&limit=10" -H "Authorization: Bearer $token"
```

**Response (200 OK):**
```json
[
  {
    "collector_run_id": 42,
    "resource_uid": "uuid-recurso",
    "source": "azure_monitor_metrics",
    "scope": "sub-1234",
    "started_at_utc": 1730123456789,
    "finished_at_utc": 1730123458456,
    "status": "ok",
    "rows_ingested": 50
  }
]
```

**Errores posibles:**
- `400 Bad Request` — `INVALID_REQUEST` (filtros inválidos)
- `503 Service Unavailable` — `STORAGE_UNAVAILABLE`
- `401/403` — authz insuficiente

---

### GET /api/v1/acquisitions/{id}

Detalle de una corrida. Requiere permiso `acquisitions:read`. Si la corrida pertenece a otra organización responde `404` (no revela su existencia).

**Request:**
```powershell
$token = "PEGAR_ACCESS_TOKEN_AQUI"
curl -X GET "http://localhost:8080/api/v1/acquisitions/42" -H "Authorization: Bearer $token"
```

**Response (200 OK):** (objeto como el de la lista, con `error_message` cuando aplique)

**Errores posibles:**
- `400 Bad Request` — `INVALID_REQUEST` (`id` no numérico)
- `404 Not Found` — `ACQUISITION_RUN_NOT_FOUND`
- `503 Service Unavailable` — `STORAGE_UNAVAILABLE`
- `401/403` — authz insuficiente

