import pytest

from src import db_supabase


class _FakeCursor:
    def __init__(self, rows=None, description=None):
        self._rows = rows or []
        self.description = description

    def execute(self, sql, params=()):
        self._sql = sql

    def fetchall(self):
        if self.description is None:
            raise AssertionError("fetchall() called on a statement with no result set")
        return self._rows

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


class _FakeConnection:
    def __init__(self, cursor):
        self._cursor = cursor

    def cursor(self, *args, **kwargs):
        return self._cursor


class _FakePsycopg2:
    class extras:  # noqa: D106 - stand-in for psycopg2.extras
        RealDictCursor = object


@pytest.fixture
def fake_psycopg2(monkeypatch):
    monkeypatch.setattr(db_supabase, "_psycopg2", lambda: _FakePsycopg2)


def test_query_returns_rows(fake_psycopg2):
    cur = _FakeCursor(rows=[{"x": 1}], description=[("x",)])
    assert db_supabase.query(_FakeConnection(cur), "select 1") == [{"x": 1}]


def test_query_write_statement_returns_empty(fake_psycopg2):
    # INSERT/UPDATE/DELETE have no result set; fetchall() would raise.
    cur = _FakeCursor(description=None)
    assert db_supabase.query(_FakeConnection(cur), "update t set x = 1") == []
