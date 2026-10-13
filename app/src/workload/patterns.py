"""Workload shapes (time series over a run) and operation mixes.

A shape maps an elapsed fraction ``f`` in [0, 1] to a multiplier in [0, 1]
applied on top of the configured intensity. This is pure, deterministic logic
(seeded) so experiments are reproducible.
"""

from __future__ import annotations

import math
import random
from typing import Callable, Dict, List

WORKLOAD_CLASSES = [
    "low_stable",
    "high_stable",
    "periodic",
    "bursty",
    "increasing",
    "cpu_intensive",
    "io_intensive",
    "mixed",
]

Shape = Callable[[float], float]


def _clamp(value: float) -> float:
    return min(1.0, max(0.0, value))


def _low_stable(f: float) -> float:
    return 0.2


def _high_stable(f: float) -> float:
    return 1.0


def _periodic(f: float) -> float:
    return _clamp(0.5 + 0.5 * math.sin(2.0 * math.pi * 3.0 * f))


def _bursty(f: float) -> float:
    # Short intense bursts (~12% duty cycle) on a low baseline.
    if (f % 0.25) < 0.12:
        return 1.0
    return 0.2


def _increasing(f: float) -> float:
    return _clamp(0.15 + 0.85 * f)


def _cpu_intensive(f: float) -> float:
    return 0.8


def _io_intensive(f: float) -> float:
    return 0.7


def _mixed(f: float) -> float:
    # Day/night-like wave plus a mid-run plateau.
    base = 0.35 + 0.45 * math.sin(2.0 * math.pi * 2.0 * f)
    return _clamp(base)


SHAPES: Dict[str, Shape] = {
    "low_stable": _low_stable,
    "high_stable": _high_stable,
    "periodic": _periodic,
    "bursty": _bursty,
    "increasing": _increasing,
    "cpu_intensive": _cpu_intensive,
    "io_intensive": _io_intensive,
    "mixed": _mixed,
}

# Operation mix per class: query_templates.template_key -> weight.
_MIX: Dict[str, Dict[str, float]] = {
    "low_stable": {"point_lookup": 0.7, "insert_event": 0.2, "range_scan": 0.1},
    "high_stable": {"insert_event": 0.45, "point_lookup": 0.3, "range_scan": 0.15, "update_row": 0.1},
    "periodic": {"insert_event": 0.35, "point_lookup": 0.35, "range_scan": 0.2, "update_row": 0.1},
    "bursty": {"insert_event": 0.45, "aggregate": 0.2, "point_lookup": 0.25, "update_row": 0.1},
    "increasing": {"insert_event": 0.4, "range_scan": 0.3, "point_lookup": 0.2, "update_row": 0.1},
    "cpu_intensive": {"aggregate": 0.5, "point_lookup": 0.25, "range_scan": 0.25},
    "io_intensive": {"insert_event": 0.5, "range_scan": 0.3, "update_row": 0.2},
    "mixed": {
        "insert_event": 0.25, "point_lookup": 0.25, "range_scan": 0.15,
        "update_row": 0.15, "aggregate": 0.2,
    },
}

DEFAULT_BASE_OPS_PER_MIN_PER_WORKER = 15


def shape_value(workload_class: str, fraction: float) -> float:
    """Multiplier in [0,1] for the given class at a run fraction."""
    fn = SHAPES.get(workload_class)
    return _clamp(fn(fraction)) if fn else 0.5


def next_intensity_factor(
    rng: random.Random,
    current: float,
    jitter: float,
    step: float = 0.25,
) -> float:
    """Bounded random walk of an intensity multiplier around 1.0.

    With ``jitter`` = 0 the factor stays at 1.0 (deterministic, the default).
    Otherwise it drifts within ``[1 - jitter, 1 + jitter]`` so the offered load
    varies naturally while remaining reproducible for a given seed.
    """
    if jitter <= 0:
        return 1.0
    low = 1.0 - jitter
    high = 1.0 + jitter
    value = current + rng.uniform(-step, step)
    return min(high, max(low, value))


def operation_distribution(workload_class: str) -> Dict[str, float]:
    """Copy of the operation mix (template_key -> weight)."""
    return dict(_MIX.get(workload_class, _MIX["mixed"]))


def pick_operation(
    rng: random.Random, workload_class: str
) -> tuple[str, float]:
    """Return (template_key, weight) selected from the class mix."""
    mix = operation_distribution(workload_class)
    keys = list(mix.keys())
    weights = [mix[k] for k in keys]
    return rng.choices(keys, weights=weights, k=1)[0], mix[keys[0]]


def validate_class(workload_class: str) -> None:
    if workload_class not in WORKLOAD_CLASSES:
        raise ValueError(f"unknown workload_class: {workload_class}")


def schedule_phases(
    classes: List[str],
    hours_per_phase: float,
) -> List[dict]:
    """Build the default experiment schedule (A..H phases).

    Each phase defines its workload_class, duration_seconds and intensity.
    """
    phases = []
    for index, wc in enumerate(classes, start=1):
        phases.append(
            {
                "phase": index,
                "workload_class": wc,
                "duration_seconds": int(hours_per_phase * 3600),
                "intensity": 0.7,
                "concurrency": 1,
            }
        )
    return phases
