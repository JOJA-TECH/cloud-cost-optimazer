# ---------------------------------------------------------------------------
# Non-sensitive outputs only. Never output passwords, tokens or connection
# strings that embed credentials.
# ---------------------------------------------------------------------------
output "experiment_uid" {
  value       = local.experiment_uid
  description = "Correlation id shared by Azure SQL and Supabase rows."
}

output "resource_group_name" {
  value       = azurerm_resource_group.main.name
  description = "Resource group that contains the whole experiment."
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
  value       = var.enable_public_ip ? azurerm_public_ip.vm[0].ip_address : null
  description = "VM public IP (pinned in the Azure SQL firewall)."
}

output "sql_server_fqdn" {
  value       = azurerm_mssql_server.main.fully_qualified_domain_name
  description = "Azure SQL logical server host name."
}

output "sql_server_admin_login" {
  value       = azurerm_mssql_server.main.administrator_login
  sensitive   = true
  description = "Azure SQL admin login (name only, never the password)."
}

output "sql_database_name" {
  value       = azurerm_mssql_database.loadtest.name
  description = "Azure SQL database under test."
}

output "sql_database_sku" {
  value       = azurerm_mssql_database.loadtest.sku_name
  description = "Effective serverless SKU."
}

output "vm_identity_principal_id" {
  value       = azurerm_linux_virtual_machine.vm.identity[0].principal_id
  description = "Managed identity principal id used for Azure Monitor reads."
}

output "ssh_connection" {
  value       = var.enable_public_ip ? "ssh ${var.admin_username}@${azurerm_public_ip.vm[0].ip_address}" : "use a private network path to ${azurerm_network_interface.vm.private_ip_address}"
  description = "Suggested SSH command (no credential embedded)."
}
