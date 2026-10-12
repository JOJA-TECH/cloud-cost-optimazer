#!/usr/bin/env bash
# Deploy the application code to the VM and restart the services.
#
# The host defaults to the Terraform `vm_public_ip` output. Secrets are never
# copied by this script: the VM already owns /opt/cco/.env (mode 0600).
#
# Usage: ./scripts/deploy.sh [user@host]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_DIR="${APP_DIR:-/opt/cco}"
SSH_USER="${SSH_USER:-cco-admin}"

HOST="${1:-}"
if [[ -z "${HOST}" ]]; then
  HOST="$(cd "${REPO_ROOT}/terraform" && terraform output -raw vm_public_ip 2>/dev/null || true)"
fi
[[ -n "${HOST}" ]] || { echo "usage: $0 user@host (or run after terraform apply)" >&2; exit 1; }
TARGET="${HOST}"
[[ "${TARGET}" == *"@"* ]] || TARGET="${SSH_USER}@${HOST}"

echo "[deploy] syncing app/ to ${TARGET}:${APP_DIR}/app"
rsync -az --delete \
  --exclude '__pycache__' --exclude '.pytest_cache' --exclude '.ruff_cache' \
  --exclude '.env' --exclude 'spool' \
  "${REPO_ROOT}/app/" "${TARGET}:${APP_DIR}/app/"

echo "[deploy] installing dependencies and restarting services"
ssh "${TARGET}" bash -s <<REMOTE
set -euo pipefail
APP_DIR="${APP_DIR}"
if [[ ! -x "\${APP_DIR}/venv/bin/python" ]]; then
  python3 -m venv "\${APP_DIR}/venv"
fi
"\${APP_DIR}/venv/bin/pip" install --upgrade pip
"\${APP_DIR}/venv/bin/pip" install -r "\${APP_DIR}/app/requirements.txt"
sudo systemctl daemon-reload
sudo systemctl restart cco-collector.service cco-telegram-bot.service cco-supervisor.service
sudo systemctl --no-pager --lines=0 status cco-collector.service || true
REMOTE

echo "[deploy] done"