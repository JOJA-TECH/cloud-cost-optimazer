"""Data quality validation of the real dataset.

The rules implemented here come straight from the plan: UTC timestamps,
declared granularity, no logical duplicates, no silent zero filling, explicit
coverage per hour, and a quality status per window. Every check returns a
dictionary that is written to the profile artifact, so a reader can tell
*what* was checked and *what* was found.
"""

from __future__ import annotations

from typing import Any, Iterable, Mapping, Sequence

import numpy as np
import pandas as pd

#: Plausible range per metric, used to detect impossible values.
EXPECTED_RANGES: Mapping[str, tuple[float, float]] = {
    "cpu_percent": (0.0, 100.0),
    "data_io_percent": (0.0, 100.0),
    "log_write_percent": (0.0, 100.0),
    "memory_percent": (0.0, 100.0),
    "sessions_percent": (0.0, 100.0),
    "workers_percent": (0.0, 100.0),
    "storage_percent": (0.0, 100.0),
    "error_rate": (0.0, 1.0),
}

#: Metrics that are counts and therefore cannot be negative.
COUNT_METRICS: tuple[str, ...] = (
    "sessions_count",
    "deadlock",
    "failed_operations",
    "successful_operations",
    "offered_operations",
)


def check_timestamps_utc(frame: pd.DataFrame, column: str = "observed_at_utc") -> dict[str, Any]:
    """Timestamps must be timezone aware and expressed in UTC."""
    if column not in frame.columns or frame.empty:
        return {"check": "timestamps_utc", "applicable": False}
    series = pd.to_datetime(frame[column], utc=True, errors="coerce")
    naive = 0
    if not isinstance(frame[column].dtype, pd.DatetimeTZDtype):
        raw = frame[column].astype(str)
        naive = int(sum(1 for value in raw if not value.endswith("Z") and "+00:00" not in value))
    return {
        "check": "timestamps_utc",
        "applicable": True,
        "rows": int(len(frame)),
        "unparseable": int(series.isna().sum()),
        "not_explicitly_utc": naive,
        "passed": bool(series.isna().sum() == 0 and naive == 0),
    }


def check_granularity(
    frame: pd.DataFrame,
    column: str = "observed_at_utc",
    interval_column: str = "interval_seconds",
    expected: int = 60,
) -> dict[str, Any]:
    """Declared interval and observed spacing of the readings."""
    if frame.empty or column not in frame.columns:
        return {"check": "granularity", "applicable": False}
    times = pd.to_datetime(frame[column], utc=True).sort_values()
    deltas = times.diff().dropna().dt.total_seconds()
    declared = (
        sorted({int(v) for v in frame[interval_column].dropna().unique()})
        if interval_column in frame.columns
        else []
    )
    return {
        "check": "granularity",
        "applicable": True,
        "declared_interval_seconds": declared,
        "expected_interval_seconds": expected,
        "observed_delta_seconds": {
            "min": float(deltas.min()) if len(deltas) else None,
            "median": float(deltas.median()) if len(deltas) else None,
            "max": float(deltas.max()) if len(deltas) else None,
        },
        "passed": bool(declared == [expected] if declared else True),
    }


def check_duplicates(
    frame: pd.DataFrame, keys: Sequence[str]
) -> dict[str, Any]:
    """Logical duplicates on the natural key."""
    present = [k for k in keys if k in frame.columns]
    if not present or frame.empty:
        return {"check": "duplicates", "applicable": False}
    duplicated = int(frame.duplicated(subset=present, keep=False).sum())
    return {
        "check": "duplicates",
        "applicable": True,
        "key": list(present),
        "duplicated_rows": duplicated,
        "passed": duplicated == 0,
    }


def check_nulls(frame: pd.DataFrame, columns: Iterable[str]) -> dict[str, Any]:
    """Null counts of the columns that matter for the model."""
    columns = [c for c in columns if c in frame.columns]
    if not columns or frame.empty:
        return {"check": "nulls", "applicable": False}
    counts = {c: int(frame[c].isna().sum()) for c in columns}
    return {
        "check": "nulls",
        "applicable": True,
        "rows": int(len(frame)),
        "null_counts": counts,
        "null_fraction": {c: (counts[c] / len(frame)) for c in columns},
        "passed": all(v == 0 for v in counts.values()),
    }


def check_ranges(frame: pd.DataFrame, ranges: Mapping[str, tuple[float, float]]) -> dict[str, Any]:
    """Values outside the physically plausible range."""
    findings: dict[str, Any] = {}
    for code, (low, high) in ranges.items():
        if code not in frame.columns or frame.empty:
            continue
        values = pd.to_numeric(frame[code], errors="coerce")
        violations = int(((values < low) | (values > high)).sum())
        findings[code] = {
            "expected_range": [low, high],
            "observed_min": float(values.min()) if values.notna().any() else None,
            "observed_max": float(values.max()) if values.notna().any() else None,
            "out_of_range_rows": violations,
        }
    if not findings:
        return {"check": "ranges", "applicable": False}
    return {
        "check": "ranges",
        "applicable": True,
        "by_metric": findings,
        "passed": all(v["out_of_range_rows"] == 0 for v in findings.values()),
    }


def check_negative_counts(frame: pd.DataFrame, columns: Iterable[str]) -> dict[str, Any]:
    """Counts cannot be negative."""
    columns = [c for c in columns if c in frame.columns]
    if not columns or frame.empty:
        return {"check": "negative_counts", "applicable": False}
    findings = {}
    for column in columns:
        values = pd.to_numeric(frame[column], errors="coerce")
        findings[column] = int((values < 0).sum())
    return {
        "check": "negative_counts",
        "applicable": True,
        "by_metric": findings,
        "passed": all(v == 0 for v in findings.values()),
    }


def hourly_completeness(
    frame: pd.DataFrame,
    *,
    time_column: str = "observed_at_utc",
    metric_column: str = "metric_code",
    expected_per_hour: int = 60,
) -> pd.DataFrame:
    """Coverage per hour and metric (rows observed vs. expected)."""
    if frame.empty:
        return pd.DataFrame()
    work = frame.copy()
    work["period_start_utc"] = pd.to_datetime(work[time_column], utc=True).dt.floor("h")
    grouped = (
        work.groupby([metric_column, "period_start_utc"])
        .size()
        .reset_index(name="rows_observed")
    )
    grouped["coverage"] = grouped["rows_observed"] / float(expected_per_hour)
    grouped["complete"] = grouped["rows_observed"] >= expected_per_hour
    return grouped.sort_values([metric_column, "period_start_utc"]).reset_index(drop=True)


def quality_status_counts(frame: pd.DataFrame, column: str = "quality_status") -> dict[str, Any]:
    """Distribution of the quality status of the windows."""
    if column not in frame.columns or frame.empty:
        return {"check": "quality_status", "applicable": False}
    counts = frame[column].fillna("unknown").astype(str).value_counts().to_dict()
    return {
        "check": "quality_status",
        "applicable": True,
        "counts": {str(k): int(v) for k, v in counts.items()},
        "invalid_rows": int(counts.get("invalid", 0)),
        "passed": int(counts.get("invalid", 0)) == 0,
    }


def detect_workload_changes(frame: pd.DataFrame) -> dict[str, Any]:
    """Changes of the workload class over time."""
    if "workload_class" not in frame.columns or frame.empty:
        return {"check": "workload_changes", "applicable": False}
    ordered = frame.sort_values("period_start_utc")
    classes = ordered["workload_class"].fillna("unknown").astype(str)
    changes = ordered.loc[classes.ne(classes.shift()) & classes.shift().notna(), "period_start_utc"]
    distinct = sorted(set(classes.unique()))
    return {
        "check": "workload_changes",
        "applicable": True,
        "distinct_classes": distinct,
        "n_changes": int(len(changes)),
        "change_points": [str(c) for c in changes.tolist()],
    }


def detect_config_changes(frame: pd.DataFrame) -> dict[str, Any]:
    """Changes of the SKU configuration over time."""
    columns = [c for c in ("sku_id", "compute_model", "capacity_vcores") if c in frame.columns]
    if not columns or frame.empty:
        return {"check": "config_changes", "applicable": False}
    ordered = frame.sort_values("period_start_utc")
    # Only use columns with at least one non-null value.
    valid = [c for c in columns if ordered[c].notna().any()]
    if not valid:
        return {"check": "config_changes", "applicable": False}
    signature = ordered[valid].astype(str).agg("|".join, axis=1)
    changes = ordered.loc[signature.ne(signature.shift()) & signature.shift().notna(), "period_start_utc"]
    return {
        "check": "config_changes",
        "applicable": True,
        "tracked_columns": valid,
        "n_changes": int(len(changes)),
        "change_points": [str(c) for c in changes.tolist()],
        "stable": bool(len(changes) == 0),
    }


def target_null_report(frame: pd.DataFrame, targets: Sequence[str]) -> dict[str, Any]:
    """Null count of every candidate target."""
    report = {}
    for target in targets:
        if target not in frame.columns:
            continue
        values = frame[target]
        report[target] = {
            "rows": int(len(frame)),
            "nulls": int(values.isna().sum()),
            "non_null": int(values.notna().sum()),
            "min": float(values.min()) if values.notna().any() else None,
            "max": float(values.max()) if values.notna().any() else None,
            "mean": float(values.mean()) if values.notna().any() else None,
        }
    return {"check": "target_nulls", "applicable": bool(report), "by_target": report}


def missingness_report(frame: pd.DataFrame, columns: Sequence[str]) -> pd.DataFrame:
    """Missingness per column of the modelling frame."""
    rows = []
    for column in columns:
        if column not in frame.columns:
            continue
        values = frame[column]
        try:
            distinct = int(values.nunique(dropna=True))
        except TypeError:
            distinct = -1  # unhashable type (e.g., list)
        rows.append(
            {
                "column": column,
                "rows": int(len(frame)),
                "nulls": int(values.isna().sum()),
                "missing_fraction": float(values.isna().mean()) if len(frame) else float("nan"),
                "distinct_values": distinct,
            }
        )
    return pd.DataFrame(rows)


def coverage_summary(frame: pd.DataFrame, time_column: str = "period_start_utc") -> dict[str, Any]:
    """Real coverage of the dataset: first, last, duration and number of windows."""
    if frame.empty or time_column not in frame.columns:
        return {"applicable": False}
    times = pd.to_datetime(frame[time_column], utc=True)
    first, last = times.min(), times.max()
    duration = (last - first).total_seconds()
    return {
        "applicable": True,
        "first_timestamp_utc": str(first),
        "last_timestamp_utc": str(last),
        "duration_seconds": duration,
        "duration_hours": duration / 3600.0,
        "windows": int(len(frame)),
        "expected_windows": int(duration / 3600.0) + 1,
        "window_completeness": float(len(frame) / (duration / 3600.0 + 1)),
    }


def run_all_checks(
    readings: pd.DataFrame,
    hourly: pd.DataFrame,
    *,
    targets: Sequence[str] = (),
    feature_columns: Sequence[str] = (),
) -> dict[str, Any]:
    """Run every data quality check and return a single report."""
    checks: dict[str, Any] = {}
    checks["timestamps_utc"] = check_timestamps_utc(readings)
    checks["granularity"] = check_granularity(readings)
    checks["duplicates"] = check_duplicates(
        readings, ("metric_code", "observed_at_utc", "interval_seconds", "source")
    )
    checks["nulls"] = check_nulls(readings, ("value",))
    checks["ranges"] = check_ranges(readings, EXPECTED_RANGES)
    checks["negative_counts"] = check_negative_counts(readings, COUNT_METRICS)
    checks["quality_status"] = quality_status_counts(readings)
    checks["targets"] = target_null_report(hourly, targets)
    checks["workload_changes"] = detect_workload_changes(hourly)
    checks["config_changes"] = detect_config_changes(hourly)
    checks["coverage"] = coverage_summary(hourly)
    checks["missingness"] = missingness_report(hourly, feature_columns).to_dict("records")
    return checks


def passed_checks(report: Mapping[str, Any]) -> list[str]:
    """Names of the checks that passed."""
    passed = []
    for name, check in report.items():
        if isinstance(check, Mapping) and check.get("applicable") and "passed" in check:
            if check["passed"]:
                passed.append(name)
    return passed


def failed_checks(report: Mapping[str, Any]) -> list[str]:
    """Names of the checks that failed."""
    failed = []
    for name, check in report.items():
        if isinstance(check, Mapping) and check.get("applicable") and "passed" in check:
            if not check["passed"]:
                failed.append(name)
    return failed
