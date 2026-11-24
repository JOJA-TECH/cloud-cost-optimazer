#!/bin/bash
set -e
cd "$(dirname "$0")"

if [ -f ../.env ]; then
    set -a
    . ../.env
    set +a
fi

PG_CONTAINER="${CCO_POSTGRES_CONTAINER:-database-db-1}"
PG_PORT="${CCO_POSTGRES_PORT:-5432}"
PG_PASSWORD="${CCO_POSTGRES_PASSWORD:-example}"

for var in CCO_OIDC_ISSUER_URL CCO_OIDC_CLIENT_ID CCO_OIDC_CLIENT_SECRET CCO_OIDC_REDIRECT_URL; do
    if [ -z "${!var:-}" ]; then
        echo "ERROR: falta $var en ../.env (o en el entorno). Entra ID no se habilitará."
        exit 1
    fi
done

echo "=== Asegurando PostgreSQL local ($PG_CONTAINER:$PG_PORT) ==="
if ! docker ps -a --format '{{.Names}}' | grep -qx "$PG_CONTAINER"; then
    docker run -d --name "$PG_CONTAINER" \
        -e "POSTGRES_PASSWORD=$PG_PASSWORD" \
        -p "${PG_PORT}:5432" \
        postgres:17
fi
docker start "$PG_CONTAINER" > /dev/null 2>&1 || true

for i in $(seq 1 30); do
    if docker exec "$PG_CONTAINER" pg_isready -U postgres > /dev/null 2>&1; then
        break
    fi
    echo "Esperando a PostgreSQL... ($i/30)"
    sleep 1
done
docker exec "$PG_CONTAINER" pg_isready -U postgres

load_baseline() {
    local db="$1" file="$2"
    echo "Cargando esquema base de $db desde $file"
    if ! grep -v '^\\\(un\)\?restrict ' "$file" | \
        docker exec -i "$PG_CONTAINER" psql -U postgres -v ON_ERROR_STOP=1 -q -d "$db"; then
        echo "ERROR: no se pudo cargar $file"
        exit 1
    fi
}

for db in cco_app cco_data; do
    case "$db" in
        cco_app) baseline="../databases/cco_app/cco_app.sql" ;;
        cco_data) baseline="../databases/cco_data/cco_data_schema.sql" ;;
    esac
    if ! docker exec "$PG_CONTAINER" psql -U postgres -tAc \
        "SELECT 1 FROM pg_database WHERE datname = '$db'" | grep -q 1; then
        docker exec "$PG_CONTAINER" psql -U postgres -c "CREATE DATABASE $db"
        load_baseline "$db" "$baseline"
    fi
done

echo "=== Compilando backend ==="
go build -o cco-app ./cmd/cco-app/

echo "=== Smoke test del backend ==="
./start_and_test.sh

echo "=== Verificando readiness y OIDC ==="
curl -fsS http://localhost:8080/readyz > /dev/null
OIDC_STATUS=$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8080/auth/oidc/login)
echo "OIDC login: HTTP $OIDC_STATUS"
if [ "$OIDC_STATUS" != "302" ]; then
    echo "ERROR: /auth/oidc/login devolvió $OIDC_STATUS; revisa las variables CCO_OIDC_*"
    exit 1
fi

echo "=== Sistema inicializado ==="
