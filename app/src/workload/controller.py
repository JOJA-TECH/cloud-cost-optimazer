"""Workload state machine.

The controller is the only place that transitions the experiment state. Every
transition is validated twice:

1) locally against ALLOWED (fail-fast, no DB round trips);
2) on Azure SQL by the INSTEAD OF UPDATE trigger (authoritative enforcement).

Supabase experiment.experiment_run is the authoritative analytical record;
Azure SQL loadtest.workload_control is the operational mirror that gates the
load generator. Both are updated together; a failure to persist on either side
leaves the experiment in STOPPED_WITH_PENDING_BUFFER territory (handled by the
shutdown coordinator).
"""

from __future__ import annotations

import datetime as dt
import json
import logging
from typing import Dict, Optional, Tuple

from .. import db_azure, db_supabase
from ..config import Config

logger = logging.getLogger(__name__)

STATUSES = frozenset(
    {"CREATED", "RUNNING", "PAUSING", "PAUSED", "STOPPING", "STOPPED", "FAILED"}
)

_TRANSITIONS: Dict[Tuple[str, str], str] = {
    ("CREATED", "RUNNING"): "start",
    ("RUNNING", "PAUSING"): "pause_request",
    ("PAUSING", "PAUSED"): "pause_ack",
    ("PAUSING", "RUNNING"): "resume",
    ("PAUSED", "RUNNING"): "resume",
    ("PAUSED", "STOPPING"): "stop_request",
    ("RUNNING", "STOPPING"): "stop_request",
    ("STOPPING", "STOPPED"): "stop_confirm",
    ("RUNNING", "FAILED"): "fail",
    ("PAUSING", "FAILED"): "fail",
    ("STOPPING", "FAILED"): "fail",
}


class TransitionError(RuntimeError):
    pass


class WorkloadController:
    def __init__(self, cfg: Config) -> None:
        self.cfg = cfg

    # -- reads --------------------------------------------------------------
    def azure_status(self) -> Optional[str]:
        conn = db_azure.connect(self.cfg)
        try:
            row = db_azure.scalar(
                conn,
                "SELECT status FROM loadtest.workload_control WHERE experiment_uid = ?",
                (self.cfg.experiment_uid,),
            )
            return row
        except Exception as exc:  # pragma: no cover - depends on ODBC
            logger.debug("azure_status failed: %s", exc)
            return None
        finally:
            conn.close()

    def supabase_status(self) -> Optional[str]:
        try:
            with db_supabase.transaction(self.cfg) as conn:
                rows = db_supabase.query(
                    conn,
                    "select status from experiment.experiment_run "
                    "where experiment_uid = %s",
                    (self.cfg.experiment_uid,),
                )
            return rows[0]["status"] if rows else None
        except Exception as exc:  # noqa: BLE001
            logger.debug("supabase_status failed: %s", exc)
            return None

    # -- transitions --------------------------------------------------------
    def transition(self, target: str, *, requested_by: str, reason: str = "") -> str:
        if target not in STATUSES:
            raise TransitionError(f"unknown status {target}")

        current = self.azure_status() or "CREATED"
        key = (current, target)
        if key not in _TRANSITIONS:
            raise TransitionError(
                f"forbidden transition {current} -> {target}"
            )

        # Supabase first (authoritative), then Azure SQL (enforced).
        try:
            with db_supabase.transaction(self.cfg) as conn:
                db_supabase.query(
                    conn,
                    "update experiment.experiment_run "
                    "set status = %s, stop_reason = %s, updated_at_utc = now() "
                    "where experiment_uid = %s",
                    (target, reason or None, self.cfg.experiment_uid),
                )
        except db_supabase.SupabaseConnectionError as exc:
            raise TransitionError(f"supabase update failed: {exc}") from exc

        conn = db_azure.connect(self.cfg)
        try:
            db_azure.execute(
                conn,
                "UPDATE loadtest.workload_control SET status = ?, requested_by = ? "
                "WHERE experiment_uid = ?",
                (target, requested_by, self.cfg.experiment_uid),
            )
        except Exception as exc:  # noqa: BLE001 - trigger rejection surfaces here
            logger.error("azure workload_control rejected transition: %s", exc)
            raise TransitionError(
                f"azure workload_control rejected {current} -> {target}: {exc}"
            ) from exc
        finally:
            conn.close()

        logger.info("transition %s -> %s (%s)", current, target, requested_by)
        return target

    # -- helpers -------------------------------------------------------------
    # Espacio finito de SKUs Azure SQL conocido (mirror de catalog.sql_sku).
    # Sirve para poblar arm_sku_name / tier / compute_model / capacity_vcores
    # / min_capacity_vcores / family sin consultar Azure de nuevo.
    SKU_CATALOG: dict[str, dict] = {
        "GP_S_Gen5_1": {
            "arm_sku_name": "SQLDB_GP_Compute_Gen5_1",
            "tier": "GeneralPurpose",
            "compute_model": "serverless",
            "capacity_vcores": 1.0,
            "min_capacity_vcores": 0.5,
            "family": "Gen5",
        },
        "GP_S_Gen5_2": {
            "arm_sku_name": "SQLDB_GP_Compute_Gen5_2",
            "tier": "GeneralPurpose",
            "compute_model": "serverless",
            "capacity_vcores": 2.0,
            "min_capacity_vcores": 0.5,
            "family": "Gen5",
        },
        "GP_Gen5_2": {
            "arm_sku_name": "SQLDB_GP_Compute_Gen5_2",
            "tier": "GeneralPurpose",
            "compute_model": "provisioned",
            "capacity_vcores": 2.0,
            "min_capacity_vcores": None,
            "family": "Gen5",
        },
        "GP_Gen5_4": {
            "arm_sku_name": "SQLDB_GP_Compute_Gen5_4",
            "tier": "GeneralPurpose",
            "compute_model": "provisioned",
            "capacity_vcores": 4.0,
            "min_capacity_vcores": None,
            "family": "Gen5",
        },
        "GP_Gen5_6": {
            "arm_sku_name": "SQLDB_GP_Compute_Gen5_6",
            "tier": "GeneralPurpose",
            "compute_model": "provisioned",
            "capacity_vcores": 6.0,
            "min_capacity_vcores": None,
            "family": "Gen5",
        },
        "GP_Gen5_8": {
            "arm_sku_name": "SQLDB_GP_Compute_Gen5_8",
            "tier": "GeneralPurpose",
            "compute_model": "provisioned",
            "capacity_vcores": 8.0,
            "min_capacity_vcores": None,
            "family": "Gen5",
        },
    }

    def _sku_fields(self) -> dict:
        """Campos de SKU y recurso desde el target configurado o fallback.

        Se leen de ``AZURE_SQL_TARGETS_JSON[0]`` (primary target) y, cuando falta,
        de los campos sueltos de Config (``sql_sku_name``, ``azure_*``).
        """
        target = self.cfg.primary_target() or {}
        server = target.get("server") or (
            self.cfg.azure_sql_server.split(".")[0] if self.cfg.azure_sql_server else None
        )
        sku_name = target.get("sku_name") or self.cfg.sql_sku_name
        catalog = self.SKU_CATALOG.get(sku_name, {})
        return {
            "subscription_id": target.get("subscription_id") or self.cfg.azure_subscription_id,
            "resource_group": target.get("resource_group") or self.cfg.azure_resource_group,
            "server_name": server,
            "database_name": target.get("database") or self.cfg.azure_sql_database,
            "region_code": target.get("location") or self.cfg.azure_location,
            "sku_name": sku_name,
            "arm_sku_name": catalog.get("arm_sku_name"),
            "tier": catalog.get("tier"),
            "compute_model": catalog.get("compute_model"),
            "capacity_vcores": catalog.get("capacity_vcores"),
            "min_capacity_vcores": catalog.get("min_capacity_vcores"),
            "family": catalog.get("family"),
            "source": target.get("source") or "config",
            "auto_pause_delay_minutes": target.get("auto_pause_delay_minutes"),
        }

    # -- convenience ---------------------------------------------------------
    def ensure_experiment(self, workload_class: str, random_seed: int, config_json: dict) -> None:
        """Idempotently create the experiment on both stores if missing."""
        exp_uid = self.cfg.experiment_uid
        resource_uid = self.cfg.primary_resource_uid()
        now = dt.datetime.now(dt.timezone.utc).isoformat(timespec="seconds")

        azure_status = self.azure_status()
        if azure_status is None:
            conn = db_azure.connect(self.cfg)
            try:
                db_azure.execute(
                    conn,
                    "INSERT INTO loadtest.experiment_context "
                    "(experiment_uid, resource_uid, started_at_utc, workload_class, "
                    " random_seed, status) "
                    "VALUES (?, ?, ?, ?, ?, N'CREATED')",
                    (exp_uid, resource_uid, now, workload_class, random_seed),
                )
                db_azure.execute(
                    conn,
                    "INSERT INTO loadtest.workload_control "
                    "(experiment_uid, status, requested_by) "
                    "VALUES (?, N'CREATED', N'system')",
                    (exp_uid,),
                )
            finally:
                conn.close()

        supabase_status = self.supabase_status()
        if supabase_status is None:
            sku = self._sku_fields()
            with db_supabase.transaction(self.cfg) as supabase_conn:
                db_supabase.query(
                    supabase_conn,
                    "insert into experiment.experiment_run "
                    "(experiment_uid, resource_uid, started_at_utc, workload_class, "
                    " random_seed, status, config_json, "
                    " subscription_id, resource_group, server_name, database_name, "
                    " region_code, sku_name, arm_sku_name, tier, compute_model, "
                    " capacity_vcores, min_capacity_vcores, family, "
                    " auto_pause_delay_minutes, source) "
                    "values (%s, %s, %s, %s, %s, 'CREATED', %s, "
                    " %s, %s, %s, %s, %s, %s, %s, %s, %s, "
                    " %s, %s, %s, %s, %s, %s) "
                    "on conflict (experiment_uid) do update set "
                    " subscription_id = excluded.subscription_id, "
                    " resource_group = excluded.resource_group, "
                    " server_name = excluded.server_name, "
                    " database_name = excluded.database_name, "
                    " region_code = excluded.region_code, "
                    " sku_name = excluded.sku_name, "
                    " arm_sku_name = excluded.arm_sku_name, "
                    " tier = excluded.tier, "
                    " compute_model = excluded.compute_model, "
                    " capacity_vcores = excluded.capacity_vcores, "
                    " min_capacity_vcores = excluded.min_capacity_vcores, "
                    " family = excluded.family, "
                    " auto_pause_delay_minutes = excluded.auto_pause_delay_minutes, "
                    " source = excluded.source, "
                    " updated_at_utc = now()",
                    (
                        exp_uid,
                        resource_uid,
                        now,
                        workload_class,
                        random_seed,
                        json.dumps(config_json),
                        sku["subscription_id"],
                        sku["resource_group"],
                        sku["server_name"],
                        sku["database_name"],
                        sku["region_code"],
                        sku["sku_name"],
                        sku["arm_sku_name"],
                        sku["tier"],
                        sku["compute_model"],
                        sku["capacity_vcores"],
                        sku["min_capacity_vcores"],
                        sku["family"],
                        sku["auto_pause_delay_minutes"],
                        sku["source"],
                    ),
                )

    # -- high level ----------------------------------------------------------
    def start(self, requested_by: str = "bot") -> str:
        return self.transition("RUNNING", requested_by=requested_by, reason="start")

    def request_pause(self, requested_by: str = "bot") -> str:
        return self.transition("PAUSING", requested_by=requested_by, reason="pause")

    def acknowledge_paused(self) -> str:
        """Called by the worker once the current operation batch finished."""
        return self.transition("PAUSED", requested_by="workload", reason="paused")

    def resume(self, requested_by: str = "bot") -> str:
        return self.transition("RUNNING", requested_by=requested_by, reason="resume")

    def request_stop(self, requested_by: str = "bot", reason: str = "requested") -> str:
        return self.transition("STOPPING", requested_by=requested_by, reason=reason)

    def confirm_stopped(self, reason: str = "completed", requested_by: str = "system") -> str:
        return self.transition("STOPPED", requested_by=requested_by, reason=reason)

    def mark_failed(self, reason: str, requested_by: str = "system") -> str:
        return self.transition("FAILED", requested_by=requested_by, reason=reason)
