from src.config import Config


def _config(**overrides):
    base = dict(
        experiment_uid="22222222-2222-2222-2222-222222222222",
        azure_sql_server="cco-test.database.windows.net",
        azure_sql_database="loadtest",
    )
    base.update(overrides)
    return Config(**base)


def test_resource_uid_is_deterministic():
    a = _config().resource_uid()
    b = _config().resource_uid()
    assert a == b


def test_resource_uid_changes_with_resource():
    first = _config().resource_uid()
    other = _config(azure_sql_server="other.database.windows.net").resource_uid()
    assert first != other


def test_azure_monitor_resource_id_prefers_explicit():
    cfg = _config(azure_sql_resource_id="/subscriptions/x/resourceGroups/rg/providers/Microsoft.Sql/servers/s/databases/d")
    assert cfg.azure_monitor_resource_id().startswith("/subscriptions/")


def test_supabase_dsn_has_no_none(cfg):
    cfg = Config(
        experiment_uid="x",
        supabase_db_host="db.example.supabase.co",
        supabase_db_user="postgres",
        supabase_db_password="secret",
    )
    dsn = cfg.supabase_dsn()
    assert "host=db.example.supabase.co" in dsn
    assert "None" not in dsn
