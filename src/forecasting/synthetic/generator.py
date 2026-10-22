"""Scenario generator: combines workload curves with the response model."""

from __future__ import annotations

import json
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Mapping, Sequence

import pandas as pd

from forecasting.config import Settings
from forecasting.synthetic.scenarios import ScenarioConfig, build_workload_curve
from forecasting.synthetic.response_model import ResponseModel


@dataclass
class ScenarioRunInfo:
    """Metadata for one generated scenario run."""

    scenario_run_uid: str
    scenario_id: str
    scenario_type: str
    base_experiment_uid: str
    base_resource_uid: str
    generation_seed: int
    formula: str
    parameter_json: dict[str, Any]
    calibration_model_id: str | None
    calibration_model_version: str | None
    started_at_utc: str
    ended_at_utc: str
    row_count: int

    def as_dict(self) -> dict[str, Any]:
        return asdict(self)


def generate_one(
    config: ScenarioConfig,
    response_model: ResponseModel,
    settings: Settings,
    *,
    base_experiment_uid: str,
    base_resource_uid: str,
    source_window_start_utc: str,
    source_window_end_utc: str,
    output_dir: Path,
    write_csv: bool = True,
    write_parquet: bool = False,
) -> tuple[ScenarioRunInfo, pd.DataFrame]:
    """Generate a single scenario and return metadata + frame."""
    import uuid

    scenario_run_uid = str(uuid.uuid4())
    started = datetime.now(timezone.utc).isoformat()

    # 1. Build workload curve
    curve = build_workload_curve(config, step_minutes=60)  # hourly steps

    # 2. Predict resource response
    predicted = response_model.predict(
        curve,
        settings,
        seed=config.seed,
    )

    # 3. Add provenance columns
    predicted = add_provenance(
        predicted,
        config,
        scenario_run_uid,
        base_experiment_uid,
        base_resource_uid,
        source_window_start_utc,
        source_window_end_utc,
        response_model,
    )

    # 4. Write artifacts
    if write_csv:
        csv_path = output_dir / f"{config.scenario_id}.csv"
        predicted.to_csv(csv_path, index=False)
    if write_parquet:
        pq_path = output_dir / f"{config.scenario_id}.parquet"
        predicted.to_parquet(pq_path, index=False)

    ended = datetime.now(timezone.utc).isoformat()
    info = ScenarioRunInfo(
        scenario_run_uid=scenario_run_uid,
        scenario_id=config.scenario_id,
        scenario_type=config.scenario_type,
        base_experiment_uid=base_experiment_uid,
        base_resource_uid=base_resource_uid,
        generation_seed=config.seed,
        formula=config.scenario_type,
        parameter_json=config.as_dict(),
        calibration_model_id=response_model.metadata_.get("model_name"),
        calibration_model_version="1.0.0",
        started_at_utc=started,
        ended_at_utc=ended,
        row_count=len(predicted),
    )
    return info, predicted


def generate_all(
    hourly: pd.DataFrame,
    response_model: ResponseModel,
    *,
    config_path: Path | None = None,
    settings: Settings,
    output_dir: Path,
    write_to_db: bool = False,
) -> dict[str, Any]:
    """Generate all scenarios from config file or mandatory list."""
    output_dir.mkdir(parents=True, exist_ok=True)

    if config_path and config_path.exists():
        configs = load_scenario_configs(config_path)
    else:
        configs = build_mandatory_scenarios(settings)

    base_experiment_uid = settings.experiment_uid
    base_resource_uid = settings.resource_uid
    source_start = str(hourly["period_start_utc"].min())
    source_end = str(hourly["period_start_utc"].max())

    results: list[pd.DataFrame] = []
    run_infos: list[ScenarioRunInfo] = []

    for config in configs:
        info, frame = generate_one(
            config,
            response_model,
            settings,
            base_experiment_uid=base_experiment_uid,
            base_resource_uid=base_resource_uid,
            source_window_start_utc=source_start,
            source_window_end_utc=source_end,
            output_dir=output_dir,
        )
        run_infos.append(info)
        results.append(frame)

    combined = pd.concat(results, ignore_index=True) if results else pd.DataFrame()

    summary = [info.as_dict() for info in run_infos]
    return {"frame": combined, "summary": summary, "runs": run_infos}


def add_provenance(
    frame: pd.DataFrame,
    config: ScenarioConfig,
    scenario_run_uid: str,
    base_experiment_uid: str,
    base_resource_uid: str,
    source_start: str,
    source_end: str,
    response_model: ResponseModel,
) -> pd.DataFrame:
    """Add the provenance columns required by the plan."""
    out = frame.copy()
    out["row_origin"] = "synthetic_scenario"
    out["scenario_id"] = config.scenario_id
    out["scenario_type"] = config.scenario_type
    out["scenario_run_uid"] = scenario_run_uid
    out["base_experiment_uid"] = base_experiment_uid
    out["base_resource_uid"] = base_resource_uid
    out["generation_seed"] = config.seed
    out["generation_formula"] = config.scenario_type
    out["noise_model"] = config.noise_distribution
    out["calibration_model_id"] = response_model.metadata_.get("model_name", "response_model_v1")
    out["calibration_model_version"] = "1.0.0"
    out["source_window_start_utc"] = source_start
    out["source_window_end_utc"] = source_end
    out["uncertainty_lower"] = out.get("cpu_avg", 0) - out.get("cpu_avg", 0).std()
    out["uncertainty_upper"] = out.get("cpu_avg", 0) + out.get("cpu_avg", 0).std()
    return out


def load_scenario_configs(path: Path) -> list[ScenarioConfig]:
    """Load scenario configs (delegates to scenarios module)."""
    from forecasting.synthetic.scenarios import load_scenario_configs as _load

    return _load(path)


def build_mandatory_scenarios(settings: Settings) -> list[ScenarioConfig]:
    """Default scenarios (delegates to scenarios module)."""
    from forecasting.synthetic.scenarios import build_mandatory_scenarios as _build

    return _build(settings)