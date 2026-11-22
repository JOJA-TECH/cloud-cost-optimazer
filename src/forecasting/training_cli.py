#!/usr/bin/env python3
"""CLI para entrenar y promover modelos de forecasting.

Uso:
    python -m forecasting.training_cli train \
        --resource-uid <uuid> \
        --target target_cpu_avg_next1h \
        --model-name cpu_forecast_1h \
        --model-version 1.0.0 \
        --algorithm random_forest \
        --hours 720

    python -m forecasting.training_cli promote \
        --model-name cpu_forecast_1h \
        --model-version 1.0.0

    python -m forecasting.training_cli list
"""
from __future__ import annotations

import argparse
import sys

from forecasting.training import DSN, list_models, promote_model, train_model


def main() -> int:
    parser = argparse.ArgumentParser(description="ML Training CLI")
    sub = parser.add_subparsers(dest="command", required=True)

    # train
    p_train = sub.add_parser("train", help="Entrenar un modelo")
    p_train.add_argument("--resource-uid", required=True)
    p_train.add_argument("--target", default="target_cpu_avg_next1h")
    p_train.add_argument("--model-name", required=True)
    p_train.add_argument("--model-version", required=True)
    p_train.add_argument("--algorithm", default="random_forest")
    p_train.add_argument("--hours", type=int, default=720)
    p_train.add_argument("--model-dir", default=None)
    p_train.add_argument("--dsn", default=DSN)

    # promote
    p_promote = sub.add_parser("promote", help="Promover modelo a champion")
    p_promote.add_argument("--model-name", required=True)
    p_promote.add_argument("--model-version", required=True)
    p_promote.add_argument("--dsn", default=DSN)

    # list
    p_list = sub.add_parser("list", help="Listar modelos")
    p_list.add_argument("--dsn", default=DSN)

    args = parser.parse_args()

    if args.command == "train":
        try:
            result = train_model(
                resource_uid=args.resource_uid,
                target=args.target,
                model_name=args.model_name,
                model_version=args.model_version,
                algorithm=args.algorithm,
                hours=args.hours,
                model_dir=args.model_dir,
                dsn=args.dsn,
            )
            print(f"Entrenamiento exitoso:")
            print(f"  training_run_id: {result['training_run_id']}")
            print(f"  model_version_id: {result['model_version_id']}")
            print(f"  model_name: {result['model_name']}")
            print(f"  model_version: {result['model_version']}")
            print(f"  MAE: {result['mae']:.4f}")
            print(f"  training_rows: {result['training_rows']}")
            print(f"  artefact_path: {result['artefact_path']}")
            return 0
        except ValueError as e:
            print(f"Error: {e}", file=sys.stderr)
            return 1

    elif args.command == "promote":
        try:
            result = promote_model(
                model_name=args.model_name,
                model_version=args.model_version,
                dsn=args.dsn,
            )
            print(f"Promoción exitosa:")
            print(f"  model_name: {result['model_name']}")
            print(f"  model_version: {result['model_version']}")
            print(f"  model_version_id: {result['model_version_id']}")
            print(f"  stage: {result['stage']}")
            return 0
        except ValueError as e:
            print(f"Error: {e}", file=sys.stderr)
            return 1

    elif args.command == "list":
        models = list_models(args.dsn)
        if not models:
            print("No hay modelos registrados.")
            return 0
        print(f"{'Model':<25} {'Ver':<6} {'Stage':<12} {'Algo':<18} {'MAE':<10} {'Rows':<8}")
        print("-" * 85)
        for m in models:
            mae = m.get("metrics_summary", {})
            mae_val = mae.get("mae", "N/A") if isinstance(mae, dict) else mae
            print(
                f"{m['model_name']:<25} {m['version']:<6} "
                f"{m['stage']:<12} {m.get('algorithm', 'N/A'):<18} "
                f"{mae_val:<10} {m.get('training_rows', 'N/A'):<8}"
            )
        return 0

    return 1


if __name__ == "__main__":
    sys.exit(main())
