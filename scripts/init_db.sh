#!/usr/bin/env bash
# =============================================================================
# init_db.sh - Initialize the local PostgreSQL database
# =============================================================================
# This script starts the docker compose stack, waits for the database to be
# healthy, and verifies the schema is in place.
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

echo "[init_db] Starting local PostgreSQL..."
docker compose up -d

echo "[init_db] Waiting for health check..."
for i in {1..30}; do
    if docker compose exec -T postgres pg_isready -U cco_forecasting -d cco_forecasting >/dev/null 2>&1; then
        echo "[init_db] Database is ready."
        break
    fi
    sleep 2
    if [[ $i -eq 30 ]]; then
        echo "[init_db] ERROR: Database did not become ready in time."
        exit 1
    fi
done

echo "[init_db] Verifying schema..."
docker compose exec -T postgres psql -U cco_forecasting -d cco_forecasting -c "
    select table_schema || '.' || table_name as table_name
    from information_schema.tables
    where table_schema in ('catalog','core','raw','feat','ml','opt','contract','synthetic','forecasting')
      and table_type = 'BASE TABLE'
    order by 1;
"

echo "[init_db] Done."