# ADR-005 — Estructura de pruebas del backend Go

- **Estado:** Aceptada para la fase inicial
- **Fecha:** 2026-10-29
- **Decisores:** Equipo del proyecto
- **Relacionado con:** RNF-08, ADR-003, ADR-004

## Contexto

El backend Go necesita pruebas para la capa de persistencia (`cco_app`) y el nuevo módulo IAM. Go tiene una convención estándar: archivos `_test.go` ubicados junto al código bajo prueba (paquete `paquete_test` para pruebas externas o `paquete` para internas). No existe una convención oficial de un directorio `test/` dedicado.

## Problema

Decidir dónde ubicar:

1. Pruebas unitarias de lógica pura (PermissionEngine, errores, contexto).
2. Pruebas de integración que cruzan paquetes (IAM + platform/database sobre SQLite).

## Alternativas consideradas

### Alternativa A: Todo en directorio `backend/test/`

**Ventajas**
- Pruebas agrupadas en un solo lugar, fácil de ver de un vistazo.

**Desventajas**
- No es idiomático Go; herramientas de cobertura y IDE esperan `_test.go` junto al código.
- Pruebas unitarias de lógica pura pierden visibilidad respecto al código que prueban.
- Mayor desplazamiento al navegar entre implementación y prueba.

### Alternativa B: Unitarias `_test.go` junto al código; integración cross-package en `backend/test/`

**Ventajas**
- Sigue la convención estándar de Go.
- Cobertura y herramientas funcionan sin configuración adicional.
- `backend/test/` solo para lo que realmente lo necesita: pruebas que importan múltiples paquetes internos y no pueden vivir dentro de uno de ellos sin crear ciclos de importación.

**Desventajas**
- Dos ubicaciones de prueba que el equipo debe conocer.

## Criterios de evaluación

| Criterio | Peso | A: todo en test/ | B: híbrido idiomático Go |
|---|---:|---:|---:|
| Convención del lenguaje | 5 | 2 | 5 |
| Visibilidad código-prueba | 5 | 3 | 5 |
| Herramientas de cobertura/IDE | 5 | 2 | 5 |
| Pruebas cross-package | 3 | 5 | 4 |

## Decisión

Se decide **Alternativa B**:

- **Unitarias:** `backend/internal/iam/*_test.go`, `backend/internal/platform/database/database_test.go`, etc. (paquete `iam_test` para externas, `database_test` ya existente).
- **Integración cross-package:** `backend/test/iam_integration_test.go` (paquete `test`), que abre SQLite, migra, y ejercita repositories de IAM contra la base real.
- No se crea `internal/iam/test/` ni directorios `test/` dentro de paquetes internos.

## Justificación

Mantiene la simplicidad y las herramientas de Go sin configuración adicional, y reserva `backend/test/` exclusivamente para lo que justifica su existencia: pruebas que cruzan límites de paquete y no pueden residir dentro de uno sin acoplamiento indebido.

## Consecuencias

### Positivas

- `go test ./...` descubre y ejecuta todas las pruebas sin parámetros extra.
- Cobertura estándar con `go test -cover`.
- IDE y linters reconocen las pruebas automáticamente.

### Negativas

- El equipo debe recordar que las pruebas de integración viven en `backend/test/`.

### Riesgos

- Ninguno significativo. Si en el futuro crecen las pruebas de integración, `backend/test/` puede organizarse en subdirectorios por dominio.

## Evidencia

- `backend/internal/iam/permission_engine_test.go`
- `backend/internal/iam/errors_test.go`
- `backend/internal/platform/database/database_test.go`
- `backend/test/iam_integration_test.go`

## Revisión

- **Fecha prevista de revisión:** Cuando las pruebas de integración superen un dominio o requieran fixtures compartidos entre múltiples módulos.
- **Condición que podría provocar una revisión:** Necesidad de un paquete de fixtures compartido o pruebas E2E que involucren frontend + backend.

## Referencias

[1]: https://go.dev/doc/#testing "Go Testing Documentation"