#!/usr/bin/env bash
# =============================================================================
# profile_data.sh - Profile the real dataset
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

PYTHONPATH=src .venv/bin/python -m forecasting.cli profile-real "$@"