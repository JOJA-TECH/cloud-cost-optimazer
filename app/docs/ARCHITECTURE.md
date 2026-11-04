# Architecture

## Roles

- **Azure SQL (system under test)** — the workload runs here. It holds the
  `loadtest` schema (experiment context, fact tables, query templates) plus
  `loadtest.workload_control`, the operational state machine. It is the only
  place the load is generated, and the workload login cannot touch anything
  outside `loadtest`.
- **Supabase PostgreSQL (analytical persistence)** — long-lived storage for
  readings, samples, hourly features and the collector registry. It never talks
  to Azure SQL.
- **Collector VM** — the only component with both connection strings. It runs
  the load generator, the metric collector, the Telegram bot and the supervisor.

## Data flow

```
workload.runner ──ops──▶ Azure SQL (loadtest)
       │                     │
       │ per-op observation  │ DMV / Query Store / Azure Monitor
       ▼                     ▼
   observe spool  ───▶  metrics.collector ──▶ sync.writer ──▶ Supabase raw.*
                                                 │  (on failure)
                                                 ▼
                                             local spool
                                                 │ retry
                                                 ▼
                                        Supabase raw.* (idempotent)

   metrics.aggregator ──5 min──▶ feat.metric_hourly
```

## Metric quality and percentiles

- Every reading passes `metrics.quality.evaluate_reading`, which classifies it
  as `usable`, `degraded`, `not_available` or `invalid` against the catalog
  bounds in `metrics.normalizer.METRIC_CATALOG`. Values far outside bounds are
  `invalid` and excluded from features; slightly outside are `degraded`.
- **Exact percentiles require individual samples.** Platform metrics and DMVs
  only expose interval aggregates (avg/min/max), so their percentile method is
  `not_available` and no value is stored. Exact p50/p95/p99 for CPU come from
  `raw.metric_sample`, and latency p95 from workload self-observations. When no
  samples exist the column stays `NULL` and a warning is recorded — a number is
  never invented.

## Idempotency and back-pressure

- `raw.metric_reading` has a unique key on
  `(experiment_uid, resource_uid, metric_code, observed_at_utc, source)`;
  `raw.metric_sample` dedups on the same columns plus `source_sample_id`.
  Inserts use `ON CONFLICT DO NOTHING`, so retries never duplicate data.
- If Supabase is unreachable, `sync.writer` serializes the batch into a
  checksummed local spool file and the cycle is marked `partial`. The next
  successful cycle drains the spool first. The spool has a hard size cap; when
  it is exceeded the condition is reported (never silently dropped).

## Workload state machine

`loadtest.workload_control` is authoritative and is enforced by an
`INSTEAD OF UPDATE` trigger. `workload.controller.WorkloadController` mirrors
each transition to `experiment.experiment_run` on Supabase:

```
CREATED ─▶ RUNNING ─▶ PAUSING ─▶ PAUSED
              │            │         │
              └────────────┴─────────┴──▶ STOPPING ─▶ STOPPED
                                   (any) ─▶ FAILED
```

`PAUSING → RUNNING` (resume) and `PAUSING → PAUSED` (ack) are allowed; a stop
from `PAUSING` is done by resuming then stopping. See `docs/OPERATIONS.md`.

## Safe shutdown

`src.shutdown.safe_stop` is the only path to a terminal record: request
`STOPPING`, confirm `STOPPED`, flush the spool within
`FINAL_FLUSH_TIMEOUT_SECONDS`. If the spool cannot be emptied the experiment is
recorded as `STOPPED_WITH_PENDING_BUFFER` and the data stays on disk.
`--supervise` forces this stop once the experiment exceeds `AUTO_STOP_AFTER_HOURS`.

## Perfiles de despliegue: pilot y full

- **pilot (predeterminado, `deployment_profile=pilot`)**: exactamente una base de
  datos y una VM, corrida corta configurable. Usa Azure SQL Free Tier con
  auto-pause al agotar la cuota solo si `FREE_TIER_ENABLED=true` (el operador
  confirma elegibilidad por lectura sola antes de `apply`). Si no se confirma,
  `scripts/preflight.sh` bloquea y presenta una alternativa de bajo costo
  claramente marcada como facturable. No escala a más bases.
- **full (`deployment_profile=full`)**: requiere `database_subscription_ids`
  (allowlist no vacía) y `database_targets` con 1..N entradas. La VM se despliega
  en `vm_subscription_id`; cada base en su suscripción explícita. Cada base
  conserva identidad (`resource_uid`) y resultados separados.

## Multi-suscripción y proveedores de Terraform

- La VM se crea en `vm_subscription_id` mediante el proveedor estático
  `azurerm.vm`, nunca en todas las suscripciones visibles por defecto.
- Las bases de datos se crean en `subscription_id` (primaria) mediante el
  proveedor por defecto, una por entrada de `database_targets`.
- **Límite de proveedores (documentado):** Terraform no permite generar aliases
  de `azurerm` dinámicamente con `for_each`. Las suscripciones de base de datos
  distintas a la primaria requieren que el operador añada bloques estáticos
  `provider "azurerm" { alias = "..." }` en `terraform/providers.tf` e
  instancie el módulo/recursos de ese target con ese alias. `az account set` no
  se usa como mecanismo oculto.
- `terraform plan` y los outputs muestran la suscripción de cada recurso
  (outputs `database_targets`, `vm_subscription_id`, `primary_subscription_id`).
- Validaciones rechazan IDs vacíos, duplicados, no allowlisted e inconsistentes.

## RBAC de solo lectura y red

- La identidad de la VM (system-assigned managed identity) recibe `Reader` +
  `Monitoring Reader` únicamente sobre el grupo de recursos de la VM.
- Para leer métricas de bases en otras suscripciones (`full`), la identidad
  necesita `Monitoring Reader` sobre cada recurso SQL de destino en su propia
  suscripción (mínimo scope, sin `Owner` ni permisos de suscripción completa).
  Eso requiere proveedores estáticos por suscripción (ver límite arriba) y se
  documenta como paso de operación.
- Firewall de Azure SQL: solo la IP pública del VM (`/32`). Sin `0.0.0.0/0` ni
  "Allow Azure services". SSH solo desde `admin_cidr`. TLS 1.2 obligatorio.

## SKU de la base de datos enviado a Supabase

Cada lectura de Azure Monitor, DMV y Query Store incluye el `sku_name` del
target de origen dentro de su `source_payload` (campo `sku_name`), de modo que
Supabase conserva, junto a cada muestra, con qué SKU se tomó la lectura. Esto
permite al pipeline de forecasting conocer la configuración real de cada base
sin depender de consultas externas a Azure.

## Exportación del dataset

- `src.export` escribe CSV (y Parquet si hay pyarrow) más `manifest.json` con
  IDs de recurso, configuración, calendario, zona horaria (UTC), semillas,
  versión de catálogo/código, conteos, huecos, quality flags y checksums.
- El paquete se escribe en disco local y sobrevive al apagado/destrucción de la
  VM; debe copiarse fuera de la VM antes de cualquier `destroy`. El spool local
  de recuperación por sí solo NO es el dataset final.