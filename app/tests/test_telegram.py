from src.config import Config
from src.telegram.authorization import is_authorized, parse_allowed_chat_ids
from src.telegram.commands import HELP_TEXT, handle_update


def _cfg(allowed=("42",)):
    return Config(
        experiment_uid="33333333-3333-3333-3333-333333333333",
        telegram_allowed_chat_ids=frozenset(allowed),
    )


def test_parse_allowed_chat_ids_stringifies_and_cleans():
    assert parse_allowed_chat_ids([123, " 456 ", ""]) == frozenset({"123", "456"})


def test_is_authorized_fails_closed_when_empty():
    assert is_authorized(_cfg(allowed=()), "42") is False


def test_is_authorized_matches_string_and_int():
    cfg = _cfg(allowed=("42",))
    assert is_authorized(cfg, 42) is True
    assert is_authorized(_cfg(), 99) is False
    assert is_authorized(_cfg(allowed=()), None) is False


def test_handle_update_ignores_non_commands():
    update = {"message": {"chat": {"id": 42}, "text": "hello"}}
    assert handle_update(update, _cfg()) is None


def test_handle_update_rejects_unauthorized():
    update = {"message": {"chat": {"id": 999}, "text": "/help"}}
    assert handle_update(update, _cfg()) == (999, "not authorized")


def test_handle_update_help_for_authorized_chat():
    update = {"message": {"chat": {"id": 42}, "text": "/help"}}
    assert handle_update(update, _cfg()) == (42, HELP_TEXT)
