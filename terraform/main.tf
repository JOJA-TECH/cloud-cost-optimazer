# =============================================================================
# Terraform configuration para Azure SQL Database (pruebas E2E Release 1.0)
# =============================================================================

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
}

provider "azurerm" {
  features {}
  subscription_id = var.subscription_id
  tenant_id       = var.tenant_id
}

# =============================================================================
# Resource Group
# =============================================================================
resource "azurerm_resource_group" "e2e" {
  name     = var.resource_group_name
  location = var.location
  tags = var.tags
}

# =============================================================================
# Azure SQL Server
# =============================================================================
resource "azurerm_mssql_server" "e2e" {
  name                         = var.sql_server_name
  resource_group_name          = azurerm_resource_group.e2e.name
  location                     = var.location
  administrator_login          = var.sql_administrator_login
  administrator_login_password = var.sql_administrator_password
  version                      = "12.0"  # SQL Server 2019 (Azure SQL Database)
  min_tls_version              = "1.2"
  public_network_access_enabled = true

  tags = var.tags
}

# =============================================================================
# Azure SQL Database (serverless GP_S_Gen5_1)
# =============================================================================
resource "azurerm_mssql_database" "e2e" {
  name                = var.sql_database_name
  resource_group_name = azurerm_resource_group.e2e.name
  location            = var.location
  server_name         = azurerm_mssql_server.e2e.name
  sku_name            = var.sku_name
  sku_tier            = var.sku_tier
  sku_family          = var.sku_family
  sku_capacity        = var.sku_capacity
  max_size_gb         = 32
  collation           = "SQL_Latin1_General_CP1_CI_AS"

  threat_detection_policy {
    state                 = "Disabled"
    storage_account_access_key = ""
    storage_endpoint      = ""
    retention_days        = 0
  }

  tags = var.tags
}

# =============================================================================
# Firewall rule: permitir acceso desde cualquier IP (solo para pruebas E2E)
# En producción, restringir a IPs específicas
# =============================================================================
resource "azurerm_mssql_server_firewall_rule" "e2e_allow_all" {
  name                = "AllowE2ETesting"
  resource_group_name = azurerm_resource_group.e2e.name
  server_name         = azurerm_mssql_server.e2e.name
  start_ip_address    = var.firewall_start_ip
  end_ip_address      = var.firewall_end_ip
}

# =============================================================================
# Outputs
# =============================================================================
output "sql_server_name" {
  value     = azurerm_mssql_server.e2e.name
  description = "Nombre del servidor Azure SQL"
}

output "sql_server_fully_qualified_domain_name" {
  value     = azurerm_mssql_server.e2e.fully_qualified_domain_name
  description = "FQDN del servidor Azure SQL"
}

output "sql_database_name" {
  value     = azurerm_mssql_database.e2e.name
  description = "Nombre de la base de datos"
}

output "sql_resource_id" {
  value     = azurerm_mssql_database.e2e.id
  description = "Resource ID completo de la base de datos"
}

output "resource_group_name" {
  value     = azurerm_resource_group.e2e.name
}

output "location" {
  value     = azurerm_resource_group.e2e.location
}
