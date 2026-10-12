# Costs

The platform is designed to stay inside a small monthly budget (default alert
at **20 USD**) and to be left running unattended safely.

## Where the money goes

| Resource | Shape | Cost driver |
| -------- | ----- | ----------- |
| Azure SQL Database | Serverless, `0.5`–`1.0` vCore, auto-pause after 60 min idle | vCore-seconds actually used |
| Collector VM | `Standard_B1s` (burstable) | hours running |
| Public IP | Standard, static | hours allocated |
| Storage / bandwidth | small | negligible at this scale |

Serverless auto-pause is the key lever: when the workload is paused or stopped
the database scales to zero and only storage remains.

## Guardrails

- `terraform/budget.tf` creates a consumption budget with alerts
  (`budget_alert_emails`) so overspend is visible early.
- `scripts/cost-check.sh` prints current spend for the resource group
  (`az consumption usage list`); read-only.
- `src.shutdown --supervise` force-stops the experiment after
  `AUTO_STOP_AFTER_HOURS`, bounding the worst case even if nobody is watching.
- Default experiment window is 14 days (`EXPERIMENT_DURATION_HOURS=336`), with a
  6-hour smoke run available for validation.

## Rough budget model

Let `H` = experiment hours, `V` = avg vCores used while running.

```
SQL   ≈ H × active_fraction × V × vcore_hour_price
VM    ≈ H × b1s_hour_price
IP    ≈ H × static_ip_hour_price
```

Idle time (paused workload) collapses the SQL term via auto-pause, so the VM and
IP dominate long low-activity runs. Keep runs bounded, verify spend with
`cost-check.sh`, and tear down with `destroy.sh` when finished.

## Reducing cost

- Shorten `experiment_duration_hours` / run a smoke test first.
- Prefer fewer, longer phases over many idle hours.
- Lower `sql_max_capacity` for exploratory runs.
- Always `destroy.sh` after collecting the dataset; Supabase keeps the data.