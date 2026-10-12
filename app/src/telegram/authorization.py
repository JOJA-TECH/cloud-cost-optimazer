"""Command authorization.

Fail-closed: if no chat id is configured, nobody is authorized. Chat ids are
compared as strings so both ``123`` and ``"123"`` work.
"""

from __future__ import annotations

import logging
from typing import FrozenSet, Iterable, Optional

from ..config import Config

logger = logging.getLogger(__name__)


def parse_allowed_chat_ids(raw: Iterable) -> FrozenSet[str]:
    return frozenset(str(item).strip() for item in raw if str(item).strip())


def is_authorized(cfg: Config, chat_id: Optional[str | int]) -> bool:
    allowed = cfg.telegram_allowed_chat_ids
    if not allowed:
        logger.warning("command rejected: TELEGRAM_ALLOWED_CHAT_IDS is empty")
        return False
    if chat_id is None:
        return False
    return str(chat_id) in allowed
