# DESIGN-CLOUD-001 — Diseño técnico del módulo Gestión de Nube y Fuentes de Datos

## Componente

`backend/internal/cloud/` — paquete Go que implementa el módulo Gestión de Nube y Fuentes de Datos del backend monolito modular (Release 1.0).

Contiene:

| Archivo | Responsabilidad |
|---------|-----------------|
| `interfaces.go` | Contratos públicos (interfaces y DTOs): `ConnectionValidator`, `ResourceProvider`, `ConnectionCredentials`, `ConnectionResult`, `ResourceDTO` |
| `errors.go` | Errores de dominio, `ErrorWithCode`, `MapErrorToCode` |
| `service.go` | Casos de uso: registrar cuenta, validar conexión, sincronizar recursos, registrar fuente de datos, desactivar fuente de datos |
| `handlers.go` | Handlers HTTP públicos bajo `/api/v1/cloud-accounts/*` y `/api/v1/resource-bindings/*` |
| `repositories/account_repository.go` | `CloudAccountRepository` adaptador GORM contra `cloud.accounts` (cco_app) |
| `repositories/resource_repository.go` | `ResourceRepository` adaptador GORM contra `cloud.resources` (cco_app) |
| `repositories/binding_repository.go` | `BindingRepository` adaptador GORM contra `cloud.resource_bindings` (cco_app) |
| `adapters/fake.go` | Adaptador en memoria para dev/tests (implementa `ConnectionValidator` + `ResourceProvider`) |
| `adapters/azure.go` | Adaptador Azure (azidentity + ARM REST GETs solo lectura), activado con `CCO_CLOUD_PROVIDER_MODE=azure` |

## Responsabilidad

Administrar las conexiones con proveedores cloud y representar de forma normalizada cuentas, suscripciones, recursos y fuentes de datos que serán monitoreadas. Es un módulo de negocio que depende de Identidad y Acceso únicamente a través de `PermissionChecker` y del `IdentityContext` inyectado en `context.Context`.

Casos de uso cubiertos en Release 1.0:

- Registrar una cuenta cloud (estado `pending`).
- Validar la conexión de una cuenta (pasa a `active` o `error`, registra `last_connection_error` sin el secreto).
- Sincronizar recursos de una cuenta (upsert normalizado de bases de datos SQL de Azure por `azure_resource_id`).
- Registrar una fuente de datos (`resource_bindings` con `is_monitored=true`, referenciando `resource_uid`).
- Desactivar una fuente de datos (`is_monitored=false`).
- Listar cuentas y recursos; consultar cuenta por ID.

Entidades propias del modelo:

| Entidad | Descripción |
|---------|-------------|
| `CloudAccount` | Cuenta de nube (proveedor, suscripción, tenant, referencia de credencial, estado) |
| `CloudResource` | Recurso normalizado descubierto (UID de contrato, servidor, base de datos, región, tipo, ciclo de vida) |
| `DataSource` / binding | Asociación proyecto ↔ recurso cloud monitoreado (`cloud.resource_bindings`) |

No calcula recomendaciones, no recolecta métricas, no ejecuta inferencia ML, no genera reportes y no muta recursos de producción (solo GETs contra Azure).

## Interfaces

Todas las interfaces públicas viven en `internal/cloud/interfaces.go`. Los DTOs no contienen etiquetas de GORM (separación de infraestructura y dominio).

### `ConnectionValidator`

```go
type ConnectionCredentials struct {
    TenantID         string // resuelto de credential_reference ("env:VAR" o bare "VAR")
    ClientID         string
    ClientSecret     string // resuelto de variable de entorno; nunca se persiste ni se loguea
    SubscriptionID   string
}

type ConnectionResult struct {
    Valid       bool
    ErrorCode   string // vacío si es válido
    ErrorDetail string // sin el secreto
}

type ConnectionValidator interface {
    ValidateConnection(ctx context.Context, cred ConnectionCredentials) ConnectionResult
}
```

Implementaciones:
- `adapters.FakeValidator` — valida si `subscription_id` no está vacío (dev/tests).
- `adapters.AzureValidator` — `azidentity` + ARM REST GET (suscripción), solo si `CCO_CLOUD_PROVIDER_MODE=azure`.

### `ResourceProvider`

```go
type ResourceDTO struct {
    ResourceUID       uuid.UUID // identificador de contrato estable (UUID v4 generado en el backend)
    ProviderCode      string    // "azure" | "aws" | "gcp"
    SubscriptionID    string
    ResourceGroup     string
    ServerName        string
    DatabaseName      string
    AzureResourceID   string    // identidad natural (ARM id), UNIQUE en DB
    RegionCode        string
    Kind              string    // ej. "Microsoft.Sql/servers/databases"
    LifecycleStatus   string    // active | paused | deleted | archived
    RegisteredAt      time.Time
    LastSeenAt        *time.Time
    DeactivatedAt     *time.Time
}

type ResourceProvider interface {
    ListSQLDatabases(ctx context.Context, cred ConnectionCredentials) ([]ResourceDTO, error)
}
```

Implementaciones:
- `adapters.FakeProvider` — devuelve recursos de ejemplo estáticos (dev/tests).
- `adapters.AzureProvider` — ARM REST `GET .../databases` solo lectura, solo si `CCO_CLOUD_PROVIDER_MODE=azure`.

### Repositorios (composite, estilo IAM)

- `CloudAccountRepository` — `CreateAccount`, `FindAccountByID`, `FindAccountBySubscription`, `ListAccounts`, `UpdateAccountStatus`, `RecordConnectionError`
- `ResourceRepository` — `UpsertResources`, `FindResourceByUID`, `FindResourceByAzureID`, `ListResourcesByAccount`, `ListResources`
- `BindingRepository` — `CreateBinding`, `FindBindingByID`, `ListBindingsByProject`, `ListBindingsByResource`, `SetMonitored`, `DeleteBinding`

## Entradas

### HTTP (endpoints protegidos)

Todos requieren `Authorization: Bearer <access_token>` y `RequirePermission`.

| Endpoint | Método | Body | Permiso requerido | Descripción |
|----------|--------|------|-------------------|-------------|
| `/api/v1/cloud-accounts` | POST | `RegisterAccountRequest` | `cloud_accounts:write` | Registrar cuenta (estado `pending`) |
| `/api/v1/cloud-accounts` | GET | — | `cloud_accounts:read` | Listar cuentas |
| `/api/v1/cloud-accounts/{id}` | GET | — | `cloud_accounts:read` | Obtener cuenta por ID |
| `/api/v1/cloud-accounts/{id}/validate` | POST | — | `cloud_accounts:write` | Validar conexión (active/error) |
| `/api/v1/cloud-accounts/{id}/sync` | POST | — | `cloud_accounts:write` | Sincronizar recursos de Azure SQL |
| `/api/v1/cloud-accounts/{id}/resources` | GET | — | `resources:read` | Listar recursos de la cuenta |
| `/api/v1/resource-bindings` | POST | `RegisterBindingRequest` | `data_sources:write` | Registrar fuente de datos (binding) |
| `/api/v1/resource-bindings` | GET | — | `data_sources:read` | Listar fuentes de datos |
| `/api/v1/resource-bindings/{id}` | PATCH | `PatchBindingRequest` | `data_sources:write` | Desactivar fuente de datos (`is_monitored=false`) |

### Credentials (dominio)

`ConnectionCredentials` se define arriba en `interfaces.go`. `credential_reference` de `cloud.accounts` tiene formato `env:VAR` o bare `VAR`; el secreto se resuelve exclusivamente de variable de entorno (iteración 1). Key Vault / Managed Identity es upgrade (RNF-02).

## Salidas

### Respuestas HTTP

Todas las respuestas de error usan formato:
```json
{"code": "CÓDIGO_DEL_ERROR", "message": "descripción"}
```

### `ConnectionResult` (dominio)

Validación de conexión: `Valid` bool, `ErrorCode` y `ErrorDetail` (sin el secreto).

### `ResourceDTO` (dominio)

Recurso normalizado descubierto, con `ResourceUID` como contrato estable (UUID).

## Dependencias

### Internas (mismo módulo)

Todas las dependencias entre archivos de `internal/cloud/` son internas al paquete, excepto `platform/database` para los modelos GORM y `iam` para `PermissionChecker` / `IdentityContext`.

### Externas (go.mod)

| Paquete | Uso |
|---------|-----|
| `github.com/google/uuid` | Identificadores UUID |
| `gorm.io/gorm` + `gorm.io/driver/postgres` + `gorm.io/driver/sqlite` | Persistencia portable contra `cco_app` |
| `github.com/Azure/azure-sdk-for-go/sdk/azidentity` | Credenciales de Azure (solo adaptador azure, modo opcional) |
| `net/http` (estándar) | Handlers HTTP (sin frameworks web externos) |

No se añaden frameworks web (Chi, Gin, Echo). Se usa `net/http` + `ServeMux`.

### Estándar de Go

- `context`, `encoding/json`, `errors`, `fmt`, `net/http`, `strings`, `time`

## Datos

### Base de datos operativa `cco_app`

El módulo Cloud persiste en el schema `cloud` de `cco_app` (PostgreSQL) o tablas sin calificar (SQLite).

#### Tablas utilizadas

| Schema | Tabla | Entidad | Notas |
|--------|-------|---------|-------|
| `cloud` | `accounts` | CloudAccount | Existe; se añaden columnas `client_id`, `last_connection_error` |
| `cloud` | `resources` | CloudResource | **Nueva tabla** (DDL abajo) |
| `cloud` | `resource_bindings` | DataSource | Existe; `data_resource_uid` apunta a `cloud.resources.resource_uid` |
| `organization` | `organizations` | Organization | FK de `cloud.accounts.organization_id` |
| `iam` | `users` | User | `created_by` de `cloud.accounts` |

#### Nueva tabla `cloud.resources` (DDL sketch)

```sql
CREATE TABLE cloud.resources (
    resource_uid         uuid DEFAULT gen_random_uuid() NOT NULL,
    cloud_account_id     uuid NOT NULL,
    provider_code        character varying(30) NOT NULL,
    subscription_id      character varying(128) NOT NULL,
    resource_group       character varying(160),
    server_name          character varying(160),
    database_name        character varying(160),
    azure_resource_id    character varying(512) NOT NULL,
    region_code          character varying(80),
    kind                 character varying(120),
    lifecycle_status     character varying(30) NOT NULL DEFAULT 'active',
    registered_at        timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at         timestamp with time zone,
    deactivated_at       timestamp with time zone,
    CONSTRAINT resources_pkey PRIMARY KEY (resource_uid),
    CONSTRAINT resources_account_fkey FOREIGN KEY (cloud_account_id)
        REFERENCES cloud.accounts(cloud_account_id) ON DELETE CASCADE,
    CONSTRAINT uq_resources_azure_id UNIQUE (azure_resource_id),
    CONSTRAINT ck_resources_provider CHECK (provider_code = ANY (ARRAY['azure','aws','gcp'])),
    CONSTRAINT ck_resources_lifecycle CHECK (lifecycle_status = ANY (ARRAY['active','paused','deleted','archived']))
);
CREATE INDEX ix_resources_account ON cloud.resources (cloud_account_id);
CREATE INDEX ix_resources_azure_id ON cloud.resources (azure_resource_id);
```

#### Nuevas columnas nullable en `cloud.accounts`

```sql
ALTER TABLE cloud.accounts ADD COLUMN client_id character varying(128);
ALTER TABLE cloud.accounts ADD COLUMN last_connection_error text;
```

`client_id` se rellena en `validate` a partir de `cred.ClientID`. `last_connection_error` guarda el mensaje de error de validación (sin el secreto) y se limpia en validación exitosa.

#### Relación con `cco_data.core.resource`

La base `cco_data` (core.resource, telemetry, ML) pertenece a un SEPARADO módulo Python y está FUERA del alcance del backend Go (RNF-10, no FKs entre bases de datos). `cloud.resources.resource_uid` es el contrato UUID estable que el módulo Python almacena en `cco_data.core.resource` para hacer el mapeo 1:1 fila a fila. Ese mismo `resource_uid` es el que `cloud.resource_bindings.data_resource_uid` almacena, por lo que la integración `cco_app ↔ cco_data` ocurre vía servicios/contratos y UUIDs estables, nunca mediante claves foráneas entre bases de datos.

### Flujo de datos de sincronización

1. Cliente registra cuenta → `cloud.accounts` (estado `pending`).
2. Cliente valida conexión → resolver secreto de env → `ConnectionValidator.ValidateConnection` → `active` o `error` + `last_connection_error`.
3. Cliente sincroniza → `ResourceProvider.ListSQLDatabases` (ARM GETs) → upsert en `cloud.resources` por `azure_resource_id` → `registered_at`/`last_seen_at`.
4. Cliente registra fuente de datos → `cloud.resource_bindings` con `data_resource_uid = resource_uid` y `is_monitored=true`.
5. Cliente desactiva fuente → `is_monitored=false` (soft delete lógico de monitoreo).

## Algoritmos / lógica

### Registro de cuenta

Recibir `RegisterAccountRequest` (display_name, provider_code, subscription_id, tenant_id, credential_reference). Verificar unicoridad `(provider_code, subscription_id)`. Insertar en `cloud.accounts` con `status='pending'`. Devolver `cloud_account_id`.

### Validación de conexión

1. Leer cuenta por ID; si no existe → `ACCOUNT_NOT_FOUND`.
2. Resolver `credential_reference`: si empieza con `env:`, leer variable de entorno del resto; si no, usar el valor directo como nombre de variable y leer `os.Getenv`. Si falta → `CREDENTIALS_UNAVAILABLE`.
3. Construir `ConnectionCredentials` (secret de env; nunca en memoria de forma persistente).
4. Llamar `ConnectionValidator.ValidateConnection`. Si inválido → actualizar `status='error'`, `last_connection_error=<detail sin secreto>`, devolver `CONNECTION_FAILED`. Si válido → `status='active'`, limpiar `last_connection_error`, guardar `client_id`, devolver éxito.

### Sincronización de recursos

1. Leer cuenta; verificar `status='active'`; si no → `ACCOUNT_NOT_FOUND` o `CONNECTION_FAILED`.
2. Llamar `ResourceProvider.ListSQLDatabases` (Azure SQL solo, Release 1.0).
3. Para cada recurso: upsert en `cloud.resources` por `azure_resource_id` (UPDATE si existe, INSERT si no) normalizando `provider_code`, `subscription_id`, `resource_group`, `server_name`, `database_name`, `azure_resource_id`, `region_code`, `kind`, `lifecycle_status='active'`, `last_seen_at=now()`.
4. Devolver lista de `resource_uid` sincronizados.

### Registro de fuente de datos

Recibir `RegisterBindingRequest` (project_id, cloud_account_id, data_resource_uid, display_name?). Verificar que `cloud_account_id` existe y que `data_resource_uid` existe en `cloud.resources`. Insertar `cloud.resource_bindings` con `is_monitored=true`. Unicoridad `(project_id, data_resource_uid)` y `(cloud_account_id, data_resource_uid)` → `DATA_SOURCE_EXISTS` si viola.

### Desactivación de fuente de datos

PATCH `/api/v1/resource-bindings/{id}` con `is_monitored=false`. Si no existe → `DATA_SOURCE_NOT_FOUND`.

## Errores y excepciones

### Errores de dominio (`internal/cloud/errors.go`)

Todos implementan `error` y se mapean a códigos de API mediante `MapErrorToCode`.

| Error | Código HTTP | Código API |
|-------|-------------|------------|
| `ErrAccountNotFound` | 404 | `ACCOUNT_NOT_FOUND` |
| `ErrAccountExists` | 409 | `ACCOUNT_EXISTS` |
| `ErrResourceNotFound` | 404 | `RESOURCE_NOT_FOUND` |
| `ErrDataSourceNotFound` | 404 | `DATA_SOURCE_NOT_FOUND` |
| `ErrDataSourceExists` | 409 | `DATA_SOURCE_EXISTS` |
| `ErrInvalidRequest` | 400 | `INVALID_REQUEST` |
| `ErrCredentialsUnavailable` | 400 | `CREDENTIALS_UNAVAILABLE` |
| `ErrConnectionFailed` | 422 | `CONNECTION_FAILED` |
| `ErrProviderUnavailable` | 503 | `PROVIDER_UNAVAILABLE` |
| `ErrUnauthorized` (IAM) | 401 | `UNAUTHORIZED` |
| `ErrForbidden` (IAM) | 403 | `FORBIDDEN` |

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

### Autenticación y autorización

- Todos los endpoints del módulo están detrás del middleware de autenticación IAM (`AuthMiddleware`) + `RequirePermission(recurso, acción)`.
- Permisos: `cloud_accounts:write/read`, `resources:read`, `data_sources:write/read`.
- RBAC de menor privilegio: el rol `viewer` solo tiene `resources:read`; `db_admin`/`platform_admin` tienen `cloud_accounts:*` y `resources:*`.

### Leer solamente contra Azure (doc §7.2 Proyecto_De_Ingenieria.md)

- El módulo 2 realiza solo GETs a Azure Resource Manager. Nunca muta bases de datos productivas ni ejecuta cambios de infraestructura.

### Secretos

- Ningún secreto se almacena en texto plano en la base de datos ni se registra en logs (RNF-02, RNF-15, CU-01).
- `credential_reference` apunta a variable de entorno (`env:VAR` o bare `VAR`). Key Vault / Managed Identity = upgrade futuro.
- `last_connection_error` almacena solo el detalle de error sin el secreto.
- Los DTOs y respuestas nunca devuelven `ClientSecret`.

### Aislamiento por organización (IDOR)

Toda operación que recibe un ID (cuenta, recurso, fuente de datos) se filtra por organización en la capa de servicio como defensa en profundidad. El servicio carga la cuenta o el binding vinculado y compara su `organization_id` con el `OrgID` del `IdentityContext` del llamante; ante desacuerdo devuelve el error de "no encontrado" (HTTP 404), nunca un 403, para no revelar que el recurso existe en otra organización. Se aplica a `GetAccount`, `ValidateAccount`, `SyncResources`, `ListResources`, `RegisterDataSource`, `GetBinding`/`SetDataSourceMonitored` y `ListBindings`, y el `project_id` de `RegisterDataSource`/`ListBindings` debe pertenecer a la misma organización (`PROJECT_NOT_FOUND` → 404). Este control es independiente de RLS de Postgres (que solo cubre `cloud.accounts` en prod): SQLite en dev no tiene RLS y `cloud.resources` no tiene política RLS, por lo que la capa de servicio es la única defensa en ambos entornos.

### Headers de trazabilidad (RNF-13)

- `X-Request-ID`, `X-Correlation-ID`, `X-Org-ID`, `X-Project-ID` propagados como en IAM.

## Rendimiento

| Operación | Complejidad | Notas |
|-----------|-------------|-------|
| `ValidateConnection` | O(1) llamada ARM | Una petición HTTP a Azure |
| `ListSQLDatabases` | O(n) recursos | n = SQL DBs en la suscripción |
| Upsert recursos | O(n) escrituras | Por `azure_resource_id` |
| Consulta cuenta/recurso | O(1) índice | PK o índice btree |

No hay consultas a base de datos en el path caliente más allá de la resolución de la cuenta y permisos.

## Decisiones relacionadas

| ID | Decisión | Documento |
|----|----------|-----------|
| ADR-002 | Separación de bases de datos `cco_app` y `cco_data` | `docs/04-architecture/decisions/ADR-002-two-postgresql-databases.md` |
| ADR-003 | Persistencia GORM portable para `cco_app` | `docs/04-architecture/decisions/ADR-003-gorm-portable-persistence.md` |
| DEC-027 | Autenticación y autorización dentro del monolito Go | `docs/registry/DECISION-LOG.md` |
| RNF-10 | No FKs entre `cco_app` y `cco_data` | `Proyecto_De_Ingenieria.md` §10.2 |
| CU-01 | Registrar fuente Azure sin exponer secretos | `Proyecto_De_Ingenieria.md` §9 |
| RF-01 / RF-02 | Registrar recursos Azure SQL / conexión segura APIs Azure | `Proyecto_De_Ingenieria.md` §10.1 |
| RNF-02 | Credenciales Azure Key Vault / Managed Identity | `Proyecto_De_Ingenieria.md` §10.2 |
| RNF-09 | Extensión futura a otros proveedores cloud | `Proyecto_De_Ingenieria.md` §10.2 |
| §7.2 | No cambios automáticos sobre bases productivas | `Proyecto_De_Ingenieria.md` §7.2 |

## Pruebas relacionadas

| ID | Prueba | Archivo | Estado |
|----|--------|---------|--------|
| TEST-CLOUD-001 | `ConnectionValidator` fake y map de errores | `backend/internal/cloud/errors_test.go` | Pendiente |
| TEST-CLOUD-002 | Servicio: registro → validate → sync → binding → desactivar | `backend/internal/cloud/service_test.go` | Pendiente |
| TEST-CLOUD-003 | Handlers: endpoints y códigos de permiso | `backend/internal/cloud/handlers_test.go` | Pendiente |
| TEST-CLOUD-004 | Integración cloud sobre SQLite | `backend/test/cloud_integration_test.go` | Pendiente |
| — | Ejecución: `go test ./... -count=1` desde `backend/` | — | — |

## Limitaciones conocidas y caminos de upgrade

| Limitación | Código | Techo | Camino de upgrade |
|------------|--------|-------|-------------------|
| Secretos solo por env var | `service.go` | Una variable por cuenta; no rotación automática | Azure Key Vault / Managed Identity (RNF-02) |
| Adaptador fake por defecto | `adapters/fake.go` | Datos estáticos, sin Azure real | `CCO_CLOUD_PROVIDER_MODE=azure` + `adapters/azure.go` |
| Sync bajo demanda | `service.go` | Sin programador | Módulo 3 (Adquisición) posee el scheduler |
| Solo Azure SQL Database | `adapters/azure.go` | `Microsoft.Sql/servers/databases` | Más tipos de recurso (RNF-09) |
| `cloud.resources` en `cco_app` ↔ `cco_data.core.resource` | `repositories/` | Mapeo 1:1 vía `resource_uid`, hecho por el módulo Python | Contrato versionado Go–Python (RNF-12) |
| `last_connection_error` en texto | `cloud.accounts` | Guarda detalle de error sin secreto | Estructurar códigos de error enumerados |

`shortcut: env var secrets → Key Vault/Managed Identity; fake adapter default → azure mode; on-demand sync → scheduler (module 3); solo Azure SQL → más tipos después; cloud.resources en cco_app ↔ core.resource sync por el módulo Python, no el backend Go.`

## Notas de implementación

- El módulo no depende de frameworks web externos; usa `net/http` de la biblioteca estándar.
- Los modelos GORM viven en `internal/platform/database/models.go`; los DTOs de Cloud no contienen etiquetas de GORM.
- Los repositorios usan `database.TableName(db, schema, table)` para calificar schemas en PostgreSQL y dejarlos sin calificar en SQLite (patrón de IAM).
- `IdentityContext` se inyecta en `context.Context` y viaja a través de los handlers para trazabilidad y autorización.
- El adaptador Azure solo se compila/activa cuando `CCO_CLOUD_PROVIDER_MODE=azure`; en dev predomina `fake.go`.

## Observabilidad y logging

El módulo Cloud emite logs estructurados a `stdout` mediante `log/slog` (estándar de Go 1.22), configurado una sola vez en `cmd/cco-app/main.go`.

| Aspecto | Descripción |
|---------|-------------|
| **Salida** | `stdout` (hacia colectores externos: Loki, ELK, Fluentd, CloudWatch) |
| **Formato** | Texto por defecto; JSON si `CCO_LOG_FORMAT=json` |
| **Niveles** | `Info` (éxitos), `Warn` (fallos de conexión/sincronización) |
| **Campos obligatorios** | `request_id`, `correlation_id`, `user_id`, `cloud_account_id`, `path`, `method` |

### Eventos registrados

| Evento | Contexto | Datos clave |
|--------|----------|-------------|
| `cloud account registered` | POST /api/v1/cloud-accounts | `cloud_account_id`, `provider_code`, `subscription_id`, `status` |
| `cloud account validated` | POST /.../validate | `cloud_account_id`, `valid`, `error_code` (sin secreto) |
| `cloud resources synced` | POST /.../sync | `cloud_account_id`, `resources_synced`, `azure_resource_id`s |
| `data source registered` | POST /api/v1/resource-bindings | `resource_binding_id`, `data_resource_uid`, `is_monitored` |
| `data source deactivated` | PATCH /.../{id} | `resource_binding_id`, `is_monitored=false` |
| `cloud connection failed` | Fallos de validación/sync | `cloud_account_id`, `path`, `error_code`, opcionalmente `user_id` |

Nunca se loguea `ClientSecret` ni `credential_reference` con el valor del secreto.

## Véase también

- `docs/05-design/DESIGN-IAM-001.md` — diseño del módulo IAM (estructura de referencia).
- `backend/API.md` — sección de endpoints del módulo Cloud (append).
- `MODULES.md` — sección "2. Gestión de Nube y Fuentes de Datos".
- `Proyecto_De_Ingenieria.md` — §7 (alcance), §9 (CU-01), §10 (RF-01, RF-02, RNF-02, RNF-09, RNF-10).
- `backend/internal/platform/database/models.go` — `CloudAccountModel`, `ResourceBindingModel`.
- `databases/cco_app/cco_app.sql` — esquema `cloud` (`accounts`, `resource_bindings`).
