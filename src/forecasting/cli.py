"""Command line interface of the forecasting pipeline.

    python -m forecasting.cli init-db
    python -m forecasting.cli import-real
    python -m forecasting.cli profile-real --output-dir artifacts/real_profile
    python -m forecasting.cli train-real --target target_cpu_avg_next1h
    python -m forecasting.cli generate-scenarios --config configs/synthetic_scenarios_v1.yaml
    python -m forecasting.cli evaluate-augmentation

Every command is reproducible: the random seed, the dataset hash and the
feature definition hash are written next to the artifacts.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from dataclasses import asdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Sequence

import joblib
import numpy as np
import pandas as pd

from . import baselines as bl
from . import conformal as cf
from . import evaluation as ev
from . import features as ft
from . import models as md
from . import selection as sel
from . import tuning as tn
from . import validation as va
from .config import (
    CONFIG_DIR,
    Settings,
    load_settings,
    redact_dsn,
)
from .db import (
    connect,
    initialize_local_database,
    read_frame,
    source_connection,
    table_row_counts,
)
from .extraction import (
    BASE_COLUMNS,
    TARGET_COLUMNS,
    extract_hourly_features,
    extract_training_base,
    split_summary,
    temporal_split,
)


# =============================================================================
# Helpers
# =============================================================================
def _utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def _write_json(path: Path, payload: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2, default=str), encoding="utf-8")


def _write_frame(path: Path, frame: pd.DataFrame) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    frame.to_csv(path, index=False)


def _git(*args: str) -> str:
    try:
        result = subprocess.run(
            ["git", *args], capture_output=True, text=True, check=False, cwd=Path.cwd()
        )
        return result.stdout.strip()
    except Exception:  # noqa: BLE001 - git is optional
        return ""


def _git_context() -> dict[str, str]:
    return {
        "code_commit": _git("rev-parse", "HEAD"),
        "branch": _git("rev-parse", "--abbrev-ref", "HEAD"),
    }


def _resolve_resource_id(conn, settings: Settings) -> int:
    frame = read_frame(
        conn,
        "select resource_id, resource_uid from core.resource order by resource_id",
    )
    if frame.empty:
        raise RuntimeError("the local database has no resource: run `import-real` first")
    if settings.resource_uid:
        match = frame[frame["resource_uid"].astype(str) == str(settings.resource_uid)]
        if not match.empty:
            return int(match.iloc[0]["resource_id"])
    return int(frame.iloc[0]["resource_id"])


def _source_schema_hash() -> str:
    schema_path = Path(__file__).resolve().parents[2] / "databases" / "cco_data" / "cco_data_schema.sql"
    if not schema_path.is_file():
        return ""
    return ft.file_hash(str(schema_path))


# =============================================================================
# init-db
# =============================================================================
def cmd_init_db(args: argparse.Namespace, settings: Settings) -> int:
    dsn = settings.local_dsn
    print(f"[init-db] target: {redact_dsn(dsn)}")
    with connect(dsn) as conn:
        result = initialize_local_database(conn)
        print(f"[init-db] baseline applied: {result['baseline_applied']}")
        print(f"[init-db] migrations: {result['migrations'] or 'none'}")
        counts = table_row_counts(conn)
    print(f"[init-db] tables: {len(counts)}")
    return 0


# =============================================================================
# import-real
# =============================================================================
def cmd_import_real(args: argparse.Namespace, settings: Settings) -> int:
    from .importer import import_real_data

    print(f"[import-real] source: {redact_dsn(settings.source_dsn)}")
    with source_connection(settings) as source, connect(settings.local_dsn) as local:
        report = import_real_data(source, local, settings)
    payload = report.as_dict()
    print(json.dumps(payload, indent=2))
    return 0


# =============================================================================
# profile-real
# =============================================================================
def cmd_profile_real(args: argparse.Namespace, settings: Settings) -> int:
    output_dir = Path(args.output_dir or (settings.artifacts_path / "real_profile"))
    output_dir.mkdir(parents=True, exist_ok=True)

    with connect(settings.local_dsn) as conn:
        resource_id = _resolve_resource_id(conn, settings)
        readings = read_frame(
            conn,
            """
            select metric_code, source, timestamp_utc as observed_at_utc, interval_seconds,
                   avg_value as value, quality_status, quality_warnings
            from raw.metric_reading
            where resource_id = %s
            order by timestamp_utc
            """,
            (resource_id,),
        )
        samples = read_frame(
            conn,
            """
            select metric_code, source, observed_at_utc, value
            from raw.metric_sample
            where resource_id = %s
            """,
            (resource_id,),
        )
        hourly = extract_hourly_features(conn, resource_id)
        training = extract_training_base(conn, resource_id)
        experiment = read_frame(
            conn,
            """
            select experiment_uid, resource_uid, workload_class, period_start_utc
            from feat.feature_snapshot
            where resource_id = %s
            order by period_start_utc limit 1
            """,
            (resource_id,),
        )
        counts = table_row_counts(conn)

    if not readings.empty:
        readings["observed_at_utc"] = pd.to_datetime(readings["observed_at_utc"], utc=True)
    if not samples.empty:
        samples["observed_at_utc"] = pd.to_datetime(samples["observed_at_utc"], utc=True)

    inventory = (
        readings.groupby(["metric_code", "source"])
        .agg(
            rows=("value", "size"),
            timestamps=("observed_at_utc", "nunique"),
            min_ts=("observed_at_utc", "min"),
            max_ts=("observed_at_utc", "max"),
            null_values=("value", lambda s: int(s.isna().sum())),
        )
        .reset_index()
        .sort_values(["metric_code", "source"])
    )
    inventory["min_ts"] = inventory["min_ts"].astype(str)
    inventory["max_ts"] = inventory["max_ts"].astype(str)
    _write_frame(output_dir / "real_metric_inventory.csv", inventory)

    quality = va.hourly_completeness(
        readings.rename(columns={"observed_at_utc": "observed_at_utc"}),
        time_column="observed_at_utc",
        metric_column="metric_code",
    )
    _write_frame(output_dir / "real_hourly_quality.csv", quality)

    missingness = va.missingness_report(
        hourly, [c for c in BASE_COLUMNS if c not in ("period_start_utc", "period_end_utc")]
    )
    _write_frame(output_dir / "real_missingness.csv", missingness)

    checks = va.run_all_checks(
        readings,
        hourly,
        targets=TARGET_COLUMNS,
        feature_columns=[c for c in BASE_COLUMNS if c not in ("period_start_utc", "period_end_utc")],
    )
    coverage = va.coverage_summary(hourly)
    profile = {
        "generated_at_utc": _utc_now(),
        "experiment_uid": str(experiment.iloc[0]["experiment_uid"]) if not experiment.empty else "",
        "resource_uid": str(experiment.iloc[0]["resource_uid"]) if not experiment.empty else "",
        "resource_id": resource_id,
        "workload_class": str(experiment.iloc[0]["workload_class"]) if not experiment.empty else "",
        "counts": {
            "metric_readings": int(len(readings)),
            "metric_samples": int(len(samples)),
            "hourly_features": int(len(hourly)),
            "training_rows": int(len(training)),
        },
        "coverage": coverage,
        "samples": {
            "rows": int(len(samples)),
            "min_ts": str(samples["observed_at_utc"].min()) if not samples.empty else None,
            "max_ts": str(samples["observed_at_utc"].max()) if not samples.empty else None,
        },
        "table_row_counts": counts.to_dict("records") if not counts.empty else [],
        "checks": checks,
        "checks_passed": va.passed_checks(checks),
        "checks_failed": va.failed_checks(checks),
        "notes": [
            "Las ausencias no se rellenan con cero: se conservan como nulos y se marcan.",
            "Los percentiles de CPU no se derivan de agregados: solo se usan muestras individuales.",
        ],
    }
    _write_json(output_dir / "real_data_profile.json", profile)
    _write_json(settings.artifacts_path / "real_data_profile.json", profile)
    print(f"[profile-real] artifacts written to {output_dir}")
    print(f"[profile-real] failed checks: {profile['checks_failed'] or 'none'}")
    return 0


# =============================================================================
# train-real
# =============================================================================
def _prepare_frames(
    conn, resource_id: int, settings: Settings, target: str
) -> tuple[pd.DataFrame, pd.DataFrame, ft.FeatureSpec]:
    spec = ft.FeatureSpec.from_mapping(settings.extra.get("features"))
    training = extract_training_base(conn, resource_id)
    if training.empty:
        raise RuntimeError("feat.v_training_base is empty: run `import-real` first")
    enriched = ft.add_derived_features(training, spec)
    modeling = ft.build_modeling_frame(enriched, target, spec)
    modeling = modeling[modeling[target].notna()].reset_index(drop=True)
    return training, modeling, spec


def _baseline_block(
    modeling: pd.DataFrame, target: str, splits: dict[str, bl.TemporalSplit] | None = None
) -> dict[str, pd.DataFrame]:
    specs = bl.build_baselines(modeling[target])
    predictions = bl.baseline_predictions(modeling, target, specs)
    for name in specs:
        modeling[f"pred_{name}"] = predictions[f"pred_{name}"].to_numpy()
    return {name: predictions for name, spec in specs.items()}


def cmd_train_real(args: argparse.Namespace, settings: Settings) -> int:
    output_dir = Path(args.output_dir or (settings.artifacts_path / "real_model"))
    output_dir.mkdir(parents=True, exist_ok=True)
    target = args.target or settings.target
    seed = int(args.seed if args.seed is not None else settings.random_seed)

    with connect(settings.local_dsn) as conn:
        resource_id = _resolve_resource_id(conn, settings)
        raw_training, modeling, spec = _prepare_frames(conn, resource_id, settings, target)

    print(f"[train-real] rows after feature engineering: {len(modeling)}")

    # --- baselines ---------------------------------------------------------
    baseline_specs = bl.build_baselines(modeling["cpu_avg"])
    baseline_predictions = bl.baseline_predictions(modeling, target, observed="cpu_avg", specs=baseline_specs)
    for name in baseline_specs:
        modeling[f"pred_{name}"] = baseline_predictions[f"pred_{name}"].to_numpy()

    # --- temporal split (after baselines, so splits inherit the columns) ---
    splits = temporal_split(
        modeling,
        train=settings.split.get("train", 0.70),
        validation=settings.split.get("validation", 0.15),
        test=settings.split.get("test", 0.15),
    )
    print(
        "[train-real] split: "
        + ", ".join(f"{k}={v.rows}" for k, v in splits.items())
    )

    columns = ft.feature_columns(modeling, spec)
    train = splits["train"].frame
    validation = splits["validation"].frame
    test = splits["test"].frame

    # Imputation statistics come from the train split only.
    train_imputed, imputation_report = ft.impute_optional(train, train, columns)
    validation_imputed, _ = ft.impute_optional(train, validation, columns)
    test_imputed, _ = ft.impute_optional(train, test, columns)
    usable_columns = [
        c for c in columns if not imputation_report["imputed"].get(c, {}).get("dropped")
    ]

    catalogue = md.default_catalogue(seed)

    # --- feature selection (calibration window: train + validation) --------
    # The design matrix is wider than long (39 candidates vs 36 calibration
    # rows), so columns are pruned before any model is fitted. The test split
    # is never read here: the window ends where the test set begins.
    calibration_frame = pd.concat([train_imputed, validation_imputed], ignore_index=True)
    X_calibration, y_calibration = md.build_matrix(calibration_frame, usable_columns, target)
    max_features = int(getattr(args, "max_features", 0) or 0)
    if max_features > 0:
        selected_columns, selection_report = sel.select_features(
            X_calibration,
            y_calibration,
            catalogue,
            max_features=max_features,
            seed=seed,
        )
        print(
            f"[train-real] feature selection: {len(usable_columns)} -> {len(selected_columns)} "
            f"candidates (variance: {len(selection_report.dropped_variance)}, "
            f"collinear: {len(selection_report.dropped_collinear)}, "
            f"ranking cut: {max_features}, folds: {selection_report.n_folds})"
        )
    else:
        selected_columns = list(usable_columns)
        selection_report = sel.SelectionReport(
            method="disabled",
            max_features=0,
            n_candidates=len(usable_columns),
            n_rows_calibration=len(train_imputed) + len(validation_imputed),
            n_folds=0,
            dropped_variance=[],
            dropped_collinear={},
            importances=[],
            selected=selected_columns,
            notes=["Seleccion deshabilitada (--max-features <= 0)."],
        )
        print(f"[train-real] feature selection disabled: {len(selected_columns)} candidates")

    X_train, y_train = md.build_matrix(train_imputed, selected_columns, target)
    X_validation, y_validation = md.build_matrix(validation_imputed, selected_columns, target)
    X_test, y_test = md.build_matrix(test_imputed, selected_columns, target)
    X_calib, y_calib = md.build_matrix(calibration_frame, selected_columns, target)

    scale = ev.naive_scale(train[target])

    # --- walk-forward model selection over the calibration window -----------
    # A single 6-point validation split makes the ranking nearly arbitrary;
    # the selection now averages the MAE of expanding-window folds over the
    # whole calibration window (the last fold equals the classic split).
    # The same CV drives a small bias-oriented hyperparameter grid.
    cv_splits = int(getattr(args, "cv_splits", 0) or 0)
    if cv_splits <= 0:
        cv_splits = 5
    # Keep the split geometry feasible: at least one training row must remain
    # before the first test block (see tuning.evaluation_points).
    cv_splits = max(2, min(cv_splits, len(X_calib) // 2 - 1))
    if len(X_calib) < 8:
        raise RuntimeError(
            "calibration window too small for walk-forward CV: "
            f"{len(X_calib)} rows (need at least 8)"
        )

    level_index = (
        selected_columns.index("cpu_avg") if "cpu_avg" in selected_columns else None
    )
    candidates = tn.candidate_catalogue(seed, level_index=level_index)
    cv_results = tn.evaluate_candidates(candidates, X_calib, y_calib, n_splits=cv_splits)
    n_eval_points = tn.evaluation_points(len(X_calib), cv_splits)
    n_diff = sum(1 for candidate in candidates if candidate.differenced)
    print(
        f"[train-real] walk-forward CV: {len(candidates)} candidates "
        f"({len(candidates) - n_diff} level + {n_diff} differenced), "
        f"{cv_splits} folds, {n_eval_points} evaluation points over "
        f"{len(X_calib)} calibration rows"
    )
    for result in cv_results[:5]:
        folds = " ".join(f"{score:.2f}" for score in result.fold_maes)
        print(
            f"[train-real]   {result.candidate.name:32s} CV MAE={result.mean_mae:.4f}  folds=[{folds}]"
        )
    if len(cv_results) > 5:
        print(f"[train-real]   ... and {len(cv_results) - 5} more candidates (cv_results.csv)")
    best_cv = cv_results[0]
    best_candidate = best_cv.candidate
    best_spec = best_candidate.spec
    best_name = best_candidate.name
    print(
        f"[train-real] selected model (walk-forward CV MAE): {best_name} "
        f"({best_cv.mean_mae:.4f}, target={best_candidate.transform_name})"
    )

    # --- selected model on the classic split (continuity metrics) -----------
    # The model fitted on train only feeds the split-conformal calibration
    # residuals on validation; its train/val numbers stay comparable with
    # the previous runs.
    train_metrics: dict[str, Any] = {}
    validation_metrics: dict[str, Any] = {}
    split_predictor = tn.fit_candidate(best_candidate.clone(), X_train, y_train)
    pred_train_selected = tn.predict_candidate(split_predictor, X_train)
    pred_validation = tn.predict_candidate(split_predictor, X_validation)
    validation_predictions: dict[str, np.ndarray] = {best_name: pred_validation}
    train_metrics[best_name] = ev.evaluate(y_train.to_numpy(), pred_train_selected, scale=scale)
    validation_metrics[best_name] = ev.evaluate(
        y_validation.to_numpy(), pred_validation, scale=scale
    )
    print(
        f"[train-real] {best_name:28s} train MAE={train_metrics[best_name]['mae']:.4f} "
        f"val MAE={validation_metrics[best_name]['mae']:.4f} (classic split, for continuity)"
    )

    # --- baseline metrics --------------------------------------------------
    for name in baseline_specs:
        for split_name, frame in (("train", train), ("validation", validation), ("test", test)):
            truth = frame[target].to_numpy(dtype=float)
            pred = frame[f"pred_{name}"].to_numpy(dtype=float)
            metrics = ev.evaluate(truth, pred, scale=scale)
            bucket = train_metrics if split_name == "train" else validation_metrics
            if split_name == "test":
                bucket = None
            if bucket is not None:
                bucket[name] = metrics
    test_metrics: dict[str, Any] = {}
    for name in baseline_specs:
        truth = test[target].to_numpy(dtype=float)
        pred = test[f"pred_{name}"].to_numpy(dtype=float)
        test_metrics[name] = ev.evaluate(truth, pred, scale=scale)

    # Refit on train + validation: the test set is touched only here.
    combined = pd.concat([train_imputed, validation_imputed], ignore_index=True)
    X_combined, y_combined = md.build_matrix(combined, selected_columns, target)
    final_model = tn.fit_candidate(best_candidate.clone(), X_combined, y_combined)
    pred_test = tn.predict_candidate(final_model, X_test)
    test_metrics[best_name] = ev.evaluate(y_test.to_numpy(), pred_test, scale=scale)

    # --- prediction intervals: conformal, calibrated without the test set ---
    # Three conformal constructions are calibrated, all with the test set
    # untouched:
    #   * split conformal (exact): validation residuals of the model fitted on
    #     train only; with n=6 the rank saturates to the max, so it is the
    #     conservative reference;
    #   * jackknife+ nominal: leave-one-out scores over the 36 rows of the
    #     final fit, computed at 1-alpha (formal guarantee 1-2*alpha);
    #   * jackknife+ guaranteed: same scores computed at 1-alpha/2 so the
    #     formal 1-2*alpha' bound reaches the requested coverage. It is the
    #     primary interval: same formal guarantee as split conformal with
    #     roughly half the width.
    residuals_validation = (
        y_validation.to_numpy(dtype=float) - validation_predictions[best_name]
    )
    coverage_level = float(args.coverage)

    legacy_interval = ev.residual_interval(residuals_validation, coverage=coverage_level)
    split_interval = cf.split_conformal_interval(residuals_validation, coverage=coverage_level)
    legacy_lower, legacy_upper = legacy_interval.apply(pred_test)
    legacy_stats = ev.interval_metrics(y_test.to_numpy(), legacy_lower, legacy_upper)

    jackknife_nominal: cf.ConformalInterval | None = None
    jackknife_guaranteed: cf.ConformalInterval | None = None
    if len(X_combined) >= 12:

        def _fit_predict(
            X_tr: pd.DataFrame, y_tr: pd.Series, X_new: pd.DataFrame
        ) -> np.ndarray:
            predictor = tn.fit_candidate(best_candidate.clone(), X_tr, y_tr)
            return tn.predict_candidate(predictor, X_new)

        loo_scores = cf.jackknife_plus_scores(_fit_predict, X_combined, y_combined)
        jackknife_nominal = cf.jackknife_plus_interval(loo_scores, coverage=coverage_level)
        guaranteed_level = 1.0 - (1.0 - coverage_level) / 2.0
        jackknife_guaranteed = cf.jackknife_plus_interval(loo_scores, coverage=guaranteed_level)

    # Primary band: guaranteed jackknife+ when available, split conformal otherwise.
    primary_interval = jackknife_guaranteed if jackknife_guaranteed is not None else split_interval
    lower, upper = primary_interval.apply(pred_test)
    interval_stats = ev.interval_metrics(y_test.to_numpy(), lower, upper)
    test_metrics[best_name].update(interval_stats)

    nan_band = (np.full(len(pred_test), np.nan), np.full(len(pred_test), np.nan))
    split_lower, split_upper = split_interval.apply(pred_test)
    split_stats = ev.interval_metrics(y_test.to_numpy(), split_lower, split_upper)
    nominal_lower, nominal_upper = (
        jackknife_nominal.apply(pred_test) if jackknife_nominal is not None else nan_band
    )
    nominal_stats = (
        ev.interval_metrics(y_test.to_numpy(), nominal_lower, nominal_upper)
        if jackknife_nominal is not None
        else {}
    )

    print(
        f"[train-real] primary interval ({primary_interval.method}, construction level "
        f"{primary_interval.nominal_coverage:.0%}): half width ±{primary_interval.half_width:.4f} "
        f"(rank {primary_interval.finite_sample_rank}/{primary_interval.n_scores}), "
        f"test coverage={interval_stats['interval_coverage']:.2%}"
    )
    print(
        f"[train-real] split conformal (exact): half width ±{split_interval.half_width:.4f} "
        f"(rank {split_interval.finite_sample_rank}/{split_interval.n_scores}), "
        f"test coverage={split_stats['interval_coverage']:.2%}"
    )
    if jackknife_nominal is not None:
        print(
            f"[train-real] jackknife+ nominal (guarantee 1-2a={max(0.0, 1 - 2 * (1 - coverage_level)):.0%}): "
            f"half width ±{jackknife_nominal.half_width:.4f}, "
            f"test coverage={nominal_stats['interval_coverage']:.2%}"
        )
    print(
        f"[train-real] legacy empirical quantiles: test coverage="
        f"{legacy_stats['interval_coverage']:.2%}"
    )

    # --- statistical validation -------------------------------------------
    stats_report = ev.statistical_validation(
        target_series=train[target],
        residuals=residuals_validation,
        exog=None,
        target_values=test[target].to_numpy(),
    )

    # --- leakage evidence --------------------------------------------------
    leakage = ev.leakage_report(modeling, selected_columns, len(modeling) - 2)
    print(f"[train-real] leakage check: {leakage.get('no_leakage')}")

    # --- artifacts ---------------------------------------------------------
    dataset_hash = ft.dataset_hash(modeling)
    feature_hash = ft.feature_definition_hash(selected_columns, spec)
    git_context = _git_context()

    joblib.dump(final_model, output_dir / "model.joblib")

    metadata = md.model_metadata(
        best_spec,
        selected_columns,
        training_rows=len(train),
        validation_rows=len(validation),
        test_rows=len(test),
        training_period=(splits["train"].period_start, splits["train"].period_end),
        validation_period=(splits["validation"].period_start, splits["validation"].period_end),
        test_period=(splits["test"].period_start, splits["test"].period_end),
        dataset_hash=dataset_hash,
        feature_definition_hash=feature_hash,
        random_seed=seed,
        extra={
            "target": target,
            "period_type": settings.period_type,
            "split_strategy": "temporal_70_15_15",
            "selected_on": "walk_forward_cv_mae",
            "target_transform": best_candidate.transform_name,
            "model_selection": {
                "protocol": "expanding_window_walk_forward",
                "n_splits": cv_splits,
                "n_calibration_rows": int(len(X_calib)),
                "n_evaluation_points": int(n_eval_points),
                "n_candidates": len(cv_results),
                "n_differenced_candidates": int(n_diff),
                "target_representations": ["level", "diff"],
                "criterion": "mean MAE across expanding-window folds on the level scale (test set untouched)",
                "selected_model": best_name,
                "selected_target_transform": best_candidate.transform_name,
                "selected_cv_mae": best_cv.mean_mae,
                "grid": tn.grid_description(),
                "top5": [result.as_row() for result in cv_results[:5]],
            },
            "baselines_compared": list(baseline_specs),
            "imputation": imputation_report,
            "feature_selection": {
                **selection_report.as_dict(),
                "n_selected": len(selected_columns),
                "top_importances": selection_report.importances[:10],
            },
            "prediction_interval": {
                "primary_method": primary_interval.method,
                **primary_interval.as_dict(),
                "formal_guarantee_level": coverage_level,
                "measured_coverage_on_test": interval_stats["interval_coverage"],
                "mean_interval_width": interval_stats["mean_interval_width"],
            },
            "prediction_interval_alternates": {
                "split_conformal_exact": {
                    **split_interval.as_dict(),
                    "measured_coverage_on_test": split_stats["interval_coverage"],
                    "mean_interval_width": split_stats["mean_interval_width"],
                },
                "jackknife_plus_nominal": (
                    {
                        **jackknife_nominal.as_dict(),
                        "formal_guarantee_level": max(0.0, 1.0 - 2.0 * (1.0 - coverage_level)),
                        "measured_coverage_on_test": nominal_stats.get("interval_coverage"),
                        "mean_interval_width": nominal_stats.get("mean_interval_width"),
                    }
                    if jackknife_nominal is not None
                    else None
                ),
                "empirical_validation_residuals": {
                    "method": "empirical_quantiles_of_validation_residuals",
                    "nominal_coverage": legacy_interval.coverage,
                    "lower_offset": legacy_interval.lower_offset,
                    "upper_offset": legacy_interval.upper_offset,
                    "half_width": max(
                        abs(legacy_interval.lower_offset), abs(legacy_interval.upper_offset)
                    ),
                    "n_residuals": legacy_interval.n_residuals,
                    "measured_coverage_on_test": legacy_stats["interval_coverage"],
                },
            },
            "interval_caveat": (
                "Las garantias conformales suponen intercambiabilidad entre los puntos de "
                "calibracion y los de test; en series temporales es una aproximacion. Por "
                "eso la cobertura medida en test se reporta siempre junto a la nominal."
            ),
            "leakage_check": leakage,
            "git": git_context,
        },
    )
    _write_json(output_dir / "model_metadata.json", metadata)

    def _drop_reason(column: str) -> str | None:
        if column in selection_report.dropped_variance:
            return "variance"
        if column in selection_report.dropped_collinear:
            return f"collinear:{selection_report.dropped_collinear[column]}"
        if column not in selected_columns:
            return "ranking_cut"
        return None

    dictionary = pd.DataFrame(
        [
            {
                "feature": column,
                "required": column in spec.required,
                "optional": column in spec.optional,
                "derived": column in spec.derived_names(),
                "missing_indicator": f"feature_missing_{column}" in usable_columns,
                "selected": column in selected_columns,
                "dropped_reason": _drop_reason(column),
            }
            for column in usable_columns
        ]
    )
    _write_frame(output_dir / "feature_dictionary.csv", dictionary)
    _write_json(output_dir / "feature_selection.json", selection_report.as_dict())
    if selection_report.importances:
        _write_frame(
            output_dir / "feature_importances.csv",
            pd.DataFrame(selection_report.importances),
        )
    (output_dir / "feature_definition_hash.txt").write_text(feature_hash + "\n", encoding="utf-8")

    _write_json(output_dir / "train_metrics.json", train_metrics)
    _write_json(output_dir / "validation_metrics.json", validation_metrics)
    _write_json(output_dir / "test_metrics.json", test_metrics)
    _write_frame(
        output_dir / "cv_results.csv",
        pd.DataFrame([result.as_row() for result in cv_results]),
    )

    predictions_frame = pd.DataFrame(
        {
            "period_start_utc": test["period_start_utc"].to_numpy(),
            "workload_class": test["workload_class"].to_numpy(),
            "intensity_band": test["intensity_band"].to_numpy(),
            "peak_band": test["peak_band"].to_numpy(),
            "quality_status": test.get("quality_status", pd.Series(["unknown"] * len(test))).to_numpy(),
            "target": test[target].to_numpy(),
            f"pred_{best_name}": pred_test,
            "y_lower": lower,
            "y_upper": upper,
            "y_lower_split_conformal": split_lower,
            "y_upper_split_conformal": split_upper,
            "y_lower_jackknife_nominal": nominal_lower,
            "y_upper_jackknife_nominal": nominal_upper,
            "y_lower_empirical": legacy_lower,
            "y_upper_empirical": legacy_upper,
        }
    )
    for name in baseline_specs:
        predictions_frame[f"pred_{name}"] = test[f"pred_{name}"].to_numpy()
    _write_frame(output_dir / "predictions_test.csv", predictions_frame)

    residuals_frame = predictions_frame[["period_start_utc", "target"]].copy()
    residuals_frame[f"pred_{best_name}"] = pred_test
    y_true = pd.to_numeric(test[target], errors="coerce").to_numpy(dtype=float)
    residuals_frame["residual"] = y_true - pred_test
    residuals_frame["abs_residual"] = residuals_frame["residual"].abs()
    _write_frame(output_dir / "residuals_test.csv", residuals_frame)

    manifest = {
        "dataset_hash": dataset_hash,
        "feature_definition_hash": feature_hash,
        "source_schema_hash": _source_schema_hash(),
        "code_commit": git_context["code_commit"],
        "branch": git_context["branch"],
        "created_at_utc": _utc_now(),
        "random_seed": seed,
        "row_count": int(len(modeling)),
        "source_type": "real",
        "experiment_uid": settings.experiment_uid,
        "period_start_utc": str(modeling["period_start_utc"].min()),
        "period_end_utc": str(modeling["period_start_utc"].max()),
        "split": split_summary(splits),
        "target": target,
        "feature_list": selected_columns,
        "candidate_feature_list": usable_columns,
        "imputation": imputation_report,
        "table_row_counts": counts_of_local(settings),
    }
    _write_json(output_dir / "dataset_manifest.json", manifest)

    grouped = ev.grouped_evaluation(
        bl.add_group_bands(
            pd.DataFrame(
                {
                    "period_start_utc": test["period_start_utc"].to_numpy(),
                    "workload_class": test["workload_class"].to_numpy(),
                    "intensity_band": test["intensity_band"].to_numpy(),
                    "peak_band": test["peak_band"].to_numpy(),
                    "quality_status": test.get(
                        "quality_status", pd.Series(["unknown"] * len(test))
                    ).to_numpy(),
                    "target": test[target].to_numpy(),
                    "prediction": pred_test,
                }
            )
        ),
        y_true_col="target",
        y_pred_col="prediction",
        scale=scale,
    )
    _write_json(output_dir / "test_metrics_by_group.json", grouped)

    stats_payload = {
        "statistical_validation": stats_report,
        "leakage_check": leakage,
        "baselines": {name: asdict(spec_model) for name, spec_model in baseline_specs.items()},
    }
    _write_json(output_dir / "statistical_validation.json", stats_payload)

    (output_dir / "README.md").write_text(
        _real_model_readme(best_name, target, test_metrics, manifest, metadata),
        encoding="utf-8",
    )

    # --- explainability plots ------------------------------------------------
    # Best effort: a plotting failure must never break the training run.
    interval_summary = [
        {
            "label": "jackknife+ garantizado (primario)",
            "half_width": primary_interval.half_width,
            "coverage": interval_stats["interval_coverage"],
        },
        {
            "label": "split conformal exacto",
            "half_width": split_interval.half_width,
            "coverage": split_stats["interval_coverage"],
        },
        {
            "label": "jackknife+ nominal",
            "half_width": jackknife_nominal.half_width if jackknife_nominal is not None else float("nan"),
            "coverage": nominal_stats.get("interval_coverage"),
        },
        {
            "label": "empirico legacy",
            "half_width": max(abs(legacy_interval.lower_offset), abs(legacy_interval.upper_offset)),
            "coverage": legacy_stats["interval_coverage"],
        },
    ]
    try:
        from . import plots as pl

        written_plots = pl.generate_all(
            output_dir,
            modeling=modeling,
            train=train,
            validation=validation,
            test=test,
            target=target,
            X_calib=X_combined,
            selection_report=selection_report,
            cv_results=cv_results,
            y_test_true=y_test.to_numpy(dtype=float),
            pred_test=pred_test,
            lower=lower,
            upper=upper,
            baseline_predictions={
                name: test[f"pred_{name}"].to_numpy(dtype=float) for name in baseline_specs
            },
            model_label=best_name,
            interval_summary=interval_summary,
        )
        print(f"[train-real] {len(written_plots)} explainability plots written to {output_dir / 'plots'}")
    except Exception as exc:  # noqa: BLE001 - plots are best effort
        print(f"[train-real] warning: plots were not generated: {exc}")

    _register_lineage(settings=settings, metadata=metadata, manifest=manifest)
    print(f"[train-real] artifacts written to {output_dir}")
    return 0


def counts_of_local(settings: Settings) -> list[dict[str, Any]]:
    try:
        with connect(settings.local_dsn) as conn:
            counts = table_row_counts(conn)
        return counts.to_dict("records") if not counts.empty else []
    except Exception:  # noqa: BLE001 - the manifest must not break on a DB hiccup
        return []


def _real_model_readme(
    model_name: str,
    target: str,
    test_metrics: dict[str, Any],
    manifest: dict[str, Any],
    metadata: dict[str, Any],
) -> str:
    selection = metadata.get("model_selection", {})
    selection_text = (
        f"por walk-forward CV de {selection.get('n_splits', '?')} folds "
        f"({selection.get('n_evaluation_points', '?')} puntos de evaluación) "
        f"sobre train+validación, test intacto"
        if selection
        else "por MAE en validación temporal"
    )
    lines = [
        "# Modelo real (Fase A)",
        "",
        f"- Modelo seleccionado: `{model_name}` ({selection_text}).",
        f"- Target: `{target}` (CPU promedio de la ventana siguiente, +1h).",
        "- Representación del objetivo: "
        + (
            "diferenciada (Δcpu, reintegrada al nivel)"
            if metadata.get("target_transform") == "diff"
            else "nivel (sin diferenciar)"
        )
        + " — ambas compitieron en el walk-forward CV.",
        f"- Filas del dataset: {manifest['row_count']}.",
        f"- Periodo: {manifest['period_start_utc']} → {manifest['period_end_utc']}.",
        f"- `dataset_hash`: `{manifest['dataset_hash']}`.",
        f"- `feature_definition_hash`: `{metadata['feature_definition_hash']}`.",
        "",
        "## Métricas en el test real",
        "",
        "| modelo | MAE | RMSE | sMAPE | MASE |",
        "|---|---|---|---|---|",
    ]
    for name, metrics in test_metrics.items():
        lines.append(
            f"| {name} | {metrics.get('mae', float('nan')):.4f} | {metrics.get('rmse', float('nan')):.4f} | "
            f"{metrics.get('smape', float('nan')):.2f} | {metrics.get('mase', float('nan')):.4f} |"
        )
    interval = metadata.get("prediction_interval", {})
    if interval:
        nominal = float(interval.get("formal_guarantee_level", 0.0))
        measured = interval.get("measured_coverage_on_test")
        measured_text = f"{float(measured):.0%}" if isinstance(measured, (int, float)) else "n/d"
        lines += [
            "",
            "## Intervalo de predicción",
            "",
            f"- Método primario: `{interval.get('primary_method', 'n/d')}`.",
            f"- Garantía formal: cobertura ≥ {nominal:.0%} bajo intercambiabilidad — "
            f"cobertura medida en test: {measured_text} (n={metadata.get('test_rows', 'n/d')}).",
            f"- Semiancho: ±{float(interval.get('half_width', float('nan'))):.4f} "
            f"(nivel de construcción {float(interval.get('nominal_coverage', 0.0)):.0%}, "
            f"rango {interval.get('finite_sample_rank', 'n/d')}/"
            f"{interval.get('n_calibration_scores', 'n/d')}).",
            "",
            "### Comparación de construcciones conformales",
            "",
            "| método | semiancho | cobertura medida | notas |",
            "|---|---|---|---|",
        ]
        rows = [("primario", interval)]
        for name, block in (metadata.get("prediction_interval_alternates") or {}).items():
            if isinstance(block, dict):
                rows.append((name, block))
        for name, block in rows:
            width = block.get("half_width")
            coverage = block.get("measured_coverage_on_test")
            lines.append(
                f"| {name} | "
                f"±{float(width):.4f} | "
                f"{float(coverage):.0%} | "
                f"{block.get('guarantee', block.get('method', ''))} |"
                if isinstance(width, (int, float)) and isinstance(coverage, (int, float))
                else f"| {name} | n/d | n/d | {block.get('method', '')} |"
            )
        lines += [
            "",
            "> La garantía conformal supone intercambiabilidad entre calibración y test;",
            "> en series temporales es una aproximación, por eso se reporta la cobertura medida.",
        ]
    lines += [
        "",
        "> Este modelo es un forecast experimental del recurso observado. No es un",
        "> modelo universal de capacidad Azure: cubre un solo SKU y una ventana",
        "> temporal corta (ver `artifacts/reports/limitations.md`).",
        "",
    ]
    return "\n".join(lines)


def _register_lineage(
    settings: Settings, metadata: dict[str, Any], manifest: dict[str, Any]
) -> None:
    """Record the dataset, the run and the model in the ml.* tables."""
    try:
        with connect(settings.local_dsn) as connection:
            with connection.cursor() as cur:
                cur.execute(
                    """
                    insert into ml.experiment (code, name, objective, kind, config, started_at)
                    values (%s, %s, %s, 'forecast', %s::jsonb, now())
                    on conflict (code) do nothing
                    """,
                    (
                        "EXP-FORECAST-01",
                        "Forecast CPU 1h del recurso Azure SQL observado",
                        "Baseline de forecasting con datos reales exclusivamente",
                        json.dumps({"target": metadata["target"], "seed": metadata["random_seed"]}),
                    ),
                )
                cur.execute(
                    """
                    insert into ml.training_dataset
                        (name, version, resource_scope, period_start_utc, period_end_utc,
                         period_type, feature_codes, target_column, row_count,
                         split_strategy, dataset_hash, created_by)
                    values (%s, 1, %s, %s, %s, %s, %s, %s, %s, %s::jsonb, %s, 'forecasting-cli')
                    on conflict (name, version) do nothing
                    """,
                    (
                        "cpu_hourly_v1",
                        manifest.get("experiment_uid", "") or "local",
                        manifest["period_start_utc"],
                        manifest["period_end_utc"],
                        settings.period_type,
                        list(metadata["feature_list"]),
                        metadata["target"],
                        manifest["row_count"],
                        json.dumps({"type": "temporal", **settings.split}),
                        manifest["dataset_hash"],
                    ),
                )
                cur.execute(
                    "select dataset_id from ml.training_dataset where name = 'cpu_hourly_v1' and version = 1"
                )
                dataset_id = cur.fetchone()[0]
                cur.execute(
                    """
                    insert into ml.training_run
                        (experiment_id, dataset_id, algorithm, algorithm_params, status,
                         finished_at, trained_by, artifact_uri, notes)
                    select e.experiment_id, %s, %s, %s::jsonb, 'succeeded', now(), 'forecasting-cli',
                           %s, 'Fase A: datos reales exclusivamente'
                    from ml.experiment e where e.code = 'EXP-FORECAST-01'
                    on conflict do nothing
                    """,
                    (
                        dataset_id,
                        metadata["model_name"],
                        json.dumps(metadata["hyperparameters"]),
                        "artifacts/real_model/model.joblib",
                    ),
                )
                cur.execute(
                    """
                    select tr.training_run_id from ml.training_run tr
                    join ml.experiment e on e.experiment_id = tr.experiment_id
                    where e.code = 'EXP-FORECAST-01' and tr.algorithm = %s
                    order by tr.training_run_id desc limit 1
                    """,
                    (metadata["model_name"],),
                )
                row = cur.fetchone()
                if row:
                    cur.execute(
                        """
                        insert into ml.model_version
                            (training_run_id, model_name, version, framework, stage,
                             params, metrics_summary)
                        values (%s, %s, 1, 'scikit-learn', 'champion', %s::jsonb, %s::jsonb)
                        on conflict (model_name, version) do nothing
                        """,
                        (
                            row[0],
                            metadata["model_name"],
                            json.dumps(metadata["hyperparameters"]),
                            json.dumps({"validation": manifest.get("validation_metrics", {})}),
                        ),
                    )
    except Exception as exc:  # noqa: BLE001 - lineage is best effort
        print(f"[train-real] warning: lineage registration failed: {exc}")


# =============================================================================
# generate-scenarios
# =============================================================================
def cmd_generate_scenarios(args: argparse.Namespace, settings: Settings) -> int:
    from .synthetic.generator import generate_all
    from .synthetic.response_model import ResponseModel

    config_path = Path(args.config or (CONFIG_DIR / "synthetic_scenarios_v1.yaml"))
    output_dir = Path(args.output_dir or (settings.artifacts_path / "synthetic_v1"))
    output_dir.mkdir(parents=True, exist_ok=True)

    with connect(settings.local_dsn) as conn:
        resource_id = _resolve_resource_id(conn, settings)
        hourly = extract_hourly_features(conn, resource_id)

    response_model = ResponseModel.fit(hourly, settings=settings)
    response_model.save(output_dir / "response_model.joblib")
    _write_json(output_dir / "response_model_metadata.json", response_model.metadata())

    results = generate_all(
        hourly,
        response_model,
        config_path=config_path,
        settings=settings,
        output_dir=output_dir,
        write_to_db=True,
    )
    manifest = {
        "dataset_hash": ft.dataset_hash(results["frame"]) if not results["frame"].empty else "",
        "feature_definition_hash": ft.feature_definition_hash(
            sorted(results["frame"].columns.tolist()) if not results["frame"].empty else []
        ),
        "source_schema_hash": _source_schema_hash(),
        "created_at_utc": _utc_now(),
        "random_seed": settings.random_seed,
        "row_count": int(len(results["frame"])),
        "source_type": "synthetic",
        "experiment_uid": settings.experiment_uid,
        "period_start_utc": str(results["frame"]["period_start_utc"].min())
        if not results["frame"].empty
        else None,
        "period_end_utc": str(results["frame"]["period_start_utc"].max())
        if not results["frame"].empty
        else None,
        "scenarios": results["summary"],
        "git": _git_context(),
    }
    _write_json(output_dir / "synthetic_dataset_manifest.json", manifest)
    print(f"[generate-scenarios] {len(results['summary'])} scenarios, {len(results['frame'])} rows")
    return 0


# =============================================================================
# evaluate-augmentation
# =============================================================================
def cmd_evaluate_augmentation(args: argparse.Namespace, settings: Settings) -> int:
    from .augmentation import run_augmentation_experiments

    output_dir = Path(args.output_dir or (settings.artifacts_path / "augmentation_v1"))
    output_dir.mkdir(parents=True, exist_ok=True)
    summary = run_augmentation_experiments(settings=settings, output_dir=output_dir)
    _write_json(output_dir / "augmentation_summary.json", summary)
    print(json.dumps(summary["verdict"], indent=2))
    return 0


# =============================================================================
# run-all
# =============================================================================
def cmd_run_all(args: argparse.Namespace, settings: Settings) -> int:
    steps = [
        ("init-db", cmd_init_db),
        ("import-real", cmd_import_real),
        ("profile-real", cmd_profile_real),
        ("train-real", cmd_train_real),
        ("generate-scenarios", cmd_generate_scenarios),
        ("evaluate-augmentation", cmd_evaluate_augmentation),
    ]
    for name, function in steps:
        print(f"\n=== {name} ===")
        code = function(args, settings)
        if code != 0:
            print(f"step {name} failed with code {code}")
            return code
    return 0


# =============================================================================
# Argument parsing
# =============================================================================
def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="forecasting", description=__doc__)
    parser.add_argument("--config", default=None, help="YAML configuration file")
    parser.add_argument("--experiment-uid", default=None)
    parser.add_argument("--seed", type=int, default=None)
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("init-db", help="create the local schema and apply the migration")

    p_import = sub.add_parser("import-real", help="copy the real telemetry into the local database")
    p_import.set_defaults(func=cmd_import_real)

    p_profile = sub.add_parser("profile-real", help="profile the real dataset")
    p_profile.add_argument("--output-dir", default=None)
    p_profile.set_defaults(func=cmd_profile_real)

    p_train = sub.add_parser("train-real", help="train and evaluate on real data only")
    p_train.add_argument("--target", default=None)
    p_train.add_argument("--period-type", default=None)
    p_train.add_argument("--output-dir", default=None)
    p_train.add_argument("--coverage", type=float, default=0.80)
    p_train.add_argument(
        "--max-features",
        type=int,
        default=15,
        help="feature selection cap (variance+collinearity prune, permutation ranking); 0 disables",
    )
    p_train.add_argument(
        "--cv-splits",
        type=int,
        default=5,
        help="expanding-window folds for walk-forward model selection and tuning",
    )
    p_train.set_defaults(func=cmd_train_real)

    p_scen = sub.add_parser("generate-scenarios", help="generate the synthetic scenarios")
    p_scen.add_argument("--config", default=None)
    p_scen.add_argument("--output-dir", default=None)
    p_scen.set_defaults(func=cmd_generate_scenarios)

    p_aug = sub.add_parser("evaluate-augmentation", help="compare real-only vs real+synthetic")
    p_aug.add_argument("--output-dir", default=None)
    p_aug.set_defaults(func=cmd_evaluate_augmentation)

    p_all = sub.add_parser("run-all", help="run the complete pipeline")
    p_all.add_argument("--output-dir", default=None)
    p_all.add_argument("--target", default=None)
    p_all.add_argument("--period-type", default=None)
    p_all.add_argument("--coverage", type=float, default=0.80)
    p_all.add_argument("--max-features", type=int, default=15)
    p_all.add_argument("--cv-splits", type=int, default=5)
    p_all.set_defaults(func=cmd_run_all)

    return parser


def main(argv: Sequence[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    overrides: dict[str, Any] = {}
    if getattr(args, "experiment_uid", None):
        overrides["experiment_uid"] = args.experiment_uid
    if getattr(args, "seed", None) is not None:
        overrides["random_seed"] = args.seed
    if getattr(args, "period_type", None):
        overrides["period_type"] = args.period_type
    settings = load_settings(args.config, **overrides)
    return int(args.func(args, settings))


if __name__ == "__main__":
    sys.exit(main())
