#!/usr/bin/env bash
# =============================================================================
# import_real_data.sh - Import real telemetry from the source database
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

PYTHONPATH=src .venv/bin/python -m forecasting.cli import-real "$@"