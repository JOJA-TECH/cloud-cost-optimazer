#!/bin/bash
cd "$(dirname "$0")"
if [ -f ../.env ]; then
    set -a
    . ../.env
    set +a
fi
export CCO_POSTGRES_PORT="${CCO_POSTGRES_PORT:-5432}"
export CCO_POSTGRES_PASSWORD="${CCO_POSTGRES_PASSWORD:-example}"
export CCO_DB_DSN="postgresql://postgres:${CCO_POSTGRES_PASSWORD}@127.0.0.1:${CCO_POSTGRES_PORT}/cco_app?sslmode=disable"
export CCO_DATA_DSN="postgresql://postgres:${CCO_POSTGRES_PASSWORD}@127.0.0.1:${CCO_POSTGRES_PORT}/cco_data?sslmode=disable"
export CCO_DB_DRIVER="postgres"
export CCO_JWT_SIGNING_KEY="clave-secreta-para-e2e"
export CCO_HTTP_ADDR=":8080"
export CCO_ENV="dev"
export CCO_LOG_FORMAT="json"
export CCO_ALLOWED_ORIGINS="*"
export CCO_COOKIE_SECURE="false"
echo "CCO_DB_DRIVER=$CCO_DB_DRIVER"
echo "CCO_DB_DSN=$CCO_DB_DSN"
echo "CCO_DATA_DSN=$CCO_DATA_DSN"
./cco-app
