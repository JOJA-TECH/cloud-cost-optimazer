import datetime as dt
from contextlib import contextmanager

from src import db_azure, db_supabase
from src.workload import scheduled


def test_schedule_index_roundtrip(tmp_path, monkeypatch):
    monkeypatch.setattr(scheduled, "STATE_FILE", tmp_path / "index")
    assert scheduled._next_index() == 0
    scheduled._save_index(5)
    assert scheduled._next_index() == 5
    assert scheduled.PROFILES[5 % len(scheduled.PROFILES)]["workload_class"] == "cpu_intensive"


def test_wake_database_retries_until_login(cfg, monkeypatch):
    calls = {"n": 0}

    class Conn:
        def close(self):
            pass

    def fake_connect(_cfg):
        calls["n"] += 1
        if calls["n"] < 3:
            raise db_azure.AzureSQLConnectionError("HYT00 login timeout")
        return Conn()

    monkeypatch.setattr(db_azure, "connect", fake_connect)
    monkeypatch.setattr(scheduled.time, "sleep", lambda _s: None)

    scheduled._wake_database(cfg, attempts=5, delay=0)
    assert calls["n"] == 3


def test_experiment_age_hours(cfg, monkeypatch):
    started = dt.datetime.now(dt.timezone.utc) - dt.timedelta(hours=10)

    @contextmanager
    def fake_tx(_cfg):
        yield None

    monkeypatch.setattr(db_supabase, "transaction", fake_tx)
    monkeypatch.setattr(
        db_supabase, "query", lambda *a, **k: [{"started_at_utc": started}]
    )

    age = scheduled._experiment_age_hours(cfg)
    assert 9.9 < age < 10.1
