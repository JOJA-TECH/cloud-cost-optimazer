import datetime as dt

import pytest

from src.config import Config
from src.metrics.normalizer import MetricReading


@pytest.fixture
def cfg(tmp_path):
    return Config(
        experiment_uid="11111111-1111-1111-1111-111111111111",
        azure_sql_server="cco-test.database.windows.net",
        azure_sql_database="loadtest",
        local_spool_dir=str(tmp_path / "spool"),
        local_spool_max_mb=1,
    )


@pytest.fixture
def reading_factory():
    def make(code="cpu_percent", value=10.0, source="azure_monitor", second=0, seconds=60):
        return MetricReading(
            metric_code=code,
            value=value,
            unit="percent",
            observed_at_utc=dt.datetime(2024, 1, 1, 0, 0, second, tzinfo=dt.timezone.utc),
            interval_seconds=seconds,
            source=source,
        )

    return make
