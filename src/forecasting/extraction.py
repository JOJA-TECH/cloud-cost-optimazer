"""Extraction of the modelling matrix from the local database.

The source of the training matrix is the view ``feat.v_training_base`` of the
``cco_data`` context: one row per hourly window with the features of the
window ``[t, t+1h)`` and the label of the window ``[t+1h, t+2h)``. The join
that builds the label is ``period_start_utc + interval '1 hour'``, so the
label is always the *next* window and never the current one.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Iterable, Sequence

import pandas as pd
import psycopg

from .db import read_frame

#: Columns read from ``feat.v_training_base``.
BASE_COLUMNS: tuple[str, ...] = (
    "resource_id",
    "period_start_utc",
    "period_end_utc",
    "experiment_uid",
    "resource_uid",
    "workload_class",
    "sku_id",
    "compute_model",
    "capacity_vcores",
    "cpu_avg",
    "cpu_p50",
    "cpu_p95",
    "cpu_p99",
    "cpu_max",
    "cpu_slope",
    "data_io_avg",
    "data_io_p95",
    "log_write_avg",
    "log_write_p95",
    "sessions_avg",
    "sessions_p95",
    "workers_avg",
    "storage_used_pct",
    "storage_used_bytes",
    "storage_growth_bytes",
    "is_peak_hour",
    "day_of_week",
    "hour_of_day",
    "workload_intensity",
    "workload_run_id",
    "deadlock_count",
    "latency_p95_ms",
    "memory_percent",
    "sessions_count",
    "sessions_percent",
    "error_rate",
    "latency_avg_ms",
    "throughput_ops_per_second",
    "offered_operations",
    "successful_operations",
    "failed_operations",
    "quality_status",
    "quality_warnings",
    "canonical_sources",
    "coverage_json",
)

#: Label columns (each one is the value of the *next* hourly window).
TARGET_COLUMNS: tuple[str, ...] = (
    "target_cpu_avg_next1h",
    "target_cpu_p95_next1h",
    "target_cpu_max_next1h",
    "target_workload_next1h",
    "target_storage_next1h",
    "target_error_rate_next1h",
)


@dataclass
class TemporalSplit:
    """A temporal partition of the modelling frame."""

    name: str
    frame: pd.DataFrame

    @property
    def rows(self) -> int:
        return len(self.frame)

    @property
    def period_start(self) -> str | None:
        return None if self.frame.empty else str(self.frame["period_start_utc"].min())

    @property
    def period_end(self) -> str | None:
        return None if self.frame.empty else str(self.frame["period_start_utc"].max())


def extract_training_base(
    conn: psycopg.Connection,
    resource_id: int,
    *,
    period_type: str = "1h",
    columns: Sequence[str] = BASE_COLUMNS,
    targets: Sequence[str] = TARGET_COLUMNS,
) -> pd.DataFrame:
    """Read the training matrix of ``feat.v_training_base`` for one resource."""
    available = set(
        read_frame(
            conn,
            "select column_name from information_schema.columns "
            "where table_schema = 'feat' and table_name = 'v_training_base'",
        )["column_name"]
    )
    selected = [c for c in columns if c in available]
    selected_targets = [c for c in targets if c in available]
    if not selected:
        raise ValueError("feat.v_training_base does not expose the expected columns")
    statement = (
        "select " + ", ".join(selected + selected_targets) + " from feat.v_training_base "
        "where resource_id = %s and period_start_utc is not null order by period_start_utc"
    )
    frame = read_frame(conn, statement, (resource_id,))
    if frame.empty:
        return frame
    frame["period_start_utc"] = pd.to_datetime(frame["period_start_utc"], utc=True)
    return frame.reset_index(drop=True)


def extract_hourly_features(
    conn: psycopg.Connection, resource_id: int, period_type: str = "1h"
) -> pd.DataFrame:
    """Read the raw hourly feature store (without the t+1 labels)."""
    frame = read_frame(
        conn,
        """
        select f.*, f.period_end_utc
        from feat.feature_snapshot f
        where f.resource_id = %s and f.period_type = %s
        order by f.period_start_utc
        """,
        (resource_id, period_type),
    )
    if frame.empty:
        return frame
    frame["period_start_utc"] = pd.to_datetime(frame["period_start_utc"], utc=True)
    return frame.reset_index(drop=True)


def temporal_split(
    frame: pd.DataFrame,
    *,
    train: float = 0.70,
    validation: float = 0.15,
    test: float = 0.15,
) -> dict[str, TemporalSplit]:
    """Split the frame by time: train (oldest) -> validation -> test (newest).

    The split is positional over the time-sorted rows, never random: a random
    split would leak the future into the training of the past.
    """
    if abs(train + validation + test - 1.0) > 1e-9:
        raise ValueError("split ratios must sum to 1")
    ordered = frame.sort_values("period_start_utc").reset_index(drop=True)
    total = len(ordered)
    train_end = int(total * train)
    validation_end = train_end + int(total * validation)
    if total == 0:
        empty = TemporalSplit("empty", ordered)
        return {"train": empty, "validation": empty, "test": empty}
    return {
        "train": TemporalSplit("train", ordered.iloc[:train_end].reset_index(drop=True)),
        "validation": TemporalSplit(
            "validation", ordered.iloc[train_end:validation_end].reset_index(drop=True)
        ),
        "test": TemporalSplit("test", ordered.iloc[validation_end:].reset_index(drop=True)),
    }


def walk_forward_splits(
    frame: pd.DataFrame,
    *,
    n_splits: int = 3,
    min_train: int = 5,
    validation_size: int = 3,
) -> list[tuple[pd.DataFrame, pd.DataFrame]]:
    """Expanding-window splits, used when the series is too short for 70/15/15."""
    ordered = frame.sort_values("period_start_utc").reset_index(drop=True)
    splits: list[tuple[pd.DataFrame, pd.DataFrame]] = []
    total = len(ordered)
    for index in range(n_splits):
        train_end = min_train + index * validation_size
        validation_end = train_end + validation_size
        if validation_end > total:
            break
        splits.append(
            (
                ordered.iloc[:train_end].reset_index(drop=True),
                ordered.iloc[train_end:validation_end].reset_index(drop=True),
            )
        )
    return splits


def split_summary(splits: dict[str, TemporalSplit]) -> dict[str, Any]:
    """Machine readable description of the split, stored in the manifests."""
    return {
        name: {
            "rows": split.rows,
            "period_start_utc": split.period_start,
            "period_end_utc": split.period_end,
        }
        for name, split in splits.items()
    }


def iter_targets(frame: pd.DataFrame, targets: Iterable[str]) -> list[str]:
    """Targets that exist in the frame and have at least one non-null value."""
    available: list[str] = []
    for target in targets:
        if target in frame.columns and frame[target].notna().any():
            available.append(target)
    return available
