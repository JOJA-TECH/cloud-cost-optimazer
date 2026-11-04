locals {
  # Unique-ish suffix derived from the primary subscription so names collide
  # less often while remaining deterministic for the same subscription.
  suffix = substr(sha256(var.subscription_id), 0, 6)

  # Single (pilot) target, or null when no targets are declared. Full profile
  # requires >= 1 target (enforced by variable validation).
  primary_target     = length(var.database_targets) >= 1 ? values(var.database_targets)[0] : null
  primary_target_key = length(var.database_targets) >= 1 ? keys(var.database_targets)[0] : ""

  # Common application installation directory on the VM.
  app_dir = "/opt/cco"

  # vm-appropriate ports.
  required_ports = {
    ssh = 22
    sql = 1433
  }

  # -------------------------------------------------------------------------
  # Collector VM (in vm_subscription_id, via azurerm.vm provider)
  # -------------------------------------------------------------------------
  vm = {
    resource_group = var.vm_resource_group
    location       = var.vm_location
    vnet           = "${var.resource_prefix}-vnet"
    subnet         = "${var.resource_prefix}-snet-app"
    nsg            = "${var.resource_prefix}-nsg-app"
    public_ip      = "${var.resource_prefix}-pip"
    nic            = "${var.resource_prefix}-nic"
    vm             = "${var.resource_prefix}-vm"
  }

  # -------------------------------------------------------------------------
  # Tags shared by every resource. experiment_uid is generated once per apply.
  # -------------------------------------------------------------------------
  tags = merge(var.tags, {
    experiment_uid     = local.experiment_uid
    deployment_profile = var.deployment_profile
    cost_center        = "academic-experiment"
  })

  # The experiment_uid is generated once per apply and injected into the VM so
  # every table on both databases can be correlated without cross-database FKs.
  experiment_uid = random_uuid.experiment.result

  # -------------------------------------------------------------------------
  # Per-target SQL settings (default provider = primary subscription).
  # Each target keeps its own identity (subscription_id, resource_uid).
  # -------------------------------------------------------------------------
  sql_by_target = {
    for key, target in var.database_targets : key => {
      key                = key
      subscription_id    = target.subscription_id
      resource_group     = target.resource_group
      location           = target.location
      server             = target.server
      database           = target.database
      sku_name           = target.sku_name
      min_capacity       = target.min_capacity
      max_capacity       = target.max_capacity
      auto_pause_delay   = target.auto_pause_delay_minutes
      max_size_gb        = target.max_size_gb
      free_tier          = target.free_tier
      resource_uid       = target.resource_uid
      server_fqdn        = "${target.server}.database.windows.net"
      firewall_rule_name = "Allow-Collector-VM-${key}"
    }
  }

  # Default storage minimum for GP serverless.
  default_max_size_gb = 5
}
