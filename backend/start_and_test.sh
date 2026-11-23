#!/bin/bash
set -e
cd "$(dirname "$0")"

if [ -f ../.env ]; then
    set -a
    . ../.env
    set +a
fi

echo "=== Configurando variables de entorno ==="
export CCO_DB_DSN="postgresql://postgres:example@127.0.0.1:5432/cco_app?sslmode=disable"
export CCO_DATA_DSN="postgresql://postgres:example@127.0.0.1:5432/cco_data?sslmode=disable"
export CCO_DB_DRIVER="postgres"
export CCO_JWT_SIGNING_KEY="clave-secreta-para-e2e"
export CCO_HTTP_ADDR=":8080"
export CCO_ENV="dev"
export CCO_LOG_FORMAT="json"
export CCO_ALLOWED_ORIGINS="*"
export CCO_COOKIE_SECURE="false"

echo "CCO_DB_DSN=$CCO_DB_DSN"
echo "CCO_DATA_DSN=$CCO_DATA_DSN"
echo "CCO_DB_DRIVER=$CCO_DB_DRIVER"
echo "CCO_HTTP_ADDR=$CCO_HTTP_ADDR"
echo "CCO_ENV=$CCO_ENV"

echo ""
echo "=== Iniciando backend Go ==="
if curl -sf http://localhost:8080/healthz > /dev/null 2>&1; then
    echo "Backend ya está corriendo en http://localhost:8080"
else
    ./cco-app > /tmp/cco-backend.log 2>&1 &
    BACKEND_PID=$!
    echo "Backend PID: $BACKEND_PID"
fi

# Esperar a que el backend esté listo
for i in $(seq 1 10); do
    if curl -s http://localhost:8080/healthz > /dev/null 2>&1; then
        echo "Backend está corriendo en http://localhost:8080"
        break
    fi
    echo "Esperando a que el backend esté listo... ($i/10)"
    sleep 1
done

if ! curl -s http://localhost:8080/healthz > /dev/null 2>&1; then
    echo "ERROR: El backend no está respondiendo"
    cat /tmp/cco-backend.log
    exit 1
fi

# Registrar usuario de prueba
echo ""
echo "=== Registrando usuario de prueba ==="
REGISTER_RESP=$(curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@cco.local","display_name":"Test User","password":"Test123!"}')
echo "Registro: $REGISTER_RESP"

echo ""
echo "=== Asignando rol analyst al usuario de prueba ==="
docker exec database-db-1 psql -U postgres -d cco_app -c \
  "INSERT INTO iam.user_roles (user_id, role_id, assigned_at)
   SELECT u.user_id, r.role_id, now()
   FROM iam.users u, iam.roles r
   WHERE u.email = 'test@cco.local' AND r.role_code = 'analyst'
   ON CONFLICT DO NOTHING;"

# Obtener token de acceso
echo ""
echo "=== Obteniendo token de acceso ==="
LOGIN_RESP=$(curl -s -X POST http://localhost:8080/auth/login -H "Content-Type: application/json" -d '{"email":"test@cco.local","password":"Test123!"}')
TOKEN=$(echo "$LOGIN_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
echo "Token obtenido: ${TOKEN:0:50}..."

# Probar endpoints de analytics
echo ""
echo "=== Probando endpoint /api/v1/analytics/snapshots ==="
SMOKE_RESOURCE_UID=$(docker exec database-db-1 psql -U postgres -d cco_data -tAc "SELECT resource_uid FROM core.resource ORDER BY 1 LIMIT 1;" | tr -d '[:space:]')
curl -s --get "http://localhost:8080/api/v1/analytics/snapshots" \
  --data-urlencode "resource_uid=$SMOKE_RESOURCE_UID" \
  -H "Authorization: Bearer $TOKEN" 2>&1
echo ""

echo ""
echo "=== Probando endpoint /api/v1/ml/models ==="
curl -s -X GET "http://localhost:8080/api/v1/ml/models" -H "Authorization: Bearer $TOKEN" 2>&1
echo ""

echo ""
echo "=== Pruebas E2E completadas ==="
