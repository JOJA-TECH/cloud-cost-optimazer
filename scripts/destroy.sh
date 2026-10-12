#!/usr/bin/env bash
# Destroy the experiment infrastructure (DESTRUCTIVE).
#
# Requires an explicit confirmation token to avoid accidents, and performs a
# safe application stop first so no metrics are lost.
#
# Usage: CONFIRM=destroy ./scripts/destroy.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_DIR="${APP_DIR:-/opt/cco}"

if [[ "${CONFIRM:-}" != "destroy" ]]; then
  cat >&2 <<'EOF'
Refusing to destroy without confirmation.

This deletes the VM, the Azure SQL server/database and the networking created by
Terraform. Data already persisted to Supabase is NOT touched.

Re-run with:  CONFIRM=destroy ./scripts/destroy.sh
Destroy everything: ALLOW_MISSING_ENV=1 ./scripts/destroy.sh
EOF
  exit 2
fi

# Best-effort safe stop on the VM so no buffered data is lost.
HOST="$(cd "${REPO_ROOT}/terraform" && terraform output -raw vm_public_ip 2>/dev/null || true)"
if [[ -n "${HOST}" ]]; then
  echo "[destroy] requesting safe stop on ${HOST}"
  ssh -o StrictHostKeyChecking=accept-new "cco-admin@${HOST}" \
    "sudo -E ${APP_DIR}/venv/bin/python -m src.shutdown --stop --reason destroy" || \
    echo "[destroy] warning: safe stop failed; continuing" 
fi

echo "[destroy] running terraform destroy"
cd "${REPO_ROOT}/terraform"
terraform destroy -auto-approve
echo "[destroy] done"