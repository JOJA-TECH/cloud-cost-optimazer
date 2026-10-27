# ADR-003 — Persistencia GORM portable para `cco_app`

- **Estado:** Propuesto para revisión del equipo
- **Rama de implementación:** `feature/gorm-cco-app`
- **Base:** `origin/database`
- **Fecha:** 2026-10-26

## Contexto

El repositorio ya contiene el diseño físico de `cco_app` en la rama `database`. El planteamiento modular define responsabilidades lógicas y contratos, pero aclara que sus entidades no implican una tabla por entidad. Por ello, la capa GORM debe adaptarse al esquema existente y no convertir el documento modular en un nuevo esquema físico.

## Decisión

Se incorpora GORM dentro de `backend/internal/platform/database` como adaptador de infraestructura. Los modelos GORM son exclusivos de persistencia. Los módulos reciben y devuelven DTOs mediante interfaces de repositorio; los DTOs no contienen etiquetas ni dependencias de GORM.

La misma colección de modelos se utiliza con SQLite y PostgreSQL. SQLite utiliza nombres de tabla sin esquema; PostgreSQL califica las tablas con `iam`, `organization`, `cloud`, `app` y `audit`. Esa diferencia queda encapsulada en el adaptador de base de datos.

La rama `feature/gorm-cco-app` debe abrirse desde `database`, porque allí viven el SQL, el diagrama y la documentación de `cco_app`. La rama `main` solo debe recibir la integración después de revisión y pruebas.

## Consecuencias

Se facilita la prueba local y la migración de datos desde SQLite hacia PostgreSQL usando los mismos contratos y modelos. Las características específicas de PostgreSQL —RLS, `citext`, `jsonb`, extensiones, triggers, constraints e índices— siguen siendo responsabilidad de la migración SQL de producción o de migraciones de infraestructura aisladas. GORM no debe eliminar ni sustituir esas políticas de seguridad.

## Estructura objetivo

```text
backend/
├── go.mod
└── internal/
    ├── iam/                         # contratos y DTOs de identidad
    └── platform/database/           # GORM, drivers y migraciones
```

Los módulos de negocio futuros deben depender de interfaces y DTOs, no importar `platform/database` directamente.
