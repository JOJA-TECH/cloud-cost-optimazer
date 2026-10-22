"""Experimental forecasting pipeline for the observed Azure SQL resource.

The package is intentionally split in two phases:

* **Fase A** -- real data only: extraction, feature engineering, temporal
  split, baselines and a first ML model evaluated on a real test set.
* **Fase B** -- synthetic scenarios: parametric workload curves pushed through
  a calibrated response model of the observed SKU, used for robustness and
  controlled training augmentation.

Nothing in this package may write to the source database: the source is read
only and the local docker PostgreSQL is the single working copy.
"""

from __future__ import annotations

__version__ = "0.1.0"

__all__ = ["__version__"]
