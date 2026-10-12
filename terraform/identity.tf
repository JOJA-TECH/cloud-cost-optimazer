# ---------------------------------------------------------------------------
# Least-privilege control-plane access for the VM's system-assigned identity.
# Only read scopes are granted: the VM observes, it does not administer.
# ---------------------------------------------------------------------------
resource "azurerm_role_assignment" "vm_monitoring_reader" {
  scope                = azurerm_resource_group.main.id
  role_definition_name = "Monitoring Reader"
  principal_id         = azurerm_linux_virtual_machine.vm.identity[0].principal_id
}

resource "azurerm_role_assignment" "vm_reader" {
  scope                = azurerm_resource_group.main.id
  role_definition_name = "Reader"
  principal_id         = azurerm_linux_virtual_machine.vm.identity[0].principal_id
}
