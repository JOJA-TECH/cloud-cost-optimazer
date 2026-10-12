# ---------------------------------------------------------------------------
# Consumption budget scoped to the experiment resource group.
#
# The budget is a guardrail: it notifies when actual or forecast spend crosses
# the configured thresholds. It does NOT stop resources by itself. Automatic
# shutdown is enforced on the VM by the watchdog service (see systemd/ and
# docs/OPERATIONS.md).
# ---------------------------------------------------------------------------
resource "azurerm_consumption_budget_resource_group" "main" {
  count = var.enable_budget ? 1 : 0

  name              = local.name.budget
  resource_group_id = azurerm_resource_group.main.id
  amount            = var.monthly_budget_usd
  time_grain        = "Monthly"

  time_period {
    start_date = formatdate("YYYY-MM-01'T'00:00:00Z", timestamp())
    end_date   = var.budget_end_date
  }

  dynamic "notification" {
    for_each = {
      actual_50    = { threshold = 50, type = "Actual" }
      actual_80    = { threshold = 80, type = "Actual" }
      actual_100   = { threshold = 100, type = "Actual" }
      forecast_100 = { threshold = 100, type = "Forecasted" }
    }

    content {
      enabled        = length(var.budget_alert_emails) > 0
      threshold      = notification.value.threshold
      operator       = "GreaterThanOrEqualTo"
      threshold_type = notification.value.type
      contact_emails = var.budget_alert_emails
    }
  }

  lifecycle {
    # start_date is derived from the current month; ignore drift across applies.
    ignore_changes = [time_period]
  }
}
