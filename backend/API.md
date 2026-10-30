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
