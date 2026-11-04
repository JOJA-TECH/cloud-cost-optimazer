# ---------------------------------------------------------------------------
# Least-privilege control-plane access for the VM's system-assigned identity.
# Only read scopes are granted: the VM observes, it does not administer.
# ---------------------------------------------------------------------------
# Metrics are read at the database scope, which lives in the target resource
# group/subscription — not in the VM's resource group.
resource "azurerm_role_assignment" "vm_monitoring_reader" {
  for_each             = var.database_targets
  scope                = azurerm_mssql_database.loadtest[each.key].id
  role_definition_name = "Monitoring Reader"
  principal_id         = azurerm_linux_virtual_machine.vm.identity[0].principal_id
}

resource "azurerm_role_assignment" "vm_reader" {
  scope                = azurerm_resource_group.vm.id
  role_definition_name = "Reader"
  principal_id         = azurerm_linux_virtual_machine.vm.identity[0].principal_id
}
