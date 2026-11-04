# ---------------------------------------------------------------------------
# Identity / subscription
# ---------------------------------------------------------------------------
# Primary subscription that hosts the Azure SQL database target(s). For the
# pilot profile this is the single target subscription. For `full` it is the
# first/primary entry; additional database subscriptions are declared through
# database_targets and the explicit provider aliases in providers.tf.
variable "subscription_id" {
  type        = string
  sensitive   = true
  description = "Primary Azure subscription id (hosts the SQL database target(s))."
}

# Subscription where the collector VM is deployed. Separate from the database
# subscription(s) on purpose: multi-subscription experiments must pin the VM to
# one explicit subscription and never default to all visible subscriptions.
variable "vm_subscription_id" {
  type        = string
  sensitive   = true
  description = "Azure subscription id where the collector VM is deployed."
}

variable "tenant_id" {
  type        = string
  sensitive   = true
  description = "Azure AD (Entra) tenant id."
}

variable "location" {
  type        = string
  default     = "eastus"
  description = "Azure region for the primary subscription. Verify SKU availability and pricing before applying."
}

variable "vm_location" {
  type        = string
  default     = "eastus"
  description = "Azure region for the collector VM subscription."
}

variable "vm_resource_group" {
  type        = string
  default     = "cco-vm-rg"
  description = "Resource group for the collector VM (in vm_subscription_id)."
}

# ---------------------------------------------------------------------------
# Deployment profile
# ---------------------------------------------------------------------------
variable "deployment_profile" {
  type        = string
  default     = "pilot"
  description = "pilot (one DB, one VM, short run) or full (explicit allowlist of DB subscriptions)."

  validation {
    condition     = contains(["pilot", "full"], var.deployment_profile)
    error_message = "deployment_profile must be 'pilot' or 'full'."
  }
}

# Allowlist of subscriptions that may host database targets. The VM subscription
# is intentionally NOT part of this list unless explicitly added.
variable "database_subscription_ids" {
  type        = list(string)
  sensitive   = true
  default     = []
  description = "Allowlist of subscription ids permitted to host database targets. Required and non-empty for profile=full."
}

# Explicit database targets. Each key is a stable target identifier; the value
# fully describes one Azure SQL database target. Used by both profiles.
# pilot: exactly one entry. full: one or more entries, each subscription_id in
# database_subscription_ids.
variable "database_targets" {
  type = map(object({
    subscription_id          = string
    resource_group           = string
    location                 = string
    server                   = string
    database                 = string
    sku_name                 = string
    min_capacity             = number
    max_capacity             = number
    auto_pause_delay_minutes = number
    max_size_gb              = number
    free_tier                = bool
    resource_uid             = string
  }))
  default     = {}
  description = "Explicit database targets (multi-subscription capable). pilot requires exactly one entry; full requires one or more."

  validation {
    condition = (
      length(var.database_targets) >= 0
      && (var.deployment_profile != "pilot" || length(var.database_targets) == 1)
    )
    error_message = "deployment_profile=pilot requires exactly one entry in database_targets."
  }
  validation {
    condition = (
      length(var.database_targets) >= 0
      && (var.deployment_profile != "full" || length(var.database_targets) >= 1)
    )
    error_message = "deployment_profile=full requires at least one entry in database_targets."
  }
  validation {
    condition = (
      length(var.database_targets) >= 0
      && (var.deployment_profile != "full" || length(var.database_subscription_ids) >= 1)
    )
    error_message = "deployment_profile=full requires a non-empty database_subscription_ids allowlist."
  }
  validation {
    condition = (
      length(var.database_targets) >= 0
      && alltrue([
        for t in var.database_targets : var.deployment_profile == "pilot" || contains(var.database_subscription_ids, t.subscription_id)
      ])
    )
    error_message = "Every database_targets.subscription_id must be listed in database_subscription_ids (allowlist)."
  }
  validation {
    condition = (
      length(var.database_targets) >= 0
      && length(distinct(keys(var.database_targets))) == length(var.database_targets)
    )
    error_message = "database_targets keys must be unique."
  }
  validation {
    condition = (
      length(var.database_targets) >= 0
      && (var.deployment_profile == "pilot"
        || length(distinct([for t in var.database_targets : t.subscription_id]))
      == length([for t in var.database_targets : t.subscription_id]))
    )
    error_message = "Duplicate subscription_id across database_targets is not allowed; each database must live in a distinct subscription."
  }
  validation {
    condition = (
      length(var.database_targets) >= 0
      && alltrue([for t in var.database_targets : t.subscription_id != "" && t.resource_group != "" && t.server != "" && t.database != "" && t.resource_uid != ""])
    )
    error_message = "database_targets entries must not contain empty subscription_id, resource_group, server, database or resource_uid."
  }
}

# Nota: los proveedores azurerm para suscripciones de base de datos distintas
# a la primaria son bloques estáticos `provider "azurerm" { alias = "..." }`
# que el operador añade manualmente en providers.tf (Terraform no los genera
# dinámicamente). No hay variable para ellos.

variable "resource_prefix" {
  type        = string
  default     = "cco-lab"
  description = "Lowercase prefix used for resource names (3-10 chars)."

  validation {
    condition     = can(regex("^[a-z0-9-]{3,10}$", var.resource_prefix))
    error_message = "resource_prefix must be 3-10 chars, lowercase letters, digits or hyphens."
  }
}

# ---------------------------------------------------------------------------
# VM access
# ---------------------------------------------------------------------------
variable "admin_username" {
  type        = string
  description = "Linux admin username for the VM."
}

variable "admin_ssh_public_key" {
  type        = string
  sensitive   = true
  description = "SSH public key installed for admin_username. The private key never leaves the operator."
}

variable "admin_cidr" {
  type        = string
  description = "CIDR allowed to reach SSH (tcp/22). Example: 203.0.113.7/32."

  validation {
    condition     = can(cidrhost(var.admin_cidr, 0))
    error_message = "admin_cidr must be a valid CIDR, e.g. 203.0.113.7/32."
  }
}

# ---------------------------------------------------------------------------
# Azure SQL (system under test)
# ---------------------------------------------------------------------------
variable "sql_admin_username" {
  type        = string
  sensitive   = true
  description = "Azure SQL logical server admin login (separate from the workload login)."
}

variable "sql_admin_password" {
  type        = string
  sensitive   = true
  description = "Azure SQL logical server admin password (16+ chars, mixed classes)."
}

variable "sql_workload_username" {
  type        = string
  default     = "cco_workload"
  description = "Least-privilege SQL login used by the load generator (loadtest schema only)."
}

variable "sql_workload_password" {
  type        = string
  sensitive   = true
  description = "Password for the least-privilege workload login."
}

variable "sql_min_capacity" {
  type        = number
  default     = 0.5
  description = "Serverless minimum vCores (0.5 when available)."

  validation {
    condition     = var.sql_min_capacity >= 0.5 && var.sql_min_capacity <= 4
    error_message = "sql_min_capacity must be between 0.5 and 4."
  }
}

variable "sql_max_capacity" {
  type        = number
  default     = 1.0
  description = "Serverless maximum vCores (start at 1)."

  validation {
    condition     = var.sql_max_capacity >= var.sql_min_capacity && var.sql_max_capacity <= 4
    error_message = "sql_max_capacity must be >= sql_min_capacity and <= 4."
  }
}

# ---------------------------------------------------------------------------
# Supabase (external PostgreSQL)
# ---------------------------------------------------------------------------
variable "supabase_db_host" {
  type        = string
  description = "Supabase PostgreSQL host (pooler or direct host)."
}

variable "supabase_db_port" {
  type        = number
  default     = 5432
  description = "Supabase PostgreSQL port (5432 direct, 6543 pooler)."
}

variable "supabase_db_name" {
  type        = string
  default     = "postgres"
  description = "Supabase database name."
}

variable "supabase_db_user" {
  type        = string
  sensitive   = true
  description = "Supabase database user."
}

variable "supabase_db_password" {
  type        = string
  sensitive   = true
  description = "Supabase database password."
}

# ---------------------------------------------------------------------------
# Telegram
# ---------------------------------------------------------------------------
variable "telegram_bot_token" {
  type        = string
  sensitive   = true
  description = "Telegram Bot API token (long polling)."
}

variable "telegram_allowed_chat_ids" {
  type        = list(string)
  sensitive   = true
  description = "Allow-list of Telegram chat ids permitted to control the experiment."
}

# ---------------------------------------------------------------------------
# VM size and experiment lifecycle
# ---------------------------------------------------------------------------
variable "vm_size" {
  type        = string
  default     = "Standard_B1s"
  description = "VM size. Preference order: Standard_B1s, Standard_B2pts_v2, Standard_B2ats_v2."

  validation {
    condition = contains(
      ["Standard_B1s", "Standard_B2pts_v2", "Standard_B2ats_v2", "Standard_B2s"],
      var.vm_size
    )
    error_message = "vm_size must be one of the reviewed economical sizes."
  }
}

variable "experiment_duration_hours" {
  type        = number
  default     = 336
  description = "Planned experiment duration in hours (14 days by default)."
}

variable "monthly_budget_usd" {
  type        = number
  default     = 20
  description = "Monthly Azure budget in USD used for budget alerts."
}

variable "max_experiment_hours" {
  type        = number
  default     = 336
  description = "Hard ceiling for the experiment runtime in hours."
}

variable "auto_stop_after_hours" {
  type        = number
  default     = 360
  description = "Auto-stop safety net (watchdog) in hours; must exceed max_experiment_hours."
}

variable "budget_alert_emails" {
  type        = list(string)
  default     = []
  description = "Emails notified by the consumption budget (optional)."
}

variable "budget_end_date" {
  type        = string
  default     = "2035-01-01T00:00:00Z"
  description = "End date of the consumption budget (RFC3339, far future)."
}

# ---------------------------------------------------------------------------
# Application source (how the VM obtains the collector code)
# ---------------------------------------------------------------------------
variable "app_git_repo_url" {
  type        = string
  default     = ""
  description = "Optional public git URL of this repository. Empty disables cloning; the operator copies app/ manually."
}

variable "app_git_ref" {
  type        = string
  default     = "dataset-metrics-collector"
  description = "Git ref (branch/tag) cloned on the VM."
}

variable "capture_raw_interval_seconds" {
  type        = number
  default     = 60
  description = "Raw metric capture cadence."

  validation {
    condition     = var.capture_raw_interval_seconds >= 30 && var.capture_raw_interval_seconds <= 300
    error_message = "capture_raw_interval_seconds must be between 30 and 300."
  }
}

variable "enable_public_ip" {
  type        = bool
  default     = true
  description = "Attach a public IP to the VM. Required unless outbound is provided by another mechanism."
}

variable "enable_budget" {
  type        = bool
  default     = true
  description = "Create the consumption budget and alerts."
}

variable "tags" {
  type = map(string)
  default = {
    project     = "cloud-cost-optimizer"
    component   = "dataset-metrics-collector"
    environment = "experiment"
    managed_by  = "terraform"
  }
  description = "Additional tags merged into every resource."
}
