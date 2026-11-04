# ---------------------------------------------------------------------------
# Azure SQL logical server(s) and database(s) — one per entry in
# var.database_targets (default provider = primary subscription).
#
# TLS 1.2 minimum. Public network access enabled only so the collector VM's
# pinned public IP can be allow-listed; the firewall never opens 0.0.0.0/0 and
# "Allow Azure services" is deliberately not created.
#
# Each target keeps its own identity: ARM resource id and resource_uid. The
# collector routes every read to the correct target and records the
# subscription and resource of origin.
#
# provider-alias limitation: these resources use the default provider
# (var.subscription_id). For a database target in a non-primary subscription
# (profile=full), the operator adds an explicit `provider "azurerm" { alias =
# "db_X" }` block in providers.tf and instantiates this target with that
# provider; Terraform cannot generate aliases dynamically.
# ---------------------------------------------------------------------------

# One resource group per target (primary subscription).
resource "azurerm_resource_group" "sql" {
  for_each = var.database_targets
  name     = each.value.resource_group
  location = each.value.location
  tags = merge(local.tags, {
    sql_target          = each.key
    sql_subscription_id = each.value.subscription_id
  })
}

# Logical server per target.
resource "azurerm_mssql_server" "main" {
  for_each                      = var.database_targets
  provider                      = azurerm
  name                          = each.value.server
  location                      = azurerm_resource_group.sql[each.key].location
  resource_group_name           = azurerm_resource_group.sql[each.key].name
  version                       = "12.0"
  public_network_access_enabled = true
  minimum_tls_version           = "1.2"

  administrator_login          = var.sql_admin_username
  administrator_login_password = var.sql_admin_password

  tags = merge(local.tags, {
    sql_target          = each.key
    sql_subscription_id = each.value.subscription_id
  })
}

# Serverless database per target. Free Tier / auto-pause honoured per target.
resource "azurerm_mssql_database" "loadtest" {
  for_each  = var.database_targets
  provider  = azurerm
  name      = each.value.database
  server_id = azurerm_mssql_server.main[each.key].id
  collation = "SQL_Latin1_General_CP1_CI_AS"

  sku_name                    = each.value.sku_name
  min_capacity                = each.value.min_capacity
  auto_pause_delay_in_minutes = each.value.auto_pause_delay_minutes
  max_size_gb                 = each.value.max_size_gb
  zone_redundant              = false
  read_scale                  = false

  tags = merge(local.tags, {
    sql_target          = each.key
    sql_subscription_id = each.value.subscription_id
  })

  lifecycle {
    ignore_changes = []
  }
}

# The app subnet has the Microsoft.Sql service endpoint, so Azure SQL sees the
# VM's private IP: an IP rule is not enough, the subnet must be allowed.
resource "azurerm_mssql_virtual_network_rule" "vm" {
  for_each  = var.database_targets
  provider  = azurerm
  name      = "Allow-Collector-Subnet-${each.key}"
  server_id = azurerm_mssql_server.main[each.key].id
  subnet_id = azurerm_subnet.app.id
}

# Firewall per target: allow ONLY the collector VM public IP.
resource "azurerm_mssql_firewall_rule" "vm" {
  for_each         = var.enable_public_ip ? var.database_targets : {}
  provider         = azurerm
  name             = "Allow-Collector-VM-${each.key}"
  server_id        = azurerm_mssql_server.main[each.key].id
  start_ip_address = azurerm_public_ip.vm.ip_address
  end_ip_address   = azurerm_public_ip.vm.ip_address
}

# ARM resource id per target, exposed for the collector's Azure Monitor reads.
locals {
  sql_target_resource_ids = {
    for key, target in var.database_targets : key => {
      id              = azurerm_mssql_database.loadtest[key].id
      subscription_id = target.subscription_id
      resource_group  = target.resource_group
      server          = target.server
      database        = target.database
      resource_uid    = target.resource_uid
    }
  }
}
