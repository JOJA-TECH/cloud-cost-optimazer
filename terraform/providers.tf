# ---------------------------------------------------------------------------
# Azure provider configurations.
#
# The default (no-alias) provider targets the PRIMARY subscription
# (var.subscription_id), which hosts the database target(s). The pilot profile
# deploys its single database there.
#
# A separate `azurerm.vm` provider targets the collector VM subscription
# (var.vm_subscription_id) so the VM is created in exactly one explicit
# subscription and never in every visible subscription by default.
#
# Provider aliases are STATIC in Terraform: they CANNOT be generated
# dynamically with for_each or a for expression. database_providers is declared
# as a map (alias -> subscription_id) so the operator can add explicit
# `provider "azurerm" { alias = "..." subscription_id = "..." }` blocks for any
# database target that lives outside the primary subscription (profile=full).
# See docs/ARCHITECTURE.md, "Provider-alias limitation".
# ---------------------------------------------------------------------------
provider "azurerm" {
  subscription_id = var.subscription_id
  tenant_id       = var.tenant_id

  features {
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
    virtual_machine {
      delete_os_disk_on_deletion     = true
      graceful_shutdown              = false
      skip_shutdown_and_force_delete = false
    }
    key_vault {
      purge_soft_delete_on_destroy    = true
      recover_soft_deleted_key_vaults = true
    }
  }
}

# Collector VM provider: explicit, separate subscription.
provider "azurerm" {
  alias           = "vm"
  subscription_id = var.vm_subscription_id
  tenant_id       = var.tenant_id

  features {
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
    virtual_machine {
      delete_os_disk_on_deletion     = true
      graceful_shutdown              = false
      skip_shutdown_and_force_delete = false
    }
    key_vault {
      purge_soft_delete_on_destroy    = true
      recover_soft_deleted_key_vaults = true
    }
  }
}

provider "random" {}

# Azure SQL free tier is only exposed through the REST API (useFreeLimit), not
# through azurerm_mssql_database, so the databases are managed with azapi.
provider "azapi" {
  subscription_id = var.subscription_id
}
