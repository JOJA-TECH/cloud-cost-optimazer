"""Provenance utilities for synthetic data.

Every synthetic row must carry its origin, seed, formula, and calibration info.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass, field
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

import pandas as pd

PROVENANCE_COLUMNS: list[str] = [
    "row_origin",
    "scenario_id",
    "scenario_type",
    "scenario_run_uid",
    "base_experiment_uid",
    "base_resource_uid",
    "generation_seed",
    "generation_formula",
    "noise_model",
    "calibration_model_id",
    "calibration_model_version",
    "source_window_start_utc",
    "source_window_end_utc",
    "uncertainty_lower",
    "uncertainty_upper",
]


@dataclass
class SyntheticManifest:
    """Manifest for a synthetic dataset."""

    dataset_hash: str
    feature_definition_hash: str
    source_schema_hash: str
    code_commit: str
    branch: str
    created_at_utc: str
    random_seed: int
    row_count: int
    source_type: str = "synthetic"
    experiment_uid: str = ""
    period_start_utc: str | None = None
    period_end_utc: str | None = None
    scenarios: list[dict[str, Any]] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    def save(self, path: Path) -> None:
        import json

        path.write_text(json.dumps(self.to_dict(), indent=2), encoding="utf-8")


def verify_provenance(frame: pd.DataFrame) -> dict[str, Any]:
    """Check that all provenance columns are present and non-null."""
    missing = [c for c in PROVENANCE_COLUMNS if c not in frame.columns]
    null_counts = {
        c: int(frame[c].isna().sum()) for c in PROVENANCE_COLUMNS if c in frame.columns
    }
    return {
        "missing_columns": missing,
        "null_counts": null_counts,
        "complete": len(missing) == 0 and all(v == 0 for v in null_counts.values()),
    }


def build_manifest(
    frame: pd.DataFrame,
    *,
    dataset_hash: str,
    feature_definition_hash: str,
    source_schema_hash: str,
    code_commit: str,
    branch: str,
    random_seed: int,
    experiment_uid: str,
    scenarios: list[dict[str, Any]],
) -> SyntheticManifest:
    return SyntheticManifest(
        dataset_hash=dataset_hash,
        feature_definition_hash=feature_definition_hash,
        source_schema_hash=source_schema_hash,
        code_commit=code_commit,
        branch=branch,
        created_at_utc=datetime.now(UTC).isoformat(),
        random_seed=random_seed,
        row_count=len(frame),
        experiment_uid=experiment_uid,
        period_start_utc=str(frame["period_start_utc"].min())
        if not frame.empty
        else None,
        period_end_utc=str(frame["period_start_utc"].max())
        if not frame.empty
        else None,
        scenarios=scenarios,
    )
