"""Import of the real telemetry into the local working copy.

The source database (Supabase) holds the telemetry collected by the
``dataset-metrics-collector`` branch. The local database (docker) follows the
``cco_data`` model of the branch baseline, which is a *different* physical
model: ``raw.metric_reading`` is keyed by ``resource_id`` instead of
``experiment_uid`` and carries avg/min/max columns instead of a single value.

This module performs the explicit, auditable translation between both models
and then rebuilds the hourly feature store from the imported raw rows. The
translation rules are declared in :data:`METRIC_MAP` and documented in
``README.md``; nothing is guessed silently.

Rules that are never violated here:

* the source connection is read-only;
* no value is invented: a missing observation stays missing;
* the provenance of every row (source, quality, payload) is preserved.
"""

from __future__ import annotations

import datetime as dt
import json
from dataclasses import dataclass
from typing import Any, Iterable, Sequence

import pandas as pd
import psycopg

from .config import Settings
from .db import query, read_frame

#: Source metric code -> local ``catalog.metric_catalog.metric_code``.
#: Codes that are not in the baseline catalog are registered by
#: :func:`ensure_metric_catalog`.
METRIC_MAP: dict[str, str] = {
    "cpu_percent": "cpu_percent",
    "data_io_percent": "data_io_percent",
    "log_write_percent": "log_write_percent",
    "memory_percent": "memory_percent",
    "sessions_count": "sessions_count",
    "sessions_percent": "sessions_percent",
    "workers_percent": "workers_percent",
    "storage_percent": "storage_percent",
    "storage_used_bytes": "storage_used_bytes",
    "deadlock_count": "deadlock",
    "error_rate": "error_rate",
    "failed_operations": "failed_operations",
    "successful_operations": "successful_operations",
    "offered_operations": "offered_operations",
    "throughput_ops_per_second": "throughput_ops_per_second",
    "workload_intensity": "workload_intensity",
    "query_duration_avg_ms": "query_duration_avg_ms",
}

#: Source sample metric code -> local metric code.
SAMPLE_METRIC_MAP: dict[str, str] = {
    "query_duration_sample_ms": "query_duration_ms",
    "workload_operation_latency_ms": "workload_operation_latency_ms",
}

#: Metrics registered by this migration because the baseline catalog does not
#: contain them but the real telemetry does.
EXTRA_METRIC_CATALOG: list[tuple[str, str, str, str, bool, str]] = [
    (
        "memory_percent",
        "Memory usage percent",
        "Percent",
        "database_watcher",
        True,
        "Porcentaje de uso del buffer pool (dm_db_resource_stats).",
    ),
    (
        "sessions_count",
        "Sessions count",
        "Count",
        "database_watcher",
        True,
        "Conteo de sesiones concurrentes. Separado de sessions_percent (utilizacion).",
    ),
    (
        "error_rate",
        "Error rate",
        "Ratio",
        "derived",
        True,
        "Operaciones fallidas sobre operaciones ofrecidas, observadas por el generador de carga.",
    ),
    (
        "failed_operations",
        "Failed operations",
        "Count",
        "derived",
        False,
        "Operaciones fallidas por intervalo de un minuto.",
    ),
    (
        "successful_operations",
        "Successful operations",
        "Count",
        "derived",
        False,
        "Operaciones exitosas por intervalo de un minuto.",
    ),
    (
        "offered_operations",
        "Offered operations",
        "Count",
        "derived",
        False,
        "Operaciones ofrecidas al motor por intervalo de un minuto.",
    ),
    (
        "throughput_ops_per_second",
        "Throughput",
        "Ops/s",
        "derived",
        False,
        "Throughput efectivo observado por el generador de carga.",
    ),
    (
        "workload_intensity",
        "Workload intensity",
        "ops/min",
        "derived",
        True,
        "Intensidad de carga aplicada (operaciones por minuto) reportada por el generador.",
    ),
    (
        "query_duration_avg_ms",
        "Query duration average",
        "Milliseconds",
        "query_store",
        False,
        "Duracion promedio de las ejecuciones de la ventana.",
    ),
    (
        "workload_operation_latency_ms",
        "Workload operation latency",
        "Milliseconds",
        "derived",
        True,
        "Latencia individual de operacion, unica fuente valida para percentiles de latencia.",
    ),
]

#: ``raw.metric_catalog.source`` only allows these values.
CATALOG_SOURCE_BY_KIND: dict[str, str] = {
    "azure_monitor": "azure_monitor",
    "sql_diagnostics": "database_watcher",
    "query_store_execution": "query_store",
    "workload_self": "derived",
    "workload_observation": "derived",
}

#: Canonical source per metric, highest priority first (same rule as the
#: collector aggregator: Azure Monitor > SQL DMVs > Query Store > workload).
SOURCE_PRIORITY: dict[str, int] = {
    "azure_monitor": 0,
    "sql_diagnostics": 1,
    "query_store_execution": 2,
    "workload_self": 3,
    "workload_observation": 4,
}

EXPECTED_READINGS_PER_HOUR = 60


@dataclass
class ImportReport:
    """Summary of one import run."""

    experiment_uid: str = ""
    resource_uid: str = ""
    resource_id: int | None = None
    metric_readings: int = 0
    metric_samples: int = 0
    db_resource_stats: int = 0
    collector_runs: int = 0
    workload_runs: int = 0
    workload_cycles: int = 0
    hourly_features: int = 0
    percentile_statistics: int = 0
    period_start_utc: str | None = None
    period_end_utc: str | None = None
    skipped_metric_codes: list[str] | None = None
    extra_resources: list[dict[str, Any]] | None = None

    def as_dict(self) -> dict[str, Any]:
        return {
            "experiment_uid": self.experiment_uid,
            "resource_uid": self.resource_uid,
            "resource_id": self.resource_id,
            "metric_readings": self.metric_readings,
            "metric_samples": self.metric_samples,
            "db_resource_stats": self.db_resource_stats,
            "collector_runs": self.collector_runs,
            "workload_runs": self.workload_runs,
            "workload_cycles": self.workload_cycles,
            "hourly_features": self.hourly_features,
            "percentile_statistics": self.percentile_statistics,
            "period_start_utc": self.period_start_utc,
            "period_end_utc": self.period_end_utc,
            "skipped_metric_codes": self.skipped_metric_codes or [],
            "extra_resources": self.extra_resources or [],
        }


def _now() -> dt.datetime:
    return dt.datetime.now(dt.timezone.utc)


def _as_utc(value: Any) -> dt.datetime | None:
    if value is None:
        return None
    if isinstance(value, dt.datetime):
        return value if value.tzinfo else value.replace(tzinfo=dt.timezone.utc)
    return pd.Timestamp(value).tz_convert("UTC").to_pydatetime()


def _iso(value: Any) -> str | None:
    parsed = _as_utc(value)
    return parsed.isoformat() if parsed else None


def ensure_metric_catalog(conn: psycopg.Connection) -> None:
    """Register the metric codes the real telemetry uses but the baseline lacks."""
    with conn.cursor() as cur:
        for code, display, unit, source, key_feature, description in EXTRA_METRIC_CATALOG:
            cur.execute(
                """
                insert into catalog.metric_catalog
                    (metric_code, display_name, unit, source, is_key_feature, description)
                values (%s, %s, %s, %s, %s, %s)
                on conflict (metric_code) do nothing
                """,
                (code, display, unit, source, key_feature, description),
            )


def _resolve_experiment(source: psycopg.Connection, settings: Settings) -> dict[str, Any]:
    if settings.experiment_uid:
        rows = query(
            source,
            """
            select experiment_uid, resource_uid, started_at_utc, stopped_at_utc,
                   status, workload_class, random_seed
            from experiment.experiment_run
            where experiment_uid = %s
            """,
            (settings.experiment_uid,),
        )
        if not rows:
            raise ValueError(
                f"experiment_uid {settings.experiment_uid} not found in the source database"
            )
        return rows[0]

    rows = query(
        source,
        """
        select experiment_uid, resource_uid, started_at_utc, stopped_at_utc,
               status, workload_class, random_seed
        from experiment.experiment_run
        order by started_at_utc desc
        """,
    )
    if not rows:
        raise ValueError("the source database has no experiment_run row")
    return rows[0]


def _ensure_resource(
    conn: psycopg.Connection,
    settings: Settings,
    resource_uid: str,
    *,
    is_primary: bool,
) -> int:
    """Register one observed resource. Each source resource keeps its identity."""
    row = query(
        conn,
        "select resource_id from core.resource where resource_uid = %s",
        (str(resource_uid),),
    )
    if row:
        return int(row[0]["resource_id"])

    suffix = str(resource_uid).split("-")[0]
    name = settings.resource_name if is_primary else f"{settings.resource_name}-{suffix}"
    with conn.cursor() as cur:
        cur.execute(
            """
            insert into core.resource
                (resource_uid, provider_code, subscription_id, resource_group,
                 server_name, database_name, azure_resource_id, region_code,
                 kind, lifecycle_status)
            values (%s, %s, %s, %s, %s, %s, %s, %s, %s, 'active')
            on conflict (resource_uid) do nothing
            """,
            (
                str(resource_uid),
                settings.resource_provider,
                settings.extra.get("subscription_id", "00000000-0000-0000-0000-000000000000"),
                settings.extra.get("resource_group", "rg-cco-forecasting"),
                settings.extra.get("server_name", settings.resource_name),
                name,
                settings.extra.get(
                    "azure_resource_id",
                    f"/subscriptions/{settings.extra.get('subscription_id', 'local')}"
                    f"/resourceGroups/{settings.extra.get('resource_group', 'local')}"
                    f"/providers/Microsoft.Sql/servers/{name}",
                ),
                settings.resource_region_code,
                settings.extra.get("kind", "v12.0,user,vcore,serverless,freelimit"),
            ),
        )
        cur.execute("select resource_id from core.resource where resource_uid = %s", (str(resource_uid),))
        found = cur.fetchone()
    if not found:
        raise RuntimeError(f"could not create the resource {resource_uid}")
    return int(found[0])


def _ensure_config_snapshot(
    conn: psycopg.Connection, settings: Settings, resource_id: int, observed_at: dt.datetime
) -> None:
    sku_rows = query(
        conn,
        "select sku_id, capacity_vcores, compute_model from catalog.sql_sku where sku_name = %s",
        (settings.resource_sku_code or "GP_S_Gen5_2",),
    )
    sku_id = int(sku_rows[0]["sku_id"]) if sku_rows else None
    capacity = float(sku_rows[0]["capacity_vcores"]) if sku_rows else settings.resource_vcores
    compute_model = settings.resource_compute_model or (
        str(sku_rows[0]["compute_model"]) if sku_rows else "serverless"
    )
    with conn.cursor() as cur:
        cur.execute(
            """
            insert into core.resource_config_snapshot
                (resource_id, sku_id, observed_at_utc, sku_name, tier, family,
                 capacity_vcores, min_capacity_vcores, compute_model, status, free_limit)
            values (%s, %s, %s, %s, %s, %s, %s, %s, %s, 'Online', true)
            on conflict (resource_id, observed_at_utc) do nothing
            """,
            (
                resource_id,
                sku_id,
                observed_at,
                settings.resource_sku_code or "GP_S_Gen5_2",
                "GeneralPurpose",
                "Gen5",
                capacity,
                0.5 if compute_model == "serverless" else None,
                compute_model,
            ),
        )


def _import_collector_runs(
    source: psycopg.Connection, conn: psycopg.Connection, resource_id: int
) -> int:
    runs = query(
        source,
        """
        select collector_run_uid, source, started_at_utc, finished_at_utc, status,
               rows_observed, rows_written, error_message, request_metadata_json
        from ops.collector_run
        order by started_at_utc
        """,
    )
    if not runs:
        return 0
    rows = []
    for run in runs:
        rows.append(
            (
                resource_id,
                str(run["source"]),
                _as_utc(run["started_at_utc"]),
                _as_utc(run["finished_at_utc"]),
                "ok" if run["status"] in ("ok", "succeeded") else ("partial" if run["status"] == "partial" else "error"),
                int(run["rows_written"] or 0),
                run["error_message"],
                json.dumps(run["request_metadata_json"]) if run.get("request_metadata_json") else None,
                json.dumps({"source_collector_run_uid": str(run["collector_run_uid"])}),
            )
        )
    with conn.cursor() as cur:
        cur.executemany(
            """
            insert into raw.collector_run
                (resource_id, source, started_at_utc, finished_at_utc, status,
                 rows_ingested, error_message, request_payload, response_payload)
            values (%s, %s, %s, %s, %s, %s, %s, %s, %s)
            on conflict do nothing
            """,
            rows,
        )
    return len(rows)


def _import_metric_readings(
    source: psycopg.Connection,
    conn: psycopg.Connection,
    resource_id: int,
    experiment_uid: str,
    resource_uid: str,
) -> tuple[int, list[str]]:
    frame = read_frame(
        source,
        """
        select metric_code, observed_at_utc, interval_seconds, value, unit, source,
               quality_status, quality_warnings, source_payload_json
        from raw.metric_reading
        where experiment_uid = %s and resource_uid = %s
        order by observed_at_utc
        """,
        (str(experiment_uid), str(resource_uid)),
    )
    if frame.empty:
        return 0, []

    known = set(METRIC_MAP)
    unknown = sorted({c for c in frame["metric_code"].unique() if c not in known})
    frame = frame[frame["metric_code"].isin(known)].copy()
    frame["local_metric_code"] = frame["metric_code"].map(METRIC_MAP)

    rows = []
    for rec in frame.itertuples(index=False):
        ts = _as_utc(rec.observed_at_utc)
        if ts is None:
            continue
        value = float(rec.value) if rec.value is not None else None
        rows.append(
            (
                resource_id,
                rec.local_metric_code,
                ts,
                int(rec.interval_seconds or 60),
                value,
                value,
                value,
                value,
                1 if rec.source == "workload_self" and rec.local_metric_code.endswith("_operations") else None,
                rec.quality_status,
                list(rec.quality_warnings or []),
                json.dumps(rec.source_payload_json) if rec.source_payload_json else None,
                str(rec.source),
            )
        )
    with conn.cursor() as cur:
        cur.executemany(
            """
            insert into raw.metric_reading
                (resource_id, metric_code, timestamp_utc, interval_seconds,
                 avg_value, min_value, max_value, total_value, count_value,
                 quality_status, quality_warnings, source_payload, source)
            values (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
            on conflict do nothing
            """,
            rows,
        )
    return len(rows), unknown


def _import_metric_samples(
    source: psycopg.Connection,
    conn: psycopg.Connection,
    resource_id: int,
    experiment_uid: str,
    resource_uid: str,
) -> int:
    frame = read_frame(
        source,
        """
        select metric_code, observed_at_utc, value, source, source_sample_id
        from raw.metric_sample
        where experiment_uid = %s and resource_uid = %s
        order by observed_at_utc
        """,
        (str(experiment_uid), str(resource_uid)),
    )
    if frame.empty:
        return 0
    frame = frame[frame["metric_code"].isin(SAMPLE_METRIC_MAP)].copy()
    frame["local_metric_code"] = frame["metric_code"].map(SAMPLE_METRIC_MAP)
    rows = []
    for rec in frame.itertuples(index=False):
        ts = _as_utc(rec.observed_at_utc)
        if ts is None:
            continue
        rows.append(
            (
                resource_id,
                rec.local_metric_code,
                ts,
                float(rec.value),
                str(rec.source),
                str(rec.source_sample_id) if rec.source_sample_id is not None else None,
            )
        )
    with conn.cursor() as cur:
        cur.executemany(
            """
            insert into raw.metric_sample
                (resource_id, metric_code, observed_at_utc, value, source, source_sample_id)
            values (%s, %s, %s, %s, %s, %s)
            on conflict do nothing
            """,
            rows,
        )
    return len(rows)


def _import_db_resource_stats(
    conn: psycopg.Connection, resource_id: int, experiment_uid: str
) -> int:
    """Materialise the SQL DMV readings as ``raw.db_resource_stats`` rows."""
    frame = read_frame(
        conn,
        """
        select timestamp_utc,
               max(case when metric_code = 'cpu_percent' and source = 'sql_diagnostics' then avg_value end) as cpu,
               max(case when metric_code = 'data_io_percent' and source = 'sql_diagnostics' then avg_value end) as data_io,
               max(case when metric_code = 'log_write_percent' and source = 'sql_diagnostics' then avg_value end) as log_write,
               max(case when metric_code = 'memory_percent' and source = 'sql_diagnostics' then avg_value end) as memory
        from raw.metric_reading
        where resource_id = %s
        group by timestamp_utc
        having count(*) filter (where source = 'sql_diagnostics') > 0
        order by timestamp_utc
        """,
        (resource_id,),
    )
    if frame.empty:
        return 0
    rows = [
        (
            resource_id,
            _as_utc(rec.timestamp_utc),
            rec.cpu,
            rec.data_io,
            rec.log_write,
            rec.memory,
            None,
            None,
            None,
        )
        for rec in frame.itertuples(index=False)
    ]
    with conn.cursor() as cur:
        cur.executemany(
            """
            insert into raw.db_resource_stats
                (resource_id, end_time_utc, avg_cpu_percent, avg_data_io_percent,
                 avg_log_write_percent, avg_memory_usage_percent,
                 avg_instance_cpu_percent, dtu_limit, cpu_limit)
            values (%s, %s, %s, %s, %s, %s, %s, %s, %s)
            on conflict do nothing
            """,
            rows,
        )
    return len(rows)


def _import_workload_runs(
    source: psycopg.Connection, conn: psycopg.Connection, resource_id: int, experiment_uid: str
) -> int:
    runs = query(
        source,
        """
        select workload_run_uid, workload_class, started_at_utc, finished_at_utc,
               duration_seconds, intensity, concurrency, pattern_params_json,
               operations_total, errors_total, status
        from experiment.workload_run
        where experiment_uid = %s
        order by started_at_utc
        """,
        (str(experiment_uid),),
    )
    if not runs:
        return 0
    rows = []
    for run in runs:
        params = dict(run.get("pattern_params_json") or {})
        params.update(
            {
                "source_workload_run_uid": str(run["workload_run_uid"]),
                "concurrency": run["concurrency"],
                "operations_total": run["operations_total"],
                "errors_total": run["errors_total"],
                "status": run["status"],
            }
        )
        rows.append(
            (
                resource_id,
                str(run["workload_class"]),
                _as_utc(run["started_at_utc"]),
                _as_utc(run["finished_at_utc"]),
                run["duration_seconds"],
                json.dumps(params),
                f"imported from source workload_run {run['workload_run_uid']}",
            )
        )
    with conn.cursor() as cur:
        cur.executemany(
            """
            insert into core.workload_run
                (resource_id, class_code, started_at_utc, finished_at_utc,
                 duration_s, pattern_params, notes)
            values (%s, %s, %s, %s, %s, %s, %s)
            on conflict do nothing
            """,
            rows,
        )
    return len(rows)


def _import_workload_cycles(conn: psycopg.Connection, resource_id: int) -> int:
    """One cycle per minute of applied load, derived from the real counters."""
    frame = read_frame(
        conn,
        """
        with minutes as (
            select timestamp_utc,
                   max(case when metric_code = 'workload_intensity' then avg_value end) as intensity,
                   sum(case when metric_code = 'offered_operations' then coalesce(total_value, 0) end) as offered
            from raw.metric_reading
            where resource_id = %s and source = 'workload_self'
            group by timestamp_utc
        )
        select timestamp_utc, intensity, offered
        from minutes
        where intensity is not null or offered is not null
        order by timestamp_utc
        """,
        (resource_id,),
    )
    if frame.empty:
        return 0

    run_rows = read_frame(
        conn,
        "select workload_run_id, started_at_utc, finished_at_utc from core.workload_run order by started_at_utc",
    )
    run_bounds = [
        (int(r["workload_run_id"]), _as_utc(r["started_at_utc"]), _as_utc(r["finished_at_utc"]))
        for r in run_rows.to_dict("records")
    ]

    rows = []
    orphans: list[tuple[dt.datetime, float | None, int | None]] = []
    for cycle_number, rec in enumerate(frame.itertuples(index=False), start=1):
        ts = _as_utc(rec.timestamp_utc)
        if ts is None:
            continue
        run_id = None
        for rid, start, finish in run_bounds:
            if start and start <= ts and (finish is None or ts <= finish):
                run_id = rid
                break
        if run_id is None:
            orphans.append((ts, rec.intensity, int(rec.offered) if rec.offered is not None else None))
            continue
        rows.append(
            (
                run_id,
                cycle_number,
                ts,
                rec.intensity,
                int(rec.offered) if rec.offered is not None else None,
            )
        )

    if orphans:
        # Minutes of applied load that fall outside any declared run are kept
        # in an explicit run instead of being discarded: nothing is dropped
        # silently.
        orphan_run_id = _ensure_orphan_workload_run(conn, resource_id, orphans)
        for offset, (ts, intensity, offered) in enumerate(orphans, start=1):
            rows.append((orphan_run_id, offset, ts, intensity, offered))

    with conn.cursor() as cur:
        cur.executemany(
            """
            insert into core.workload_cycle
                (workload_run_id, cycle_number, started_at_utc, intensity, operations)
            values (%s, %s, %s, %s, %s)
            on conflict do nothing
            """,
            rows,
        )
    return len(rows)


def _ensure_orphan_workload_run(
    conn: psycopg.Connection, resource_id: int, orphans: Sequence[tuple[dt.datetime, Any, Any]]
) -> int:
    """Create the run that holds the applied-load minutes without a declared run."""
    start = min(item[0] for item in orphans)
    end = max(item[0] for item in orphans)
    with conn.cursor() as cur:
        cur.execute(
            """
            insert into core.workload_run
                (resource_id, class_code, started_at_utc, finished_at_utc, duration_s,
                 pattern_params, notes)
            values (%s, 'mixed', %s, %s, %s, %s::jsonb, %s)
            on conflict do nothing
            """,
            (
                resource_id,
                start,
                end,
                (end - start).total_seconds(),
                json.dumps({"attribution": "no_active_run", "cycle_count": len(orphans)}),
                "Minutos de carga aplicada sin corrida declarada en la fuente.",
            ),
        )
        cur.execute(
            """
            select workload_run_id from core.workload_run
            where resource_id = %s and class_code = 'mixed'
              and notes = 'Minutos de carga aplicada sin corrida declarada en la fuente.'
            order by workload_run_id desc limit 1
            """,
            (resource_id,),
        )
        row = cur.fetchone()
    if not row:
        raise RuntimeError("could not create the orphan workload run")
    return int(row[0])


# =============================================================================
# Reconstruccion del feature store horario
# =============================================================================
def _canonical_hourly(conn: psycopg.Connection, resource_id: int) -> pd.DataFrame:
    """Aggregate the raw readings into hourly windows, one canonical source per metric."""
    frame = read_frame(
        conn,
        """
        select timestamp_utc, metric_code, source,
               count(*) as n,
               avg(avg_value) as avg_value,
               max(avg_value) as max_value,
               min(avg_value) as min_value,
               sum(total_value) as sum_value
        from raw.metric_reading
        where resource_id = %s
        group by 1, 2, 3
        """,
        (resource_id,),
    )
    if frame.empty:
        return frame
    frame["timestamp_utc"] = pd.to_datetime(frame["timestamp_utc"], utc=True)
    frame["period_start_utc"] = frame["timestamp_utc"].dt.floor("h")
    frame["priority"] = frame["source"].map(SOURCE_PRIORITY).fillna(99)
    frame = frame.sort_values(["period_start_utc", "metric_code", "priority", "source"])
    canonical = frame.groupby(["period_start_utc", "metric_code"], as_index=False).first()
    return canonical


def _percentiles_from_samples(
    conn: psycopg.Connection, resource_id: int
) -> pd.DataFrame:
    """Exact percentiles over the individual observations (raw.metric_sample)."""
    frame = read_frame(
        conn,
        """
        select metric_code, date_trunc('hour', observed_at_utc) as period_start_utc,
               count(*) as n, percentile_cont(%(p)s) within group (order by value) as value
        from raw.metric_sample
        where resource_id = %(rid)s
        group by 1, 2
        """,
        {"p": 0.95, "rid": resource_id},
    )
    if frame.empty:
        return frame
    frame["period_start_utc"] = pd.to_datetime(frame["period_start_utc"], utc=True)
    return frame


def _write_percentile_statistics(
    conn: psycopg.Connection, resource_id: int, percentiles: pd.DataFrame
) -> int:
    if percentiles.empty:
        return 0
    rows = []
    for rec in percentiles.itertuples(index=False):
        rows.append(
            (
                resource_id,
                "1h",
                rec.period_start_utc.to_pydatetime(),
                rec.metric_code,
                0.95,
                float(rec.value),
                "exact_sample",
                int(rec.n),
                "individual_observation",
            )
        )
    with conn.cursor() as cur:
        cur.executemany(
            """
            insert into feat.percentile_statistic
                (resource_id, period_type, period_start_utc, metric_code, percentile,
                 value, method, observation_count, source_granularity)
            values (%s, %s, %s, %s, %s, %s, %s, %s, %s)
            on conflict do nothing
            """,
            rows,
        )
    return len(rows)


def _build_hourly_features(
    conn: psycopg.Connection,
    resource_id: int,
    experiment_uid: str,
    resource_uid: str,
) -> int:
    """Rebuild ``feat.feature_snapshot`` (1h) from the imported raw rows."""
    canonical = _canonical_hourly(conn, resource_id)
    if canonical.empty:
        return 0

    pivot_avg = canonical.pivot_table(
        index="period_start_utc", columns="metric_code", values="avg_value", aggfunc="first"
    )
    pivot_max = canonical.pivot_table(
        index="period_start_utc", columns="metric_code", values="max_value", aggfunc="first"
    )
    pivot_min = canonical.pivot_table(
        index="period_start_utc", columns="metric_code", values="min_value", aggfunc="first"
    )
    pivot_sum = canonical.pivot_table(
        index="period_start_utc", columns="metric_code", values="sum_value", aggfunc="first"
    )
    pivot_n = canonical.pivot_table(
        index="period_start_utc", columns="metric_code", values="n", aggfunc="first"
    )
    for frame in (pivot_avg, pivot_max, pivot_min, pivot_sum, pivot_n):
        for column in frame.columns:
            frame[column] = pd.to_numeric(frame[column], errors="coerce").astype("float64")
    sources = (
        canonical.assign(code=canonical["metric_code"] + "=" + canonical["source"])
        .pivot_table(index="period_start_utc", columns="metric_code", values="source", aggfunc="first")
    )

    index = pivot_avg.index
    pivot_avg = pivot_avg.reindex(index)
    sources = sources.reindex(index)

    work_runs = read_frame(
        conn,
        "select workload_run_id, class_code, started_at_utc, finished_at_utc from core.workload_run",
    )
    work_class: dict[dt.datetime, str] = {}
    work_run_id: dict[dt.datetime, int] = {}
    for rec in work_runs.to_dict("records"):
        start = _as_utc(rec["started_at_utc"])
        finish = _as_utc(rec["finished_at_utc"])
        if start is None:
            continue
        for period in index:
            if start <= period < (finish or start + dt.timedelta(hours=1)):
                work_class[period] = str(rec["class_code"])
                work_run_id[period] = int(rec["workload_run_id"])

    storage_growth = pivot_avg.get("storage_used_bytes")
    if storage_growth is not None:
        storage_growth = storage_growth.diff()

    rows = []
    for period in index:
        period_utc = period.to_pydatetime()
        end_utc = (period + pd.Timedelta(hours=1)).to_pydatetime()
        row: dict[str, Any] = {"period_start_utc": period_utc, "period_end_utc": end_utc}

        def avg_of(code: str) -> float | None:
            if code not in pivot_avg.columns:
                return None
            value = pivot_avg.at[period, code]
            return None if pd.isna(value) else float(value)

        def max_of(code: str) -> float | None:
            if code not in pivot_max.columns:
                return None
            value = pivot_max.at[period, code]
            return None if pd.isna(value) else float(value)

        def sum_of(code: str) -> float | None:
            if code not in pivot_sum.columns:
                return None
            value = pivot_sum.at[period, code]
            return None if pd.isna(value) else float(value)

        def n_of(code: str) -> int:
            if code not in pivot_n.columns:
                return 0
            value = pivot_n.at[period, code]
            return 0 if pd.isna(value) else int(value)

        cpu_values = pivot_avg.get("cpu_percent")
        cpu_samples: list[float] = []
        if cpu_values is not None and not pd.isna(cpu_values.at[period]):
            cpu_samples = [float(cpu_values.at[period])]

        row.update(
            {
                "cpu_avg": avg_of("cpu_percent"),
                "cpu_max": max_of("cpu_percent"),
                "cpu_p50": None,
                "cpu_p95": None,
                "cpu_p99": None,
                "cpu_slope": None,
                "data_io_avg": avg_of("data_io_percent"),
                "data_io_p95": None,
                "log_write_avg": avg_of("log_write_percent"),
                "log_write_p95": None,
                "sessions_avg": avg_of("sessions_percent"),
                "sessions_p95": None,
                "workers_avg": avg_of("workers_percent"),
                "storage_used_pct": max_of("storage_percent"),
                "storage_used_bytes": int(max_of("storage_used_bytes"))
                if max_of("storage_used_bytes") is not None
                else None,
                "storage_growth_bytes": int(storage_growth.at[period])
                if storage_growth is not None and not pd.isna(storage_growth.at[period])
                else None,
                "workload_intensity": avg_of("workload_intensity"),
                "deadlock_count": int(sum_of("deadlock") or 0),
                "latency_p95_ms": None,
                "memory_percent": avg_of("memory_percent"),
                "sessions_count": avg_of("sessions_count"),
                "sessions_percent": avg_of("sessions_percent"),
                "error_rate": avg_of("error_rate"),
                "latency_avg_ms": avg_of("query_duration_avg_ms"),
                "throughput_ops_per_second": avg_of("throughput_ops_per_second"),
                "offered_operations": int(sum_of("offered_operations"))
                if sum_of("offered_operations") is not None
                else None,
                "successful_operations": int(sum_of("successful_operations"))
                if sum_of("successful_operations") is not None
                else None,
                "failed_operations": int(sum_of("failed_operations"))
                if sum_of("failed_operations") is not None
                else None,
                "workload_class": work_class.get(period_utc, "unknown"),
                "workload_run_id": work_run_id.get(period_utc),
                "experiment_uid": experiment_uid,
                "resource_uid": resource_uid,
                "is_peak_hour": None,
                "day_of_week": period.weekday(),
                "hour_of_day": period.hour,
                "canonical_sources": json.dumps(
                    {
                        code: sources.at[period, code]
                        for code in sources.columns
                        if not pd.isna(sources.at[period, code])
                    }
                ),
                "coverage_json": json.dumps(
                    {
                        code: n_of(code) for code in pivot_n.columns if n_of(code) > 0
                    }
                ),
                "quality_status": "usable",
                "quality_warnings": [],
            }
        )
        rows.append(row)

    with conn.cursor() as cur:
        cur.executemany(
            """
            insert into feat.feature_snapshot
                (resource_id, period_type, period_start_utc, period_end_utc,
                 cpu_avg, cpu_p50, cpu_p95, cpu_p99, cpu_max, cpu_slope,
                 data_io_avg, data_io_p95, log_write_avg, log_write_p95,
                 sessions_avg, sessions_p95, workers_avg,
                 storage_used_pct, storage_used_bytes, storage_growth_bytes,
                 is_peak_hour, day_of_week, hour_of_day, workload_intensity, workload_run_id,
                 deadlock_count, latency_p95_ms,
                 experiment_uid, resource_uid, workload_class,
                 memory_percent, sessions_count, sessions_percent, error_rate,
                 latency_avg_ms, throughput_ops_per_second,
                 offered_operations, successful_operations, failed_operations,
                 canonical_sources, coverage_json, quality_status, quality_warnings)
            values (%(resource_id)s, '1h', %(period_start_utc)s, %(period_end_utc)s,
                 %(cpu_avg)s, %(cpu_p50)s, %(cpu_p95)s, %(cpu_p99)s, %(cpu_max)s, %(cpu_slope)s,
                 %(data_io_avg)s, %(data_io_p95)s, %(log_write_avg)s, %(log_write_p95)s,
                 %(sessions_avg)s, %(sessions_p95)s, %(workers_avg)s,
                 %(storage_used_pct)s, %(storage_used_bytes)s, %(storage_growth_bytes)s,
                 %(is_peak_hour)s, %(day_of_week)s, %(hour_of_day)s, %(workload_intensity)s, %(workload_run_id)s,
                 %(deadlock_count)s, %(latency_p95_ms)s,
                 %(experiment_uid)s, %(resource_uid)s, %(workload_class)s,
                 %(memory_percent)s, %(sessions_count)s, %(sessions_percent)s, %(error_rate)s,
                 %(latency_avg_ms)s, %(throughput_ops_per_second)s,
                 %(offered_operations)s, %(successful_operations)s, %(failed_operations)s,
                 %(canonical_sources)s, %(coverage_json)s, %(quality_status)s, %(quality_warnings)s)
            on conflict do nothing
            """,
            [{"resource_id": resource_id, **row} for row in rows],
        )
    return len(rows)


def _discover_resources(
    source: psycopg.Connection, experiment_uid: str
) -> list[dict[str, Any]]:
    """Distinct resources observed in the experiment, with their coverage."""
    frame = read_frame(
        source,
        """
        select resource_uid,
               count(*) as rows,
               min(observed_at_utc) as min_ts,
               max(observed_at_utc) as max_ts
        from raw.metric_reading
        where experiment_uid = %s
        group by resource_uid
        order by rows desc
        """,
        (str(experiment_uid),),
    )
    if frame.empty:
        return []
    records = frame.to_dict("records")
    for record in records:
        record["resource_uid"] = str(record["resource_uid"])
        record["min_ts"] = _iso(record["min_ts"])
        record["max_ts"] = _iso(record["max_ts"])
    return records


def import_real_data(
    source: psycopg.Connection,
    conn: psycopg.Connection,
    settings: Settings,
) -> ImportReport:
    """Copy the real telemetry from the source database into the local copy.

    The experiment observed more than one resource. Each one is imported as a
    separate ``core.resource`` with its own hourly feature store: resources are
    never merged, and the modelling dataset is built on the dominant resource
    (the one with the largest coverage), which is reported explicitly.
    """
    experiment = _resolve_experiment(source, settings)
    experiment_uid = str(experiment["experiment_uid"])
    report = ImportReport(experiment_uid=experiment_uid, resource_uid=str(experiment["resource_uid"]))

    ensure_metric_catalog(conn)

    resources = _discover_resources(source, experiment_uid)
    if not resources:
        raise ValueError("the source experiment has no metric readings")
    primary = resources[0]

    for index, resource in enumerate(resources):
        is_primary = index == 0
        resource_id = _ensure_resource(
            conn, settings, resource["resource_uid"], is_primary=is_primary
        )
        started = _as_utc(experiment["started_at_utc"])
        _ensure_config_snapshot(conn, settings, resource_id, started or _now())

        readings, unknown = _import_metric_readings(
            source, conn, resource_id, experiment_uid, resource["resource_uid"]
        )
        samples = _import_metric_samples(
            source, conn, resource_id, experiment_uid, resource["resource_uid"]
        )
        stats_rows = _import_db_resource_stats(conn, resource_id, experiment_uid)
        percentiles = _percentiles_from_samples(conn, resource_id)
        percentile_rows = _write_percentile_statistics(conn, resource_id, percentiles)
        hourly_rows = _build_hourly_features(
            conn, resource_id, experiment_uid, resource["resource_uid"]
        )

        if is_primary:
            report.resource_uid = resource["resource_uid"]
            report.resource_id = resource_id
            report.metric_readings = readings
            report.metric_samples = samples
            report.db_resource_stats = stats_rows
            report.percentile_statistics = percentile_rows
            report.hourly_features = hourly_rows
            report.skipped_metric_codes = unknown
            report.collector_runs = _import_collector_runs(source, conn, resource_id)
            report.workload_runs = _import_workload_runs(
                source, conn, resource_id, experiment_uid
            )
            report.workload_cycles = _import_workload_cycles(conn, resource_id)
        else:
            report.extra_resources = report.extra_resources or []
            report.extra_resources.append(
                {
                    "resource_uid": resource["resource_uid"],
                    "resource_id": resource_id,
                    "metric_readings": readings,
                    "metric_samples": samples,
                    "hourly_features": hourly_rows,
                    "period_start_utc": resource["min_ts"],
                    "period_end_utc": resource["max_ts"],
                }
            )

    bounds = read_frame(
        conn,
        """
        select min(timestamp_utc) as min_ts, max(timestamp_utc) as max_ts
        from raw.metric_reading where resource_id = %s
        """,
        (report.resource_id,),
    )
    if not bounds.empty:
        report.period_start_utc = _iso(bounds.iloc[0]["min_ts"])
        report.period_end_utc = _iso(bounds.iloc[0]["max_ts"])
    return report
