# =============================================================================
# Terraform variables para Azure SQL Database (pruebas E2E Release 1.0)
# =============================================================================

# Suscripción de Azure (proporcionada por el usuario)
subscription_id = "9490c340-5fd8-44b1-8995-0fa8ad71c767"
tenant_id       = "25f92c31-7566-45dc-aca9-1442bfbadaa6"

# Grupo de recursos
resource_group_name = "rg-proyecto-grado"
location            = "eastus"

# Servidor Azure SQL
sql_server_name          = "sql-proyecto-grado-e2e"
sql_administrator_login  = "cco_admin"
sql_administrator_password = "CcoProyectoGrado2026!"  # CAMBIAR en producción

# Base de datos
sql_database_name = "cco-e2e-test"
sku_name          = "GP_S_Gen5_1"  # SKU serverless 1 vCore
sku_tier          = "GeneralPurpose"
sku_family        = "Gen5"
sku_capacity      = 1.0

# Firewall: permitir acceso desde la IP local (para pruebas E2E)
# Reemplaza con tu IP pública actual
firewall_start_ip = "0.0.0.0"
firewall_end_ip   = "255.255.255.255"

# Tags
environment = "e2e-testing"
project     = "cloud-cost-optimizer"
managed_by  = "terraform"
