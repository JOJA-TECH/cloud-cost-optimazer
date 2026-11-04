# Esquema de variables de Terraform (solo placeholders)

Todos los valores son placeholders. Nunca commitear `terraform.tfvars` con
valores reales. Los secretos se inyectan por variable sensible, Key Vault o el
almacén de secretos del pipeline; `sensitive=true` los oculta en pantalla pero
**el state de Terraform puede conservarlos**, por lo que el state debe guardarse
en backend remoto con control de acceso (o protegerse como si fuera un secreto).

## Suscripciones y perfil

| Variable | Tipo | Ejemplo (placeholder) | Descripción |
|----------|------|-----------------------|-------------|
| `subscription_id` | string (sensitive) | `"00000000-0000-0000-0000-000000000000"` | Suscripción primaria: aloja el/los target(s) de SQL. |
| `vm_subscription_id` | string (sensitive) | `"00000000-0000-0000-0000-000000000001"` | Suscripción donde se crea la VM (explícita, separada). |
| `tenant_id` | string (sensitive) | `"00000000-0000-0000-0000-000000000000"` | Entra ID. |
| `deployment_profile` | string | `"pilot"` | `"pilot"` (predeterminado) o `"full"`. |
| `location` | string | `"eastus"` | Región de la suscripción primaria. |
| `vm_location` | string | `"eastus"` | Región de la suscripción de la VM. |
| `vm_resource_group` | string | `"cco-vm-rg"` | Grupo de recursos de la VM. |

## Targets de base de datos y allowlist

| Variable | Tipo | Ejemplo (placeholder) | Descripción |
|----------|------|-----------------------|-------------|
| `database_subscription_ids` | list(string) (sensitive) | `["0000...0000"]` | Allowlist de suscripciones que pueden alojar DB. Obligatorio y no vacío en `full`. |
| `database_targets` | map(object) | ver abajo | Uno o más targets. `pilot`: exactamente 1. `full`: 1..N, cada `subscription_id` en el allowlist. |

Ejemplo `database_targets` (pilot, un solo target):

```
database_targets = {
  "pilot-db-1" = {
    subscription_id        = "00000000-0000-0000-0000-000000000000"
    resource_group         = "cco-pilot-rg"
    location               = "eastus"
    server                 = "cco-pilot-sql"
    database               = "loadtest"
    sku_name               = "GP_S_Gen5_1"
    min_capacity           = 0.5
    max_capacity           = 1.0
    auto_pause_delay_minutes = 60
    max_size_gb            = 5
    free_tier              = true      # confirmar elegibilidad en preflight
    resource_uid           = "00000000-0000-0000-0000-000000000000"  # estable, por base
  }
}
```

`resource_uid` es un identificador estable y único por base (no se deriva de un
UUID aleatorio por corrida).

## Acceso VM y SQL

| Variable | Ejemplo (placeholder) |
|----------|-----------------------|
| `admin_username` | `"cco-admin"` |
| `admin_ssh_public_key` | `"ssh-rsa AAAA...replace-with-your-public-key..."` |
| `admin_cidr` | `"203.0.113.7/32"` (IP/CIDR del operador, /32) |
| `sql_admin_username` / `sql_admin_password` | secretos (no commitear) |
| `sql_workload_username` | `"cco_workload"` |
| `sql_workload_password` | secreto (mínimos privilegios, solo `loadtest`) |

## Supabase, Telegram y ciclo

| Variable | Ejemplo (placeholder) |
|----------|-----------------------|
| `supabase_db_host` | `"db.xxxxxxxxxxxxxxxx.supabase.co"` |
| `supabase_db_user` / `supabase_db_password` | secretos del almacén de secretos |
| `telegram_bot_token` | `"123456:REPLACE-WITH-BOTFATHER-TOKEN"` |
| `telegram_allowed_chat_ids` | `["123456789"]` |

## Capacidad, duración y presupuesto

| Variable | Ejemplo | Nota |
|----------|---------|------|
| `vm_size` | `"Standard_B1s"` | Tamaño económico; si no es gratuito, declarar alternativo facturable. |
| `experiment_duration_hours` | `336` | Duración planeada. Pilot: usar corta (p. ej. 1-2 h). |
| `max_experiment_hours` | `336` | Techo duro del experimento. |
| `auto_stop_after_hours` | `360` | Watchdog de seguridad (debe superar max_experiment_hours). |
| `monthly_budget_usd` | `20` | Alerta de consumo; **notifica, no corta inmediatamente**. |
| `budget_alert_emails` | `["you@example.com"]` | Destinatarios de la alerta de presupuesto. |
| `enable_budget` | `true` | Crear presupuesto y alertas. |
| `capture_raw_interval_seconds` | `60` | Cadencia de captura (30-300 s). |
| `enable_public_ip` | `true` | IP pública estática (pone el firewall de SQL a la VM). |

## Mecanismo de secretos y state

- Secrets: variables sensibles (`sensitive=true`) de Terraform; en la VM se
  escriben en `/opt/cco/.env` (modo 0600) por cloud-init. Nunca en logs ni
  outputs.
- State: configurar backend remoto (`azurerm` Storage Account o equivalente) con
  control de acceso antes del `apply` en entornos compartidos. El state puede
  contener secretos (cloud-init). No afirmar que los secretos quedan fuera del
  state.

## Envío de SKU a Supabase

Cada lectura de métrica (Azure Monitor, DMV, Query Store) enviada a Supabase
incluye el campo `sku_name` dentro de `source_payload`, con el SKU real del
target de origen (p. ej. `GP_S_Gen5_1`). Así el pipeline de forecasting conoce
la configuración exacta de cada base sin consultar Azure de nuevo. El SKU se
declara por target en `database_targets[].sku_name`.

## Bloqueos de decisión pendientes (no se infieren)

Antes de `apply` se requieren valores explícitos del operador:
- `vm_subscription_id` y `subscription_id` (o `database_targets` completos).
- Región y número de bases.
- `FREE_TIER_ENABLED=true` (o confirmación de alternativa facturable).
- Presupuesto máximo (`monthly_budget_usd`).
Sin ellos, `preflight.sh` bloquea y `terraform validate` los pide como variables requeridas.
