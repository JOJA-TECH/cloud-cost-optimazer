# cco-forecasting

Experimental forecasting pipeline for the observed Azure SQL resource (cco_data context).

## Overview

This package implements a two-phase forecasting pipeline:

**Fase A** — Real data only:
- Extraction from local PostgreSQL (cco_data schema)
- Feature engineering with temporal leakage prevention
- Temporal split (70/15/15) with walk-forward fallback
- Baselines: naive_last, moving_average, seasonal_naive
- Models: Linear Regression, Ridge, Random Forest, Gradient Boosting
- Statistical validation (stationarity, autocorrelation, normality, homoscedasticity)
- Leakage detection test

**Fase B** — Synthetic scenarios:
- 8 parametric workload types: low_stable, high_stable, periodic, bursty, spike, increasing, decreasing, mixed
- Response model: workload_intensity → CPU, I/O, latency, errors, storage
- Reproducible generation with full provenance per row
- Augmentation experiments: real-only vs real+synthetic vs real+calibrated

## Quick Start

```bash
# Start local database
./scripts/init_db.sh

# Import real telemetry from source Supabase
./scripts/import_real_data.sh

# Profile the real dataset
./scripts/profile_data.sh

# Train and evaluate on real data only
./scripts/train_real.sh

# Generate synthetic scenarios
./scripts/generate_scenarios.sh

# Run augmentation experiments
./scripts/run_all.sh
```

Or use the Python CLI directly:

```bash
PYTHONPATH=src python -m forecasting.cli init-db
PYTHONPATH=src python -m forecasting.cli import-real
PYTHONPATH=src python -m forecasting.cli profile-real --output-dir artifacts/real_profile
PYTHONPATH=src python -m forecasting.cli train-real --target target_cpu_avg_next1h --output-dir artifacts/real_model
PYTHONPATH=src python -m forecasting.cli generate-scenarios --config configs/synthetic_scenarios_v1.yaml --output-dir artifacts/synthetic_v1
PYTHONPATH=src python -m forecasting.cli evaluate-augmentation --output-dir artifacts/augmentation_v1
```

## Configuration

Copy `.env.example` to `.env` and fill in:
- Local PostgreSQL credentials
- Source Supabase DSN (read-only)
- Experiment/resource identifiers

YAML configs in `configs/`:
- `real_v1.yaml` — feature selection, split ratios, target
- `synthetic_scenarios_v1.yaml` — scenario parameters, seeds, noise models

## Key Results (Fase A)

| Model | Test MAE | Test RMSE | Test sMAPE | Test MASE |
|-------|----------|-----------|------------|-----------|
| naive_last | 5.82 | 6.30 | 37.69 | 2.48 |
| moving_average | 3.61 | 4.52 | 23.62 | 1.54 |
| seasonal_naive | 4.60 | 5.61 | 45.68 | 1.96 |
| **random_forest** | **3.45** | **4.05** | **23.22** | **1.47** |

- Random Forest beats all baselines on the held-out test set
- Temporal leakage test: **passed**
- Statistical validation: series stationary (ADF p=0.01), unit coherence OK
- Limitation: only 46 hourly windows (single SKU, 2-day experiment)

## Structure

```
forecasting/
├── configs/                 # YAML configurations
├── sql/                     # SQL migrations (forecasting extension)
├── scripts/                 # Shell entry points
├── src/forecasting/         # Python package
│   ├── cli.py               # Command line interface
│   ├── config.py            # Configuration loading
│   ├── db.py                # Database helpers
│   ├── importer.py          # Source → local data import
│   ├── extraction.py        # Training matrix extraction
│   ├── validation.py        # Data quality checks
│   ├── features.py          # Feature engineering (leakage-safe)
│   ├── baselines.py         # Explainable baselines
│   ├── models.py            # ML model wrappers
│   ├── evaluation.py        # Metrics, intervals, stats
│   ├── augmentation.py      # Augmentation experiments
│   └── synthetic/
│       ├── scenarios.py     # Workload curve generators
│       ├── response_model.py# Load → metrics mapping
│       ├── generator.py     # Scenario generation + provenance
│       └── provenance.py    # Manifests, verification
├── tests/                   # Unit/integration tests
└── artifacts/               # Generated outputs (git-ignored)
```

## Reproducibility

- Random seed fixed via `RANDOM_SEED` (default 42)
- Dataset hash and feature definition hash in every manifest
- Git commit and branch recorded
- All synthetic generation uses deterministic RNG per scenario

## Limitations

1. **Single SKU, short history**: 46 hourly windows from one Azure SQL serverless GP_Gen5_2 database over ~2 days
2. **Interval coverage**: Empirical prediction intervals show 29% coverage on 7 test points (nominal 80%)
3. **No cross-SKU validation**: Transferability to other SKUs not assessed
4. **Seasonal naive**: 24h period requires more history than available

See `artifacts/reports/limitations.md` for full discussion.

## License

Academic use only. Part of the Cloud Database Cost Optimizer thesis project.