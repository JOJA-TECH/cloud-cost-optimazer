"""Data quality rules and percentile methods.

Rules:
- a value outside the catalog bounds is `invalid` and rejected from features;
- a value slightly out of range (<=5% outside) is `degraded` but kept;
- a missing value is `not_available`;
- a valid value is `usable`.

Percentile policy: exact percentiles require individual samples. When only
avg/min/max exist the method is 'not_available' and no value is produced.
"""

from __future__ import annotations

from typing import Iterable, Optional, Tuple

from .normalizer import METRIC_CATALOG, bounds_for, canonical_metric

QUALITY_STATUSES = ("usable", "degraded", "not_available", "invalid")
PERCENTILE_METHODS = ("exact_sample", "weighted_histogram", "not_available")

DEGRADED_TOLERANCE = 0.05


def evaluate_reading(metric_code: str, value: Optional[float], unit: str) -> Tuple[str, list]:
    """Return (quality_status, warnings) for a single reading."""
    warnings: list[str] = []
    if metric_code not in METRIC_CATALOG:
        return "invalid", [f"unknown_metric:{metric_code}"]
    if value is None:
        return "not_available", ["missing_value"]
    low, high = bounds_for(metric_code)
    if low is not None and value < low:
        if low > 0 and value >= low * (1 - DEGRADED_TOLERANCE):
            warnings.append("below_expected_range")
        elif low == 0 and value >= -abs(low):
            warnings.append("negative_expected_zero")
            return "invalid", warnings
        else:
            return "invalid", ["below_min"]
    if high is not None and value > high:
        if value <= high * (1 + DEGRADED_TOLERANCE):
            warnings.append("above_expected_range")
        else:
            return "invalid", ["above_max"]
    return ("degraded" if warnings else "usable"), warnings


def percentile_method(has_samples: bool) -> str:
    return "exact_sample" if has_samples else "not_available"


def compute_missing_fraction(observed: int, expected: int) -> float:
    if expected <= 0:
        return 0.0
    missing = max(0, expected - observed)
    return round(min(1.0, missing / expected), 4)


def percentiles_from_values(values: Iterable[float], points=(50.0, 95.0, 99.0)) -> dict:
    """Exact percentiles using linear interpolation between order statistics.

    Returns a mapping percentile -> value. Empty input yields not_available
    semantics (the caller must not persist a number in that case).
    """
    ordered = sorted(float(v) for v in values)
    n = len(ordered)
    if n == 0:
        return {p: None for p in points}
    result: dict = {}
    for p in points:
        if n == 1:
            result[p] = ordered[0]
            continue
        rank = (p / 100.0) * (n - 1)
        lower = int(rank)
        upper = min(lower + 1, n - 1)
        frac = rank - lower
        result[p] = ordered[lower] + (ordered[upper] - ordered[lower]) * frac
    return result


def normalize_metric_name(raw_name: str) -> Optional[str]:
    return canonical_metric(raw_name)
