"""Feature engineering without temporal leakage.

The forecasting contract is explicit:

    X_t = features of the resource during [t, t + 1h)
    y_t = cpu_avg during [t + 1h, t + 2h)

so the forecast for ``y_t`` is produced at the end of the current window
``t + 1h``. At that instant the rows ``<= t`` are fully observed and the row
``t + 1`` (which *is* the target) is not. Every transformation below therefore
reads the current row and past rows only.

Consequences that are enforced by the tests:

* a lag of ``k`` periods is ``series.shift(k)``;
* a rolling window of ``n`` periods is ``series.rolling(n)`` **without** an
  extra shift, because the current window is already closed at forecast time;
* nothing in this module ever calls ``shift(-1)`` on a feature (only the
  target construction in :mod:`forecasting.extraction` does, and it is done
  once, on the label side).
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from typing import Any, Mapping, Sequence

import numpy as np
import pandas as pd

#: Calendar features derived from the window start timestamp.
CALENDAR_FEATURES: tuple[str, ...] = ("hour_sin", "hour_cos", "day_of_week", "is_peak_hour")

#: Lag horizons, in hourly periods.
DEFAULT_LAGS: Mapping[str, tuple[int, ...]] = {
    "cpu_avg": (1, 2, 3),
    "workload_intensity": (1,),
    "storage_growth_bytes": (1,),
}

#: Rolling windows, in hourly periods.
DEFAULT_ROLLINGS: Mapping[str, tuple[int, ...]] = {
    "cpu_avg": (3, 6),
    "workload_intensity": (3,),
}

#: Rules applied to the optional features when they are missing.
IMPUTATION_RULE = "train_median_with_missing_indicator"


@dataclass(frozen=True)
class FeatureSpec:
    """Declarative definition of the modelling features."""

    required: tuple[str, ...] = ("cpu_avg", "workload_intensity")
    optional: tuple[str, ...] = (
        "cpu_max",
        "data_io_avg",
        "log_write_avg",
        "memory_percent",
        "sessions_count",
        "sessions_percent",
        "workers_avg",
        "storage_used_pct",
        "storage_used_bytes",
        "storage_growth_bytes",
        "deadlock_count",
        "latency_p95_ms",
    )
    lags: Mapping[str, tuple[int, ...]] = field(default_factory=lambda: dict(DEFAULT_LAGS))
    rollings: Mapping[str, tuple[int, ...]] = field(default_factory=lambda: dict(DEFAULT_ROLLINGS))
    calendar: tuple[str, ...] = CALENDAR_FEATURES

    @classmethod
    def from_mapping(cls, data: Mapping[str, Any] | None) -> "FeatureSpec":
        data = data or {}
        return cls(
            required=tuple(data.get("required", cls.required)),
            optional=tuple(data.get("optional", cls.optional)),
            lags={k: tuple(v) for k, v in (data.get("lags") or DEFAULT_LAGS).items()},
            rollings={k: tuple(v) for k, v in (data.get("rollings") or DEFAULT_ROLLINGS).items()},
            calendar=tuple(data.get("calendar", CALENDAR_FEATURES)),
        )

    def derived_names(self) -> list[str]:
        names: list[str] = []
        for base, horizons in self.lags.items():
            names.extend(f"{base}_lag_{h}h" for h in horizons)
        for base, windows in self.rollings.items():
            names.extend(f"{base}_rolling_{w}h" for w in windows)
        names.extend(self.calendar)
        return names

    def as_dict(self) -> dict[str, Any]:
        return {
            "required": list(self.required),
            "optional": list(self.optional),
            "lags": {k: list(v) for k, v in self.lags.items()},
            "rollings": {k: list(v) for k, v in self.rollings.items()},
            "calendar": list(self.calendar),
            "imputation_rule": IMPUTATION_RULE,
        }


def add_derived_features(frame: pd.DataFrame, spec: FeatureSpec | None = None) -> pd.DataFrame:
    """Return a copy of ``frame`` with the derived features appended.

    ``frame`` must be sorted by ``period_start_utc`` and contain one row per
    hourly window. The function never reads a row that comes *after* the row it
    is enriching.
    """
    spec = spec or FeatureSpec()
    out = frame.copy()
    out["period_start_utc"] = pd.to_datetime(out["period_start_utc"], utc=True)
    out = out.sort_values("period_start_utc").reset_index(drop=True)

    for base, horizons in spec.lags.items():
        if base not in out.columns:
            continue
        for horizon in horizons:
            out[f"{base}_lag_{horizon}h"] = out[base].shift(horizon)

    for base, windows in spec.rollings.items():
        if base not in out.columns:
            continue
        for window in windows:
            # No extra shift: the current window is closed at forecast time.
            out[f"{base}_rolling_{window}h"] = out[base].rolling(window, min_periods=window).mean()

    if "period_start_utc" in out.columns:
        hours = out["period_start_utc"].dt.hour
        out["hour_sin"] = np.sin(2.0 * np.pi * hours / 24.0)
        out["hour_cos"] = np.cos(2.0 * np.pi * hours / 24.0)
        out["day_of_week"] = out["period_start_utc"].dt.dayofweek.astype("Int64")
        if "is_peak_hour" not in out.columns or out["is_peak_hour"].isna().all():
            out["is_peak_hour"] = ((hours >= 9) & (hours < 18)).astype(float)
        else:
            out["is_peak_hour"] = out["is_peak_hour"].astype("float64")
    return out


def intensity_band(values: pd.Series, edges: Sequence[float] = (0, 100, 250, 500, 1000)) -> pd.Series:
    """Bucket the workload intensity into ordered bands."""
    labels = []
    for value in values:
        if pd.isna(value):
            labels.append("unknown")
            continue
        for index, edge in enumerate(edges[1:], start=1):
            if value < edge:
                labels.append(f"lt_{int(edge)}")
                break
        else:
            labels.append(f"gte_{int(edges[-1])}")
    return pd.Series(labels, index=values.index, dtype="object")


def peak_band(hours: pd.Series) -> pd.Series:
    """Split the windows into ``peak`` and ``non_peak``."""
    return pd.Series(
        np.where((hours >= 9) & (hours < 18), "peak", "non_peak"), index=hours.index
    )


def build_modeling_frame(
    frame: pd.DataFrame,
    target: str,
    spec: FeatureSpec | None = None,
) -> pd.DataFrame:
    """Select the modelling columns, drop invalid rows and add missing flags.

    * rows without a required feature are **dropped** (never imputed);
    * optional features are kept even when missing: the imputation and the
      missingness indicator are applied later, using train statistics only.
    """
    spec = spec or FeatureSpec()
    out = frame.copy()
    out["period_start_utc"] = pd.to_datetime(out["period_start_utc"], utc=True)
    out = out.sort_values("period_start_utc").reset_index(drop=True)

    for column in spec.required:
        if column in out.columns:
            out = out[out[column].notna()]

    if "workload_intensity" in out.columns:
        out["intensity_band"] = intensity_band(out["workload_intensity"])
    if "period_start_utc" in out.columns:
        out["peak_band"] = peak_band(out["period_start_utc"].dt.hour)
    if "workload_class" in out.columns:
        out["workload_class"] = out["workload_class"].fillna("unknown")
    else:
        out["workload_class"] = "unknown"

    for column in spec.optional:
        if column in out.columns:
            out[f"feature_missing_{column}"] = out[column].isna().astype(int)
    return out.reset_index(drop=True)


def feature_columns(frame: pd.DataFrame, spec: FeatureSpec | None = None) -> list[str]:
    """Ordered list of the numeric columns usable as model inputs."""
    spec = spec or FeatureSpec()
    columns: list[str] = []
    for column in list(spec.required) + list(spec.optional):
        if column in frame.columns:
            columns.append(column)
    for name in spec.derived_names():
        if name in frame.columns:
            columns.append(name)
    columns.extend(f"feature_missing_{c}" for c in spec.optional if f"feature_missing_{c}" in frame.columns)
    return columns


def impute_optional(
    train_frame: pd.DataFrame,
    target_frame: pd.DataFrame,
    columns: Sequence[str],
) -> tuple[pd.DataFrame, dict[str, Any]]:
    """Impute optional features with the **train** median and flag them.

    The medians are computed on the training rows only, so no information of
    the validation or test window reaches the training pipeline.
    """
    out = target_frame.copy()
    report: dict[str, Any] = {"rule": IMPUTATION_RULE, "imputed": {}}
    for column in columns:
        if column not in out.columns:
            continue
        flag = f"feature_missing_{column}"
        if flag not in out.columns:
            out[flag] = out[column].isna().astype(int)
        else:
            out[flag] = out[flag].fillna(0).astype(int)
        if column in train_frame.columns:
            median = train_frame[column].median(skipna=True)
        else:
            median = np.nan
        missing = out[column].isna()
        if pd.isna(median):
            # No train information at all: the feature carries no signal and is
            # dropped instead of being filled with a constant.
            report["imputed"][column] = {"median": None, "rows_imputed": 0, "dropped": True}
            continue
        out.loc[missing, column] = float(median)
        report["imputed"][column] = {
            "median": float(median),
            "rows_imputed": int(missing.sum()),
            "dropped": False,
        }
    return out, report


def feature_definition_hash(
    columns: Sequence[str], spec: FeatureSpec | None = None
) -> str:
    """Stable hash of the feature definition (names + construction rules)."""
    spec = spec or FeatureSpec()
    payload = {
        "columns": list(columns),
        "spec": spec.as_dict(),
    }
    blob = json.dumps(payload, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(blob.encode("utf-8")).hexdigest()


def dataset_hash(frame: pd.DataFrame) -> str:
    """Stable hash of a dataset content (column names + rows, order sensitive)."""
    canonical = frame.copy()
    for column in canonical.columns:
        if pd.api.types.is_datetime64_any_dtype(canonical[column]):
            canonical[column] = canonical[column].astype(str)
    csv = canonical.to_csv(index=False, float_format="%.10g", lineterminator="\n")
    return hashlib.sha256(csv.encode("utf-8")).hexdigest()


def file_hash(path: str) -> str:
    """SHA-256 of a file (used for the source schema hash)."""
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(65536), b""):
            digest.update(chunk)
    return digest.hexdigest()
