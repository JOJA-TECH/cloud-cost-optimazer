# Cloud Cost Optimizer - collector application

Python application that runs on the collector VM. It generates a controlled
Azure SQL workload, samples platform + database metrics, persists them to
Supabase and exposes control over Telegram.

The VM is the only component that knows both databases:

```
        +-------------------+        metrics        +------------------+
        |   Azure SQL       |  <--- DMV / QS ---   |                  |
   loadtest schema          |                      |   collector VM   |
   workload_control  <------+---- state machine ---|  (this app)      |
                                                   |  - workload gen  |
                                                   |  - collector     |
                                                   |  - telegram bot  |
                                                   |  - supervisor    |
Supabase (PostgreSQL) <---------------------------/  + local spool
experiment / raw / feat / ops
```

## Layout

```
src/
  config.py              environment-driven configuration (no secrets in code)
  db_azure.py            Azure SQL (pyodbc) - system under test
  db_supabase.py         Supabase PostgreSQL (psycopg2) analytical store
  migrations.py          applies app/migrations/* to both stores
  shutdown.py            ordered safe stop + budget watchdog (--supervise)
  workload/              controlled load generator (patterns, controller, runner)
  metrics/               collection, normalization, quality, aggregation, collector
  sync/                  idempotent writes to Supabase + local spool
  telegram/              command channel (authorization, commands, bot)
migrations/
  azure_sql/             loadtest schema, state machine, workload login
  supabase/              experiment/ops/raw/feat schemas, views, indexes
tests/                   unit tests (no database required)
docs/                    architecture, dataset, operations, costs
```

## Quick start (local, no databases)

```bash
python3 -m venv .venv && . .venv/bin/activate
pip install pytest            # tests do not need DB drivers
pytest
```

To run the collector against real stores, copy `.env.example` to `.env`, fill
the values, install `requirements.txt`, then:

```bash
python -m src.migrations --apply        # create schemas (idempotent)
python -m src.metrics.collector --once  # a single collection cycle
python -m src.metrics.collector --service
```

## Services (VM)

| Unit                    | Entry point                              |
| ----------------------- | ---------------------------------------- |
| `cco-migrate.service`   | `python -m src.migrations --apply`       |
| `cco-collector.service` | `python -m src.metrics.collector --service` |
| `cco-workload.service`  | `python -m src.workload.runner --service` |
| `cco-telegram-bot.service` | `python -m src.telegram.bot --service` |
| `cco-supervisor.service`   | `python -m src.shutdown --supervise`  |

See `docs/OPERATIONS.md` for the runbook and `docs/ARCHITECTURE.md` for the
design. Nothing here commits or logs secrets: configuration arrives only through
the environment (`/opt/cco/.env`, mode 0600).