#!/usr/bin/env bash
# Preflight check BEFORE terraform apply.
#
# Verifies the deployment profile, database targets, allowlist and Free Tier /
# cost guardrails. It is read-only: it never calls terraform apply, never
# creates Azure resources, and never prints secrets.
#
# Required environment (mirrors terraform variables; fill with placeholders or
# real values, never commit real secrets):
#   DEPLOYMENT_PROFILE        pilot (default) | full
#   DATABASE_TARGETS_JSON     JSON map of database targets (pilot: exactly 1)
#   DATABASE_SUBSCRIPTION_IDS allowlist (full: non-empty)
#   VM_SUBSCRIPTION_ID        subscription for the collector VM
#   SUBSCRIPTION_ID           primary subscription (hosts DB targets)
#   FREE_TIER_ENABLED         "true" only after the operator confirmed Free Tier
#                             eligibility for the chosen subscription + region
#                             (read-only check via Azure portal/CLI, not here)
#   SQL_MAX_SIZE_GB, SKU, VM_SIZE, LOCATION, VM_LOCATION, MONTHLY_BUDGET_USD
#
# Usage: ./scripts/preflight.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

DEPLOYMENT_PROFILE="${DEPLOYMENT_PROFILE:-pilot}"
FREE_TIER_ENABLED="${FREE_TIER_ENABLED:-false}"
VM_SUBSCRIPTION_ID="${VM_SUBSCRIPTION_ID:-}"
SUBSCRIPTION_ID="${SUBSCRIPTION_ID:-}"
VM_SIZE="${VM_SIZE:-Standard_B1s}"
MONTHLY_BUDGET_USD="${MONTHLY_BUDGET_USD:-20}"
LOCATION="${LOCATION:-eastus}"
VM_LOCATION="${VM_LOCATION:-eastus}"

fail() { echo "[preflight] BLOCKED: $*" >&2; exit 1; }

echo "[preflight] profile=${DEPLOYMENT_PROFILE}"
echo "[preflight] primary_subscription_id=(hidden) vm_subscription_id=(hidden)"
echo "[preflight] location=${LOCATION} vm_location=${VM_LOCATION} vm_size=${VM_SIZE}"
echo "[preflight] monthly_budget_usd=${MONTHLY_BUDGET_USD} (budget notifies; does NOT cut spend)"

# --- target list -----------------------------------------------------------
if command -v python3 >/dev/null 2>&1; then
  TARGET_COUNT="$(python3 -c 'import json,os
d=json.loads(os.environ.get("DATABASE_TARGETS_JSON","{}") or "{}")
print(len(d))' 2>/dev/null || echo 0)"
else
  TARGET_COUNT=0
fi
echo "[preflight] database_targets count=${TARGET_COUNT}"

case "${DEPLOYMENT_PROFILE}" in
  pilot)
    [[ "${TARGET_COUNT}" == "1" ]] || fail "pilot requires exactly 1 database_target (got ${TARGET_COUNT})"
    ;;
  full)
    [[ "${TARGET_COUNT}" -ge 1 ]] || fail "full requires at least 1 database_target"
    [[ -n "${DATABASE_SUBSCRIPTION_IDS:-}" ]] || fail "full requires non-empty DATABASE_SUBSCRIPTION_IDS allowlist"
    ;;
  *) fail "DEPLOYMENT_PROFILE must be pilot or full" ;;
esac

# --- Free Tier ------------------------------------------------------------
if [[ "${FREE_TIER_ENABLED}" != "true" ]]; then
  cat >&2 <<EOF
[preflight] BLOCKED: Free Tier no confirmado.

Free Tier (SQL Database gratuito con auto-pause al agotar la cuota) solo puede
usarse si la suscripción y la región son elegibles y la cuota gratuita quedó
activada. Esto debe verificarse por el operador por lectura sola (Azure Portal
o az, sin crear recursos) y confirmarse con FREE_TIER_ENABLED=true.

Alternativa de bajo costo claramente facturable (sin Free Tier):
  - Azure SQL Database serverless, sku mínimo (p. ej. GP_S_Gen5_1,
    min_capacity=0.5, max_capacity=1), auto-pause 60 min.
  - VM ${VM_SIZE} en ${VM_LOCATION}.
  - Se aplican cargos por vCore-segundo, horas de VM, IP pública y storage.
  - Ajuste MONTHLY_BUDGET_USD y revise ./scripts/cost-check.sh antes de apply.

No se continua hasta que FREE_TIER_ENABLED=true o el operador acepte la
alternativa facturable y lo declare explícitamente.
EOF
  exit 1
fi

echo "[preflight] Free Tier habilitado (confirmado por el operador)."
echo "[preflight] OK: preflight superado. Revisar luego: ./scripts/cost-check.sh y terraform plan (sin apply)."
