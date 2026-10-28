"""Conformal prediction intervals with finite-sample guarantees.

Two constructions are provided:

* **Split conformal** (inductive): the nonconformity scores are the absolute
  residuals of a model that was fitted **without** the calibration rows
  (here: the validation residuals of the model fitted on train only). The
  half width is the ``k``-th order statistic of the scores with
  ``k = ceil((n + 1) * coverage)``, which guarantees

      P(y_new in [f(x_new) - q, f(x_new) + q]) >= coverage

  exactly for every ``n`` under exchangeability of the calibration and the
  new point — no asymptotics involved. When ``k`` would exceed ``n`` the
  level is not exactly attainable (it would need an infinite score); the
  rank is clamped to ``n`` and the interval is flagged as ``clamped``.

* **Jackknife+** (leave-one-out): the scores are ``|y_i - f_{-i}(x_i)|``
  computed on the same rows the final model is trained on, so ``n`` scores
  are available instead of the small validation split. The formal guarantee
  under exchangeability is ``1 - 2*alpha`` (weaker), but the empirical
  quantile is far more stable with tiny datasets.

An asymmetric (Bonferroni) split conformal is also provided: each tail is
calibrated at ``alpha / 2`` so the joint coverage stays at ``1 - alpha``.
With very small calibration sets the two ranks saturate to the extreme
order statistics, which is why the symmetric version is the default choice
of the pipeline.

Caveat for time series: exchangeability is an assumption that sequential
data does not strictly satisfy. The measured coverage on the test set is
always reported next to the nominal level for honest evaluation.
"""

from __future__ import annotations

from collections.abc import Callable, Sequence
from dataclasses import dataclass

import numpy as np
import pandas as pd

#: Signature of a refit-and-predict callback used by :func:`jackknife_plus_scores`.
#: It receives the training design matrix, the training labels and the rows
#: to predict, and must return the point predictions of a model refitted on
#: the given training rows.
FitPredict = Callable[[pd.DataFrame, pd.Series, pd.DataFrame], np.ndarray]


def conformal_rank(n: int, coverage: float) -> int:
    """Finite-sample corrected rank ``k = ceil((n + 1) * coverage)`` clamped to ``[1, n]``."""
    if n < 1:
        raise ValueError("n must be >= 1")
    if not (0.0 < coverage < 1.0):
        raise ValueError("coverage must be strictly between 0 and 1")
    rank = int(np.ceil((n + 1) * coverage))
    return int(min(max(rank, 1), n))


@dataclass(frozen=True)
class ConformalInterval:
    """Calibrated interval offsets plus the evidence of the calibration."""

    method: str
    lower_offset: float
    upper_offset: float
    nominal_coverage: float
    alpha: float
    n_scores: int
    finite_sample_rank: int
    clamped: bool
    guarantee: str

    def apply(self, y_pred: np.ndarray) -> tuple[np.ndarray, np.ndarray]:
        values = np.asarray(y_pred, dtype=float)
        return values + self.lower_offset, values + self.upper_offset

    @property
    def half_width(self) -> float:
        return float(max(abs(self.lower_offset), abs(self.upper_offset)))

    def as_dict(self) -> dict[str, object]:
        return {
            "method": self.method,
            "nominal_coverage": self.nominal_coverage,
            "alpha": self.alpha,
            "lower_offset": self.lower_offset,
            "upper_offset": self.upper_offset,
            "half_width": self.half_width,
            "n_calibration_scores": self.n_scores,
            "finite_sample_rank": self.finite_sample_rank,
            "clamped": self.clamped,
            "guarantee": self.guarantee,
        }


def _finite_scores(values: Sequence[float]) -> np.ndarray:
    array = np.asarray(list(values), dtype=float)
    return array[np.isfinite(array)]


def split_conformal_interval(
    residuals: Sequence[float],
    coverage: float = 0.80,
    *,
    symmetric: bool = True,
) -> ConformalInterval:
    """Split conformal interval from calibration residuals.

    ``symmetric=True`` uses the absolute residuals (the k-th order statistic
    with ``k = ceil((n + 1) * coverage)``); this is the exact marginal
    construction. ``symmetric=False`` calibrates each tail at ``alpha / 2``
    (Bonferroni), keeping the joint level at ``1 - alpha`` at the price of
    wider extremes on tiny calibration sets.
    """
    values = _finite_scores(residuals)
    n = int(values.size)
    alpha = float(1.0 - coverage)
    if n == 0:
        return ConformalInterval(
            method="split_conformal" if symmetric else "split_conformal_asymmetric",
            lower_offset=0.0,
            upper_offset=0.0,
            nominal_coverage=coverage,
            alpha=alpha,
            n_scores=0,
            finite_sample_rank=0,
            clamped=False,
            guarantee="sin scores de calibracion: intervalo degenerado",
        )

    if symmetric:
        scores = np.sort(np.abs(values))
        k = conformal_rank(n, coverage)
        q = float(scores[k - 1])
        clamped = (n + 1) * coverage > n
        return ConformalInterval(
            method="split_conformal",
            lower_offset=-q,
            upper_offset=q,
            nominal_coverage=coverage,
            alpha=alpha,
            n_scores=n,
            finite_sample_rank=k,
            clamped=clamped,
            guarantee=(
                f"P(y en [pred - q, pred + q]) >= {coverage:.0%} bajo intercambiabilidad "
                "(exacto en muestra finita)"
            ),
        )

    # Asymmetric Bonferroni: each tail at alpha / 2.
    ordered = np.sort(values)
    k_upper = conformal_rank(n, 1.0 - alpha / 2.0)  # k-th smallest for the upper bound
    j_lower = max(1, n + 1 - k_upper)  # symmetric j-th smallest for the lower bound
    lower = float(ordered[j_lower - 1])
    upper = float(ordered[k_upper - 1])
    clamped = (n + 1) * (1.0 - alpha / 2.0) > n
    return ConformalInterval(
        method="split_conformal_asymmetric",
        lower_offset=lower,
        upper_offset=upper,
        nominal_coverage=coverage,
        alpha=alpha,
        n_scores=n,
        finite_sample_rank=k_upper,
        clamped=clamped,
        guarantee=(
            f"P(y en [pred + lo, pred + hi]) >= {coverage:.0%} bajo intercambiabilidad "
            "(Bonferroni, alpha/2 por cola)"
        ),
    )


def jackknife_plus_scores(
    fit_predict: FitPredict,
    X: pd.DataFrame,
    y: pd.Series,
) -> np.ndarray:
    """Leave-one-out nonconformity scores ``|y_i - f_{-i}(x_i)|``.

    ``fit_predict`` must refit the model on the given rows and predict the
    held-out row; the caller controls which estimator is used so the scores
    mirror the final fit exactly.
    """
    n = len(X)
    if n < 2:
        return np.empty(0, dtype=float)
    values = np.asarray(y, dtype=float)
    scores = np.empty(n, dtype=float)
    index = np.arange(n)
    for i in range(n):
        mask = index != i
        prediction = fit_predict(X.iloc[mask], y.iloc[mask], X.iloc[[i]])
        scores[i] = abs(values[i] - float(np.asarray(prediction)[0]))
    return scores


def jackknife_plus_interval(
    scores: Sequence[float],
    coverage: float = 0.80,
) -> ConformalInterval:
    """jackknife+ interval from leave-one-out scores.

    The finite-sample guarantee under exchangeability is ``1 - 2*alpha``
    (weaker than split conformal), but the quantile is estimated from ``n``
    scores instead of the small validation split, which makes it far more
    stable with tiny datasets.
    """
    values = _finite_scores(scores)
    n = int(values.size)
    alpha = float(1.0 - coverage)
    if n == 0:
        return ConformalInterval(
            method="jackknife_plus",
            lower_offset=0.0,
            upper_offset=0.0,
            nominal_coverage=coverage,
            alpha=alpha,
            n_scores=0,
            finite_sample_rank=0,
            clamped=False,
            guarantee="sin scores de calibracion: intervalo degenerado",
        )
    ordered = np.sort(values)
    k = conformal_rank(n, coverage)
    q = float(ordered[k - 1])
    clamped = (n + 1) * coverage > n
    guaranteed = max(0.0, 1.0 - 2.0 * alpha)
    return ConformalInterval(
        method="jackknife_plus",
        lower_offset=-q,
        upper_offset=q,
        nominal_coverage=coverage,
        alpha=alpha,
        n_scores=n,
        finite_sample_rank=k,
        clamped=clamped,
        guarantee=(
            f"P(y en [pred - q, pred + q]) >= {guaranteed:.0%} bajo intercambiabilidad "
            f"(garantia jackknife+ de 1 - 2*alpha; nivel nominal {coverage:.0%})"
        ),
    )
