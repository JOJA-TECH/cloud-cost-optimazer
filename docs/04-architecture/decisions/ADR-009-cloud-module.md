# ADR-009 — Módulo Cloud: Gestión de Nube y Fuentes de Datos (Module 2)

- **Estado:** Aceptado e implementado
- **Fecha:** 2026-11-12
- **Decisores:** Equipo del proyecto
- **Relacionado con:** ADR-001, ADR-003, ADR-004, MODULES.md (módulo 2)

## Contexto

El módulo Gestión de Nube y Fuentes de Datos es el segundo módulo del backend (orden sugerido en MODULES.md). Debe administrar conexiones con proveedores cloud y representar de forma normalizada cuentas, suscripciones, recursos y fuentes de datos que serán monitoreadas por el módulo Adquisición.

## Problema

Definir el diseño del módulo Cloud en Report 1.0:

1. ¿Cómo se registran y validan cuentas cloud sin exponer secretos?
2. ¿Cómo se descubren y normalizan recursos cloud (bases de datos Azure SQL)?
3. ¿Cómo se asocian proyectos con recursos cloud (fuentes de datos/bindings)?

## Alternativas consideradas

### Alternativa A: SDK de Azure llamado directamente desde el dominio

**Desventajas:** Acopla el dominio a Azure; imposible de probar sin credenciales reales; viola "dependencias dirigidas" de MODULES.md.

### Alternativa B: Adaptadores en los bordes + interfaces en el dominio (seleccionada)

**Ventajas**
- El dominio nunca importa SDK de Azure; usa `ConnectionValidator` y `ResourceProvider`.
- Adaptador `FakeAdapter` para desarrollo y pruebas sin credenciales.
- Adaptador `AzureAdapter` (ARM) activado solo con `CCO_CLOUD_PROVIDER_MODE=azure`.
- Las credenciales se resuelven desde variables de entorno vía `credential_reference` (`env:VAR` o `VAR`); nunca se persisten.

## Decisión

Se decide **Alternativa B**:

1. **Entidades:** `CloudAccount`, `CloudResource`, `ResourceBinding` (fuente de datos).
2. **Interfaces:** `ConnectionValidator.ValidateConnection`, `ResourceProvider.ListSQLDatabases`, repositorios `AccountRepository`, `ResourceRepository`, `DataSourceRepository`.
3. **Endpoints (todos protegidos con `Authorization: Bearer` + permiso):**
   - `POST /api/v1/cloud-accounts` — registrar cuenta (estado `pending`), permiso `cloud_accounts:write`.
   - `GET /api/v1/cloud-accounts` — listar cuentas, permiso `cloud_accounts:read`.
   - `GET /api/v1/cloud-accounts/{id}` — obtener cuenta, permiso `cloud_accounts:read`.
   - `POST /api/v1/cloud-accounts/{id}/validate` — validar conexión, permiso `cloud_accounts:write`.
   - `POST /api/v1/cloud-accounts/{id}/sync` — sincronizar recursos Azure, permiso `cloud_accounts:write`.
   - `GET /api/v1/cloud-accounts/{id}/resources` — listar recursos, permiso `resources:read`.
   - `POST /api/v1/resource-bindings` — registrar fuente de datos, permiso `data_sources:write`.
   - `GET /api/v1/resource-bindings` — listar fuentes de datos, permiso `data_sources:read`.
   - `PATCH /api/v1/resource-bindings/{id}` — activar/desactivar monitoreo, permiso `data_sources:write`.
4. **Persistencia:** tablas `cloud.accounts`, `cloud.resources`, `cloud.resource_bindings` en `cco_app` (GORM, esquemas cualificados en PostgreSQL).
5. **Release 1.0:** solo proveedor `azure`; bases de datos SQL de Azure.

## Justificación

Los adaptadores en los bordes mantienen el dominio independiente del proveedor cloud, permiten pruebas con adaptadores falsos y cumplen el principio de dependencias dirigidas de MODULES.md. La referencia de credenciales (`env:VAR`) evita persistir secretos.

## Consecuencias

### Positivas

- Dominio desacoplado de Azure; fácil de probar con `FakeAdapter`.
- Credenciales nunca persistidas; se resuelven de variables de entorno en tiempo de validación.
- Endpoints y permisos explícitos por operación.

### Negativas

- Release 1.0 limitado a Azure SQL; otros proveedores requieren nuevos adaptadores.
- La sincronización de recursos es on-demand (endpoint `/sync`); no hay descubrimiento automático continuo.

### Riesgos

- El adaptador Azure real depende de credenciales válidas en el entorno; sin ellas, `validate` y `sync` fallan con `CONNECTION_FAILED` / `PROVIDER_UNAVAILABLE`.

## Evidencia

- `backend/internal/cloud/` — handlers, service, interfaces, adapters, repositories.
- `backend/cmd/cco-app/main.go` — wiring del módulo cloud.
- `API.md` §12 — referencia de endpoints cloud.
- `docs/05-design/DESIGN-AMDA-001.md` — diseño técnico del módulo cloud/adquisición.

## Revisión

- **Fecha prevista de revisión:** Cuando se agreguen proveedores adicionales (AWS, GCP) o descubrimiento automático de recursos.

## Referencias

[1]: docs/MODULES.md — Módulo 2, Gestión de Nube y Fuentes de Datos.
