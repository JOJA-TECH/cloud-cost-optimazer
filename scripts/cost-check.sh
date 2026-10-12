#!/usr/bin/env bash
# Report current Azure spend for the experiment resource group.
#
# Read-only. Requires the Azure CLI to be logged in (`az login`).
#
# Usage:
#   ./scripts/cost-check.sh [resource-group]
#   COST_DAYS=30 ./scripts/cost-check.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COST_DAYS="${COST_DAYS:-30}"

RESOURCE_GROUP="${1:-}"
if [[ -z "${RESOURCE_GROUP}" ]]; then
  RESOURCE_GROUP="$(cd "${REPO_ROOT}/terraform" && terraform output -raw resource_group_name 2>/dev/null || true)"
fi
[[ -n "${RESOURCE_GROUP}" ]] || { echo "usage: $0 <resource-group>" >&2; exit 1; }

command -v az >/dev/null 2>&1 || { echo "azure CLI (az) is required" >&2; exit 1; }

FROM="$(date -u -d "-${COST_DAYS} days" +%Y-%m-%d)"
TO="$(date -u +%Y-%m-%d)"

echo "[cost-check] scope=${RESOURCE_GROUP} from=${FROM} to=${TO}"
az consumption usage list \
  --start-date "${FROM}" --end-date "${TO}" \
  --query "[?contains(instanceId, '${RESOURCE_GROUP}')].{resource:instanceId, cost:pretaxCost, currency:currency}" \
  --output table