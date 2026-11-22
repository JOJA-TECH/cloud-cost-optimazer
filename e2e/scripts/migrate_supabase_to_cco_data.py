#!/usr/bin/env python3
"""Migra datos de Supabase (dataset-metrics-collector) a cco_data del backend.

Lee feat.metric_hourly de Supabase (SOURCE_DSN) y lo inserta en
feat.feature_snapshot de cco_data (CCO_DATA_DSN), insertando primero los
recursos necesarios en core.resource y el SKU en catalog.sql_sku.

Idempotente: usa ON CONFLICT DO NOTHING, por lo que puede reejecutarse sin
duplicar filas.

Variables de entorno requeridas:
    SOURCE_DSN        DSN de Supabase (lectura solo de telemetría recolectada)
    CCO_DATA_DSN      DSN de cco_data (escritura de features para ML)
    SUPABASE_SCHEMA   schema de Supabase donde está feat.metric_hourly (default: feat)
    CCO_SCHEMA        schema de cco_data donde está feat.feature_snapshot (default: feat)

Ejemplo:
    SOURCE_DSN="postgresql://..." CCO_DATA_DSN="postgresql://..." \
        python e2e/scripts/migrate_supabase_to_cco_data.py
"""
from __future__ import annotations

import os
import sys
from datetime import timezone


def _require_env(name: str) -> str:
    value = os.environ.get(name)
    if not value:
        raise SystemExit(f"Falta variable de entorno requerida: {name}")
    return value


SUPABASE_DSN = _require_env("SOURCE_DSN")
CCO_DATA_DSN = _require_env("CCO_DATA_DSN")
SUPABASE_FEAT_SCHEMA = os.environ.get("SUPABASE_FEAT_SCHEMA", "feat")
CCO_FEAT_SCHEMA = os.environ.get("CCO_FEAT_SCHEMA", "feat")


def get_or_create_provider(cur_cco, provider_code: str = "azure") -> str:
    cur_cco.execute(
        "SELECT 1 FROM catalog.cloud_provider WHERE provider_code = %s",
        (provider_code,),
    )
    if not cur_cco.fetchone():
        cur_cco.execute(
            "INSERT INTO catalog.cloud_provider (provider_code, name, enabled) "
            "VALUES (%s, %s, %s)",
            (provider_code, provider_code.title(), True),
        )
    return provider_code


def get_or_create_region(cur_cco, provider_code: str, region_code: str) -> tuple[str, str]:
    cur_cco.execute(
        "SELECT 1 FROM catalog.region WHERE provider_code = %s AND region_code = %s",
        (provider_code, region_code),
    )
    if not cur_cco.fetchone():
        cur_cco.execute(
            "INSERT INTO catalog.region (provider_code, region_code, display_name) "
            "VALUES (%s, %s, %s)",
            (provider_code, region_code, region_code),
        )
    return provider_code, region_code


def get_or_create_sku(cur_cco, sku_name: str = "GP_S_Gen5_1") -> int:
    """Obtener o crear SKU en cco_data catalog.sql_sku"""
    cur_cco.execute("SELECT sku_id FROM catalog.sql_sku WHERE sku_name = %s", (sku_name,))
    row = cur_cco.fetchone()
    if row:
        return row[0]
    cur_cco.execute(
        """
        INSERT INTO catalog.sql_sku (
            sku_name, arm_sku_name, tier, family, compute_model,
            capacity_vcores, min_capacity_vcores, max_size_gb, in_scope_r1
        )
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s) RETURNING sku_id
        """,
        (
            sku_name,
            f"SQLDB_{sku_name}",
            "GeneralPurpose",
            "Gen5",
            "serverless",
            1.0,
            0.5,
            32,
            True,
        ),
    )
    return cur_cco.fetchone()[0]


def get_or_create_resource(cur_cco, resource_uid: str, sku_id: int) -> int:
    """Insertar recurso en cco_data.core.resource si no existe"""
    cur_cco.execute(
        "SELECT resource_id FROM core.resource WHERE resource_uid = %s",
        (str(resource_uid),),
    )
    row = cur_cco.fetchone()
    if row:
        return row[0]
    # Asegurar proveedor y región (FK)
    get_or_create_provider(cur_cco, "azure")
    get_or_create_region(cur_cco, "azure", "centralus")
    cur_cco.execute(
        """
        INSERT INTO core.resource (
            resource_uid, provider_code, subscription_id, resource_group,
            server_name, database_name, azure_resource_id, region_code,
            kind, lifecycle_status
        )
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s) RETURNING resource_id
        """,
        (
            str(resource_uid),
            "azure",
            "supabase-sub",
            "supabase-rg",
            "supabase-server",
            f"supabase-db-{str(resource_uid)[:8]}",
            f"/subscriptions/supabase-sub/resourceGroups/supabase-rg/"
            f"providers/Microsoft.Sql/servers/supabase-server/"
            f"databases/supabase-db-{str(resource_uid)[:8]}",
            "centralus",
            "v12.0,user,vcore,serverless",
            "active",
        ),
    )
    return cur_cco.fetchone()[0]


def _to_utc(dt) -> object:
    """Asegura que un timestamp tenga zona horaria UTC."""
    if dt is None:
        return None
    if dt.tzinfo is None:
        return dt.replace(tzinfo=timezone.utc)
    return dt


def migrate() -> None:
    import psycopg2

    supa = psycopg2.connect(SUPABASE_DSN, connect_timeout=10)
    supa.autocommit = True
    cco = psycopg2.connect(CCO_DATA_DSN)
    cco.autocommit = False

    try:
        # 1. Obtener o crear SKU
        with cco.cursor() as cur:
            sku_id = get_or_create_sku(cur, "GP_S_Gen5_1")
            cco.commit()
            print(f"SKU GP_S_Gen5_1 id: {sku_id}")

        # 2. Leer recursos únicos de Supabase
        with supa.cursor() as cur:
            cur.execute(
                f"""
                SELECT DISTINCT resource_uid
                FROM {SUPABASE_FEAT_SCHEMA}.metric_hourly
                ORDER BY resource_uid
                """
            )
            supa_resources = [r[0] for r in cur.fetchall()]
            print(
                f"Recursos en Supabase {SUPABASE_FEAT_SCHEMA}.metric_hourly: "
                f"{len(supa_resources)}"
            )
            for r in supa_resources:
                print(f"  {r}")

        # 3. Insertar recursos en cco_data
        resource_map: dict[str, int] = {}
        with cco.cursor() as cur:
            for ru in supa_resources:
                resource_id = get_or_create_resource(cur, str(ru), sku_id)
                resource_map[str(ru)] = resource_id
                print(f"  Recurso {ru} -> cco_data resource_id {resource_id}")
            cco.commit()

        # 4. Leer feat.metric_hourly de Supabase
        with supa.cursor() as cur:
            cur.execute(
                f"""
                SELECT feature_row_uid, experiment_uid, resource_uid,
                       period_start_utc, period_end_utc, workload_class,
                       cpu_avg, cpu_p50, cpu_p95, cpu_p99, cpu_max,
                       data_io_avg, log_write_avg, sessions_avg, workers_avg,
                       storage_used_pct, storage_used_bytes, storage_growth_bytes,
                       workload_intensity, deadlock_count, latency_p95_ms,
                       sample_count, missing_fraction, quality_status,
                       quality_warnings, computed_at_utc
                FROM {SUPABASE_FEAT_SCHEMA}.metric_hourly
                ORDER BY resource_uid, period_start_utc
                """
            )
            rows = cur.fetchall()
            cols = [d.name for d in cur.description]
            print(f"\nLeyendo {len(rows)} filas de Supabase {SUPABASE_FEAT_SCHEMA}.metric_hourly")

        # 5. Insertar en cco_data feat.feature_snapshot
        inserted = 0
        with cco.cursor() as cur:
            for row in rows:
                d = dict(zip(cols, row))
                resource_uid = str(d["resource_uid"])
                resource_id = resource_map.get(resource_uid)
                if resource_id is None:
                    continue

                period_start = _to_utc(d["period_start_utc"])
                period_end = _to_utc(d["period_end_utc"])

                cur.execute(
                    f"""
                    INSERT INTO {CCO_FEAT_SCHEMA}.feature_snapshot (
                        resource_id, period_type, period_start_utc, period_end_utc,
                        sku_id, compute_model, capacity_vcores,
                        cpu_avg, cpu_p50, cpu_p95, cpu_p99, cpu_max, cpu_slope,
                        data_io_avg, log_write_avg,
                        sessions_avg, workers_avg,
                        storage_used_pct, storage_used_bytes, storage_growth_bytes,
                        is_peak_hour, day_of_week, hour_of_day,
                        workload_intensity, workload_run_id,
                        deadlock_count, latency_p95_ms,
                        computed_at
                    ) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)
                    ON CONFLICT (resource_id, period_type, period_start_utc) DO NOTHING
                    """,
                    (
                        resource_id,
                        "1h",
                        period_start,
                        period_end,
                        sku_id,
                        "serverless",
                        1.0,
                        float(d["cpu_avg"]) if d["cpu_avg"] is not None else None,
                        float(d["cpu_p50"]) if d["cpu_p50"] is not None else None,
                        float(d["cpu_p95"]) if d["cpu_p95"] is not None else None,
                        float(d["cpu_p99"]) if d["cpu_p99"] is not None else None,
                        float(d["cpu_max"]) if d["cpu_max"] is not None else None,
                        None,  # cpu_slope
                        float(d["data_io_avg"]) if d["data_io_avg"] is not None else None,
                        float(d["log_write_avg"]) if d["log_write_avg"] is not None else None,
                        float(d["sessions_avg"]) if d["sessions_avg"] is not None else None,
                        float(d["workers_avg"]) if d["workers_avg"] is not None else None,
                        float(d["storage_used_pct"]) if d["storage_used_pct"] is not None else None,
                        int(d["storage_used_bytes"]) if d["storage_used_bytes"] is not None else None,
                        int(d["storage_growth_bytes"]) if d["storage_growth_bytes"] is not None else None,
                        None,  # is_peak_hour (Supabase no tiene)
                        period_start.weekday(),  # day_of_week
                        period_start.hour,  # hour_of_day
                        float(d["workload_intensity"]) if d["workload_intensity"] is not None else None,
                        None,  # workload_run_id
                        int(d["deadlock_count"]) if d["deadlock_count"] is not None else None,
                        float(d["latency_p95_ms"]) if d["latency_p95_ms"] is not None else None,
                        d["computed_at_utc"] if d["computed_at_utc"] else None,
                    ),
                )
                inserted += 1
            cco.commit()
            print(f"\nInsertadas {inserted} filas en {CCO_FEAT_SCHEMA}.feature_snapshot")

        # Verificación
        with cco.cursor() as cur:
            cur.execute(
                f"""
                SELECT resource_id, COUNT(*), MIN(period_start_utc), MAX(period_start_utc)
                FROM {CCO_FEAT_SCHEMA}.feature_snapshot
                GROUP BY resource_id
                ORDER BY resource_id
                """
            )
            print(f"\n=== Estado final de {CCO_FEAT_SCHEMA}.feature_snapshot ===")
            for r in cur.fetchall():
                print(f"  resource_id={r[0]}: {r[1]:,} filas, {r[2]} a {r[3]}")

    finally:
        supa.close()
        cco.close()


if __name__ == "__main__":
    migrate()
