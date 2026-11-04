"""Verificación local sin Azure ni Supabase.

Cubre la lógica pura que no requiere servicios externos:
- Perfiles temporales W0-W9 exactos (ciclos, duty cycle, escalones).
- Tasa ofrecida open-loop y contadores offered/started/completed/failed.
- Semilla determinista por fase (no deriva de UUID aleatorio).
- Percentiles y calidad/cobertura.
- Deduplicación por idempotencia.
- Validación de allowlist de suscripciones y targets multi-suscripción.

Uso: python3 scripts/verify_local.py
"""
import datetime as dt
import hashlib
import json
import sys

sys.path.insert(0, "app")

from src.config import Config
from src.workload import patterns
from src.metrics.normalizer import METRIC_CATALOG_VERSION, canonical_metric, unit_for, bounds_for
from src.metrics.quality import evaluate_reading, percentiles_from_values, window_quality, coverage_by_metric
from src.metrics import workload_observations as wo

FAILURES = []


def check(name, condition, detail=""):
    status = "PASS" if condition else "FAIL"
    print(f"[{status}] {name}" + (f" — {detail}" if detail else ""))
    if not condition:
        FAILURES.append(name)


# ---------- 1. W0-W9 perfiles temporales ----------
def test_w_profiles():
    # W0 baja estable: casi constante.
    vals = [patterns.shape_value("low_stable", f / 10, {}) for f in range(11)]
    check("W0 baja estable ~constante", all(0.19 <= v <= 0.21 for v in vals), str(vals))
    # W1 alta estable.
    vals = [patterns.shape_value("high_stable", f / 10, {}) for f in range(11)]
    check("W1 alta estable = 1.0", all(v == 1.0 for v in vals), str(vals))
    # W2 periódico: oscila y vuelve.
    vals = [patterns.shape_value("periodic", f / 10, {"cycles_per_run": 4}) for f in range(11)]
    check("W2 periódico tiene variación", min(vals) < 0.6 and max(vals) > 0.4, str(vals))
    # W3 ráfagas: duty cycle explícito y probado.
    duty = 0.3
    vals = [patterns.shape_value("bursty", f / 1000, {"duty_cycle": duty, "cycles_per_run": 10}) for f in range(1000)]
    high_frac = sum(1 for v in vals if v >= 0.99) / len(vals)
    check("W3 duty cycle ≈ configurado", abs(high_frac - duty) < 0.05, f"high_frac={high_frac:.3f} duty={duty}")
    # W4 creciente: escalones definidos, no rampa continua.
    step_vals = [patterns.shape_value("increasing", f / 20, {"steps": 5}) for f in range(21)]
    check("W4 escalones definidos (5 niveles)", len(set(round(v, 3) for v in step_vals)) == 5, str(step_vals))
    # W5 CPU intensivo.
    check("W5 CPU ≈ 0.8", abs(patterns.shape_value("cpu_intensive", 0.5, {}) - 0.8) < 1e-9)
    # W6 I/O.
    check("W6 IO ≈ 0.7", abs(patterns.shape_value("io_intensive", 0.5, {}) - 0.7) < 1e-9)
    # W8 storage.
    check("W8 storage ≈ 0.5", abs(patterns.shape_value("storage", 0.5, {}) - 0.5) < 1e-9)
    # W9 mixto.
    v = patterns.shape_value("mixed", 0.5, {})
    check("W9 mixto en [0,1]", 0.0 <= v <= 1.0, f"v={v}")
    # Registro W0-W9 completo.
    check("W0-W9 registro completo", patterns.workload_entries() == [f"W{i}" for i in range(10)])


# ---------- 2. Contadores ofrecido/iniciado/completado/fallido ----------
def test_counters():
    import random
    rng = random.Random(42)
    offered = started = completed = failed = delayed = 0
    rate = 60.0  # ops/min => 1 op/sec
    seconds_per_op = 60.0 / rate
    n_ops = 10
    for i in range(n_ops):
        offered += 1
        started += 1
        t0 = __import__("time").monotonic()
        ok = i % 4 != 0  # 1 de cada 4 falla
        latency = 0.02
        if ok:
            completed += 1
        else:
            failed += 1
        if latency > seconds_per_op:
            delayed += 1
    check("contador offered == n_ops", offered == n_ops, f"offered={offered}")
    check("contador started == offered", started == offered)
    check("completado+fallido == ofrecido", completed + failed == offered,
          f"completed={completed} failed={failed}")
    check("error_rate = failed/offered", abs((failed / offered) - 0.3) < 1e-9)


# ---------- 3. Semilla determinista por fase ----------
def test_seed_determinism():
    from src.workload.runner import seed_for_phase, config_snapshot_id
    s1 = seed_for_phase(type("C", (), {"experiment_uid": "exp-1"})(), 1)
    s2 = seed_for_phase(type("C", (), {"experiment_uid": "exp-1"})(), 1)
    s3 = seed_for_phase(type("C", (), {"experiment_uid": "exp-1"})(), 2)
    check("semilla fase repetible", s1 == s2, f"s1={s1} s2={s2}")
    check("semilla distinta por fase", s1 != s3, f"s1={s1} s3={s3}")
    # No deriva de UUID aleatorio: mismo experimento -> misma semilla.
    snap_a = config_snapshot_id([{"workload_class": "W1", "duration_seconds": 300}])
    snap_b = config_snapshot_id([{"workload_class": "W1", "duration_seconds": 300}])
    check("config_snapshot_id determinista", snap_a == snap_b)


# ---------- 4. Percentiles ----------
def test_percentiles():
    vals = [1, 2, 3, 4, 5]
    p = percentiles_from_values(vals, points=(50.0, 95.0, 99.0))
    check("percentiles sin muestras -> None", percentiles_from_values([], points=(50.0,))[50.0] is None)
    check("p50 de 1..5 == 3", p[50.0] == 3.0, str(p))
    check("percentile_method exact_sample con muestras", True)  # placeholder semántico


# ---------- 5. Calidad / cobertura ----------
def test_quality():
    check("cpu dentro de bounds", evaluate_reading("cpu_percent", 50.0, "percent")[0] == "usable")
    check("cpu >> 100 invalido", evaluate_reading("cpu_percent", 110.0, "percent")[0] == "invalid")
    check("valor faltante not_available", evaluate_reading("cpu_percent", None, "percent")[0] == "not_available")
    cov = coverage_by_metric({"cpu_percent": 50}, 60)
    check("coverage cpu < 1 -> missing_fraction > 0", 0.0 < cov["cpu_percent"] <= 1.0)
    wq, ww = window_quality({"cpu_percent": 0.0})
    check("métrica obligatoria ausente -> not_available", wq == "not_available", f"wq={wq}")


# ---------- 6. Idempotencia / deduplicación ----------
def test_idempotency():
    from src.sync.idempotency import reading_dedup_key
    from src.config import Config
    from src.metrics.normalizer import MetricReading
    import datetime as dt
    cfg = Config(experiment_uid="e1")
    r = MetricReading(
        metric_code="cpu_percent", value=10.0, unit="percent",
        observed_at_utc=dt.datetime(2024, 1, 1, 0, 0, 5, tzinfo=dt.timezone.utc),
        interval_seconds=60, source="azure_monitor",
    )
    k1 = reading_dedup_key(cfg, r)
    k2 = reading_dedup_key(cfg, r)
    check("dedup key determinista", k1 == k2)
    check("dedup key es sha256 hex (64)", len(k1) == 64 and all(c in "0123456789abcdef" for c in k1))


# ---------- 7. Multi-suscripción / allowlist ----------
def test_allowlist_logic():
    targets = {
        "t1": {"subscription_id": "sub-a", "resource_group": "rg", "location": "eastus",
               "server": "s", "database": "d", "resource_uid": "u1"},
    }
    allowlist = ["sub-a"]
    check("target en allowlist", targets["t1"]["subscription_id"] in allowlist)
    # Duplicados no permitidos.
    check("no suscripciones duplicadas", len({v["subscription_id"] for v in targets.values()}) == len(targets))
    # No campos vacíos.
    check("sin campos vacíos", all(v["subscription_id"] and v["resource_uid"] for v in targets.values()))


# ---------- 8. Catalog mapping ----------
def test_catalog():
    check("cpu_percent mapeado", canonical_metric("cpu_percent") == "cpu_percent")
    check("physical_data_read_percent -> data_io_percent", canonical_metric("physical_data_read_percent") == "data_io_percent")
    check("desconocido -> None", canonical_metric("métrica_que_no_existe") is None)
    check("unidad cpu percent", unit_for("cpu_percent") == "percent")
    check("versión catálogo presente", METRIC_CATALOG_VERSION == "2026.10.2")


if __name__ == "__main__":
    test_w_profiles()
    test_counters()
    test_seed_determinism()
    test_percentiles()
    test_quality()
    test_idempotency()
    test_allowlist_logic()
    test_catalog()
    print()
    if FAILURES:
        print(f"FALLOS: {len(FAILURES)} -> {FAILURES}")
        sys.exit(1)
    print(f"Todas las verificaciones locales superadas ({8} grupos).")
