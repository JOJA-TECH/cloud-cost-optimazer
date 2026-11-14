from src.metrics import sql_extras
from src.metrics.normalizer import METRIC_CATALOG


def test_benign_waits_are_excluded():
    # These dominate raw sums and would drown the real bottleneck.
    assert sql_extras.wait_class("SOS_WORK_DISPATCHER") is None
    assert sql_extras.wait_class("HADR_WORK_QUEUE") is None
    assert sql_extras.wait_class("SLEEP_TASK") is None
    assert sql_extras.wait_class("PREEMPTIVE_OS_FILEOPS") is None


def test_waits_are_classified_by_bottleneck():
    assert sql_extras.wait_class("WRITELOG") == "log"
    assert sql_extras.wait_class("LOG_RATE_GOVERNOR") == "log"
    assert sql_extras.wait_class("LCK_M_X") == "lock"
    assert sql_extras.wait_class("PAGEIOLATCH_SH") == "io"
    assert sql_extras.wait_class("SOS_SCHEDULER_YIELD") == "cpu"
    assert sql_extras.wait_class("RESOURCE_SEMAPHORE") == "memory"
    assert sql_extras.wait_class("ASYNC_NETWORK_IO") == "other"


def test_rates_are_per_second_and_drop_a_restarted_counter():
    assert sql_extras.rates({"WRITELOG": 900}, {"WRITELOG": 100}, 10) == {"WRITELOG": 80.0}
    # A database resume resets sys.dm_os_wait_stats: drop the cycle, never emit
    # a negative or a fabricated spike.
    assert sql_extras.rates({"WRITELOG": 40}, {"WRITELOG": 100}, 10) == {}
    assert sql_extras.rates({"WRITELOG": 40}, None, 10) == {}


def test_every_emitted_code_exists_in_the_catalog():
    # A code missing from the catalog is rejected by the quality policy and the
    # reading is lost silently, so the two lists must not drift apart.
    codes = set(sql_extras.COUNTER_CODES.values()) | {"wait_critical_ms_per_sec"}
    codes |= {f"wait_{name}_ms_per_sec" for name in sql_extras.WAIT_CLASSES}
    codes |= {
        "rows_read_bytes_per_sec",
        "rows_write_bytes_per_sec",
        "rows_stall_ms_per_sec",
        "log_stall_ms_per_sec",
    }
    # Caps and slow inventory emitted by _slow().
    codes |= {
        "cpu_limit_vcores",
        "max_dop",
        "max_sessions",
        "max_memory_percent",
        "max_memory_grant_percent",
        "log_rate_cap_bytes_per_sec",
        "instance_max_log_rate_bytes_per_sec",
        "running_requests",
        "blocked_requests",
        "longest_running_ms",
        "log_size_bytes",
        "log_vlf_count",
        "index_seeks_total",
        "index_scans_total",
        "index_lookups_total",
        "index_updates_total",
        "tuning_recommendations_count",
        "tempdb_used_bytes",
        "runnable_tasks",
        "work_queue_count",
    }
    assert sorted(code for code in codes if code not in METRIC_CATALOG) == []
