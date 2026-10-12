# Operations runbook

## Provision

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars   # fill values; never commit
terraform init
terraform apply
```

Cloud-init on the VM installs the runtime, writes `/opt/cco/.env` (0600),
installs the services and runs migrations. `AUTO_START=false` by default, so the
workload waits for a Telegram `/start`.

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