# ADR-006 — Estándar de logging del backend

- **Estado:** Aceptada; complementada por ADR-014
- **Fecha:** 2026-10-30
- **Decisores:** Equipo del proyecto
- **Relacionado con:** RNF-13, ADR-004, ADR-014, DEC-027

## Contexto

El módulo Identidad y Acceso (IAM) fue el primero en implementarse del backend monolito modular en Go. RNF-13 exige correlacionar operaciones mediante un identificador de solicitud (`request_id`) presente en logs y resultados. La decisión original de usar `slog` define observabilidad operativa; no establece que los logs sean el almacenamiento de auditoría.

Auditoría y Trazabilidad ya cuenta con un módulo persistente definido por [ADR-014](ADR-014-audit-traceability.md). El sistema conserva destinos distintos: `slog` para diagnóstico operativo, `cco_app.audit.events` para auditoría consultable de acciones de aplicación y `cco_data.raw.activity_log` para actividad recolectada de proveedores cloud.

## Problema

Definir cómo emite el backend los logs de las operaciones de autenticación y autorización, de forma que:

1. Sean correlacionables entre módulos (request_id / correlation_id).
2. No requieran dependencias externas ni infraestructura adicional en Report 1.0.
3. Sean exportables a un colector externo (Loki, ELK, CloudWatch) sin modificar el código cuando sea necesario.

## Alternativas consideradas

### Alternativa A: Paquete de logging externo (zap, zerolog, logrus)

**Ventajas**
- Funcionalidades avanzadas (sampling, múltiples sinks, niveles estructurados).

**Desventajas**
- Dependencia externa evitable para una necesidad inicial simple.
- Incrementa la superficie de dependencias y el tiempo de build.
- Sobre-diseño para Report 1.0, donde el volumen de logs es bajo.

### Alternativa B: `fmt.Println` / `log.Printf` a stdout

**Ventajas**
- Sin dependencias.
- Interfaz nativa hacia cualquier colector (stdout es el contrato de contenedores).

**Desventajas**
- No estructurado: difícil de filtrar y parsear por colectores.
- No soporta niveles de severidad ni campos clave-valor consistentes.

### Alternativa C: `log/slog` (estándar de Go 1.21+)

**Ventajas**
- Estándar de Go, sin dependencias externas.
- Estructurado (key-value) y con niveles (Debug, Info, Warn, Error).
- Interfaz hacia stdout; trivial de redirigir a un colector externo.
- `request_id` y `correlation_id` viajan como atributos consistentes en cada entrada.

**Desventajas**
- Requiere Go 1.21+ (el proyecto usa Go 1.22, por lo tanto compatible).

## Criterios de evaluación

| Criterio | Peso | A: paquete externo | B: fmt.Println | C: log/slog |
|---|---:|---:|---:|---:|
| Sin dependencias externas | 5 | 2 | 5 | 5 |
| Estructurado / consultable | 5 | 5 | 1 | 5 |
| Niveles de severidad | 4 | 5 | 1 | 5 |
| Exportable vía stdout | 5 | 3 | 5 | 5 |
| Tiempo de implementación | 5 | 2 | 5 | 5 |

## Decisión

Se decide **Alternativa C**: `log/slog` como estándar de logging del backend Go.

1. **Un solo handler global** configurado en `cmd/cco-app/main.go`.
   - Formato texto por defecto (desarrollo local).
   - Formato JSON si `CCO_LOG_FORMAT=json` (producción / colector externo).
2. **Salida a stdout**: cualquier colector (Loki, ELK, Fluentd, CloudWatch) consume stdout sin cambios de código.
3. **Campos obligatorios en cada log de autenticación**: `request_id`, `correlation_id`, `user_id`, `email`, `auth_method`, `org_id`, `org_role`, `project_id`, `path`, `method`.
4. **Todos los módulos** reutilizan el handler global de `slog` para eventos operativos y agregan `module` cuando sea útil.
5. **Auditoría persistente es independiente de `slog`:** los eventos sujetos a auditoría se registran mediante `audit.AuditLogger` en `cco_app.audit.events`; el servicio también emite una entrada estructurada a `slog` para diagnóstico. Un log no reemplaza ni garantiza la persistencia del evento de auditoría.
6. La actividad recolectada desde el proveedor cloud permanece en `cco_data.raw.activity_log`; no se usa como auditoría de usuarios de la aplicación.

## Justificación

`log/slog` cumple RNF-13 (identificador de solicitud en logs), evita dependencias externas y su salida a stdout es la interfaz nativa hacia cualquier sistema de observabilidad. La necesidad distinta de un historial autorizado y persistente se resuelve en `audit.events`, sin sustituir el logger ni mezclar su ciclo de vida con los logs.

## Consecuencias

### Positivas

- Logs estructurados y correlacionables desde el primer módulo.
- Cero dependencias nuevas.
- Mismo patrón para todos los módulos futuros.
- `CCO_LOG_FORMAT=json` habilita consumo directo por colectores externos.
- La persistencia, consulta y autorización de eventos de auditoría quedan a cargo del módulo `audit`, no del handler de logging.

### Negativas

- Un solo handler global por ahora; si distintos módulos necesitan niveles diferentes, habrá que refactorizar.
- No hay rotación de logs ni compresión en la aplicación (delega al contenedor/orquestador).

### Riesgos

- Si se olvida propagar `request_id` en algún handler, la trazabilidad se rompe para esa ruta. Mitigación: el middleware de auth lo inyecta para todas las rutas autenticadas.

## Evidencia

- `backend/cmd/cco-app/main.go` — inicialización de `slog`.
- `backend/internal/iam/middleware/auth_middleware.go` — logs de autenticación.
- `backend/internal/iam/handlers.go` — logs de registro, login y M2M.
- `backend/internal/iam/identity_context.go` — `IdentityContext` con `RequestID` y `CorrelationID`.

## Revisión

- **Fecha prevista de revisión:** Antes de producción o cuando el volumen de logs requiera niveles diferentes por módulo.
- **Condición que podría provocar una revisión:** Múltiples instancias de backend con necesidad de agregación centralizada, o cumplimiento que exija almacenamiento de logs a largo plazo.

## Referencias

[1]: https://pkg.go.dev/log/slog "Package slog — Structured logging"
[2]: Proyecto de Ingeniería I, Sección 10.2 RNF-13 — Observabilidad.
[3]: docs/04-architecture/MODULES.md — Catálogo y límites de los módulos.