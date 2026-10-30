# ADR-006 — Estándar de logging del backend

- **Estado:** Aceptada para la fase inicial
- **Fecha:** 2026-10-30
- **Decisores:** Equipo del proyecto
- **Relacionado con:** RNF-13, ADR-004, DEC-027

## Contexto

El módulo Identidad y Acceso (IAM) es el primero en implementarse del backend monolito modular en Go. MODULES.md establece que la trazabilidad transversal debe permitir relacionar operaciones con actor, recurso, ejecución y evidencia. RNF-13 exige que todo el sistema correlacione operaciones mediante un identificador de solicitud (`request_id`) presente en logs y resultados.

No existe todavía un módulo dedicado a Auditoría y Trazabilidad (módulo 8 del orden sugerido), ni un colector de logs definido.

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
4. **Los módulos de negocio futuros** (Nube, Adquisición, Analítica, etc.) reutilizan `slog` con el mismo patrón; solo agregan su campo `module` cuando lo necesiten.

## Justificación

`log/slog` cumple RNF-13 (identificador de solicitud en logs), evita dependencias evitable (ponytail), y su salida a stdout es la interfaz nativa hacia cualquier sistema de observabilidad. No hay razón para crear un servicio de logs separado en Report 1.0; eso corresponde al módulo 8 (Auditoría y Trazabilidad) o a infraestructura de operaciones.

## Consecuencias

### Positivas

- Logs estructurados y correlacionables desde el primer módulo.
- Cero dependencias nuevas.
- Mismo patrón para todos los módulos futuros.
- `CCO_LOG_FORMAT=json` habilita consumo directo por colectores externos.

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

- **Fecha prevista de revisión:** Cuando se implemente el módulo 8 (Auditoría y Trazabilidad) o cuando el volumen de logs requiera niveles diferentes por módulo.
- **Condición que podría provocar una revisión:** Múltiples instancias de backend con necesidad de agregación centralizada, o cumplimiento que exija almacenamiento de logs a largo plazo.

## Referencias

[1]: https://pkg.go.dev/log/slog "Package slog — Structured logging"
[2]: Proyecto de Ingeniería I, Sección 10.2 RNF-13 — Observabilidad.
[3]: docs/04-architecture/MODULES.md — Principio de trazabilidad transversal.