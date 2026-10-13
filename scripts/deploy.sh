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
SSH_KEY="${SSH_KEY:-${HOME}/.ssh/cco_vm_rsa}"

SSH_OPTS=(-o StrictHostKeyChecking=accept-new)
if [[ -f "${SSH_KEY}" ]]; then
  SSH_OPTS+=(-i "${SSH_KEY}" -o IdentitiesOnly=yes)
else
  echo "[deploy] warning: SSH key ${SSH_KEY} not found; using the ssh-agent/default keys" >&2
fi

HOST="${1:-}"
if [[ -z "${HOST}" ]]; then
  HOST="$(cd "${REPO_ROOT}/terraform" && terraform output -raw vm_public_ip 2>/dev/null || true)"
fi
[[ -n "${HOST}" ]] || { echo "usage: $0 user@host (or run after terraform apply)" >&2; exit 1; }
TARGET="${HOST}"
[[ "${TARGET}" == *"@"* ]] || TARGET="${SSH_USER}@${HOST}"

# /opt/cco is root-owned and the services run as root, so both the file sync and
# the remote steps require sudo (cco-admin has passwordless sudo).
echo "[deploy] syncing app/ to ${TARGET}:${APP_DIR}/app"
rsync -az --delete -e "ssh ${SSH_OPTS[*]}" \
  --rsync-path="sudo rsync" --chown=root:root \
  --exclude '__pycache__' --exclude '.pytest_cache' --exclude '.ruff_cache' \
  --exclude '.env' --exclude 'spool' \
  "${REPO_ROOT}/app/" "${TARGET}:${APP_DIR}/app/"

echo "[deploy] installing dependencies, applying migrations and restarting services"
ssh "${SSH_OPTS[@]}" "${TARGET}" sudo bash -s <<REMOTE
set -euo pipefail
APP_DIR="${APP_DIR}"
mkdir -p "\${APP_DIR}/app"
if [[ ! -x "\${APP_DIR}/venv/bin/python" ]]; then
  python3 -m venv "\${APP_DIR}/venv"
fi
"\${APP_DIR}/venv/bin/pip" install --quiet --upgrade pip
"\${APP_DIR}/venv/bin/pip" install --quiet -r "\${APP_DIR}/app/requirements.txt"
systemctl daemon-reload

# Migrations must succeed before the collector starts writing rows.
systemctl restart cco-migrate.service
systemctl --no-pager --lines=0 status cco-migrate.service || true

systemctl restart cco-collector.service cco-telegram-bot.service cco-supervisor.service
systemctl --no-pager --lines=0 status cco-collector.service || true

# The workload generator has Requires=cco-migrate, so restarting migrations
# stops it. Bring it back only when the experiment is meant to auto-start.
if grep -q '^AUTO_START=true' "\${APP_DIR}/.env" 2>/dev/null; then
  systemctl restart cco-workload.service
  systemctl --no-pager --lines=0 status cco-workload.service || true
fi
REMOTE

echo "[deploy] done"