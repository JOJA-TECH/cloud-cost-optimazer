terraform {
  required_version = ">= 1.5.0"

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.100"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }

  # Remote state is intentionally not configured here. For a reproducible
  # experiment, configure an azurerm backend with your own storage account, or
  # keep local state on the operator workstation. Never commit state: it may
  # contain sensitive values rendered into cloud-init custom_data.
  #
  # backend "azurerm" {
  #   resource_group_name  = "cco-tfstate-rg"
  #   storage_account_name = "ccotfstate"
  #   container_name       = "tfstate"
  #   key                  = "dataset-metrics-collector.tfstate"
  # }
}
