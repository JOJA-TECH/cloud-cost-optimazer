import contextlib

import src.sync.writer as writer


def test_persist_spools_when_supabase_unavailable(cfg, reading_factory, monkeypatch):
    def boom(_cfg):
        raise RuntimeError("supabase down")

    monkeypatch.setattr(writer.db_supabase, "transaction", boom)

    reading = reading_factory(code="cpu_percent", value=42.0)
    result = writer.persist(cfg, "collector-uid", [reading])

    assert result.rows_observed == 1
    assert result.rows_written == 0
    assert result.spooled is True
    assert result.error == "supabase down"

    spool = writer._spool(cfg)
    pending = list(spool.iter_pending())
    assert len(pending) == 1
    _, kind, rows = pending[0]
    assert kind == "metric_reading"
    assert rows[0]["metric_code"] == "cpu_percent"
    assert isinstance(rows[0]["observed_at_utc"], str)  # serialized for the spool


def test_spool_status_shape(cfg):
    status = writer.spool_status(cfg)
    assert set(status) == {"files", "mb", "max_mb", "over_limit"}


def test_flush_registers_missing_collector_run(cfg, monkeypatch):
    # Spooled during an outage: the referenced collector_run was never stored.
    spool = writer._spool(cfg)
    spool.write_batch(
        "metric_reading",
        [
            {
                "collector_run_uid": "aaaaaaaa-0000-0000-0000-000000000000",
                "experiment_uid": "11111111-1111-1111-1111-111111111111",
                "source": "collector",
                "metric_code": "cpu_percent",
            }
        ],
    )

    statements = []

    @contextlib.contextmanager
    def fake_transaction(_cfg):
        yield object()

    def fake_query(conn, sql, params=()):
        statements.append(sql)
        return []

    def fake_insert(conn, table, columns, rows):
        assert any("ops.collector_run" in sql for sql in statements)
        return len(rows)

    monkeypatch.setattr(writer.db_supabase, "transaction", fake_transaction)
    monkeypatch.setattr(writer.db_supabase, "query", fake_query)
    monkeypatch.setattr(writer, "_insert_rows", fake_insert)

    result = writer.flush_spool(cfg)

    assert result["files"] == 1
    assert result["rows"] == 1
    assert result["pending_files"] == 0

