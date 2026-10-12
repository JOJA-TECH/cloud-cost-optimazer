# ---------------------------------------------------------------------------
# Azure SQL logical server (system under test)
#
# TLS 1.2 minimum. Public network access is enabled only so the VM's pinned
# public IP can be allow-listed; the firewall never opens 0.0.0.0/0 and the
# "Allow Azure services" rule is deliberately not created.
# ---------------------------------------------------------------------------
resource "azurerm_mssql_server" "main" {
  name                          = local.name.sql_server
  location                      = azurerm_resource_group.main.location
  resource_group_name           = azurerm_resource_group.main.name
  version                       = "12.0"
  public_network_access_enabled = true
  minimum_tls_version           = "1.2"

  administrator_login          = var.sql_admin_username
  administrator_login_password = var.sql_admin_password

  tags = local.tags
}

# ---------------------------------------------------------------------------
# Serverless General Purpose database, 0.5-1 vCore, no zone redundancy,
# no elastic pool, minimum storage, auto-pause enabled.
# ---------------------------------------------------------------------------
resource "azurerm_mssql_database" "loadtest" {
  name      = local.name.sql_database
  server_id = azurerm_mssql_server.main.id
  collation = "SQL_Latin1_General_CP1_CI_AS"

  sku_name                    = local.sql_sku_name
  min_capacity                = var.sql_min_capacity
  auto_pause_delay_in_minutes = local.sql_auto_pause_delay_minutes
  max_size_gb                 = local.sql_max_size_gb
  zone_redundant              = false
  read_scale                  = false

  tags = local.tags

  lifecycle {
    ignore_changes = []
  }
}

# ---------------------------------------------------------------------------
# Firewall: allow ONLY the VM public IP to reach the logical server.
# The VM itself reaches the database over the Azure SQL public endpoint
# (tcp/1433). No private endpoint is created to keep the cost minimal.
# ---------------------------------------------------------------------------
resource "azurerm_mssql_firewall_rule" "vm" {
  count            = var.enable_public_ip ? 1 : 0
  name             = "Allow-Collector-VM"
  server_id        = azurerm_mssql_server.main.id
  start_ip_address = azurerm_public_ip.vm[0].ip_address
  end_ip_address   = azurerm_public_ip.vm[0].ip_address
}
