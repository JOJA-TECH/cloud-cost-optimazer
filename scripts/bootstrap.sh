#!/usr/bin/env bash
# Bootstrap the CCO runtime on a Debian/Ubuntu VM (idempotent).
#
# Installs msodbcsql18, creates a virtualenv, installs Python dependencies and
# applies the database migrations. Assumes the repository is checked out at
# APP_DIR/app and that APP_DIR/.env already exists (created by Terraform).
#
# Usage: sudo APP_DIR=/opt/cco ./scripts/bootstrap.sh
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/cco}"
APP_PATH="${APP_DIR}/app"
VENV="${APP_DIR}/venv"
ENV_FILE="${APP_DIR}/.env"

log() { printf '[bootstrap] %s\n' "$*"; }
die() { printf '[bootstrap] error: %s\n' "$*" >&2; exit 1; }

[[ -d "${APP_PATH}/src" ]] || die "app not found at ${APP_PATH}"
[[ -f "${ENV_FILE}" ]] || die "missing env file ${ENV_FILE}"

log "installing system packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y python3 python3-venv python3-pip curl gnupg ca-certificates unixodbc unixodbc-dev jq

if ! command -v sqlcmd >/dev/null 2>&1; then
  log "adding Microsoft apt repository (msodbcsql18)"
  curl -fsSL https://packages.microsoft.com/keys/microsoft.asc | gpg --dearmor -o /usr/share/keyrings/microsoft-prod.gpg
  curl -fsSL https://packages.microsoft.com/config/ubuntu/22.04/prod.list \
    > /etc/apt/sources.list.d/mssql-release.list
  apt-get update -y
  ACCEPT_EULA=Y apt-get install -y msodbcsql18
fi

log "creating virtualenv at ${VENV}"
python3 -m venv "${VENV}"
"${VENV}/bin/pip" install --upgrade pip
"${VENV}/bin/pip" install -r "${APP_PATH}/requirements.txt"

log "applying migrations"
( cd "${APP_PATH}" && set -a && . "${ENV_FILE}" && set +a && "${VENV}/bin/python" -m src.migrations --apply )

log "done. Enable services with: systemctl enable --now cco-collector cco-telegram-bot cco-supervisor"