# ADR-004 — Arquitectura de Autenticación y Autorización del módulo Identidad y Acceso

- **Estado:** Aceptada para la fase inicial
- **Fecha:** 2026-10-29
- **Decisores:** Equipo del proyecto
- **Relacionado con:** REQ-002, REQ-005, DEC-005, DEC-011, DEC-018, ADR-003

## Contexto

El módulo Identidad y Acceso es el primer módulo de implementación del backend monolito modular en Go (Alternative A del Proyecto de Ingeniería I). Debe gestionar usuarios, roles, permisos, sesiones/tokens y autorización para el resto de módulos, sin exponer secretos en repositorio ni logs (RNF-02, RNF-15).

DEC-005 acordó "Backend como API Gateway en Go" y DEC-011 acordó "Autenticación con proveedor externo (proveedor a definir)". Ninguno de los dos define si el punto de entrada de autenticación debe ser un servicio gateway separado o un middleware dentro del monolito Go.

## Problema

Definir cómo expone el backend la autenticación y autorización en Report 1.0:

1. ¿Un servicio gateway separado frente al monolito Go?
2. ¿Qué proveedores de identidad se soportan en Report 1.0?
3. ¿Dónde residen los contratos de autorización que consumen los demás módulos?

## Alternativas consideradas

### Alternativa A: Gateway de autenticación como servicio separado (microservicio)

**Ventajas**
- Superficie de autenticación aislada y reemplazable.
- Posible reuso por otros servicios futuros.

**Desventajas**
- Despliegue, operación y monitoreo de un componente adicional en fase inicial local.
- Red adicional (network hop) para cada llamada autorizada desde otros módulos.
- Contrato serializado (gRPC/HTTP) que añade complejidad sin beneficio inmediato en un monolito.
- Incrementa el costo operativo del laboratorio (más contenedores, puertos, secrets).

### Alternativa B: Autenticación dentro del monolito Go (router + middleware + interfaces)

**Ventajas**
- Cero componentes adicionales: un solo binario Go, despliegue local simple.
- Zero network hop para la autorización: módulos de negocio inyectan `PermissionChecker` y llaman in-process.
- Contratos Go nativos (interfaces + DTOs) sin serialización inter-servicio.
- Coherente con DEC-005 (backend en Go) y ADR-003 (persistencia portable SQLite/Postgres).
- Los adapters de proveedor (OIDC, password, Service Principal, Managed Identity) son intercambiables sin tocar el dominio.

**Desventajas**
- La superficie de autenticación crece con el monolito; en una fase distribuida futura podría extraerse a su propio servicio.
- Requiere disciplina para no mezclar lógica de auth con lógica de negocio (mitigado por interfaces y capas separadas).

## Criterios de evaluación

| Criterio | Peso | A: gateway separado | B: monolito Go |
|---|---:|---:|---:|
| Despliegue local simple | 5 | 2 | 5 |
| Latencia de autorización | 4 | 2 | 5 |
| Complejidad de contratos | 4 | 2 | 5 |
| Intercambiabilidad de proveedor | 5 | 4 | 5 |
| Acoplamiento módulos de negocio | 5 | 2 | 5 |

## Decisión

Se decide **Alternativa B**: autenticación y autorización dentro del monolito Go, con:

1. **Router stdlib `net/http` + middleware** en `cmd/cco-app/main.go` (edge único).
2. **Paquete `internal/iam`** con interfaces públicas: `AuthenticationProvider`, `TokenValidator`, `PermissionChecker`, `AuthorizationService`, `IdentityRepository`.
3. **Adapters en los bordes** (intercambiables): OIDC (Entra ID/Keycloak/Google), password (solo dev/local), Service Principal (client_credentials, colector/ML), Managed Identity (Azure MI).
4. **`PermissionChecker` con strings `"recurso:acción"`** y mapa estático rol → permisos (cargado de DB en futuro).
5. **`IdentityContext`** inyectado en `context.Context` con `request_id` y `correlation_id` para trazabilidad (RNF-13).
6. **JWT HS256** firmado simétricamente (clave por variable de entorno `CCO_JWT_SIGNING_KEY`).

## Justificación

La Alternativa B cumple DEC-005 y DEC-011 sin agregar componentes operativos en fase inicial, mantiene la autorización in-process para los módulos de negocio, y permite conectar a un proveedor externo de identidad (Entra ID, Keycloak, Google u otro OIDC) mediante el adapter `AuthenticationProvider` sin modificar el dominio. El gateway, si se requiere en una fase distribuida, puede extraerse más adelante exponiendo los mismos contratos HTTP.

## Consecuencias

### Positivas

- Un solo binario Go, cero servicios adicionales en laboratorio.
- Los módulos de negocio (Nube, Adquisición, Analítica, Recomendaciones, Reportes) dependen solo de `PermissionChecker` y nunca tocan JWT, OIDC ni secretos.
- Pruebas unitarias rápidas con SQLite en memoria.
- Contratos listos para extraer a un servicio separado si el despliegue se distribuye.

### Negativas

- El binario crece con la responsabilidad de auth; en fase distribuida podría extraerse.
- JWT HS256 con una sola clave de firma requiere rotación coordinada si hay múltiples instancias.

### Riesgos

- Clave de firma en variable de entorno; si se filtra, se deben rotar todos los tokens emitidos. Mitigación: TTL corto (15 min) y rotación de clave por variable de entorno.
- `PasswordHash` como columna adicional en `iam.users` no existe en el dump SQL de producción (`cco_app.sql`). Requiere `ALTER TABLE` antes de habilitar login por contraseña fuera de desarrollo.

## Evidencia

- `backend/internal/iam/` (interfaces, auth, repositories, adapters, middleware, handlers).
- `backend/cmd/cco-app/main.go` (DI y router).
- `backend/test/iam_integration_test.go` (prueba de integración end-to-end sobre SQLite).
- DEC-005, DEC-011, DEC-018, ADR-003, RNF-02, RNF-13, RNF-15.

## Revisión

- **Fecha prevista de revisión:** Antes de una versión distribuida o productiva, o cuando se requiera MFA / passkeys / device auth.
- **Condición que podría provocar una revisión:** Múltiples instancias de backend, necesidad de zero-trust entre servicios, o cumplimiento que exija gateway dedicado.

## Referencias

[1]: Proyecto de Ingeniería I, Sección 14 (Tecnologías propuestas) — Backend Go + REST.
[2]: Proyecto de Ingeniería I, Sección 13.7 (Regla de alineación entre predicción y decisión) — La IA predice; el backend Go decide; el usuario autoriza.