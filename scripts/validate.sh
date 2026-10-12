#!/usr/bin/env bash
# Validate the repository: Python syntax/tests and Terraform formatting/validity.
#
# Safe to run locally and in CI. It never applies or destroys infrastructure.
#
# Usage: ./scripts/validate.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PYTHON="${PYTHON:-python3}"

echo "[validate] Python byte-compile"
( cd "${REPO_ROOT}/app" && "${PYTHON}" -m compileall -q src )

if "${PYTHON}" -m pytest --version >/dev/null 2>&1; then
  echo "[validate] pytest"
  ( cd "${REPO_ROOT}/app" && "${PYTHON}" -m pytest )
else
  echo "[validate] pytest not installed; skipping tests"
fi

if command -v ruff >/dev/null 2>&1; then
  echo "[validate] ruff"
  ( cd "${REPO_ROOT}/app" && ruff check . )
else
  echo "[validate] ruff not installed; skipping lint"
fi

if command -v terraform >/dev/null 2>&1; then
  echo "[validate] terraform fmt -check"
  ( cd "${REPO_ROOT}/terraform" && terraform fmt -check -recursive )
  echo "[validate] terraform validate"
  ( cd "${REPO_ROOT}/terraform" && terraform init -backend=false -input=false >/dev/null && terraform validate )
else
  echo "[validate] terraform not installed; skipping"
fi

echo "[validate] ok"