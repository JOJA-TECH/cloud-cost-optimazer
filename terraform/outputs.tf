# ---------------------------------------------------------------------------
# Non-sensitive outputs only. Never output passwords, tokens or connection
# strings that embed credentials.
# Every resource is tagged with the subscription in which it is created.
# ---------------------------------------------------------------------------
output "experiment_uid" {
  value       = local.experiment_uid
  description = "Correlation id shared by Azure SQL and Supabase rows."
}

output "deployment_profile" {
  value       = var.deployment_profile
  description = "Active deployment profile: pilot or full."
}

output "vm_subscription_id" {
  value       = var.vm_subscription_id
  sensitive   = true
  description = "Subscription where the collector VM is deployed."
}

output "primary_subscription_id" {
  value       = var.subscription_id
  sensitive   = true
  description = "Primary subscription hosting the database target(s)."
}

output "resource_group_name" {
  value       = azurerm_resource_group.vm.name
  description = "Resource group that contains the collector VM (vm_subscription_id)."
}

output "vm_name" {
  value       = azurerm_linux_virtual_machine.vm.name
  description = "Collector VM name."
}

output "vm_private_ip" {
  value       = azurerm_network_interface.vm.private_ip_address
  description = "VM private IP."
}

output "vm_public_ip" {
  value       = var.enable_public_ip ? azurerm_public_ip.vm.ip_address : null
  description = "VM public IP (pinned in every Azure SQL firewall)."
}

output "vm_identity_principal_id" {
  value       = azurerm_linux_virtual_machine.vm.identity[0].principal_id
  description = "Managed identity principal id used for Azure Monitor reads."
}

output "database_targets" {
  value = {
    for key, target in local.sql_by_target : key => {
      subscription_id      = target.subscription_id
      resource_group       = target.resource_group
      location             = target.location
      server               = target.server
      database             = target.database
      server_fqdn          = target.server_fqdn
      sku_name             = target.sku_name
      free_tier            = target.free_tier
      resource_uid         = target.resource_uid
      database_resource_id = azurerm_mssql_database.loadtest[key].id
      server_resource_id   = azurerm_mssql_server.main[key].id
      resource_group_id    = azurerm_resource_group.sql[key].id
    }
  }
  sensitive   = true
  description = "One entry per database target, with the subscription and resource ids where each was created."
}

output "sql_database_sku" {
  value       = length(var.database_targets) >= 1 ? values(azurerm_mssql_database.loadtest)[0].sku_name : null
  description = "Effective serverless SKU of the primary (pilot) database."
}

output "ssh_connection" {
  value       = var.enable_public_ip ? "ssh ${var.admin_username}@${azurerm_public_ip.vm.ip_address}" : "use a private network path to ${azurerm_network_interface.vm.private_ip_address}"
  description = "Suggested SSH command (no credential embedded)."
}
