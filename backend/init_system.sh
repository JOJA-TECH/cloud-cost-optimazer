#!/bin/bash
set -e
cd "$(dirname "$0")"

echo "=== Asegurando PostgreSQL local ==="
if ! docker ps -a --format '{{.Names}}' | grep -qx database-db-1; then
    docker run -d --name database-db-1 \
        -e POSTGRES_PASSWORD=example \
        -p 5432:5432 \
        postgres:17
fi
docker start database-db-1 > /dev/null 2>&1 || true

for i in $(seq 1 30); do
    if docker exec database-db-1 pg_isready -U postgres > /dev/null 2>&1; then
        break
    fi
    echo "Esperando a PostgreSQL... ($i/30)"
    sleep 1
done
docker exec database-db-1 pg_isready -U postgres

for db in cco_app cco_data; do
    if ! docker exec database-db-1 psql -U postgres -tAc \
        "SELECT 1 FROM pg_database WHERE datname = '$db'" | grep -q 1; then
        docker exec database-db-1 psql -U postgres -c "CREATE DATABASE $db"
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
[ "$OIDC_STATUS" = "302" ]

echo "=== Sistema inicializado ==="
