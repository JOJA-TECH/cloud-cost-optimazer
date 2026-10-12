"""Telegram bot long-polling loop (stdlib only, no extra dependencies).

Uses the Bot API over HTTPS with urllib so the VM needs no Telegram SDK. Only
authorized chat ids are answered (see authorization.py).
"""

from __future__ import annotations

import argparse
import json
import logging
import signal
import sys
import time
import urllib.parse
import urllib.request
from typing import Optional

from ..config import Config, from_env, load_env_file
from .commands import handle_update

logger = logging.getLogger(__name__)

API_ROOT = "https://api.telegram.org"
POLL_TIMEOUT = 30  # long-poll seconds
HTTP_TIMEOUT = POLL_TIMEOUT + 15


class TelegramError(RuntimeError):
    pass


def _call(token: str, method: str, params: Optional[dict] = None) -> dict:
    if not token:
        raise TelegramError("TELEGRAM_BOT_TOKEN is empty")
    url = f"{API_ROOT}/bot{token}/{method}"
    data = urllib.parse.urlencode(params or {}).encode("utf-8")
    request = urllib.request.Request(url, data=data)
    try:
        with urllib.request.urlopen(request, timeout=HTTP_TIMEOUT) as response:
            payload = json.loads(response.read().decode("utf-8"))
    except Exception as exc:  # noqa: BLE001
        raise TelegramError(str(exc)) from exc
    if not payload.get("ok"):
        raise TelegramError(payload.get("description", "telegram api error"))
    return payload.get("result")


def send_message(cfg: Config, chat_id: int, text: str) -> None:
    try:
        _call(cfg.telegram_bot_token, "sendMessage", {"chat_id": chat_id, "text": text})
    except TelegramError as exc:
        logger.warning("cannot send telegram message: %s", exc)


def run_bot(cfg: Config) -> int:
    if not cfg.telegram_bot_token:
        logger.error("no TELEGRAM_BOT_TOKEN configured; refusing to start")
        return 2
    if not cfg.telegram_allowed_chat_ids:
        logger.warning("TELEGRAM_ALLOWED_CHAT_IDS is empty; every command is denied")

    stop = {"requested": False}

    def _handler(signum, _frame):
        logger.info("received signal %s; stopping bot", signum)
        stop["requested"] = True

    signal.signal(signal.SIGTERM, _handler)
    signal.signal(signal.SIGINT, _handler)

    try:
        me = _call(cfg.telegram_bot_token, "getMe")
        logger.info("telegram bot @%s started", me.get("username"))
    except TelegramError as exc:
        logger.error("getMe failed: %s", exc)
        return 1

    offset = 0
    while not stop["requested"]:
        try:
            updates = _call(
                cfg.telegram_bot_token,
                "getUpdates",
                {"offset": offset, "timeout": POLL_TIMEOUT,
                 "allowed_updates": json.dumps(["message", "edited_message"])},
            )
        except TelegramError as exc:
            logger.warning("getUpdates failed: %s", exc)
            time.sleep(5)
            continue

        for update in updates or []:
            offset = max(offset, int(update.get("update_id", 0)) + 1)
            try:
                reply = handle_update(update, cfg)
            except Exception as exc:  # noqa: BLE001 - one bad update must not stop the bot
                logger.error("update handling failed: %s", exc)
                continue
            if reply is not None:
                chat_id, text = reply
                send_message(cfg, chat_id, text)

    logger.info("telegram bot stopped")
    return 0


def _setup_logging(verbose: bool = False) -> None:
    logging.basicConfig(
        level=logging.DEBUG if verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="CCO telegram control bot")
    parser.add_argument("--service", action="store_true", help="run as a systemd service")
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    _setup_logging(args.verbose)
    load_env_file()
    cfg = from_env()
    return run_bot(cfg)


if __name__ == "__main__":
    raise SystemExit(main())
