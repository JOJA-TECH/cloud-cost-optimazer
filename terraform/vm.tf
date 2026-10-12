# ---------------------------------------------------------------------------
# Collector VM (workload generator + collector + normalizer + sync + bot).
# System-assigned managed identity is enabled for Azure control-plane reads.
# No Azure credentials are stored on disk: azure-identity uses the IMDS.
# ---------------------------------------------------------------------------
resource "azurerm_linux_virtual_machine" "vm" {
  name                            = local.name.vm
  computer_name                   = "cco-collector"
  location                        = azurerm_resource_group.main.location
  resource_group_name             = azurerm_resource_group.main.name
  size                            = var.vm_size
  admin_username                  = var.admin_username
  disable_password_authentication = true
  tags                            = local.tags

  network_interface_ids = [azurerm_network_interface.vm.id]

  identity {
    type = "SystemAssigned"
  }

  admin_ssh_key {
    username   = var.admin_username
    public_key = var.admin_ssh_public_key
  }

  os_disk {
    name                 = "${local.name.vm}-osdisk"
    caching              = "ReadWrite"
    storage_account_type = "Standard_LRS"
    disk_size_gb         = 30
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "0001-com-ubuntu-server-jammy"
    sku       = "22_04-lts-gen2"
    version   = "latest"
  }

  # custom_data contains the experiment configuration, including secrets. It is
  # stored in Terraform state, so the state file MUST be protected exactly like
  # the tfvars file. See docs/OPERATIONS.md (secret rotation / state hygiene).
  custom_data = base64encode(templatefile("${path.module}/cloud-init.yaml.tftpl", {
    experiment_uid   = local.experiment_uid
    app_dir          = local.app_dir
    app_git_repo_url = var.app_git_repo_url
    app_git_ref      = var.app_git_ref
    admin_username   = var.admin_username

    azure_sql_server         = azurerm_mssql_server.main.fully_qualified_domain_name
    azure_sql_database       = azurerm_mssql_database.loadtest.name
    azure_sql_admin_user     = var.sql_admin_username
    azure_sql_admin_password = var.sql_admin_password
    azure_sql_user           = var.sql_workload_username
    azure_sql_password       = var.sql_workload_password

    supabase_db_host     = var.supabase_db_host
    supabase_db_port     = var.supabase_db_port
    supabase_db_name     = var.supabase_db_name
    supabase_db_user     = var.supabase_db_user
    supabase_db_password = var.supabase_db_password

    telegram_bot_token = var.telegram_bot_token
    telegram_chat_ids  = join(",", var.telegram_allowed_chat_ids)

    capture_interval_seconds  = var.capture_raw_interval_seconds
    experiment_duration_hours = var.experiment_duration_hours
    max_experiment_hours      = var.max_experiment_hours
    auto_stop_after_hours     = var.auto_stop_after_hours
  }))

  lifecycle {
    ignore_changes = [custom_data]
  }
}

# NOTE: no Log Analytics workspace and no diagnostic settings are created.
# Platform and database metrics are read by the VM through the Azure Monitor
# metrics/data-plane REST APIs with the managed identity, which keeps the
# monitoring cost at zero for this experiment. If log retention is required
# later, add azurerm_log_analytics_workspace and document its cost first.
