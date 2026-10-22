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
from . import evaluation as ev
from . import features as ft
from . import models as md
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

    X_train, y_train = md.build_matrix(train_imputed, usable_columns, target)
    X_validation, y_validation = md.build_matrix(validation_imputed, usable_columns, target)
    X_test, y_test = md.build_matrix(test_imputed, usable_columns, target)

    scale = ev.naive_scale(train[target])

    # --- candidate models --------------------------------------------------
    catalogue = md.default_catalogue(seed)
    train_metrics: dict[str, Any] = {}
    validation_metrics: dict[str, Any] = {}
    fitted: dict[str, md.Pipeline] = {}
    validation_predictions: dict[str, np.ndarray] = {}

    for spec_model in catalogue:
        if len(X_train) < 5:
            break
        model = md.fit_model(spec_model, X_train, y_train)
        fitted[spec_model.name] = model
        pred_train = md.predict_model(model, X_train)
        pred_validation = md.predict_model(model, X_validation)
        validation_predictions[spec_model.name] = pred_validation
        train_metrics[spec_model.name] = ev.evaluate(
            y_train.to_numpy(), pred_train, scale=scale
        )
        validation_metrics[spec_model.name] = ev.evaluate(
            y_validation.to_numpy(), pred_validation, scale=scale
        )
        print(
            f"[train-real] {spec_model.name:20s} train MAE={train_metrics[spec_model.name]['mae']:.4f} "
            f"val MAE={validation_metrics[spec_model.name]['mae']:.4f}"
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

    # --- model selection on the validation split ---------------------------
    if not validation_metrics:
        raise RuntimeError("no model could be fitted: not enough training rows")
    best_name = min(
        (m for m in validation_metrics if m in fitted),
        key=lambda m: validation_metrics[m]["mae"],
    )
    best_spec = next(s for s in catalogue if s.name == best_name)
    print(f"[train-real] selected model (validation MAE): {best_name}")

    # Refit on train + validation: the test set is touched only here.
    combined = pd.concat([train_imputed, validation_imputed], ignore_index=True)
    X_combined, y_combined = md.build_matrix(combined, usable_columns, target)
    final_model = md.fit_model(best_spec, X_combined, y_combined)
    pred_test = md.predict_model(final_model, X_test)
    test_metrics[best_name] = ev.evaluate(y_test.to_numpy(), pred_test, scale=scale)

    # --- prediction intervals (from validation residuals, no test leakage) --
    residuals_validation = (
        y_validation.to_numpy(dtype=float) - validation_predictions[best_name]
    )
    interval = ev.residual_interval(residuals_validation, coverage=float(args.coverage))
    lower, upper = interval.apply(pred_test)
    interval_stats = ev.interval_metrics(y_test.to_numpy(), lower, upper)
    test_metrics[best_name].update(interval_stats)
    validation_interval = ev.residual_interval(
        np.concatenate([residuals_validation]), coverage=float(args.coverage)
    )

    # --- statistical validation -------------------------------------------
    stats_report = ev.statistical_validation(
        target_series=train[target],
        residuals=residuals_validation,
        exog=None,
        target_values=test[target].to_numpy(),
    )

    # --- leakage evidence --------------------------------------------------
    leakage = ev.leakage_report(modeling, usable_columns, len(modeling) - 2)
    print(f"[train-real] leakage check: {leakage.get('no_leakage')}")

    # --- artifacts ---------------------------------------------------------
    dataset_hash = ft.dataset_hash(modeling)
    feature_hash = ft.feature_definition_hash(usable_columns, spec)
    git_context = _git_context()

    joblib.dump(final_model, output_dir / "model.joblib")

    metadata = md.model_metadata(
        best_spec,
        usable_columns,
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
            "selected_on": "validation_mae",
            "baselines_compared": list(baseline_specs),
            "imputation": imputation_report,
            "prediction_interval": {
                "method": "empirical_quantiles_of_validation_residuals",
                "nominal_coverage": interval.coverage,
                "lower_offset": interval.lower_offset,
                "upper_offset": interval.upper_offset,
                "n_residuals": interval.n_residuals,
                "measured_coverage_on_test": interval_stats["interval_coverage"],
            },
            "leakage_check": leakage,
            "git": git_context,
        },
    )
    _write_json(output_dir / "model_metadata.json", metadata)

    dictionary = pd.DataFrame(
        [
            {
                "feature": column,
                "required": column in spec.required,
                "optional": column in spec.optional,
                "derived": column in spec.derived_names(),
                "missing_indicator": f"feature_missing_{column}" in usable_columns,
            }
            for column in usable_columns
        ]
    )
    _write_frame(output_dir / "feature_dictionary.csv", dictionary)
    (output_dir / "feature_definition_hash.txt").write_text(feature_hash + "\n", encoding="utf-8")

    _write_json(output_dir / "train_metrics.json", train_metrics)
    _write_json(output_dir / "validation_metrics.json", validation_metrics)
    _write_json(output_dir / "test_metrics.json", test_metrics)

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
        "feature_list": usable_columns,
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
    lines = [
        "# Modelo real (Fase A)",
        "",
        f"- Modelo seleccionado: `{model_name}` (por MAE en validación temporal).",
        f"- Target: `{target}` (CPU promedio de la ventana siguiente, +1h).",
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
