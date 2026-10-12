"""Telegram control channel.

Only chat ids listed in TELEGRAM_ALLOWED_CHAT_IDS may issue commands. The check
fails closed: an empty allowlist authorizes nobody.
"""

from .authorization import is_authorized, parse_allowed_chat_ids
from .bot import run_bot, send_message
from .commands import HELP_TEXT, handle_command, handle_update

__all__ = [
    "is_authorized",
    "parse_allowed_chat_ids",
    "send_message",
    "run_bot",
    "handle_command",
    "handle_update",
    "HELP_TEXT",
]