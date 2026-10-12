# dataset-metrics-collector

Reproducible experiment platform for **Cloud Cost Optimizer** built on a strict
**two-database** separation:

```text
Azure SQL Database      -> system under test (controlled load + diagnostics)
Azure Linux VM          -> workload generator + collector + normalizer + sync + Telegram bot
Supabase PostgreSQL     -> persistent analytical store (metrics, features, datasets)
```

Terraform provisions the Azure side exclusively. The VM is the only application
that knows both database connections. Telegram controls the experiment. The
dataset survives even when the VM is stopped or the Azure side is destroyed.

## Repository layout

```text
.
├── databases/cco_data/     # existing analytical schema (PostgreSQL reference)
├── terraform/              # Azure infrastructure (owns Azure only)
├── app/                    # Python application executed on the VM
│   ├── src/                # config, db clients, workload, metrics, sync, telegram
│   ├── migrations/         # azure_sql (T-SQL) and supabase (SQL) migrations
│   ├── docs/               # ARCHITECTURE, COSTS, DATASET, OPERATIONS
│   └── tests/
├── systemd/                # systemd unit templates installed on the VM
└── scripts/                # bootstrap, deploy, validate, cost-check, destroy
```

## Status legend (never claim what was not proven)

| State | Meaning |
|---|---|
| implemented in code | files exist and are self-consistent |
| validated locally | `terraform validate`, `pytest`, `py_compile` passed locally |
| validated against Azure | connectivity/plan checked against a live subscription |
| validated against Supabase | migrations applied to the real Supabase project |
| actually deployed | resources exist and are running |

This branch is **implemented in code** and **validated locally** only. No
`terraform apply` and no push are performed. See `app/docs/OPERATIONS.md`.

## Quick start

```bash
cp terraform/terraform.tfvars.example terraform/terraform.tfvars  # fill secrets
cp app/.env.example app/.env                                      # fill secrets
./scripts/validate.sh
./scripts/cost-check.sh
cd terraform && terraform init && terraform plan -out=tfplan
```

Never commit `terraform.tfvars`, `.env`, tokens or passwords.
