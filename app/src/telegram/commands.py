"""Telegram command handling (transport-agnostic).

``handle_command`` returns the reply text; ``handle_update`` enforces
authorization and dispatches a raw Bot API update. Both are pure enough to unit
test without a network.
"""

from __future__ import annotations

import logging
from typing import Optional

from .. import db_supabase
from ..config import Config
from ..sync import writer
from ..workload.controller import TransitionError, WorkloadController
from .authorization import is_authorized

logger = logging.getLogger(__name__)

HELP_TEXT = (
    "Cloud Cost Optimizer control\n"
    "/status - experiment, collector and spool status\n"
    "/start - start the workload\n"
    "/pause - request pause (acknowledged after the current batch)\n"
    "/resume - resume from paused\n"
    "/stop - request a safe stop\n"
    "/collector - last collector cycle\n"
    "/spool - pending local spool buffer\n"
    "/help - this message"
)


def _last_collector_run(cfg: Config) -> Optional[dict]:
    try:
        with db_supabase.transaction(cfg) as conn:
            rows = db_supabase.query(
                conn,
                "select status, rows_observed, rows_written, started_at_utc, "
                "finished_at_utc, error_message from ops.collector_run "
                "where experiment_uid = %s order by started_at_utc desc limit 1",
                (cfg.experiment_uid,),
            )
        return rows[0] if rows else None
    except Exception as exc:  # noqa: BLE001
        logger.debug("last collector run read failed: %s", exc)
        return None


def _safe(fn):
    try:
        return fn()
    except Exception:  # noqa: BLE001
        return None


def _status_text(cfg: Config) -> str:
    controller = WorkloadController(cfg)
    supabase_status = _safe(controller.supabase_status)
    last = _last_collector_run(cfg)
    azure_status = _safe(controller.azure_status)
    spool = writer.spool_status(cfg)

    lines = [
        f"experiment: {cfg.experiment_uid}",
        f"supabase status: {supabase_status or 'unknown'}",
        f"azure status: {azure_status or 'unknown'}",
    ]
    if supabase_status is None:
        lines.append("warning: experiment not registered on Supabase")
    if last:
        lines.append(
            f"last collector: {last.get('status')} "
            f"({last.get('rows_written')}/{last.get('rows_observed')} rows)"
        )
    else:
        lines.append("last collector: n/a")
    lines.append(
        f"spool: {spool['files']} files, {spool['mb']} MB / {spool['max_mb']} MB"
        + (" (OVER LIMIT)" if spool["over_limit"] else "")
    )
    return "\n".join(lines)


def handle_command(cfg: Config, command: str, args: list[str]) -> str:
    command = (command or "").lower()
    if command in ("/help", "/start_help", "help"):
        return HELP_TEXT

    controller = WorkloadController(cfg)

    if command == "/status":
        return _status_text(cfg)

    if command == "/collector":
        last = _last_collector_run(cfg)
        if not last:
            return "no collector cycles recorded"
        return (
            f"collector {last.get('status')}: observed={last.get('rows_observed')} "
            f"written={last.get('rows_written')} "
            f"error={last.get('error_message') or '-'}"
        )

    if command == "/spool":
        spool = writer.spool_status(cfg)
        return (
            f"spool: {spool['files']} files, {spool['mb']} MB / {spool['max_mb']} MB"
            + (" (OVER LIMIT)" if spool["over_limit"] else "")
        )

    if command in ("/start", "/pause", "/resume", "/stop"):
        try:
            if command == "/start":
                controller.start(requested_by="bot")
                return "workload starting"
            if command == "/pause":
                controller.request_pause(requested_by="bot")
                return "pause requested"
            if command == "/resume":
                controller.resume(requested_by="bot")
                return "workload resuming"
            controller.request_stop(requested_by="bot", reason="telegram_stop")
            return "stop requested"
        except TransitionError as exc:
            return f"rejected: {exc}"
        except Exception as exc:  # noqa: BLE001
            logger.error("command %s failed: %s", command, exc)
            return f"error: {exc}"

    return HELP_TEXT


def handle_update(update: dict, cfg: Config) -> Optional[tuple[int, str]]:
    """Return (chat_id, reply) for a text command, or None if not applicable.

    Unauthorized senders get a single refusal and nothing else happens.
    """
    message = update.get("message") or update.get("edited_message") or {}
    chat = message.get("chat") or {}
    chat_id = chat.get("id")
    text = (message.get("text") or "").strip()
    if not text or not text.startswith("/"):
        return None

    parts = text.split()
    command, args = parts[0], parts[1:]

    if not is_authorized(cfg, chat_id):
        logger.warning("unauthorized command from chat %s", chat_id)
        return (chat_id, "not authorized") if chat_id is not None else None

    reply = handle_command(cfg, command, args)
    return chat_id, reply