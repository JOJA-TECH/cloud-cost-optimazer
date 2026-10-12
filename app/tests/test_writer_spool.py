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
