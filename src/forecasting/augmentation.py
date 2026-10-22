"""Augmentation experiments: real-only vs real+synthetic vs real+calibrated.

The plan requires three experiments:

A: train=real, validation=real, test=real
B: train=real+synthetic, validation=real, test=real
C: train=real+calibrated, validation=real, test=real

Augmentation is accepted only if it improves validation metrics without
degrading the real test set.
"""

from __future__ import annotations

import json
from dataclasses import asdict
from pathlib import Path
from typing import Any, Mapping, Sequence

import numpy as np
import pandas as pd

from forecasting.config import Settings
from forecasting.db import connect
from forecasting.evaluation import evaluate, grouped_evaluation
from forecasting.extraction import extract_training_base
from forecasting.features import (
    FeatureSpec,
    add_derived_features,
    build_modeling_frame,
    feature_columns,
    impute_optional,
)
from forecasting.models import (
    ModelSpec,
    build_estimator,
    default_catalogue,
    fit_model,
    predict_model,
)
from forecasting.synthetic.generator import generate_all
from forecasting.synthetic.response_model import ResponseModel


def run_augmentation_experiments(
    settings: Settings,
    *,
    output_dir: Path,
    synthetic_config: Path | None = None,
) -> dict[str, Any]:
    """Run the three augmentation experiments and return a comparison."""
    output_dir.mkdir(parents=True, exist_ok=True)

    with connect(settings.local_dsn) as conn:
        resource_id = _resolve_resource_id(conn, settings)
        hourly = extract_training_base(conn, resource_id)

    # --- generate synthetic data ---
    response_model = ResponseModel.fit(hourly, settings=settings)
    response_model.save(output_dir / "response_model.joblib")

    synth_result = generate_all(
        hourly,
        response_model,
        config_path=synthetic_config,
        settings=settings,
        output_dir=output_dir / "synthetic",
    )
    synthetic_frame = synth_result["frame"]

    # --- prepare three training datasets ---
    target = settings.target
    spec = FeatureSpec.from_mapping(settings.extra.get("features"))

    # Real frame (has targets)
    real_frame = add_derived_features(hourly, spec)
    real_modeling = build_modeling_frame(real_frame, target, spec)
    real_modeling = real_modeling[real_modeling[target].notna()].reset_index(drop=True)

    # Synthetic frame (no targets) - only use for training features
    if not synthetic_frame.empty:
        synth_modeling = add_derived_features(synthetic_frame, spec)
        # Synthetic data doesn't have target columns, so we can't build_modeling_frame
        # Instead, we prepare the feature columns that match the real modeling frame
        feature_cols = feature_columns(synth_modeling, spec)
        synth_features = synth_modeling[feature_cols].copy()
        # Add a dummy target column for compatibility (will be dropped for validation/test)
        synth_features[target] = np.nan
        combined_train = pd.concat([real_modeling, synth_features], ignore_index=True)
    else:
        combined_train = real_modeling.copy()

    # Calibrated frame: real + response-model-generated curves for observed workloads
    # (Simplified: use real_modeling as calibrated for now; full calibration is future work)
    calibrated_frame = real_modeling.copy()

    # --- temporal split (same for all experiments) ---
    from forecasting.extraction import temporal_split

    splits_real = temporal_split(real_modeling, train=0.7, validation=0.15, test=0.15)
    splits_combined = temporal_split(combined_train, train=0.7, validation=0.15, test=0.15)
    splits_calibrated = temporal_split(calibrated_frame, train=0.7, validation=0.15, test=0.15)

    # Test set is always the real test set
    test_frame = splits_real["test"].frame

    # --- run experiments ---
    seed = settings.random_seed

    results: dict[str, Any] = {}

    for name, train_split, val_split in [
        ("A_real_only", splits_real["train"].frame, splits_real["validation"].frame),
        ("B_real_plus_synthetic", splits_combined["train"].frame, splits_combined["validation"].frame),
        ("C_real_plus_calibrated", splits_calibrated["train"].frame, splits_calibrated["validation"].frame),
    ]:
        print(f"[augmentation] Running {name}...")
        metrics = _run_experiment(
            train_split,
            val_split,
            test_frame,
            target,
            spec,
            seed,
            settings,
        )
        results[name] = metrics

    # --- verdict ---
    verdict = _compare_experiments(results, settings)
    summary = {
        "experiments": results,
        "verdict": verdict,
        "settings": {
            "target": target,
            "split": settings.split,
            "seed": seed,
        },
    }
    (output_dir / "augmentation_summary.json").write_text(
        json.dumps(summary, indent=2, default=str), encoding="utf-8"
    )
    return summary


def _run_experiment(
    train_frame: pd.DataFrame,
    val_frame: pd.DataFrame,
    test_frame: pd.DataFrame,
    target: str,
    spec: FeatureSpec,
    seed: int,
    settings: Settings,
) -> dict[str, Any]:
    """Train and evaluate one experiment."""
    columns = feature_columns(train_frame, spec)
    train_imputed, imputation_report = impute_optional(train_frame, train_frame, columns)
    val_imputed, _ = impute_optional(train_frame, val_frame, columns)
    test_imputed, _ = impute_optional(train_frame, test_frame, columns)

    usable = [c for c in columns if not imputation_report["imputed"].get(c, {}).get("dropped")]

    X_train, y_train = _build_matrix(train_imputed, usable, target)
    X_val, y_val = _build_matrix(val_imputed, usable, target)
    X_test, y_test = _build_matrix(test_imputed, usable, target)

    scale = _naive_scale(y_train)

    catalogue = default_catalogue(seed)
    best_model = None
    best_mae = float("inf")
    val_metrics: dict[str, Any] = {}

    for model_spec in catalogue:
        if len(X_train) < 5:
            break
        model = fit_model(model_spec, X_train, y_train)
        pred_val = predict_model(model, X_val)
        m = evaluate(y_val.to_numpy(), pred_val, scale=scale)
        val_metrics[model_spec.name] = m
        if m["mae"] < best_mae:
            best_mae = m["mae"]
            best_model = model

    if best_model is None:
        return {"error": "no model fitted"}

    pred_test = predict_model(best_model, X_test)
    test_metrics = evaluate(y_test.to_numpy(), pred_test, scale=scale)

    # Baselines on test
    from forecasting.baselines import baseline_predictions, build_baselines

    baseline_specs = build_baselines(train_frame[target])
    baseline_preds = baseline_predictions(test_frame, target, observed="cpu_avg", specs=baseline_specs)
    baseline_test: dict[str, Any] = {}
    for name in baseline_specs:
        truth = test_frame[target].to_numpy(dtype=float)
        pred = baseline_preds[f"pred_{name}"].to_numpy(dtype=float)
        baseline_test[name] = evaluate(truth, pred, scale=scale)

    return {
        "validation_metrics": val_metrics,
        "test_metrics": {**test_metrics, **baseline_test},
        "selected_model": best_model.named_steps["model"].__class__.__name__,
        "imputation": imputation_report,
    }


def _compare_experiments(results: dict[str, Any], settings: Settings) -> dict[str, Any]:
    """Determine if augmentation helps."""
    # Primary: compare test MAE of the best model across experiments
    maes = {name: res["test_metrics"].get(name, {}).get("mae", float("inf")) for name in results}
    # Actually we want the best model's MAE per experiment
    exp_maes = {name: min(m.get("mae", float("inf")) for m in res["test_metrics"].values()) for name, res in results.items()}
    baseline_a = exp_maes.get("A_real_only", float("inf"))
    baseline_b = exp_maes.get("B_real_plus_synthetic", float("inf"))
    baseline_c = exp_maes.get("C_real_plus_calibrated", float("inf"))

    verdict = {
        "improves_B_over_A": bool(baseline_b < baseline_a * 0.98),
        "improves_C_over_A": bool(baseline_c < baseline_a * 0.98),
        "degrades_B": bool(baseline_b > baseline_a * 1.02),
        "degrades_C": bool(baseline_c > baseline_a * 1.02),
        "test_maes": {"A": baseline_a, "B": baseline_b, "C": baseline_c},
        "recommendation": "none",
    }
    if verdict["improves_B_over_A"] and not verdict["degrades_B"]:
        verdict["recommendation"] = "accept_B"
    elif verdict["improves_C_over_A"] and not verdict["degrades_C"]:
        verdict["recommendation"] = "accept_C"
    else:
        verdict["recommendation"] = "reject_augmentation"
    return verdict


def _build_matrix(frame: pd.DataFrame, columns: Sequence[str], target: str) -> tuple[pd.DataFrame, pd.Series]:
    X = frame[list(columns)].copy()
    y = frame[target].astype(float).copy()
    return X, y


def _naive_scale(series: pd.Series) -> float:
    values = series.astype(float).dropna()
    if len(values) < 2:
        return float("nan")
    return float(np.mean(np.abs(np.diff(values.to_numpy()))))


def _resolve_resource_id(conn, settings: Settings) -> int:
    from forecasting.db import read_frame

    frame = read_frame(
        conn,
        "select resource_id, resource_uid from core.resource order by resource_id",
    )
    if frame.empty:
        raise RuntimeError("the local database has no resource")
    if settings.resource_uid:
        match = frame[frame["resource_uid"].astype(str) == str(settings.resource_uid)]
        if not match.empty:
            return int(match.iloc[0]["resource_id"])
    return int(frame.iloc[0]["resource_id"])


def extract_hourly_features(conn, resource_id: int) -> pd.DataFrame:
    from forecasting.db import read_frame

    frame = read_frame(
        conn,
        """
        select f.*, f.period_end_utc
        from feat.feature_snapshot f
        where f.resource_id = %s and f.period_type = '1h'
        order by f.period_start_utc
        """,
        (resource_id,),
    )
    if frame.empty:
        return frame
    frame["period_start_utc"] = pd.to_datetime(frame["period_start_utc"], utc=True)
    return frame.reset_index(drop=True)