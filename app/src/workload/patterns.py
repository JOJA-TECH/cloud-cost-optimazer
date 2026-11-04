"""Workload shapes W0-W9 y mezclas de operaciones.

Cada entrada del registro ``W0..W9`` define:
- ``class``: nombre de forma usado por ``shape_value``.
- ``params``: parámetros temporales observables (duración, ciclo, duty cycle, …).
- ``primary_metric``: señal que debe moverse en este escenario.
- ``secondary``: señales que pueden moverse como efecto secundario.
- ``negative_controls``: señales que deberían permanecer aproximadamente
  constantes (no deben interpretarse como movimiento del objetivo).
- ``ops_mix``: mezcla de operaciones (template_key -> peso).

Todas las funciones son puras y deterministas: dada una fracción ``f`` en [0,1]
y unos parámetros, devuelven el mismo multiplicador en [0,1]. Esto hace las
corridas reproducibles semilla a semilla.
"""

from __future__ import annotations

import math
from typing import Callable, Dict, List

Shape = Callable[[float, Dict], float]


def _clamp(value: float) -> float:
    return min(1.0, max(0.0, value))


# ---------------------------------------------------------------------------
# Formas temporales (multiplicador sobre la intensidad base)
# ---------------------------------------------------------------------------

def _low_stable(f: float, params: Dict) -> float:
    """W0: carga baja y estable (control). ~constante."""
    return _clamp(params.get("base", 0.2))


def _high_stable(f: float, params: Dict) -> float:
    """W1: carga alta y estable. Tasa fija calibrada por el piloto."""
    return _clamp(params.get("base", 1.0))


def _periodic(f: float, params: Dict) -> float:
    """W2: ciclo periódico.

    params: period_minutes (5-10), amplitude (0-1), offset (0-1).
    La fracción f se mapea a minutos dentro de la corrida.
    """
    amplitude = _clamp(float(params.get("amplitude", 0.5)))
    offset = _clamp(float(params.get("offset", 0.5)))
    # f es fracción [0,1]; sin total de minutos no podemos conocer el minuto
    # absoluto, así que usamos ciclos sobre la corrida completa.
    cycles_per_run = float(params.get("cycles_per_run", 6))
    value = offset + amplitude * math.sin(2.0 * math.pi * cycles_per_run * f)
    return _clamp(value)


def _bursty(f: float, params: Dict) -> float:
    """W3: ráfagas con duty cycle explícito.

    params: burst_minutes (2-5), recover_minutes (5-10), duty_cycle (0-1).
    duty_cycle = burst_minutes / (burst_minutes + recover_minutes).
    Se alternan pulsos de intensidad 1.0 y recuperación base.
    """
    duty = _clamp(float(params.get("duty_cycle", 0.25)))
    base = _clamp(float(params.get("base", 0.2)))
    # Patrón repetitivo sobre la fracción de corrida.
    cycles_per_run = float(params.get("cycles_per_run", 6))
    phase = (f * cycles_per_run) % 1.0
    return 1.0 if phase < duty else base


def _increasing(f: float, params: Dict) -> float:
    """W4: escalones definidos de carga.

    params: steps (>=2). Cada escalón mantiene un nivel constante durante su
    fracción de la corrida (no es una rampa implícita).
    """
    steps = max(2, int(params.get("steps", 5)))
    step = min(steps - 1, int(f * steps))
    level = step / max(1, steps - 1)
    return _clamp(params.get("base", 0.1) + (1.0 - params.get("base", 0.1)) * level)


def _cpu_intensive(f: float, params: Dict) -> float:
    """W5: cómputo acotado. Resultado esperado de CPU documentado por el piloto."""
    return _clamp(float(params.get("base", 0.8)))


def _io_intensive(f: float, params: Dict) -> float:
    """W6: I/O intensivo. Precondición: working set mayor que la caché."""
    return _clamp(float(params.get("base", 0.7)))


def _storage(f: float, params: Dict) -> float:
    """W8: crecimiento de almacenamiento acotado y medible."""
    return _clamp(float(params.get("base", 0.5)))


def _mixed(f: float, params: Dict) -> float:
    """W9: carga combinada documentada (ola + meseta)."""
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
    "storage": _storage,
    "mixed": _mixed,
}

WORKLOAD_CLASSES = list(SHAPES.keys())


def shape_value(workload_class: str, fraction: float, params: Dict | None = None) -> float:
    """Multiplicador en [0,1] para la clase en la fracción de corrida."""
    fn = SHAPES.get(workload_class)
    if fn is None:
        return 0.5
    return _clamp(fn(fraction, params or {}))


# ---------------------------------------------------------------------------
# Registro W0-W9: clase, parámetros, señal primaria, secundarios y controles
# negativos. El piloto usa esto para calibrar y congelar parámetros.
# ---------------------------------------------------------------------------

W_REGISTRY: Dict[str, Dict] = {
    "W0": {
        "workload_class": "low_stable",
        "params": {"base": 0.2},
        "primary_metric": "cpu_percent",
        "secondary": ("data_io_percent", "log_write_percent"),
        "negative_controls": ("storage_used_bytes", "deadlock_count"),
        "ops_mix": {"point_lookup": 0.7, "range_scan": 0.2, "insert_event": 0.1},
        "read_only": False,
        "description": "Carga baja y estable (control). Señales casi constantes.",
    },
    "W1": {
        "workload_class": "high_stable",
        "params": {"base": 1.0},
        "primary_metric": "cpu_percent",
        "secondary": ("data_io_percent", "log_write_percent", "sessions_count"),
        "negative_controls": ("storage_used_bytes",),
        "ops_mix": {
            "insert_event": 0.45,
            "point_lookup": 0.3,
            "range_scan": 0.15,
            "update_row": 0.1,
        },
        "read_only": False,
        "description": "Carga alta estable, tasa fija calibrada por el piloto.",
    },
    "W2": {
        "workload_class": "periodic",
        "params": {"period_minutes": 7.5, "amplitude": 0.5, "offset": 0.5,
                   "cycles_per_run": 6},
        "primary_metric": "cpu_percent",
        "secondary": ("data_io_percent", "workload_intensity"),
        "negative_controls": ("storage_used_bytes", "deadlock_count"),
        "ops_mix": {
            "insert_event": 0.35,
            "point_lookup": 0.35,
            "range_scan": 0.2,
            "update_row": 0.1,
        },
        "read_only": False,
        "description": "Ciclos periódicos 5-10 min, amplitud y duración configurables.",
    },
    "W3": {
        "workload_class": "bursty",
        "params": {"burst_minutes": 3, "recover_minutes": 7, "duty_cycle": 0.3,
                   "base": 0.2, "cycles_per_run": 6},
        "primary_metric": "cpu_percent",
        "secondary": ("data_io_percent", "log_write_percent"),
        "negative_controls": ("storage_used_bytes",),
        "ops_mix": {
            "insert_event": 0.45,
            "aggregate": 0.2,
            "point_lookup": 0.25,
            "update_row": 0.1,
        },
        "read_only": False,
        "description": "Pulsos 2-5 min con recuperación 5-10 min y duty cycle explícito.",
    },
    "W4": {
        "workload_class": "increasing",
        "params": {"steps": 5, "base": 0.1},
        "primary_metric": "cpu_percent",
        "secondary": ("data_io_percent", "workers_percent"),
        "negative_controls": ("storage_used_bytes",),
        "ops_mix": {
            "insert_event": 0.4,
            "range_scan": 0.3,
            "point_lookup": 0.2,
            "update_row": 0.1,
        },
        "read_only": False,
        "description": "Escalones de carga definidos (no rampa implícita).",
    },
    "W5": {
        "workload_class": "cpu_intensive",
        "params": {"base": 0.8},
        "primary_metric": "cpu_percent",
        "secondary": ("workers_percent", "sessions_count"),
        "negative_controls": ("data_io_percent", "log_write_percent"),
        "ops_mix": {"aggregate": 0.5, "point_lookup": 0.25, "range_scan": 0.25},
        "read_only": False,
        "description": "Consultas de cómputo acotado; resultado de CPU documentado por el piloto.",
    },
    "W6": {
        "workload_class": "io_intensive",
        "params": {"base": 0.7},
        "primary_metric": "data_io_percent",
        "secondary": ("log_write_percent", "cpu_percent"),
        "negative_controls": ("storage_used_bytes",),
        "ops_mix": {"insert_event": 0.5, "range_scan": 0.3, "update_row": 0.2},
        "read_only": False,
        "description": "Lectura/escritura para mover data/log I/O; working set > caché.",
    },
    "W7": {
        "workload_class": "high_stable",
        "params": {"base": 1.0},
        "primary_metric": "sessions_count",
        "secondary": ("workers_percent", "cpu_percent"),
        "negative_controls": ("storage_used_bytes", "deadlock_count"),
        "ops_mix": {"point_lookup": 0.5, "range_scan": 0.3, "insert_event": 0.2},
        "read_only": False,
        "description": "Escalera de workers/conexiones concurrentes; se miden sesiones y workers.",
    },
    "W8": {
        "workload_class": "storage",
        "params": {"base": 0.5},
        "primary_metric": "storage_used_bytes",
        "secondary": ("log_write_percent", "storage_percent"),
        "negative_controls": ("cpu_percent", "data_io_percent"),
        "ops_mix": {"insert_event": 0.9, "update_row": 0.1},
        "read_only": False,
        "description": "Crecimiento acotado y medible con tope preventivo y parada automática.",
    },
    "W9": {
        "workload_class": "mixed",
        "params": {},
        "primary_metric": "cpu_percent",
        "secondary": ("data_io_percent", "log_write_percent", "sessions_count"),
        "negative_controls": ("storage_used_bytes",),
        "ops_mix": {
            "insert_event": 0.25,
            "point_lookup": 0.25,
            "range_scan": 0.15,
            "update_row": 0.15,
            "aggregate": 0.2,
        },
        "read_only": False,
        "description": "Carga combinada documentada.",
    },
}


def workload_entries() -> List[str]:
    return [f"W{i}" for i in range(10)]


def operation_distribution(workload_class: str) -> Dict[str, float]:
    """Mezcla de operaciones para una clase (fallback a mixed)."""
    entry = None
    # Buscar por clase en el registro.
    for info in W_REGISTRY.values():
        if info["workload_class"] == workload_class:
            entry = info
            break
    if entry is not None:
        return dict(entry["ops_mix"])
    return {
        "insert_event": 0.25,
        "point_lookup": 0.25,
        "range_scan": 0.15,
        "update_row": 0.15,
        "aggregate": 0.2,
    }


def pick_operation(
    rng, workload_class: str
):
    """Devuelve (template_key, weight) seleccionado de la mezcla de la clase."""
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
    """Construye el calendario por defecto (fases A..H).

    Cada fase define workload_class, duration_seconds, intensity y concurrency.
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
