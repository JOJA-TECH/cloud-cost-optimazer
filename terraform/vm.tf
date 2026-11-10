# ---------------------------------------------------------------------------
# Collector VM (workload generator + collector + normalizer + sync + bot).
# Deployed in vm_subscription_id via the azurerm.vm provider — exactly one
# explicit subscription, never every visible subscription.
#
# System-assigned managed identity for Azure control-plane reads. No Azure
# credentials are stored on disk: azure-identity uses the IMDS.
#
# custom_data contains the experiment configuration, including secrets. It is
# stored in Terraform state, so the state file MUST be protected exactly like
# the tfvars file. See docs/OPERATIONS.md (secret rotation / state hygiene).
# ---------------------------------------------------------------------------
resource "azurerm_linux_virtual_machine" "vm" {
  provider                        = azurerm.vm
  name                            = local.vm.vm
  computer_name                   = "cco-collector"
  location                        = azurerm_resource_group.vm.location
  resource_group_name             = azurerm_resource_group.vm.name
  size                            = var.vm_size
  admin_username                  = var.admin_username
  disable_password_authentication = true
  # Matches the platform default; without it azurerm 3.x plans a no-op flip.
  vm_agent_platform_updates_enabled = true
  tags                              = local.tags

  network_interface_ids = [azurerm_network_interface.vm.id]

  identity {
    type = "SystemAssigned"
  }

  admin_ssh_key {
    username   = var.admin_username
    public_key = var.admin_ssh_public_key
  }

  os_disk {
    name                 = "${local.vm.vm}-osdisk"
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

  # One target is passed as the legacy single-connection env vars; the full
  # target list (multi-subscription) is passed as JSON for the collector.
  custom_data = base64encode(templatefile("${path.module}/cloud-init.yaml.tftpl", {
    experiment_uid   = local.experiment_uid
    app_dir          = local.app_dir
    app_git_repo_url = var.app_git_repo_url
    app_git_ref      = var.app_git_ref
    admin_username   = var.admin_username

    # Primary (single-connection) target — pilot uses exactly this one.
    azure_sql_server         = local.primary_target != null ? "${local.primary_target.server}.database.windows.net" : ""
    azure_sql_database       = local.primary_target != null ? local.primary_target.database : ""
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

    azure_sql_resource_id = local.primary_target != null ? azapi_resource.loadtest[local.primary_target_key].id : ""
    azure_subscription_id = local.primary_target != null ? local.primary_target.subscription_id : var.subscription_id
    azure_resource_group  = local.primary_target != null ? local.primary_target.resource_group : local.vm.resource_group
    azure_location        = local.primary_target != null ? local.primary_target.location : var.location
    azure_sql_max_size_gb = local.primary_target != null ? local.primary_target.max_size_gb : local.default_max_size_gb

    # Full target list for the multi-subscription collector (JSON).
    azure_sql_targets_json = jsonencode([
      for key, target in var.database_targets : {
        target_key               = key
        subscription_id          = target.subscription_id
        resource_group           = target.resource_group
        location                 = target.location
        server                   = "${target.server}.database.windows.net"
        database                 = target.database
        resource_uid             = target.resource_uid
        sku_name                 = target.sku_name
        min_capacity             = target.min_capacity
        max_capacity             = target.max_capacity
        auto_pause_delay_minutes = target.auto_pause_delay_minutes
        max_size_gb              = target.max_size_gb
        free_tier                = target.free_tier
        resource_id              = azapi_resource.loadtest[key].id
      }
    ])

    deployment_profile = var.deployment_profile
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
