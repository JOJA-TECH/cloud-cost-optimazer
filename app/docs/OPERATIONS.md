# Operations runbook

## Perfiles de despliegue

- **pilot (predeterminado, seguro):** 1 base de datos + 1 VM + corrida corta.
  `deployment_profile=pilot` y exactamente una entrada en `database_targets`.
- **full:** `deployment_profile=full`, `database_subscription_ids` (allowlist no
  vacía) y `database_targets` con las suscripciones de base explícitas. La VM va
  en `vm_subscription_id`.

Nunca desplegar en todas las suscripciones visibles por defecto.

## 0. Preflight (obligatorio, read-only, sin Azure real necesario)

```bash
cd <repo>
# Definir valores con placeholders o reales (nunca commitear secretos).
export DEPLOYMENT_PROFILE=pilot
export DATABASE_TARGETS_JSON='{"pilot-db-1":{...}}'   # ver VARIABLES.md
export DATABASE_SUBSCRIPTION_IDS="[]"                  # full: allowlist no vacía
export VM_SUBSCRIPTION_ID="..."
export SUBSCRIPTION_ID="..."
export FREE_TIER_ENABLED=true   # solo tras confirmar elegibilidad (lectura sola)
./scripts/preflight.sh
```

Si Free Tier no está confirmado, `preflight.sh` se detiene y presenta una
alternativa de bajo costo claramente marcada como facturable. No continuar hasta
resolverlo.

## 1. Revisar variables y validar

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars   # rellenar placeholders; no commitear
terraform init -backend=false -input=false
terraform validate                             # configuración válida, sin Azure
```

## 2. Plan (revisar antes de aplicar)

El plan lee la configuración y compila qué se crearía y en qué suscripción. No
crea recursos. Requiere suscripciones reales conocidas por la sesión de `az`
autenticada (solo lectura):

```bash
cd terraform
terraform plan -var-file=terraform.tfvars -out=tfplan
# Inspeccionar tfplan (o `terraform show tfplan`). Verificar:
#   - un solo target de DB (pilot) y suscripción de cada recurso
#   - VM en vm_subscription_id
#   - SKU, auto-pause, firewall (solo IP del VM), no 0.0.0.0/0
```

**No ejecutar `terraform apply` automáticamente.** El operador revisa el plan y
aplica manualmente cuando confirme targets, suscripciones, costo y Free Tier.

## 3. Aplicar (manual, tras revisar)

```bash
cd terraform
terraform apply tfplan   # solo si el operador revisó y aprobó el plan
```

Cloud-init en la VM instala el runtime, escribe `/opt/cco/.env` (0600), aplica
migraciones y arranca los servicios. `AUTO_START=false` por defecto, así que la
carga espera a un Telegram `/start`.

## Deploy application changes

```bash
./scripts/deploy.sh              # syncs app/ over SSH and restarts services
```

`bootstrap.sh` does the same on the host itself (installs `msodbcsql18`,
venv, dependencies, migrations).

## Control (Telegram)

Only chat ids in `TELEGRAM_ALLOWED_CHAT_IDS` are answered.

| Command     | Effect                                             |
| ----------- | -------------------------------------------------- |
| `/status`   | experiment, collector and spool summary            |
| `/start`    | CREATED/PAUSED -> RUNNING                           |
| `/pause`    | RUNNING -> PAUSING (acknowledged after the batch)  |
| `/resume`   | PAUSED -> RUNNING                                   |
| `/stop`     | -> STOPPING -> STOPPED (safe, flushes the spool)   |
| `/collector`| last collection cycle (rows observed/written)      |
| `/spool`    | pending local buffer size                          |

## Services

```bash
systemctl status cco-collector cco-telegram-bot cco-supervisor
journalctl -u cco-collector -f
journalctl -u cco-workload -n 200
```

`cco-supervisor` forces a safe stop once the experiment exceeds
`AUTO_STOP_AFTER_HOURS`; it protects against a forgotten experiment.

## Spool recovery

If Supabase is unreachable the collector spools batches locally and marks the
cycle `partial`. Once connectivity returns, the next cycle flushes the buffer
automatically. Inspect it with `/spool`; the files live under
`LOCAL_SPOOL_DIR/workload_obs` (workload observations) and `LOCAL_SPOOL_DIR`
(readings/samples). Do not delete spool files by hand unless you accept data
loss.

## Common problems

| Symptom | Check |
| ------- | ----- |
| collector shows `partial` | Supabase reachable? `/spool` growing? `journalctl -u cco-collector`. |
| workload never starts | `AUTO_START=true` in `/opt/cco/.env`; send `/start`; check `cco-workload` logs. |
| `azure status: unknown` | ODBC driver installed and network path to Azure SQL open. |
| bot silent | token valid (`getMe`), your chat id is allowlisted, `cco-telegram-bot` running. |
| percentiles are `NULL` | No individual samples in that hour — expected for aggregate-only metrics. |

## Teardown

```bash
./scripts/cost-check.sh          # review spend first
./scripts/destroy.sh             # requires confirmation; safe-stops first
```

`destroy.sh` performs a safe stop before `terraform destroy`, so buffered data
is flushed (or explicitly flagged as pending) before the VM disappears.