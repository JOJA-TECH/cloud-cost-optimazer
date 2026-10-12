"""Metric normalization: canonical codes, units and source mapping.

Every reading that reaches Supabase carries: code, unit, source, observed_at_utc,
interval_seconds, value and provenance. The collector must never invent a code:
unknown raw names are dropped (and logged), never stored.
"""

from __future__ import annotations

import datetime as dt
from dataclasses import dataclass, field
from typing import Any, Dict, Optional

# Canonical metric code -> (unit, min, max, participates_in_features)
METRIC_CATALOG: Dict[str, tuple] = {
    "cpu_percent": ("percent", 0.0, 100.0, True),
    "storage_percent": ("percent", 0.0, 100.0, True),
    "storage_used_bytes": ("bytes", 0.0, None, True),
    "data_io_percent": ("percent", 0.0, 100.0, True),
    "log_write_percent": ("percent", 0.0, 100.0, True),
    "memory_percent": ("percent", 0.0, 100.0, True),
    "sessions_count": ("count", 0.0, None, True),
    "sessions_percent": ("percent", 0.0, 100.0, True),
    "workers_percent": ("percent", 0.0, 100.0, True),
    "deadlock_count": ("count", 0.0, None, True),
    "query_duration_avg_ms": ("milliseconds", 0.0, None, True),
    "query_duration_sample_ms": ("milliseconds", 0.0, None, True),
    "workload_intensity": ("ops_per_min", 0.0, None, True),
    "dtu_consumption_percent": ("percent", 0.0, 100.0, False),
}

# Raw name (Azure Monitor / DMV) -> canonical code.
NORMALIZATION_MAP: Dict[str, str] = {
    # Azure Monitor for Azure SQL databases
    "cpu_percent": "cpu_percent",
    "physical_data_read_percent": "data_io_percent",
    "log_write_percent": "log_write_percent",
    "dtu_consumption_percent": "dtu_consumption_percent",
    "storage_percent": "storage_percent",
    "storage": "storage_used_bytes",
    "workers_percent": "workers_percent",
    "sessions_percent": "sessions_percent",
    "sessions_count": "sessions_count",
    "deadlock": "deadlock_count",
    "deadlocks": "deadlock_count",
    # sys.dm_db_resource_stats columns
    "avg_cpu_percent": "cpu_percent",
    "avg_data_io_percent": "data_io_percent",
    "avg_log_write_percent": "log_write_percent",
    "avg_memory_usage_percent": "memory_percent",
    # Query Store aggregates
    "avg_duration_ms": "query_duration_avg_ms",
    # Workload generator self-observation
    "ops_per_min": "workload_intensity",
    "operation_duration_ms": "query_duration_sample_ms",
}

PERCENT_UNITS = {"percent", "milliseconds", "bytes", "count", "ops_per_min"}


@dataclass
class MetricReading:
    metric_code: str
    value: float
    unit: str
    observed_at_utc: dt.datetime
    interval_seconds: int
    source: str
    source_record_id: Optional[str] = None
    source_payload: Dict[str, Any] = field(default_factory=dict)
    quality_status: str = "usable"
    quality_warnings: list = field(default_factory=list)

    def as_dict(self) -> dict:
        return {
            "metric_code": self.metric_code,
            "value": self.value,
            "unit": self.unit,
            "observed_at_utc": self.observed_at_utc,
            "interval_seconds": self.interval_seconds,
            "source": self.source,
            "source_record_id": self.source_record_id,
            "source_payload": self.source_payload,
            "quality_status": self.quality_status,
            "quality_warnings": self.quality_warnings,
        }


def canonical_metric(raw_name: str) -> Optional[str]:
    """Map a raw provider name to a canonical code (or None if unknown)."""
    if not raw_name:
        return None
    normalized = raw_name.strip().lower()
    if normalized in METRIC_CATALOG:
        return normalized
    return NORMALIZATION_MAP.get(normalized)


def unit_for(metric_code: str) -> str:
    entry = METRIC_CATALOG.get(metric_code)
    return entry[0] if entry else "unknown"


def bounds_for(metric_code: str) -> tuple:
    entry = METRIC_CATALOG.get(metric_code)
    if not entry:
        return (None, None)
    return (entry[1], entry[2])


def participates_in_features(metric_code: str) -> bool:
    entry = METRIC_CATALOG.get(metric_code)
    return bool(entry[3]) if entry else False


def make_reading(
    raw_name: str,
    value: float,
    observed_at_utc: dt.datetime,
    interval_seconds: int,
    source: str,
    *,
    source_record_id: Optional[str] = None,
    source_payload: Optional[dict] = None,
) -> Optional[MetricReading]:
    code = canonical_metric(raw_name)
    if code is None:
        return None
    return MetricReading(
        metric_code=code,
        value=float(value),
        unit=unit_for(code),
        observed_at_utc=observed_at_utc,
        interval_seconds=interval_seconds,
        source=source,
        source_record_id=source_record_id,
        source_payload=source_payload or {},
    )
