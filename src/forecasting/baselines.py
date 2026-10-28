"""Explainable baselines.

The baselines are defined on the *observed* series ``s`` (the value of the
target metric on every hourly window) and produce, for the window ``t + 1``:

* ``naive_last``:      ŷ = s[t]
* ``moving_average``:  ŷ = mean(s[t - k + 1 .. t])
* ``seasonal_naive``:  ŷ = s[t + 1 - P]  (P = 24 h, requires enough history)

They only read values that are already observed when the forecast is issued,
and they are the reference every learned model has to beat.
"""

from __future__ import annotations

from dataclasses import dataclass

import numpy as np
import pandas as pd

from .features import intensity_band, peak_band


@dataclass(frozen=True)
class BaselineSpec:
    """Definition of one baseline."""

    name: str
    kind: str
    params: dict[str, float | int]

    def describe(self) -> str:
        if not self.params:
            return self.name
        params = ", ".join(f"{k}={v}" for k, v in sorted(self.params.items()))
        return f"{self.name} ({params})"


def naive_last(series: pd.Series) -> np.ndarray:
    """ŷ[t] = s[t] (the last observed value)."""
    values = series.to_numpy(dtype=float)
    out = np.full(len(values), np.nan)
    if len(values):
        out[:] = values
    return out


def moving_average(series: pd.Series, window: int = 3) -> np.ndarray:
    """ŷ[t] = mean of the ``window`` last observed values."""
    values = series.to_numpy(dtype=float)
    out = np.full(len(values), np.nan)
    for index in range(len(values)):
        start = max(0, index - window + 1)
        chunk = values[start : index + 1]
        chunk = chunk[~np.isnan(chunk)]
        if chunk.size:
            out[index] = float(np.mean(chunk))
    return out


def seasonal_naive(series: pd.Series, period: int = 24) -> np.ndarray:
    """ŷ[t] = s[t + 1 - period]; NaN while the history is too short."""
    values = series.to_numpy(dtype=float)
    out = np.full(len(values), np.nan)
    for index in range(len(values)):
        source = index + 1 - period
        if 0 <= source < len(values):
            out[index] = values[source]
    return out


def build_baselines(
    series: pd.Series, *, seasonal_period: int = 24
) -> dict[str, BaselineSpec]:
    """The baseline catalogue evaluated in every experiment."""
    return {
        "naive_last": BaselineSpec("naive_last", "naive_last", {}),
        "moving_average": BaselineSpec(
            "moving_average", "moving_average", {"window": 3}
        ),
        "seasonal_naive": BaselineSpec(
            "seasonal_naive", "seasonal_naive", {"period": seasonal_period}
        ),
    }


def predict_baseline(spec: BaselineSpec, series: pd.Series) -> np.ndarray:
    """Apply one baseline to the observed series."""
    if spec.kind == "naive_last":
        return naive_last(series)
    if spec.kind == "moving_average":
        return moving_average(series, window=int(spec.params.get("window", 3)))
    if spec.kind == "seasonal_naive":
        return seasonal_naive(series, period=int(spec.params.get("period", 24)))
    raise ValueError(f"unknown baseline kind: {spec.kind}")


def baseline_predictions(
    frame: pd.DataFrame,
    target: str,
    observed: str = "cpu_avg",
    specs: dict[str, BaselineSpec] | None = None,
) -> pd.DataFrame:
    """Predict every baseline for the rows of ``frame``.

    ``frame`` must contain the ``observed`` column (the feature to forecast from)
    and the ``target`` column (the value to predict). The prediction of a
    baseline for row ``t`` is the forecast of the *next* window, therefore it
    is compared against ``target`` (the value of the next window) and never
    against the current one.
    """
    specs = specs or build_baselines(frame[observed])
    ordered = frame.sort_values("period_start_utc").reset_index(drop=True)
    observed_series = ordered[observed]
    out = pd.DataFrame({"period_start_utc": ordered["period_start_utc"]})
    for name, spec in specs.items():
        out[f"pred_{name}"] = predict_baseline(spec, observed_series)
    return out


def add_group_bands(frame: pd.DataFrame) -> pd.DataFrame:
    """Attach the reporting bands used by the grouped evaluation."""
    out = frame.copy()
    if "workload_intensity" in out.columns:
        out["intensity_band"] = intensity_band(out["workload_intensity"])
    if "period_start_utc" in out.columns:
        out["peak_band"] = peak_band(
            pd.to_datetime(out["period_start_utc"], utc=True).dt.hour
        )
    if "workload_class" in out.columns:
        out["workload_class"] = out["workload_class"].fillna("unknown")
    if "quality_status" in out.columns:
        out["quality_status"] = out["quality_status"].fillna("unknown")
    return out
