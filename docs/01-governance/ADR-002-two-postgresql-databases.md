# ADR-002 — Separación inicial de bases de datos PostgreSQL

- **Estado:** Aceptada para la fase inicial
- **Fecha:** 2026-09-12
- **Decisores:** Equipo del proyecto
- **Relacionado con:** REQ-002, REQ-007, MTG-002

## Contexto

La reunión determinó que el proyecto necesita separar la información de interacción de la aplicación de los datos recolectados para el módulo de inteligencia artificial y futuros entrenamientos.

## Problema

Se necesita evitar que los datos operativos de la aplicación y los datos de recolección/analítica compartan inicialmente el mismo espacio lógico de persistencia, porque tienen propósitos, ciclos de vida y necesidades de acceso diferentes.

## Alternativas consideradas

### Alternativa A: Una única base de datos

**Ventajas**

- Menor complejidad inicial.
- Menor cantidad de componentes que administrar.

**Desventajas**

- Mezcla datos operativos y datos de recolección.
- Aumenta el acoplamiento entre la aplicación y el módulo de inteligencia artificial.
- Puede dificultar el control de permisos y la evolución independiente.

### Alternativa B: Dos bases de datos PostgreSQL separadas

**Ventajas**

- Separa la comunicación usuario-aplicación de la recolección para inteligencia artificial.
- Permite evolucionar los esquemas de forma independiente.
- Facilita establecer permisos y políticas de retención diferentes.

**Desventajas**

- Incrementa la complejidad de configuración y respaldo.
- Requiere definir sincronización o intercambio de datos si algún flujo lo necesita.
- La operación local debe documentar dos conexiones y dos esquemas de despliegue.

## Criterios de evaluación

| Criterio | Peso | A: una base | B: dos bases |
|---|---:|---:|---:|
| Separación de responsabilidades | 5 | 2 | 5 |
| Evolución independiente | 4 | 2 | 5 |
| Simplicidad inicial | 3 | 5 | 3 |
| Control de acceso | 4 | 3 | 5 |
| Adecuación a la reunión | 5 | 1 | 5 |

## Decisión

Se decide utilizar dos bases de datos PostgreSQL durante la fase inicial:

1. Una base para la comunicación entre el usuario y la aplicación.
2. Una base para la recolección de datos y el módulo de inteligencia artificial.

El despliegue inicial será local. La decisión deberá revisarse antes de una versión productiva o distribuida.

## Justificación

La alternativa se selecciona debido a que refleja el acuerdo de la Reunión 2 y separa los datos operativos de los datos destinados a analítica y futuros entrenamientos. La implementación local permite validar los esquemas sin introducir todavía costos o complejidad de infraestructura cloud adicional.

## Consecuencias

### Positivas

Se obtiene una separación clara de responsabilidades, permisos y ciclos de vida. El equipo puede diseñar el esquema de la aplicación y el esquema de inteligencia artificial de manera independiente.

### Negativas

Se deben administrar dos conexiones, dos esquemas de migración y procedimientos de respaldo diferenciados. También se deberá definir cómo se transfieren los datos necesarios entre ambas bases.

### Riesgos

La separación puede generar duplicación, inconsistencias o dificultades de sincronización. Estos riesgos deben revisarse cuando se defina la arquitectura de integración.

## Evidencia

- MTG-002.
- Reunión 2 del Proyecto de Grado.
- REQ-002.
- REQ-007.

## Revisión

- **Fecha prevista de revisión:** Antes de implementar una versión distribuida o productiva.
- **Condición que podría provocar una revisión:** Necesidad de transacciones entre dominios, incremento significativo de la complejidad operativa o migración a servicios administrados.

## Referencias

[1]: https://www.postgresql.org/docs/current/ddl-schemas.html "PostgreSQL Documentation: Schemas"
