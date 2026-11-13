# ADR-010 — Módulo Adquisición y Monitoreo de Datos (Module 3)

- **Estado:** Aceptado e implementado
- **Fecha:** 2026-11-12
- **Decisores:** Equipo del proyecto
- **Relacionado con:** ADR-001, ADR-003, ADR-004, ADR-009, MODULES.md (módulo 3)

## Contexto

El módulo Adquisición y Monitoreo de Datos obtiene métricas operativas de los recursos configurados, valida y normaliza los datos, y registra el resultado de cada proceso de adquisición. Es el módulo que alimenta `cco_data` (base de telemetría) desde `cco_app` (cuentas y recursos).

## Problema

Definir el diseño del ciclo de adquisición en Report 1.0:

1. ¿Cómo se programa y ejecuta la adquisición periódica?
2. ¿De qué fuentes se obtienen métricas, diagnósticos y costos?
3. ¿Cómo se persiste lo recolectado sin acoplar el módulo a `cco_data`?
4. ¿Qué ocurre si `cco_data` no está configurado?

## Alternativas consideradas

### Alternativa A: Adquisición directa a `cco_data` desde el módulo acquisition

**Desventajas:** Acopla el módulo al esquema de `cco_data` y a GORM; imposible de probar sin base real; viola la separación de MODULES.md.

### Alternativa B: Contratos de Gestión de Datos (`data.DataWriter` / `data.RunReader`) (seleccionada)

**Ventajas**
- El módulo acquisition nunca importa `cco_data` ni GORM de datos; escribe mediante interfaz.
- Idempotencia por claves naturales/únicas (`ON CONFLICT DO NOTHING`).
- Fallback: si `data.DataWriter` es nil (`CCO_DATA_DSN` no configurado), cada run falla con `ErrStorageUnavailable` y los endpoints responden `503`.
- Scheduler con mutex: un ciclo a la vez; solapamientos devuelven `ErrCycleInProgress`.

## Decisión

Se decide **Alternativa B**:

1. **Arranque:** `go acquisition.NewScheduler(acqSvc, interval).Run(ctx)` en `main.go` cuando `CCO_DATA_DSN` está configurado. Intervalo por defecto 60 s (`CCO_ACQ_INTERVAL_SECONDS`).
2. **Ciclo de adquisición (`RunCycle`):**
   - Resuelve targets monitoreados de `cco_app` (via `TargetRepository`).
   - Para cada target y cada job habilitado, ejecuta fetch → write → finish en paralelo (semáforo `FetchParallel`, default 8).
   - Cada (target, source) produce una fila en `raw.collector_run`.
   - Un fallo de una fuente no aborta las demás ni el ciclo.
3. **Fuentes de adquisición (jobs):**
   - `azure_monitor_metrics`: métricas platform PT1M (ventana `now - delay - lookback` a `now - delay`).
   - `sql_diagnostics`: `sys.dm_db_resource_stats` + sesiones (requiere `CCO_ACQ_SQL_USER`/`PASSWORD`).
   - `query_store`: top queries y runtime stats (mismas credenciales SQL).
   - `arm_config`: snapshot de configuración ARM (SKU, tier, vcores, etc.).
   - `cost_management`: costos diarios por suscripción (ciclo separado, 6 h, `CCO_ACQ_COST_INTERVAL_SECONDS`).
   - `metric_sample`: observaciones individuales para percentiles exactos (fake en dev).
4. **Credenciales:** resueltas de `cco_app` vía `credential_reference` (`env:VAR`); nunca se persisten.
5. **Endpoints (todos protegidos + permiso `acquisitions:read` o `acquisitions:run`):**
   - `POST /api/v1/acquisitions/run` — ejecuta ciclo síncrono ahora.
   - `GET /api/v1/acquisitions` — lista corridas con filtros (source, status, limit).
   - `GET /api/v1/acquisitions/{id}` — detalle de una corrida (aislamiento por organización).
6. **Persistencia de telemetría:** `cco_data` (`raw.*`, `core.*`), tablas definidas en `databases/cco_data/`.

## Justificación

Los contratos de `data` mantienen a `acquisition` desacoplado del esquema de telemetría y testeable con fakes. El scheduler interno en el mismo binario evita infraestructura adicional en fase inicial. El aislamiento por organización en `GetRun` evita fugas de información entre clientes.

## Consecuencias

### Positivas

- Múltiples fuentes de adquisición en un solo ciclo, con tolerancia a fallos parciales.
- Scheduler y ciclo de costos independientes; cada uno con su propio ticker.
- `cco_data` opcional: sin él, los endpoints responden `503` y el scheduler no arranca (modo dev funcional).

### Negativas

- El trigger de adquisición es síncrono y está limitado por el timeout HTTP (10 s); ciclos grandes pueden excederlo.
- DMVs y Query Store requieren credenciales SQL de workload con `VIEW DATABASE STATE`.

### Riesgos

- Si `CCO_DATA_DSN` no está configurado, ninguna adquisición persiste; el módulo es efectivamente no operativo para telemetría.

## Evidencia

- `backend/internal/acquisition/` — service, scheduler, handlers, adapters, normalizer.
- `backend/internal/data/interfaces.go` — `DataWriter`, `RunReader` (contratos consumidos).
- `backend/cmd/cco-app/main.go` — wiring y schedulers.
- `API.md` §13 — referencia de endpoints de adquisición.
- `docs/05-design/DESIGN-AMDA-001.md` — diseño técnico de adquisición.

## Revisión

- **Fecha prevista de revisión:** Cuando los ciclos de adquisición superen el timeout HTTP (migrar a jobs asíncronos 202) o cuando se requieran más proveedores de métricas.

## Referencias

[1]: docs/MODULES.md — Módulo 3, Adquisición y Monitoreo de Datos.
