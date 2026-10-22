"""Metrics, grouped evaluation, prediction intervals and statistical checks.

Metric policy (fixed by the plan):

* ``MAE`` and ``RMSE`` are always reported;
* ``sMAPE`` is reported instead of ``MAPE``: the target has values very close
  to zero, where MAPE explodes;
* ``MASE`` is reported when the in-sample naive scale is computable.

Prediction intervals are the **empirical quantiles of the training
residuals**. They are reported with their empirical coverage on the real test
set: until that coverage is measured, an interval is not "reliable", it is
simply a calibrated offset.
"""

from __future__ import annotations

import warnings
from dataclasses import dataclass
from typing import Any, Iterable, Mapping, Sequence

import numpy as np
import pandas as pd

EPS = 1e-9


# =============================================================================
# Point metrics
# =============================================================================
def mae(y_true: np.ndarray, y_pred: np.ndarray) -> float:
    return float(np.mean(np.abs(y_true - y_pred)))


def rmse(y_true: np.ndarray, y_pred: np.ndarray) -> float:
    return float(np.sqrt(np.mean((y_true - y_pred) ** 2)))


def smape(y_true: np.ndarray, y_pred: np.ndarray) -> float:
    """Symmetric MAPE in percent (0 when both values are zero)."""
    numerator = np.abs(y_true - y_pred)
    denominator = np.abs(y_true) + np.abs(y_pred)
    ratio = np.where(denominator < EPS, 0.0, numerator / np.maximum(denominator, EPS))
    return float(100.0 * np.mean(ratio))


def mape(y_true: np.ndarray, y_pred: np.ndarray) -> float:
    """MAPE in percent. Only meaningful when the target is far from zero."""
    denominator = np.abs(y_true)
    valid = denominator > EPS
    if not valid.any():
        return float("nan")
    return float(100.0 * np.mean(np.abs(y_true[valid] - y_pred[valid]) / denominator[valid]))


def naive_scale(series: pd.Series) -> float:
    """In-sample scale of the one-step naive forecast: mean(|y_t - y_{t-1}|)."""
    values = series.astype(float).dropna()
    if len(values) < 2:
        return float("nan")
    return float(np.mean(np.abs(np.diff(values.to_numpy()))))


def mase(y_true: np.ndarray, y_pred: np.ndarray, scale: float) -> float:
    if not np.isfinite(scale) or scale <= EPS:
        return float("nan")
    return float(np.mean(np.abs(y_true - y_pred)) / scale)


def evaluate(
    y_true: Sequence[float],
    y_pred: Sequence[float],
    *,
    scale: float | None = None,
    include_mape: bool = False,
) -> dict[str, float]:
    """Standard metric block."""
    true = np.asarray(y_true, dtype=float)
    pred = np.asarray(y_pred, dtype=float)
    mask = np.isfinite(true) & np.isfinite(pred)
    true, pred = true[mask], pred[mask]
    if true.size == 0:
        return {"n": 0, "mae": float("nan"), "rmse": float("nan"), "smape": float("nan"), "mase": float("nan")}
    metrics = {
        "n": int(true.size),
        "mae": mae(true, pred),
        "rmse": rmse(true, pred),
        "smape": smape(true, pred),
    }
    metrics["mase"] = mase(true, pred, scale) if scale is not None else float("nan")
    if include_mape:
        metrics["mape"] = mape(true, pred)
    return metrics


# =============================================================================
# Prediction intervals
# =============================================================================
@dataclass(frozen=True)
class ResidualInterval:
    """Empirical residual interval built from the training residuals."""

    lower_offset: float
    upper_offset: float
    coverage: float
    n_residuals: int

    def apply(self, y_pred: np.ndarray) -> tuple[np.ndarray, np.ndarray]:
        return y_pred + self.lower_offset, y_pred + self.upper_offset


def residual_interval(
    residuals: Sequence[float], coverage: float = 0.80
) -> ResidualInterval:
    """Empirical quantiles of the training residuals."""
    values = np.asarray([r for r in residuals if np.isfinite(r)], dtype=float)
    if values.size == 0:
        return ResidualInterval(0.0, 0.0, coverage, 0)
    alpha = 1.0 - coverage
    lower = float(np.quantile(values, alpha / 2.0))
    upper = float(np.quantile(values, 1.0 - alpha / 2.0))
    return ResidualInterval(lower, upper, coverage, int(values.size))


def interval_metrics(
    y_true: Sequence[float], lower: Sequence[float], upper: Sequence[float]
) -> dict[str, float]:
    """Empirical coverage and mean width of a prediction interval."""
    true = np.asarray(y_true, dtype=float)
    lo = np.asarray(lower, dtype=float)
    hi = np.asarray(upper, dtype=float)
    mask = np.isfinite(true) & np.isfinite(lo) & np.isfinite(hi)
    if not mask.any():
        return {"interval_coverage": float("nan"), "mean_interval_width": float("nan"), "n": 0}
    true, lo, hi = true[mask], lo[mask], hi[mask]
    inside = (true >= lo) & (true <= hi)
    return {
        "interval_coverage": float(np.mean(inside)),
        "mean_interval_width": float(np.mean(hi - lo)),
        "n": int(true.size),
    }


# =============================================================================
# Grouped evaluation
# =============================================================================
GROUP_KEYS: tuple[str, ...] = (
    "workload_class",
    "intensity_band",
    "peak_band",
    "quality_status",
    "scenario_type",
    "source_type",
)


def grouped_evaluation(
    frame: pd.DataFrame,
    *,
    y_true_col: str,
    y_pred_col: str,
    group_keys: Sequence[str] = GROUP_KEYS,
    scale: float | None = None,
) -> list[dict[str, Any]]:
    """Metrics broken down by every available grouping column."""
    results: list[dict[str, Any]] = []
    for key in group_keys:
        if key not in frame.columns:
            continue
        for value, subset in frame.groupby(frame[key].astype(str), dropna=False):
            if subset.empty:
                continue
            metrics = evaluate(
                subset[y_true_col].to_numpy(dtype=float),
                subset[y_pred_col].to_numpy(dtype=float),
                scale=scale,
            )
            results.append({"group": key, "value": str(value), **metrics})
    return results


# =============================================================================
# Statistical validation
# =============================================================================
def _safe(value: Any, default: Any = None) -> Any:
    try:
        if value is None:
            return default
        if isinstance(value, (np.floating, float)):
            if not np.isfinite(value):
                return default
            return float(value)
        return value
    except Exception:  # noqa: BLE001 - defensive, the report must not crash
        return default


def stationarity_test(series: pd.Series) -> dict[str, Any]:
    """Augmented Dickey-Fuller on the target series."""
    try:
        from statsmodels.tsa.stattools import adfuller
    except Exception as exc:  # pragma: no cover - statsmodels is a hard dependency
        return {"test": "adfuller", "available": False, "reason": str(exc)}
    values = series.astype(float).dropna().to_numpy()
    if len(values) < 10:
        return {"test": "adfuller", "available": False, "reason": "series too short"}
    with warnings.catch_warnings():
        warnings.simplefilter("ignore")
        result = adfuller(values, autolag="AIC")
    statistic, pvalue = float(result[0]), float(result[1])
    return {
        "test": "adfuller",
        "available": True,
        "statistic": statistic,
        "p_value": pvalue,
        "stationary_at_5pct": bool(pvalue < 0.05),
        "interpretation": (
            "la serie es estacionaria" if pvalue < 0.05 else "la serie no es estacionaria (p>0.05)"
        ),
    }


def residual_autocorrelation(residuals: Sequence[float], lags: int = 6) -> dict[str, Any]:
    """Ljung-Box on the residuals of the selected model."""
    try:
        from statsmodels.stats.diagnostic import acorr_ljungbox
    except Exception as exc:  # pragma: no cover
        return {"test": "ljung_box", "available": False, "reason": str(exc)}
    values = np.asarray([r for r in residuals if np.isfinite(r)], dtype=float)
    if len(values) < lags + 5:
        return {"test": "ljung_box", "available": False, "reason": "too few residuals"}
    with warnings.catch_warnings():
        warnings.simplefilter("ignore")
        result = acorr_ljungbox(values, lags=[lags], return_df=True)
    pvalue = float(result["lb_pvalue"].iloc[0])
    return {
        "test": "ljung_box",
        "available": True,
        "lags": lags,
        "statistic": float(result["lb_stat"].iloc[0]),
        "p_value": pvalue,
        "no_autocorrelation_at_5pct": bool(pvalue >= 0.05),
        "interpretation": (
            "no autocorrelacion significativa" if pvalue >= 0.05 else "residuos autocorrelacionados"
        ),
    }


def residual_normality(residuals: Sequence[float]) -> dict[str, Any]:
    """Jarque-Bera normality test on the residuals."""
    try:
        from scipy import stats
    except Exception as exc:  # pragma: no cover
        return {"test": "jarque_bera", "available": False, "reason": str(exc)}
    values = np.asarray([r for r in residuals if np.isfinite(r)], dtype=float)
    if len(values) < 8:
        return {"test": "jarque_bera", "available": False, "reason": "too few residuals"}
    statistic, pvalue = stats.jarque_bera(values)
    return {
        "test": "jarque_bera",
        "available": True,
        "statistic": float(statistic),
        "p_value": float(pvalue),
        "normal_at_5pct": bool(pvalue >= 0.05),
        "interpretation": (
            "los residuos no se apartan de la normalidad"
            if pvalue >= 0.05
            else "los residuos se apartan de la normalidad (documentado, no bloqueante para RF/GB)"
        ),
    }


def residual_homoscedasticity(residuals: Sequence[float], exog: np.ndarray | None = None) -> dict[str, Any]:
    """Breusch-Pagan test for constant variance of the residuals."""
    try:
        from statsmodels.stats.diagnostic import het_breuschpagan
    except Exception as exc:  # pragma: no cover
        return {"test": "breusch_pagan", "available": False, "reason": str(exc)}
    values = np.asarray([r for r in residuals if np.isfinite(r)], dtype=float)
    if len(values) < 12:
        return {"test": "breusch_pagan", "available": False, "reason": "too few residuals"}
    if exog is None:
        exog = np.ones((len(values), 1))
    with warnings.catch_warnings():
        warnings.simplefilter("ignore")
        lm, lm_p, f, f_p = het_breuschpagan(values, exog)
    return {
        "test": "breusch_pagan",
        "available": True,
        "lm_statistic": float(lm),
        "lm_p_value": float(lm_p),
        "f_statistic": float(f),
        "f_p_value": float(f_p),
        "homoscedastic_at_5pct": bool(lm_p >= 0.05),
        "interpretation": (
            "varianza constante" if lm_p >= 0.05 else "varianza no constante (heterocedasticidad)"
        ),
    }


def outlier_report(residuals: Sequence[float], factor: float = 1.5) -> dict[str, Any]:
    """Outliers of the residual distribution (Tukey fences)."""
    values = np.asarray([r for r in residuals if np.isfinite(r)], dtype=float)
    if values.size < 4:
        return {"available": False, "reason": "too few residuals"}
    q1, q3 = np.quantile(values, [0.25, 0.75])
    iqr = q3 - q1
    low, high = q1 - factor * iqr, q3 + factor * iqr
    outliers = values[(values < low) | (values > high)]
    return {
        "available": True,
        "n_outliers": int(outliers.size),
        "outlier_fraction": float(outliers.size / values.size),
        "lower_fence": float(low),
        "upper_fence": float(high),
    }


def unit_coherence(
    values: Sequence[float], *, lower: float, upper: float, unit: str, name: str
) -> dict[str, Any]:
    """Check that the values stay inside the physically plausible range."""
    try:
        array = np.asarray(values, dtype=float)
    except Exception:
        array = np.array([float(v) for v in values if v is not None and np.isfinite(float(v))])
    if array.size == 0:
        return {"available": False, "reason": "no values"}
    mask = np.isfinite(array)
    finite = array[mask]
    violations = int(np.sum((finite < lower) | (finite > upper)))
    return {
        "available": True,
        "metric": name,
        "unit": unit,
        "expected_range": [lower, upper],
        "observed_min": float(finite.min()) if finite.size else None,
        "observed_max": float(finite.max()) if finite.size else None,
        "out_of_range_rows": violations,
        "coherent": violations == 0,
    }


def leakage_report(
    frame: pd.DataFrame, feature_columns: Sequence[str], cutoff_index: int
) -> dict[str, Any]:
    """Evidence that the features of rows <= cutoff do not depend on rows > cutoff."""
    from .features import add_derived_features

    if cutoff_index < 1 or cutoff_index >= len(frame) - 1:
        return {"available": False, "reason": "cutoff out of range"}
    original = frame.copy()
    mutated = frame.copy()
    value_columns = [
        c for c in original.columns
        if c not in ("period_start_utc", "quality_status", "quality_warnings", "canonical_sources", "coverage_json")
        and pd.api.types.is_numeric_dtype(original[c])
    ]
    # Mutate ONLY rows strictly after the cutoff (future data).
    future_idx = mutated.index[cutoff_index + 1 :]
    for column in value_columns:
        mutated.loc[future_idx, column] = mutated.loc[future_idx, column] * 7.0 + 13.0

    # Check features at the cutoff row and one before (past/present).
    check_indices = [cutoff_index, cutoff_index - 1]
    differences: dict[str, float] = {}
    for idx in check_indices:
        if idx < 0 or idx >= len(frame):
            continue
        features_original = add_derived_features(original).iloc[idx]
        features_mutated = add_derived_features(mutated).iloc[idx]
        for column in feature_columns:
            if column not in features_original.index or column not in features_mutated.index:
                continue
            left = features_original[column]
            right = features_mutated[column]
            if pd.isna(left) and pd.isna(right):
                continue
            if pd.isna(left) != pd.isna(right):
                differences[f"{column}[t={idx}]"] = float("inf")
                continue
            differences[f"{column}[t={idx}]"] = float(abs(float(left) - float(right)))
    worst = max(differences.values()) if differences else 0.0
    return {
        "available": True,
        "cutoff_index": int(cutoff_index),
        "checked_features": len(differences),
        "max_absolute_difference": worst,
        "no_leakage": bool(worst <= 1e-9),
        "differences": {k: v for k, v in differences.items() if v > 0},
    }


def statistical_validation(
    *,
    target_series: pd.Series,
    residuals: Sequence[float],
    exog: np.ndarray | None = None,
    target_values: Sequence[float] | None = None,
) -> dict[str, Any]:
    """Bundle of every statistical check required before training."""
    report: dict[str, Any] = {
        "stationarity": stationarity_test(target_series),
        "residual_autocorrelation": residual_autocorrelation(residuals),
        "residual_normality": residual_normality(residuals),
        "residual_homoscedasticity": residual_homoscedasticity(residuals, exog),
        "outliers": outlier_report(residuals),
    }
    if target_values is not None:
        report["unit_coherence"] = unit_coherence(
            target_values, lower=0.0, upper=100.0, unit="percent", name="cpu_avg"
        )
    return report


def summarise(entries: Iterable[Mapping[str, Any]]) -> pd.DataFrame:
    """Turn a list of metric dictionaries into a tidy DataFrame."""
    rows = [dict(entry) for entry in entries]
    if not rows:
        return pd.DataFrame()
    return pd.DataFrame(rows)
