#!/bin/bash
# =============================================================================
# Preparar servidor Python ML para pruebas E2E
# =============================================================================
set -euo pipefail

REPO_DIR="/home/chaos/Projects/universidad/proyecto-grado/develop"
cd "$REPO_DIR"

# Activar entorno virtual
source .venv/bin/activate

# Variables de entorno
export FORECAST_MODEL_DIR="/tmp/forecast_models_test"
export FORECAST_DATA_DSN="postgresql://postgres:example@127.0.0.1:5432/cco_data?sslmode=disable"
export FORECAST_GRPC_ADDR="127.0.0.1:50051"
export PYTHONPATH="$REPO_DIR/src"

# Recurso de prueba (el de Supabase con más datos: 91 filas)
export E2E_TEST_RESOURCE_UID="6bb20d1f-cc04-fe5c-a992-39fbb17f6d02"

echo "=== Entrenando modelo con datos de Supabase (cco_data) ==="
python -m forecasting.training train \
  --resource-uid "$E2E_TEST_RESOURCE_UID" \
  --target target_cpu_avg_next1h \
  --model-name cpu_forecast_1h \
  --model-version 1.0.0 \
  --algorithm random_forest \
  --hours 720 \
  2>&1 | tail -10

echo ""
echo "=== Promoviendo modelo a champion ==="
python -m forecasting.training promote \
  --model-name cpu_forecast_1h \
  --model-version 1.0.0 \
  2>&1 | tail -5

echo ""
echo "=== Listando modelos ==="
python -m forecasting.training list \
  2>&1

echo ""
echo "=== Para iniciar el servidor gRPC: ==="
echo "FORECAST_MODEL_DIR=/tmp/forecast_models_test \\"
echo "FORECAST_DATA_DSN=\"postgresql://postgres:example@127.0.0.1:5432/cco_data?sslmode=disable\" \\"
echo "FORECAST_GRPC_ADDR=127.0.0.1:50051 \\"
echo "PYTHONPATH=src python -m forecasting.serving.server"
