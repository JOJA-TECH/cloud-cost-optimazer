#!/usr/bin/env bash
# =============================================================================
# train_real.sh - Train and evaluate the forecasting model on real data
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

PYTHONPATH=src .venv/bin/python -m forecasting.cli train-real "$@"