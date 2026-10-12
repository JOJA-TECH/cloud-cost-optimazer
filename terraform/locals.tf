locals {
  # Unique-ish suffix derived from the subscription so names collide less often
  # while remaining deterministic for the same subscription.
  suffix = substr(sha256(var.subscription_id), 0, 6)

  name = {
    resource_group = "${var.resource_prefix}-rg"
    vnet           = "${var.resource_prefix}-vnet"
    subnet         = "${var.resource_prefix}-snet-app"
    nsg            = "${var.resource_prefix}-nsg-app"
    public_ip      = "${var.resource_prefix}-pip"
    nic            = "${var.resource_prefix}-nic"
    vm             = "${var.resource_prefix}-vm"
    sql_server     = "${var.resource_prefix}-sql-${local.suffix}"
    sql_database   = "loadtest"
    budget         = "${var.resource_prefix}-budget"
  }

  tags = merge(var.tags, {
    experiment_uid = local.experiment_uid
    location       = var.location
    cost_center    = "academic-experiment"
  })

  # The experiment_uid is generated once per apply and injected into the VM so
  # every table on both databases can be correlated without cross-database FKs.
  experiment_uid = random_uuid.experiment.result

  # Serverless SKU: GP_S_Gen5_<max vcores>. min_capacity is expressed in vCores.
  sql_sku_name = "GP_S_Gen5_${ceil(var.sql_max_capacity)}"

  # Auto-pause after 60 minutes is the lowest tier-supported value; it lets the
  # database scale to zero when the workload is idle (serverless billing stops).
  sql_auto_pause_delay_minutes = 60

  # Minimum storage permitted for GP serverless in most regions.
  sql_max_size_gb = 5

  # Application installation directory on the VM.
  app_dir = "/opt/cco"

  # vm-appropriate cloud-init packages.
  required_ports = {
    ssh = 22
    sql = 1433
  }
}
