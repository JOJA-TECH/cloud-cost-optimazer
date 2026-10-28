"""Parametric workload scenario definitions and curve generators.

Each scenario defines a time-varying intensity λ(t) in operations per minute.
The curves are reproducible given a seed and fully documented.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import numpy as np
import pandas as pd
import yaml

from forecasting.config import Settings


# =============================================================================
# Scenario configuration
# =============================================================================
@dataclass(frozen=True)
class ScenarioConfig:
    """Complete parametrisation of one synthetic scenario."""

    scenario_id: str
    scenario_type: str
    seed: int
    base_intensity_ops_min: float
    amplitude_ops_min: float = 0.0
    period_minutes: float = 360.0
    noise_distribution: str = "truncated_normal"
    noise_scale: float = 0.0
    min_intensity_ops_min: float = 0.0
    max_intensity_ops_min: float = 500.0
    concurrency: int = 2
    batch_size: int = 50
    horizon_hours: int = 168
    params: dict[str, Any] = field(default_factory=dict)

    def as_dict(self) -> dict[str, Any]:
        return {
            "scenario_id": self.scenario_id,
            "scenario_type": self.scenario_type,
            "seed": self.seed,
            "base_intensity_ops_min": self.base_intensity_ops_min,
            "amplitude_ops_min": self.amplitude_ops_min,
            "period_minutes": self.period_minutes,
            "noise_distribution": self.noise_distribution,
            "noise_scale": self.noise_scale,
            "min_intensity_ops_min": self.min_intensity_ops_min,
            "max_intensity_ops_min": self.max_intensity_ops_min,
            "concurrency": self.concurrency,
            "batch_size": self.batch_size,
            "horizon_hours": self.horizon_hours,
            "params": self.params,
        }


def load_scenario_configs(path: str | Path) -> list[ScenarioConfig]:
    """Load scenario configurations from a YAML file."""
    path = Path(path)
    if not path.is_file():
        raise FileNotFoundError(f"scenario config not found: {path}")
    data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    if "scenarios" not in data:
        raise ValueError(f"YAML must contain a 'scenarios' key: {path}")
    configs: list[ScenarioConfig] = []
    for entry in data["scenarios"]:
        configs.append(ScenarioConfig(**entry))
    return configs


# =============================================================================
# Curve generators
# =============================================================================
def _rng(seed: int, step: int) -> np.random.Generator:
    """Deterministic RNG for a given step of the generation."""
    return np.random.default_rng(seed + step * 1009)


def _apply_noise(
    values: np.ndarray, scale: float, distribution: str, seed: int
) -> np.ndarray:
    """Add reproducible noise to a curve."""
    if scale <= 0:
        return values
    n = len(values)
    if distribution == "truncated_normal":
        rng = _rng(seed, 1)
        noise = rng.normal(0, scale, n)
        # Clip to ±3σ to approximate truncation
        noise = np.clip(noise, -3 * scale, 3 * scale)
    elif distribution == "normal":
        rng = _rng(seed, 1)
        noise = rng.normal(0, scale, n)
    elif distribution == "empirical":
        # Not implemented here: requires external residual array
        rng = _rng(seed, 1)
        noise = rng.normal(0, scale, n)
    elif distribution == "poisson":
        rng = _rng(seed, 1)
        # Poisson noise for counts: variance = mean
        noise = rng.poisson(np.maximum(values, 1)) - values
    elif distribution == "lognormal":
        rng = _rng(seed, 1)
        # Multiplicative lognormal noise
        noise = rng.lognormal(mean=0.0, sigma=scale, size=n) - 1.0
        noise = values * noise
    else:
        rng = _rng(seed, 1)
        noise = rng.normal(0, scale, n)
    return values + noise


def _clip(values: np.ndarray, low: float, high: float) -> np.ndarray:
    return np.clip(values, low, high)


def _stable_linspace(start: float, end: float, steps: int) -> np.ndarray:
    return np.linspace(start, end, steps)


def build_workload_curve(config: ScenarioConfig, step_minutes: int = 1) -> pd.DataFrame:
    """Generate the workload intensity curve λ(t) for a scenario.

    Returns a DataFrame with columns:
    - ts: timestamp (UTC)
    - workload_intensity: intensity in ops/min
    - formula: string describing the component (base, periodic, spike, etc.)
    """
    steps = config.horizon_hours * 60 // step_minutes
    t_min = np.arange(steps) * step_minutes
    t_h = t_min / 60.0
    base = config.base_intensity_ops_min

    # ---- base component -----------------------------------------------------
    curve = np.full(steps, base, dtype=float)
    formula = f"base={base:.1f}"

    # ---- type-specific components ------------------------------------------
    if config.scenario_type == "low_stable":
        pass  # flat base only
    elif config.scenario_type == "high_stable":
        pass  # flat base only (base is high)
    elif config.scenario_type == "periodic":
        amp = config.amplitude_ops_min
        period = config.period_minutes
        periodic = amp * np.sin(2 * math.pi * t_min / period)
        curve += periodic
        formula += f" + {amp:.1f}*sin(2πt/{period:.0f}min)"
    elif config.scenario_type == "spike":
        spikes = config.params.get("spikes", [])
        for spike in spikes:
            tau = spike["center_min"]
            sigma = spike["width_min"]
            amp = spike["amplitude"]
            spike_curve = amp * np.exp(-0.5 * ((t_min - tau) / sigma) ** 2)
            curve += spike_curve
            formula += f" + {amp:.1f}*exp(-(t-{tau:.0f})^2/(2*{sigma:.0f}^2))"
    elif config.scenario_type == "increasing":
        rate = config.params.get("rate_per_hour", 10.0)
        max_int = config.params.get("max_intensity", config.max_intensity_ops_min)
        inc = rate * t_h
        curve = np.minimum(base + inc, max_int)
        formula += f" + min({rate:.1f}*t, {max_int:.0f})"
    elif config.scenario_type == "decreasing":
        rate = config.params.get("rate_per_hour", 10.0)
        min_int = config.params.get("min_intensity", config.min_intensity_ops_min)
        dec = rate * t_h
        curve = np.maximum(base - dec, min_int)
        formula += f" - min({rate:.1f}*t, {base - min_int:.0f})"
    elif config.scenario_type == "bursty":
        burst_len = config.params.get("burst_length_min", 30)
        idle_len = config.params.get("idle_length_min", 60)
        burst_int = config.params.get("burst_intensity", config.max_intensity_ops_min)
        idle_int = config.params.get("idle_intensity", config.min_intensity_ops_min)
        cycle = burst_len + idle_len
        in_burst = (t_min % cycle) < burst_len
        curve = np.where(in_burst, burst_int, idle_int)
        formula += f" burst={burst_int:.0f}/{burst_len:.0f}min idle={idle_int:.0f}/{idle_len:.0f}min"
    elif config.scenario_type == "mixed":
        # periodic + spike + noise (handled by noise step)
        amp = config.amplitude_ops_min
        period = config.period_minutes
        periodic = amp * np.sin(2 * math.pi * t_min / period)
        curve += periodic
        formula += f" + {amp:.1f}*sin(2πt/{period:.0f}min)"
        spikes = config.params.get("spikes", [])
        for spike in spikes:
            tau = spike["center_min"]
            sigma = spike["width_min"]
            amp_s = spike["amplitude"]
            spike_curve = amp_s * np.exp(-0.5 * ((t_min - tau) / sigma) ** 2)
            curve += spike_curve
            formula += f" + {amp_s:.1f}*exp(-(t-{tau:.0f})^2/(2*{sigma:.0f}^2))"
    else:
        raise ValueError(f"unknown scenario_type: {config.scenario_type}")

    # ---- noise --------------------------------------------------------------
    curve = _apply_noise(
        curve, config.noise_scale, config.noise_distribution, config.seed
    )

    # ---- clip to bounds -----------------------------------------------------
    curve = _clip(curve, config.min_intensity_ops_min, config.max_intensity_ops_min)

    # ---- build dataframe ----------------------------------------------------
    start_utc = pd.Timestamp("2026-01-01 00:00:00", tz="UTC")
    timestamps = start_utc + pd.to_timedelta(t_min, unit="min")
    df = pd.DataFrame(
        {
            "ts": timestamps,
            "workload_intensity": curve,
            "formula": formula,
        }
    )
    return df


def build_mandatory_scenarios(settings: Settings) -> list[ScenarioConfig]:
    """Default small matrix of mandatory scenarios from the plan."""
    seed = settings.random_seed
    return [
        ScenarioConfig(
            scenario_id="low_stable_01",
            scenario_type="low_stable",
            seed=seed + 1,
            base_intensity_ops_min=50.0,
            noise_distribution="truncated_normal",
            noise_scale=5.0,
            min_intensity_ops_min=0.0,
            max_intensity_ops_min=200.0,
            horizon_hours=168,
        ),
        ScenarioConfig(
            scenario_id="high_stable_01",
            scenario_type="high_stable",
            seed=seed + 2,
            base_intensity_ops_min=800.0,
            noise_distribution="truncated_normal",
            noise_scale=30.0,
            min_intensity_ops_min=500.0,
            max_intensity_ops_min=1500.0,
            horizon_hours=168,
        ),
        ScenarioConfig(
            scenario_id="periodic_01",
            scenario_type="periodic",
            seed=seed + 3,
            base_intensity_ops_min=200.0,
            amplitude_ops_min=100.0,
            period_minutes=360.0,
            noise_distribution="truncated_normal",
            noise_scale=10.0,
            horizon_hours=168,
        ),
        ScenarioConfig(
            scenario_id="bursty_01",
            scenario_type="bursty",
            seed=seed + 4,
            base_intensity_ops_min=200.0,
            noise_scale=10.0,
            params={
                "burst_length_min": 30,
                "idle_length_min": 60,
                "burst_intensity": 800.0,
                "idle_intensity": 50.0,
            },
            horizon_hours=168,
        ),
        ScenarioConfig(
            scenario_id="spike_01",
            scenario_type="spike",
            seed=seed + 5,
            base_intensity_ops_min=100.0,
            params={
                "spikes": [
                    {"center_min": 24 * 60, "width_min": 15, "amplitude": 500.0},
                    {"center_min": 72 * 60, "width_min": 20, "amplitude": 700.0},
                ]
            },
            noise_distribution="truncated_normal",
            noise_scale=15.0,
            horizon_hours=168,
        ),
        ScenarioConfig(
            scenario_id="increasing_01",
            scenario_type="increasing",
            seed=seed + 6,
            base_intensity_ops_min=50.0,
            params={"rate_per_hour": 10.0, "max_intensity": 800.0},
            noise_distribution="truncated_normal",
            noise_scale=8.0,
            horizon_hours=168,
        ),
        ScenarioConfig(
            scenario_id="decreasing_01",
            scenario_type="decreasing",
            seed=seed + 7,
            base_intensity_ops_min=800.0,
            params={"rate_per_hour": 8.0, "min_intensity": 50.0},
            noise_distribution="truncated_normal",
            noise_scale=8.0,
            horizon_hours=168,
        ),
        ScenarioConfig(
            scenario_id="mixed_01",
            scenario_type="mixed",
            seed=seed + 8,
            base_intensity_ops_min=150.0,
            amplitude_ops_min=80.0,
            period_minutes=360.0,
            params={
                "spikes": [
                    {"center_min": 48 * 60, "width_min": 10, "amplitude": 400.0},
                ]
            },
            noise_distribution="truncated_normal",
            noise_scale=12.0,
            horizon_hours=168,
        ),
    ]
