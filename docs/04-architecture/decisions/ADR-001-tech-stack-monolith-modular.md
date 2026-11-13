# ADR-001 — Stack tecnológico y arquitectura monolito modular en Go

- **Estado:** Aceptada para la fase inicial
- **Fecha:** 2026-09-12
- **Decisores:** Equipo del proyecto
- **Relacionado con:** DEC-005, DEC-011, ADR-003, ADR-004

## Contexto

El proyecto Cloud Cost Optimizer requiere un backend que exponga una API HTTP, gestione autenticación/autorización, se conecte a fuentes cloud (Azure), adquiera métricas y costos, y provea evidencia analítica. El Proyecto de Ingeniería I definió "Backend como API Gateway en Go" y autenticación con proveedor externo, pero no especifica si el backend es un monolito o un conjunto de microservicios.

## Problema

Definir el estilo arquitectónico y el stack tecnológico del backend en Report 1.0:

1. ¿Monolito modular o microservicios?
2. ¿Qué lenguaje y framework usar?
3. ¿Cómo se comunican los módulos entre sí?

## Alternativas consideradas

### Alternativa A: Microservicios en Go (un servicio por módulo)

**Ventajas**
- Despliegue y escalado independiente por módulo.
- Fallos aislados; un módulo no derriba a los demás.

**Desventajas**
- Despliegue, operación y monitoreo de múltiples componentes en fase inicial local.
- Red adicional (network hop) entre módulos para cada llamada.
- Contratos serializados (gRPC/HTTP) que añaden complejidad sin beneficio inmediato.
- Mayor costo operativo del laboratorio (más contenedores, puertos, secrets).

### Alternativa B: Monolito modular en Go (seleccionada)

**Ventajas**
- Un solo binario Go, despliegue local simple (`go build` + un ejecutable).
- Zero network hop: módulos de negocio llaman in-process mediante interfaces Go.
- Contratos Go nativos (interfaces + DTOs) sin serialización inter-servicio.
- Coherente con ADR-003 (persistencia portable SQLite/Postgres) y ADR-004 (auth dentro del monolito).
- Pruebas unitarias rápidas con SQLite en memoria.
- Los módulos pueden extraerse a servicios separados en una fase distribuida futura exponiendo los mismos contratos HTTP.

**Desventajas**
- El binario crece con la responsabilidad de todos los módulos; en fase distribuida futura podría extraerse.
- Requiere disciplina para no mezclar lógica de auth con lógica de negocio (mitigado por capas y paquetes separados).

## Criterios de evaluación

| Criterio | Peso | A: microservicios | B: monolito modular |
|---|---:|---:|---:|
| Despliegue local simple | 5 | 2 | 5 |
| Latencia entre módulos | 4 | 2 | 5 |
| Complejidad de contratos | 4 | 2 | 5 |
| Simplicidad operativa (laboratorio) | 5 | 2 | 5 |
| Evolución hacia servicios separados | 3 | 5 | 3 |

## Decisión

Se decide **Alternativa B**: monolito modular en Go, con:

1. **Lenguaje:** Go 1.25 (`go.mod`), stdlib `net/http` para el router.
2. **Entrada única:** `cmd/cco-app/main.go` hace DI (inyección de dependencias) y registra las rutas de cada módulo sobre un `*http.ServeMux`.
3. **Módulos como paquetes internos:** `internal/iam`, `internal/cloud`, `internal/acquisition`, `internal/data`, `internal/analytics`, `internal/platform/database`, `internal/platform/valkey`.
4. **Comunicación in-process:** interfaces Go + DTOs; ningún módulo de negocio importa `platform/database` directamente.
5. **Persistencia:** GORM portable sobre SQLite (dev/tests) y PostgreSQL (prod), dos bases de datos separadas (`cco_app` y `cco_data`, ver ADR-002).
6. **Estado compartido anti-abuso:** Valkey opcional con fallback en memoria (ver ADR-008).
7. **Autenticación/autorización:** dentro del monolito, paquetes `iam` y `iam/middleware` (ver ADR-004, ADR-007).

## Justificación

La Alternativa B cumple DEC-005 y DEC-011 sin agregar componentes operativos en fase inicial, mantiene la autorización in-process para los módulos de negocio, y permite conectar a un proveedor externo de identidad (Entra ID, Keycloak, Google) mediante adapters sin modificar el dominio. El monolito modular es el punto de partida más simple que permite extraer servicios después si el despliegue se distribuye.

## Consecuencias

### Positivas

- Un solo binario Go, cero servicios adicionales en laboratorio.
- Los módulos de negocio dependen solo de contratos Go y nunca tocan JWT, OIDC ni secretos.
- `go test ./...` descubre y ejecuta todas las pruebas sin configuración adicional.
- Contratos listos para extraer a servicios separados si el despliegue se distribuye.

### Negativas

- El binario crece con todos los módulos; en fase distribuida futura podría extraerse.
- Un fallo en un módulo puede afectar al proceso completo (mitigado por aislamiento de paquetes y errores controlados).

### Riesgos

- Acoplamiento accidental entre módulos si se importan paquetes internos en lugar de interfaces. Mitigación: revisión de código y dependencias dirigidas (MODULES.md).

## Evidencia

- `backend/cmd/cco-app/main.go` — DI y router.
- `backend/internal/` — paquetes de módulo.
- `backend/go.mod` — Go 1.25 y dependencias.
- `docs/MODULES.md` — definición de módulos y dependencias prohibidas.

## Revisión

- **Fecha prevista de revisión:** Antes de una versión distribuida o productiva.
- **Condición que podría provocar una revisión:** Múltiples instancias de backend, necesidad de zero-trust entre servicios, o requerimiento de escalado independiente por módulo.

## Referencias

[1]: Proyecto de Ingeniería I, Sección 14 (Tecnologías propuestas) — Backend Go + REST.
