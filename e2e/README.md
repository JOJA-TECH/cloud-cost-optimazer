# Pruebas E2E - Pipeline ML Release 1.0

## Resumen

Este documento describe cómo ejecutar las pruebas end-to-end del pipeline ML (forecasting + recomendación) utilizando:

1. **Backend Go** - API autenticada con Entra ID (OIDC)
2. **Servicio Python ML** - gRPC server para predicciones
3. **Base de datos Azure SQL** - Creada en tu suscripción AzureCloud
4. **Base de datos cco_data local** - Para pruebas con datos de Supabase

## Prerrequisitos

### Software instalado
- Go 1.25+
- Python 3.11+ con entorno virtual (.venv)
- Azure CLI (`az`)
- Terraform >= 1.5.0
- PostgreSQL cliente (`psql`)

### Credenciales necesarias

#### 1. Suscripción de Azure (ya proporcionada)
- **Subscription ID**: `9490c340-5fd8-44b1-8995-0fa8ad71c767`
- **Tenant ID**: `25f92c31-7566-45dc-aca9-1442bfbadaa6`
- **Nombre**: AzureCloud
- **Estado**: Enabled

#### 2. Entra ID (OIDC) - Para autenticación
Necesitas registrar una aplicación en Entra ID y proporcionar:
- `CCO_OIDC_CLIENT_ID` - ID de cliente de la app
- `CCO_OIDC_CLIENT_SECRET` - Secreto de cliente

#### 3. Service Principal de Azure - Para adquisición de Azure Monitor
- `CCO_AZURE_CLIENT_ID` - ID del Service Principal
- `CCO_AZURE_CLIENT_SECRET` - Secreto del Service Principal

## Pasos para ejecutar las pruebas E2E

### Paso 1: Configurar variables de entorno

El archivo `.env` en la raíz del proyecto ya contiene los valores de tu suscripción de Azure. Edita el archivo y completa los valores faltantes:

```bash
# Autenticación Entra ID (obtén estos valores de Azure Portal > App Registrations)
CCO_OIDC_CLIENT_ID=TU_CLIENT_ID
CCO_OIDC_CLIENT_SECRET=TU_CLIENT_SECRET

# Service Principal para Azure SDK (obtén estos valores de Azure Portal)
CCO_AZURE_CLIENT_ID=TU_SP_CLIENT_ID
CCO_AZURE_CLIENT_SECRET=TU_SP_CLIENT_SECRET

# Usuario de prueba para el backend (debe existir en cco_app)
E2E_TEST_EMAIL=admin@cco.local
E2E_TEST_PASSWORD=Admin123!
E2E_TEST_RESOURCE_UID=6bb20d1f-cc04-fe5c-a992-39fbb17f6d02
```

### Paso 2: Crear Azure SQL Database

```bash
cd /home/chaos/Projects/universidad/proyecto-grado/develop

# Autenticar en Azure
az login --tenant 25f92c31-7566-45dc-aca9-1442bfbadaa6
az account set --subscription 9490c340-5fd8-44b1-8995-0fa8ad71c767

# Inicializar y aplicar Terraform
cd terraform
terraform init
terraform apply -auto-approve
cd ..

# Esto creará:
# - Grupo de recursos: rg-proyecto-grado
# - Servidor SQL: sql-proyecto-grado-e2e
# - Base de datos: cco-e2e-test (GP_S_Gen5_1 serverless)
```

### Paso 3: Migrar datos de Supabase a cco_data (opcional)

Si tienes datos en Supabase que quieres usar para pruebas:

```bash
source .env
export SOURCE_DSN="tu_dsn_de_supabase"
export CCO_DATA_DSN="postgresql://postgres:example@127.0.0.1:5432/cco_data"

python e2e/scripts/migrate_supabase_to_cco_data.py
```

### Paso 4: Entrenar modelo ML

```bash
source .env
source .venv/bin/activate

python -m forecasting.training_cli train \
  --resource-uid $E2E_TEST_RESOURCE_UID \
  --target target_cpu_avg_next1h \
  --model-name cpu_forecast_1h \
  --model-version 1.0.0 \
  --algorithm random_forest \
  --hours 720

python -m forecasting.training_cli promote \
  --model-name cpu_forecast_1h \
  --model-version 1.0.0
```

### Paso 5: Iniciar el servidor Python ML

```bash
source .env
source .venv/bin/activate

export FORECAST_MODEL_DIR="/tmp/forecast_models"
export FORECAST_DATA_DSN="postgresql://postgres:example@127.0.0.1:5432/cco_data?sslmode=disable"
export FORECAST_GRPC_ADDR="127.0.0.1:50051"
export PYTHONPATH="$PWD/src"

python -m forecasting.serving.server &
```

### Paso 6: Iniciar el backend Go

```bash
source .env
source .venv/bin/activate  # si es necesario

cd backend
go build -o cco-app ./cmd/cco-app/
./cco-app &
```

### Paso 7: Ejecutar pruebas E2E

```bash
source .env
source .venv/bin/activate
export PYTHONPATH="$PWD/src"

python e2e/scripts/run_e2e.py
```

### Paso 8: Usar el script automatizado (opcional)

```bash
chmod +x e2e/scripts/setup_azure.sh
./e2e/scripts/setup_azure.sh
```

## Estructura de archivos creados

```
databases/cco_data/roles/
├── ml_service.sql              # Rol app_mlservice con permisos mínimos
└── promotion_policy.sql        # Tabla ml.promotion_policy + función ml.can_promote()

e2e/scripts/
├── migrate_supabase_to_cco_data.py   # Migrar datos Supabase → cco_data
├── prepare_ml_server.sh             # Preparar servidor ML con datos locales
├── e2e_forecast_test.py             # Pruebas E2E completas
├── setup_e2e_azure.sh              # Setup completo con Azure
└── README_E2E.md                    # Este documento

terraform/
├── main.tf                        # Configuración Terraform Azure SQL
└── terraform.tfvars               # Variables con tu suscripción

.env                               # Variables de entorno (tu suscripción AzureCloud)
```

## Flujo de pruebas E2E

1. **Autenticación**: Login via Entra ID (OIDC) o contraseña dev
2. **Forecast**: Solicitar predicción vía Jobs worker → Python ML → `ml.forecast`
3. **Verificación**: Consultar forecast vía `contract.v_forecast`
4. **Modelos**: Listar versiones registradas vía `GET /api/v1/ml/models`
5. **Recomendación**: El motor Go consume el forecast y produce `no_action` si no hay evidencia suficiente

## Códigos de estado esperados

| Endpoint | Estado esperado | Descripción |
|----------|----------------|-------------|
| `GET /healthz` | 200 | Backend vivo |
| `GET /auth/oidc/login` | 302 | Redirect a Entra ID |
| `POST /auth/login` | 200 | Login dev exitoso |
| `POST /api/v1/jobs/analytics` | 202 | Job encolado |
| `GET /api/v1/jobs/{id}` | 200 | Estado del job |
| `GET /api/v1/analytics/forecasts` | 200 | Lista de forecasts |
| `GET /api/v1/ml/models` | 200 | Lista de modelos |

## Solución de problemas

### Error: "Forecast UID vacío"
- Verifica que el servidor Python ML esté corriendo en puerto 50051
- Verifica que `FORECAST_DATA_DSN` apunte a la base correcta
- Verifica que existan datos en `feat.feature_snapshot` para el recurso

### Error: "Modelo no encontrado"
- Entrena un modelo primero con `training_cli train`
- Promueve el modelo a champion con `training_cli promote`

### Error: "ML service not available"
- Verifica que `CCO_ML_MODE=grpc` y `CCO_ML_ENDPOINT=127.0.0.1:50051`
- Verifica que el servidor Python esté escuchando en el puerto 50051

### Error de autenticación Azure
- Verifica que `az login` se haya ejecutado con el tenant correcto
- Verifica que la suscripción `9490c340-5fd8-44b1-8995-0fa8ad71c767` esté activa
- Verifica que tu usuario tenga permisos en la suscripción

## Contacto

Para problemas con Azure, verifica en Azure Portal:
- Suscripción: AzureCloud (9490c340-...)
- Tenant: 25f92c31-7566-45dc-aca9-1442bfbadaa6
- Regiones disponibles para tu suscripción
