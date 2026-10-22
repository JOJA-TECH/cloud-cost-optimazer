#!/usr/bin/env bash
# =============================================================================
# generate_synthetic.sh - Generate synthetic scenarios
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

PYTHONPATH=src .venv/bin/python -m forecasting.cli generate-scenarios "$@"