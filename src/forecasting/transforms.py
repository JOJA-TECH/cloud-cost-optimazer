"""Target representations for near-random-walk series.

With MASE close to 1 the series behaves almost like a random walk, and the
classic remedy is to predict the **change** instead of the level: fit the
model on ``delta = y - level`` where ``level`` is the current-window value
of the level column (``cpu_avg`` at time t, a regular feature of the same
row as the target, known at prediction time), and reconstruct the level
forecast as ``level + delta_hat``.

No future information is involved: the differencing is a per-row
reparameterisation of the learning problem, so the walk-forward folds and
the conformal calibration operate on the reconstructed level scale and stay
comparable with the level candidates. Whether the differenced representation
helps is an empirical question the CV decides; on this window it helps the
regularised linear family and hurts the trees.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

import numpy as np
import pandas as pd

from .models import ModelSpec, fit_model, predict_model


@dataclass
class DifferencedPredictor:
    """Wraps a pipeline so it learns the target change and predicts the level.

    ``level_index`` is the column position of the level feature inside the
    design matrix (e.g. ``cpu_avg``). The wrapped pipeline is fitted on
    ``y - X[:, level_index]`` and the prediction is reintegrated as
    ``delta_hat + X[:, level_index]``.
    """

    spec: ModelSpec
    level_index: int
    model: Any = field(default=None, repr=False)

    def fit(self, X: pd.DataFrame, y: pd.Series) -> "DifferencedPredictor":
        matrix = X.to_numpy(dtype=float)
        delta = y.to_numpy(dtype=float) - matrix[:, self.level_index]
        self.model = fit_model(self.spec, X, pd.Series(delta))
        return self

    def predict(self, X: pd.DataFrame) -> np.ndarray:
        matrix = X.to_numpy(dtype=float)
        delta_hat = predict_model(self.model, X)
        return np.asarray(delta_hat, dtype=float) + matrix[:, self.level_index]
