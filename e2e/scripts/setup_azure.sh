#!/bin/bash
# =============================================================================
# Script de setup para pruebas E2E del pipeline ML Release 1.0
# =============================================================================
# Este script configura todo el entorno para pruebas E2E:
# 1. Crea el recurso Azure SQL Database en tu suscripción
# 2. Configura el entorno Go y Python
# 3. Ejecuta las pruebas E2E
#
# Requisitos previos:
#   - Azure CLI instalado (az)
#   - Terraform instalado (>= 1.5.0)
#   - Credenciales de Azure con permisos en la suscripción
#   - .env configurado con CCO_OIDC_* para Entra ID
#
# Uso:
#   ./e2e/scripts/setup_azure.sh
# =============================================================================
set -euo pipefail

REPO_DIR="/home/chaos/Projects/universidad/proyecto-grado/develop"
cd "$REPO_DIR"

echo "============================================================"
echo "SETUP E2E - Pipeline ML Release 1.0 con Azure SQL"
echo "============================================================"

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# -----------------------------------------------------------------------------
# Paso 1: Verificar requisitos previos
# -----------------------------------------------------------------------------
log_info "Paso 1: Verificando requisitos previos..."

if ! command -v az &> /dev/null; then
    log_error "Azure CLI no está instalado. Instala desde: https://learn.microsoft.com/azure/cli/install-azure-cli"
    exit 1
fi
log_info "Azure CLI: $(az version --query '"azure-cli"' -o tsv 2>/dev/null || echo 'desconocido')"

if ! command -v terraform &> /dev/null; then
    log_error "Terraform no está instalado. Instala desde: https://developer.hashicorp.com/terraform/tutorials/aws-get-started/install-cli"
    exit 1
fi
log_info "Terraform: $(terraform version -json 2>/dev/null | python3 -c 'import sys,json; print(json.load(sys.stdin)["terraform_version"])' 2>/dev/null || echo 'desconocido')"

if ! command -v go &> /dev/null; then
    log_error "Go no está instalado."
    exit 1
fi
log_info "Go: $(go version)"

# -----------------------------------------------------------------------------
# Paso 2: Autenticación en Azure
# -----------------------------------------------------------------------------
log_info "Paso 2: Autenticando en Azure..."

az account show &> /dev/null || {
    log_warn "No hay sesión activa de Azure. Iniciando sesión..."
    az login --tenant 25f92c31-7566-45dc-aca9-1442bfbadaa6
}

az account set --subscription 9490c340-5fd8-44b1-8995-0fa8ad71c767

SUBSCRIPTION=$(az account show --query '"name"' -o tsv 2>/dev/null)
TENANT=$(az account show --query '"tenantId"' -o tsv 2>/dev/null)
log_info "Suscripción activa: $SUBSCRIPTION ($TENANT)"

# -----------------------------------------------------------------------------
# Paso 3: Crear Azure SQL Database con Terraform
# -----------------------------------------------------------------------------
log_info "Paso 3: Creando Azure SQL Database con Terraform..."

cd terraform

terraform init

terraform apply -auto-approve

# Obtener outputs
SQL_SERVER=$(terraform output -raw sql_server_name)
SQL_DATABASE=$(terraform output -raw sql_database_name)
SQL_FQDN=$(terraform output -raw sql_server_fully_qualified_domain_name)
SQL_RESOURCE_ID=$(terraform output -raw sql_resource_id)
RESOURCE_GROUP=$(terraform output -raw resource_group_name)
LOCATION=$(terraform output -raw location)

log_info "Servidor SQL: $SQL_SERVER"
log_info "Base de datos: $SQL_DATABASE"
log_info "FQDN: $SQL_FQDN"
log_info "Resource ID: $SQL_RESOURCE_ID"

cd "$REPO_DIR"

# -----------------------------------------------------------------------------
# Paso 4: Obtener resource_uid de la base de datos creada
# -----------------------------------------------------------------------------
log_info "Paso 4: Obteniendo resource_uid de la base de datos..."

# El resource_uid en cco_data es el resource_uid de core.resource
# Para Azure SQL, el resource_uid es gen_random_uuid() en core.resource
# Necesitamos crear el recurso en cco_data o consultar si ya existe

# Primero, verificar si el backend puede conectar a Azure y sincronizar recursos
# El resource_uid se genera automáticamente en core.resource

# -----------------------------------------------------------------------------
# Paso 5: Configurar variables de entorno para el backend
# -----------------------------------------------------------------------------
log_info "Paso 4: Configurando variables de entorno..."

# Actualizar .env con los valores de Azure
cat >> .env << 'EOF'

# =============================================================================
# Azure SQL Database creado por Terraform (pruebas E2E)
# =============================================================================
CCO_AZURE_SERVER_NAME='$SQL_SERVER'
CCO_AZURE_DATABASE_NAME='$SQL_DATABASE'
CCO_AZURE_RESOURCE_GROUP='$RESOURCE_GROUP'
CCO_AZURE_REGION='$LOCATION'
CCO_AZURE_RESOURCE_ID='$SQL_RESOURCE_ID'
EOF

log_info "Variables de entorno actualizadas en .env"

# -----------------------------------------------------------------------------
# Paso 5: Iniciar el backend Go
# -----------------------------------------------------------------------------
log_info "Paso 5: Iniciando backend Go..."

cd backend

# Cargar variables de entorno
set -a
source ../.env
set +a

# Construir el backend
go build -o cco-app ./cmd/cco-app/

# Iniciar el backend en background
./cco-app &
BACKEND_PID=$!
log_info "Backend iniciado con PID $BACKEND_PID"

# Esperar a que el backend esté listo
sleep 5

# Verificar que el backend responde
if curl -s http://localhost:8080/healthz &> /dev/null; then
    log_info "Backend respondiendo en http://localhost:8080"
else
    log_error "Backend no respondió. Revisa los logs."
    kill $BACKEND_PID 2>/dev/null
    exit 1
fi

cd "$REPO_DIR"

# -----------------------------------------------------------------------------
# Paso 6: Iniciar el servidor Python ML
# -----------------------------------------------------------------------------
log_info "Paso 6: Iniciando servidor Python ML..."

source .venv/bin/activate
export FORECAST_MODEL_DIR="/tmp/forecast_models"
export FORECAST_DATA_DSN="postgresql://postgres:example@127.0.0.1:5432/cco_data?sslmode=disable"
export FORECAST_GRPC_ADDR="127.0.0.1:50051"
export PYTHONPATH="$REPO_DIR/src"

# Entrenar modelo si no existe
if [ ! -d "/tmp/forecast_models/target_cpu_avg_next1h/1.0.0" ]; then
    log_info "Entrenando modelo inicial..."
    python -m forecasting.training_cli train \
        --resource-uid "$E2E_TEST_RESOURCE_UID" \
        --target target_cpu_avg_next1h \
        --model-name cpu_forecast_1h \
        --model-version 1.0.0 \
        --algorithm random_forest \
        --hours 720 2>&1 | tail -5
fi

# Promover a champion
python -m forecasting.training_cli promote \
    --model-name cpu_forecast_1h \
    --model-version 1.0.0 2>&1 | tail -3

# Iniciar servidor gRPC en background
python -m forecasting.serving.server &
PYTHON_PID=$!
log_info "Servidor Python ML iniciado con PID $PYTHON_PID"

sleep 3

# -----------------------------------------------------------------------------
# Paso 7: Ejecutar pruebas E2E
# -----------------------------------------------------------------------------
log_info "Paso 7: Ejecutando pruebas E2E..."

python e2e/scripts/run_e2e.py

# -----------------------------------------------------------------------------
# Paso 8: Limpiar
# -----------------------------------------------------------------------------
log_info "Paso 8: Limpiando procesos..."

kill $BACKEND_PID 2>/dev/null
kill $PYTHON_PID 2>/dev/null

log_info "Pruebas E2E completadas."
