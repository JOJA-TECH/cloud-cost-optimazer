# Tests for the read-only feature-store reader (mocked database).

from __future__ import annotations

import pandas as pd
import pytest

import forecasting.serving.data as data_mod
from forecasting.serving.data import FeatureStoreError, FeatureStoreReader


class FakeCursor:
    def __init__(self):
        self.stmt = None
        self.params = None
        self._desc = None
        self._rows = []

    def __enter__(self):
        return self

    def __exit__(self, *a):
        pass

    def execute(self, stmt, params=None):
        self.stmt = stmt
        self.params = params

    @property
    def description(self):
        return self._desc

    @description.setter
    def description(self, value):
        self._desc = value

    def fetchall(self):
        return self._rows


class FakeConn:
    def __init__(self, rows=None, desc=None):
        self.rows = rows or []
        self.desc = desc or []
        self.closed = False

    def __enter__(self):
        return self

    def __exit__(self, *a):
        pass

    def cursor(self):
        cur = FakeCursor()
        cur._desc = self.desc
        cur._rows = self.rows
        return cur

    def close(self):
        self.closed = True


def make_reader(monkeypatch, rows=None, desc=None):
    conn = FakeConn(rows=rows, desc=desc)

    def fake_connect(dsn, *, read_only=False, autocommit=True):
        assert read_only is True, "reader must open read-only connections"
        return conn

    monkeypatch.setattr(data_mod, "connect", fake_connect)
    return FeatureStoreReader("postgresql://user:pass@127.0.0.1:5432/cco_data")


def test_fetch_is_parameterized_by_resource_uid(monkeypatch):
    rows = [("2026-09-10 00:00:00+00", 30.0, 100.0)]
    desc = [("period_start_utc",), ("cpu_avg",), ("workload_intensity",)]
    reader = make_reader(monkeypatch, rows=rows, desc=desc)

    # Monkeypatch read_frame to capture statement and params without real parsing.
    captured = {}

    def fake_read_frame(conn, statement, params=None):
        captured["statement"] = statement
        captured["params"] = params
        return pd.DataFrame(rows, columns=[d[0] for d in desc])

    monkeypatch.setattr(data_mod, "read_frame", fake_read_frame)

    df = reader.fetch_hourly_features("uid-abc")
    stmt = captured["statement"]
    assert "%s" in stmt
    assert captured["params"] == ("uid-abc",)
    # No future labels (target_*) are ever selected.
    assert "target_" not in stmt
    # The internal resource_id is resolved through core.resource, never sent by the caller.
    assert "core.resource" in stmt
    assert "resource_uid = %s" in stmt


def test_resource_exists_queries_core_resource(monkeypatch):
    captured = {}

    def fake_read_frame(conn, statement, params=None):
        captured["statement"] = statement
        captured["params"] = params
        return pd.DataFrame([{"ok": 1}])

    monkeypatch.setattr(data_mod, "read_frame", fake_read_frame)
    reader = make_reader(monkeypatch)

    assert reader.resource_exists("uid-abc") is True
    assert captured["params"] == ("uid-abc",)
    assert "core.resource" in captured["statement"]


def test_empty_dsn_raises(monkeypatch):
    with pytest.raises(FeatureStoreError, match="FORECAST_DATA_DSN is not configured"):
        FeatureStoreReader("")
