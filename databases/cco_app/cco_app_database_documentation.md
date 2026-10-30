# Documentación de la base de datos `cco_app`

## 1. Descripción general

`cco_app` es una base de datos PostgreSQL 14+ para una aplicación
denominada **Cloud Cost Optimizer**. Su objetivo es almacenar la
información necesaria para la gestión de usuarios, organizaciones,
proyectos, cuentas de servicios cloud, estado de la aplicación y
auditoría.

La base de datos está diseñada para trabajar con autenticación externa
mediante un proveedor de identidad **OIDC/OAuth2**. Por lo tanto, la
base de datos no se encarga directamente de almacenar contraseñas.

El script utiliza las extensiones `pgcrypto` y `citext`, crea cinco
esquemas y aplica mecanismos de seguridad mediante **Row-Level Security
(RLS)**.

## 2. Esquemas

La base de datos se divide en cinco esquemas:

  Esquema          Propósito
  ---------------- -------------------------------------------------------
  `iam`            Identidad, usuarios, roles y permisos
  `organization`   Organizaciones, miembros y proyectos
  `cloud`          Metadatos de cuentas cloud y asociación con proyectos
  `app`            Preferencias y estado funcional de la aplicación
  `audit`          Registro de eventos de auditoría

El script revoca los permisos sobre estos esquemas al usuario `PUBLIC`.

------------------------------------------------------------------------

# 3. Esquema `iam`

El esquema `iam` (Identity and Access Management) administra los
usuarios, sus identidades externas, roles y permisos.

## 3.1 `iam.users`

Almacena los usuarios de la aplicación.

Campos principales:

-   `user_id`: identificador UUID y clave primaria.
-   `email`: correo electrónico del usuario.
-   `display_name`: nombre mostrado en la aplicación.
-   `status`: estado del usuario: `pending`, `active`, `suspended` o
    `deleted`.
-   `email_verified`: indica si el correo fue verificado.
-   `last_login_at`: fecha y hora del último inicio de sesión.
-   `created_at`: fecha de creación.
-   `updated_at`: fecha de última actualización.

El correo es único y se utiliza el tipo `CITEXT`, que permite manejar el
texto sin distinguir mayúsculas y minúsculas.

## 3.2 `iam.external_identities`

Relaciona un usuario con una identidad proporcionada por un proveedor
externo de autenticación.

Campos importantes:

-   `external_identity_id`: clave primaria.
-   `user_id`: referencia a `iam.users`.
-   `provider`: proveedor de identidad.
-   `subject`: identificador del usuario dentro del proveedor.
-   `last_seen_at`: última vez que se observó esa identidad.

La combinación `(provider, subject)` es única.

## 3.3 `iam.roles`

Define los roles disponibles en el sistema.

Cada rol tiene:

-   `role_id`
-   `role_code`
-   `description`
-   `is_system_role`
-   `created_at`

El script inicializa cuatro roles:

-   `platform_admin`
-   `organization_admin`
-   `analyst`
-   `viewer`

## 3.4 `iam.permissions`

Contiene los permisos disponibles para las diferentes operaciones de la
aplicación.

Algunos ejemplos son:

-   `organization:read`
-   `organization:manage`
-   `project:read`
-   `project:manage`
-   `cloud_account:read`
-   `cloud_account:manage`
-   `resource:read`
-   `resource:manage`
-   `forecast:read`
-   `recommendation:read`
-   `recommendation:review`
-   `report:read`
-   `report:create`
-   `notification:read`
-   `notification:manage`
-   `audit:read`

## 3.5 `iam.role_permissions`

Es una tabla intermedia entre roles y permisos.

Permite establecer una relación muchos-a-muchos:

**Rol → muchos permisos**

y

**Permiso → puede pertenecer a muchos roles.**

Su clave primaria está formada por:

`(role_id, permission_id)`.

## 3.6 `iam.user_roles`

Relaciona usuarios con roles.

Un usuario puede tener varios roles y un rol puede estar asignado a
varios usuarios.

También registra:

-   `assigned_at`: cuándo se asignó.
-   `assigned_by`: usuario que realizó la asignación.

------------------------------------------------------------------------

# 4. Esquema `organization`

Este esquema representa la estructura multi-organización de la
aplicación.

## 4.1 `organization.organizations`

Almacena las organizaciones o tenants.

Campos principales:

-   `organization_id`: identificador UUID.
-   `name`: nombre.
-   `slug`: identificador textual único.
-   `status`: `active`, `suspended` o `deleted`.
-   `created_at`
-   `updated_at`

El `slug` debe tener un formato compuesto por letras minúsculas, números
y guiones.

## 4.2 `organization.members`

Relaciona usuarios con organizaciones.

Una organización puede tener muchos usuarios y un usuario puede
pertenecer a varias organizaciones.

Los roles de membresía disponibles son:

-   `owner`
-   `admin`
-   `member`
-   `viewer`

Los estados de la membresía son:

-   `invited`
-   `active`
-   `suspended`
-   `removed`

La clave primaria es `(organization_id, user_id)`.

## 4.3 `organization.projects`

Representa los proyectos pertenecientes a una organización.

Cada proyecto contiene:

-   `project_id`
-   `organization_id`
-   `name`
-   `description`
-   `status`
-   `created_by`
-   `created_at`
-   `updated_at`

Los estados permitidos son:

-   `active`
-   `archived`
-   `deleted`

No puede existir el mismo nombre de proyecto dos veces dentro de una
misma organización, debido a la restricción:

`UNIQUE (organization_id, name)`.

------------------------------------------------------------------------

# 5. Esquema `cloud`

Este esquema contiene los metadatos relacionados con las cuentas de
proveedores cloud.

## 5.1 `cloud.accounts`

Representa una cuenta cloud conectada a una organización.

Los proveedores permitidos son:

-   `azure`
-   `aws`
-   `gcp`

Campos principales:

-   `cloud_account_id`
-   `organization_id`
-   `provider_code`
-   `display_name`
-   `subscription_id`
-   `tenant_id`
-   `credential_reference`
-   `status`
-   `created_by`
-   `created_at`
-   `updated_at`

Los estados posibles son:

-   `pending`
-   `active`
-   `error`
-   `disabled`
-   `deleted`

### Seguridad de credenciales

La base de datos **no almacena directamente secretos o tokens**. El
campo `credential_reference` funciona como referencia a un gestor
externo de secretos.

La combinación `(provider_code, subscription_id)` es única.

## 5.2 `cloud.account_projects`

Tabla intermedia que permite asociar cuentas cloud con proyectos.

Relaciona:

`cloud_account_id ↔ project_id`

La clave primaria es `(cloud_account_id, project_id)`.

## 5.3 `cloud.resource_bindings`

Relaciona un proyecto y una cuenta cloud con un recurso almacenado en
otra fuente de datos.

El campo `data_resource_uid` es un UUID que identifica el recurso
externo.

El script especifica que no existe una clave foránea entre bases de
datos para este recurso.

Campos importantes:

-   `resource_binding_id`
-   `project_id`
-   `cloud_account_id`
-   `data_resource_uid`
-   `display_name`
-   `is_monitored`
-   `created_at`
-   `updated_at`

Existen restricciones de unicidad para evitar duplicaciones de recursos
dentro del proyecto o de la cuenta cloud.

------------------------------------------------------------------------

# 6. Esquema `app`

Contiene información relacionada con el estado y las preferencias
funcionales de la aplicación.

## 6.1 `app.user_preferences`

Almacena las preferencias generales de cada usuario.

Incluye:

-   zona horaria (`timezone`)
-   idioma/localización (`locale`)
-   moneda (`currency`)
-   tema visual (`theme`)
-   configuración de notificaciones (`notification_settings`)

La configuración de notificaciones se almacena como `JSONB`.

Los temas permitidos son:

-   `system`
-   `light`
-   `dark`

La moneda debe tener el formato de tres letras mayúsculas.

## 6.2 `app.dashboard_preferences`

Almacena la configuración del dashboard.

Puede existir una configuración:

-   global para el usuario, cuando `project_id` es `NULL`;
-   específica para un proyecto.

Los campos `layout` y `widgets` utilizan `JSONB`.

Los períodos predeterminados disponibles son:

-   `24h`
-   `7d`
-   `30d`
-   `90d`
-   `1y`

El script utiliza índices únicos parciales para garantizar que un
usuario tenga una única configuración global y una única configuración
por proyecto.

## 6.3 `app.notifications`

Almacena las notificaciones dirigidas a los usuarios.

Incluye:

-   título
-   mensaje
-   tipo
-   severidad
-   referencia opcional a otra entidad
-   fecha de lectura
-   fecha de creación
-   fecha de expiración

Las severidades permitidas son:

-   `info`
-   `success`
-   `warning`
-   `critical`

## 6.4 `app.notification_deliveries`

Registra el proceso de entrega de una notificación mediante diferentes
canales.

Canales disponibles:

-   `in_app`
-   `email`
-   `webhook`

Estados:

-   `pending`
-   `sent`
-   `delivered`
-   `failed`
-   `cancelled`

También registra los intentos realizados, la fecha del último intento y
posibles errores.

## 6.5 `app.saved_views`

Permite que los usuarios guarden vistas o filtros personalizados.

Almacena filtros de:

-   recursos (`resource_filters`)
-   métricas (`metric_filters`)

Ambos campos son `JSONB`.

Una vista puede ser global o pertenecer a un proyecto.

El nombre de la vista es único para cada usuario dentro del mismo
alcance.

## 6.6 `app.report_requests`

Representa solicitudes de generación de informes.

Cada solicitud contiene:

-   proyecto
-   usuario que solicita el informe
-   tipo de informe
-   parámetros
-   estado
-   fechas de inicio y finalización
-   mensaje de error, si existe

Los estados son:

-   `queued`
-   `running`
-   `completed`
-   `failed`
-   `cancelled`

## 6.7 `app.report_artifacts`

Almacena la información de los archivos generados por las solicitudes de
informes.

No almacena necesariamente el archivo dentro de la tabla; contiene
principalmente información para localizarlo mediante `storage_uri`.

También registra:

-   tipo MIME
-   tamaño
-   checksum SHA-256
-   fecha de creación
-   fecha de expiración

## 6.8 `app.recommendation_interactions`

Registra las acciones de los usuarios sobre recomendaciones de
optimización.

Las acciones permitidas son:

-   `viewed`
-   `accepted`
-   `rejected`
-   `deferred`
-   `dismissed`
-   `restored`

Esto permite conservar el historial de interacción con las
recomendaciones.

------------------------------------------------------------------------

# 7. Esquema `audit`

## 7.1 `audit.events`

Registra eventos de auditoría de la aplicación.

Entre los datos almacenados están:

-   organización relacionada
-   usuario
-   acción realizada
-   tipo de entidad
-   identificador de entidad
-   fecha y hora
-   `request_id`
-   dirección IP
-   agente de usuario
-   metadatos adicionales

Los metadatos se almacenan como `JSONB`.

Esta tabla permite consultar las acciones realizadas dentro de la
aplicación y asociarlas con usuarios, organizaciones, entidades o
solicitudes.

------------------------------------------------------------------------

# 8. Relaciones principales

La estructura general puede resumirse de la siguiente forma:

``` text
iam.users
   │
   ├── iam.external_identities
   ├── iam.user_roles ── iam.roles ── iam.role_permissions ── iam.permissions
   │
   ├── organization.members ── organization.organizations
   │                              │
   │                              └── organization.projects
   │                                       │
   │                                       ├── cloud.resource_bindings
   │                                       ├── app.report_requests
   │                                       ├── app.saved_views
   │                                       └── app.recommendation_interactions
   │
   ├── app.user_preferences
   ├── app.dashboard_preferences
   └── app.notifications

organization.organizations
   │
   └── cloud.accounts
          │
          └── cloud.account_projects ── organization.projects

app.report_requests
   │
   └── app.report_artifacts

audit.events
```

Una relación importante es:

**Usuario → Organización → Proyecto → Cuenta Cloud → Recursos**

Esto permite organizar los datos por organización y proyecto.

------------------------------------------------------------------------

# 9. Control de acceso

La base utiliza dos mecanismos principales:

1.  **Roles y permisos**
2.  **Row-Level Security (RLS)**

Los roles determinan qué operaciones puede realizar un usuario.

Los permisos representan acciones concretas, mientras que las tablas
`role_permissions` y `user_roles` conectan usuarios, roles y permisos.

## Roles iniciales

### `platform_admin`

Tiene todos los permisos definidos en la base de datos.

### `organization_admin`

Tiene permisos administrativos sobre organizaciones, proyectos, cuentas
cloud, recursos, informes, recomendaciones, notificaciones y auditoría.

### `analyst`

Tiene permisos orientados al análisis, consulta de datos,
recomendaciones e informes.

### `viewer`

Tiene permisos principalmente de lectura.

------------------------------------------------------------------------

# 10. Row-Level Security (RLS)

La base habilita RLS en las tablas que contienen información dependiente
del usuario, organización o proyecto.

Entre ellas:

-   `organization.organizations`
-   `organization.members`
-   `organization.projects`
-   `cloud.accounts`
-   `cloud.account_projects`
-   `cloud.resource_bindings`
-   `app.user_preferences`
-   `app.dashboard_preferences`
-   `app.notifications`
-   `app.saved_views`
-   `app.report_requests`
-   `app.report_artifacts`
-   `app.recommendation_interactions`
-   `audit.events`

El objetivo es impedir que un usuario pueda consultar o modificar filas
pertenecientes a organizaciones o proyectos a los que no tiene acceso.

------------------------------------------------------------------------

# 11. Funciones de seguridad

## `app.current_user_id()`

Obtiene el UUID del usuario actual a partir de la configuración de
sesión:

``` text
app.user_id
```

El backend debe establecer este valor mediante `SET LOCAL` para cada
solicitud.

El script especifica que, cuando existe un pool de conexiones, debe
utilizarse `SET LOCAL` y no una configuración permanente de sesión.

## `app.is_platform_admin()`

Comprueba si el usuario actual posee el rol `platform_admin`.

## `app.is_org_member()`

Comprueba si el usuario actual es miembro activo de una organización.

## `app.is_project_member()`

Comprueba si el usuario actual pertenece a la organización propietaria
del proyecto.

Estas funciones son utilizadas por las políticas RLS.

------------------------------------------------------------------------

# 12. Triggers y actualización de fechas

La función:

``` sql
app.set_updated_at()
```

actualiza automáticamente el campo `updated_at` al modificar una fila.

Se utiliza mediante triggers en:

-   `iam.users`
-   `organization.organizations`
-   `organization.projects`
-   `cloud.accounts`
-   `cloud.resource_bindings`
-   `app.user_preferences`
-   `app.dashboard_preferences`
-   `app.saved_views`

Esto evita depender de que la aplicación actualice manualmente estos
campos.

------------------------------------------------------------------------

# 13. Índices

La base incorpora índices para mejorar las consultas más frecuentes.

Algunos ejemplos:

-   búsqueda de usuarios por estado y fecha;
-   miembros por usuario y organización;
-   proyectos por organización y estado;
-   cuentas cloud por organización y estado;
-   notificaciones no leídas;
-   solicitudes de informes por proyecto;
-   eventos de auditoría por organización, usuario, entidad o acción;
-   recomendaciones por usuario, proyecto o recomendación.

También se utilizan **índices únicos parciales** para manejar casos
donde una relación puede ser global o específica de un proyecto.

------------------------------------------------------------------------

# 14. Restricciones e integridad

El diseño utiliza varias restricciones para mantener la integridad de
los datos:

-   `PRIMARY KEY` para identificar registros.
-   `FOREIGN KEY` para establecer relaciones.
-   `UNIQUE` para evitar duplicados.
-   `CHECK` para limitar valores válidos.
-   `NOT NULL` para campos obligatorios.
-   `ON DELETE CASCADE` para eliminar dependencias relacionadas cuando
    corresponde.
-   `ON DELETE SET NULL` cuando se desea conservar el registro
    dependiente.
-   `ON DELETE RESTRICT` para impedir eliminaciones que romperían
    determinadas relaciones.

También se generan UUID automáticamente mediante `gen_random_uuid()`.

------------------------------------------------------------------------

# 15. Datos iniciales

El script incluye datos iniciales para los roles y permisos.

Los roles creados son:

``` text
platform_admin
organization_admin
analyst
viewer
```

Posteriormente se asignan los permisos correspondientes a cada rol.

El rol `platform_admin` recibe todos los permisos existentes.

Los demás roles reciben subconjuntos de permisos de acuerdo con su
función.

------------------------------------------------------------------------

# 16. Roles de PostgreSQL

Al final del script aparece un bloque separado para ser ejecutado por un
DBA o administrador.

Se proponen los siguientes roles de PostgreSQL:

-   `cco_app_owner`
-   `cco_app_backend`
-   `cco_app_migrator`
-   `cco_app_readonly`

El script aclara que estos roles son propios del clúster de PostgreSQL y
deben manejarse separadamente.

También se proponen permisos diferentes para:

-   el backend de la aplicación;
-   migraciones;
-   acceso de solo lectura.

Esto separa los permisos administrativos de los permisos utilizados por
la aplicación.

------------------------------------------------------------------------

# 17. Flujo general de la base de datos

Un flujo típico puede representarse así:

``` text
1. Usuario
   ↓
2. Autenticación externa OIDC/OAuth2
   ↓
3. iam.users
   ↓
4. Organización
   ↓
5. organization.members
   ↓
6. Proyecto
   ↓
7. Cuenta cloud
   ↓
8. Recursos asociados
   ↓
9. Análisis / recomendaciones / informes
   ↓
10. Notificaciones y auditoría
```

La autenticación se realiza externamente, mientras que `cco_app`
administra la identidad asociada, autorización, organizaciones,
proyectos y estado de la aplicación.

------------------------------------------------------------------------

# 18. Resumen

La base de datos `cco_app` está organizada alrededor de cinco áreas:

-   **IAM:** administra usuarios, identidades, roles y permisos.
-   **Organization:** administra organizaciones, miembros y proyectos.
-   **Cloud:** administra las cuentas cloud y sus asociaciones con
    proyectos y recursos.
-   **App:** administra preferencias, dashboards, notificaciones,
    vistas, informes y recomendaciones.
-   **Audit:** registra las actividades realizadas en el sistema.

Su diseño utiliza PostgreSQL, UUID, JSONB, claves foráneas, índices,
restricciones `CHECK`, triggers y Row-Level Security. Además, separa la
autenticación externa de la gestión de autorización y evita almacenar
directamente credenciales o tokens de proveedores cloud.
