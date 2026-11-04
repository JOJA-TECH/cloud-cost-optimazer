"""Exportación del dataset a un paquete consumible por el pipeline de forecasting.

Escribe CSV (y Parquet si pyarrow está instalado) bajo --out-dir junto con un
manifiesto JSON que incluye: IDs de recursos, configuración, calendario, zona
horaria, semillas, versión de catálogo/código, conteos, huecos, quality flags
y checksums.

El paquete se escribe en disco local para que sobreviva al apagado o
destrucción de la VM. El spool local de recuperación NO es el dataset final:
debe exportarse y copiarse fuera de la VM antes de cualquier destroy.
"""

from __future__ import annotations

import csv
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List

from .. import db_supabase
from ..config import Config, from_env, load_env_file
from ..metrics.normalizer import METRIC_CATALOG_VERSION

EXPORT_VERSION = "1.0.0"


def _checksum(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def _json_default(value):
    if isinstance(value, datetime):
        return value.isoformat()
    raise TypeError(f"not JSON serializable: {type(value)!r}")


def _serialize(value: Any) -> Any:
    if isinstance(value, datetime):
        return value.isoformat()
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False, default=_json_default)
    return value


def _rows_to_csv(path: Path, rows: List[Dict[str, Any]]) -> int:
    if not rows:
        path.write_text("", encoding="utf-8")
        return 0
    fields = list(rows[0].keys())
    with path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=fields, extrasaction="ignore")
        writer.writeheader()
        for row in rows:
            writer.writerow({k: _serialize(row.get(k)) for k in fields})
    return len(rows)


def _try_parquet(path: Path, rows: List[Dict[str, Any]]) -> bool:
    try:
        import pyarrow as pa
        import pyarrow.parquet as pq

        if not rows:
            table = pa.table({})
        else:
            fields = list(rows[0].keys())
            columns = {
                f: pa.array([_serialize(r.get(f)) for r in rows]) for f in fields
            }
            table = pa.table(columns)
        pq.write_table(table, path)
        return True
    except Exception:
        return False


# ---------------------------------------------------------------------------
# Consultas de lectura (solo lectura, sin crear recursos en Azure)
# ---------------------------------------------------------------------------

def _query(conn, sql: str, params: tuple = ()) -> List[Dict[str, Any]]:
    import psycopg2.extras

    with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
        cur.execute(sql, params)
        if cur.description is None:
            return []
        return [dict(r) for r in cur.fetchall()]


def export(cfg: Config, out_dir: str | Path) -> Dict[str, Any]:
    out = Path(out_dir)
    out.mkdir(parents=True, exist_ok=True)
    resource_uid = cfg.resource_uid()
    manifest: Dict[str, Any] = {
        "export_version": EXPORT_VERSION,
        "catalog_version": METRIC_CATALOG_VERSION,
        "exported_at_utc": datetime.now(timezone.utc).isoformat(),
        "timezone": "UTC",
        "experiment_uid": cfg.experiment_uid,
        "resource_uid": resource_uid,
        "deployment_profile": cfg.deployment_profile,
        "azure_sql_targets": cfg.azure_sql_targets,
        "status": "started",
    }

    files: Dict[str, Dict[str, Any]] = {}

    with db_supabase.transaction(cfg) as conn:
        # Experiment / workload calendar.
        experiment_rows = _query(
            conn,
            "select * from experiment.experiment_run where experiment_uid = %s",
            (cfg.experiment_uid,),
        )
        workload_rows = _query(
            conn,
            "select * from experiment.workload_run where experiment_uid = %s "
            "order by started_at_utc",
            (cfg.experiment_uid,),
        )
        collector_rows = _query(
            conn,
            "select * from ops.collector_run where experiment_uid = %s "
            "order by started_at_utc",
            (cfg.experiment_uid,),
        )

        manifest["experiment"] = experiment_rows
        manifest["workload_calendar"] = workload_rows
        manifest["collector_runs"] = collector_rows

        # Raw readings / samples (idempotentes, con huella de origen).
        reading_rows = _query(
            conn,
            "select * from raw.metric_reading where experiment_uid = %s and resource_uid = %s "
            "order by observed_at_utc, metric_code, source",
            (cfg.experiment_uid, resource_uid),
        )
        sample_rows = _query(
            conn,
            "select * from raw.metric_sample where experiment_uid = %s and resource_uid = %s "
            "order by observed_at_utc, metric_code",
            (cfg.experiment_uid, resource_uid),
        )
        manifest["raw_reading_count"] = len(reading_rows)
        manifest["raw_sample_count"] = len(sample_rows)

        # Hourly features (el agregado listo para forecasting).
        feature_rows = _query(
            conn,
            "select * from feat.metric_hourly where experiment_uid = %s and resource_uid = %s "
            "order by period_start_utc",
            (cfg.experiment_uid, resource_uid),
        )
        manifest["feature_hourly_count"] = len(feature_rows)

        # Huecos por métrica (coverage JSON de la última hora disponible).
        manifest["quality"] = {
            "catalog_version": METRIC_CATALOG_VERSION,
            "feature_rows": len(feature_rows),
        }

    # Escritura de archivos.
    writers = [
        ("raw_readings", reading_rows),
        ("raw_samples", sample_rows),
        ("feature_hourly", feature_rows),
        ("workload_calendar", workload_rows),
        ("collector_runs", collector_rows),
    ]
    for name, rows in writers:
        csv_path = out / f"{name}.csv"
        n = _rows_to_csv(csv_path, rows)
        pq_path = out / f"{name}.parquet"
        parquet_ok = _try_parquet(pq_path, rows)
        files[name] = {
            "rows": n,
            "csv": str(csv_path),
            "csv_sha256": _checksum(csv_path),
            "parquet": str(pq_path) if parquet_ok else None,
            "parquet_sha256": _checksum(pq_path) if parquet_ok else None,
        }

    manifest["files"] = files
    manifest["checksums_manifest"] = _checksum(out / "manifest.json")  # se rellena abajo
    manifest["status"] = "completed"

    manifest_path = out / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, indent=2, default=_json_default, ensure_ascii=False), encoding="utf-8")
    manifest["checksums_manifest"] = _checksum(manifest_path)
    manifest_path.write_text(json.dumps(manifest, indent=2, default=_json_default, ensure_ascii=False), encoding="utf-8")

    return manifest


def main() -> int:
    import argparse
    import logging

    parser = argparse.ArgumentParser(description="Exporta el dataset a CSV/Parquet + manifiesto")
    parser.add_argument("--out-dir", required=True, help="directorio de salida del paquete")
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    load_env_file()
    cfg = from_env()
    manifest = export(cfg, args.out_dir)
    print(json.dumps({
        "status": manifest["status"],
        "out_dir": str(args.out_dir),
        "experiment_uid": manifest["experiment_uid"],
        "resource_uid": manifest["resource_uid"],
        "raw_reading_count": manifest["raw_reading_count"],
        "raw_sample_count": manifest["raw_sample_count"],
        "feature_hourly_count": manifest["feature_hourly_count"],
        "files": {k: v["rows"] for k, v in manifest["files"].items()},
    }, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
