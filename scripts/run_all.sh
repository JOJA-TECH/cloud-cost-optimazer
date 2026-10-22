#!/usr/bin/env bash
# =============================================================================
# run_all.sh - Run the complete pipeline
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

echo "=== Phase A: Real data pipeline ==="
./scripts/init_db.sh
./scripts/import_real_data.sh
./scripts/profile_data.sh
./scripts/train_real.sh

echo ""
echo "=== Phase B: Synthetic scenarios and augmentation ==="
./scripts/generate_synthetic.sh
PYTHONPATH=src .venv/bin/python -m forecasting.cli evaluate-augmentation

echo ""
echo "=== Pipeline completed ==="