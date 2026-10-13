-- ==========================================================================
-- 005_enable_query_store.sql
-- Enable Query Store so the collector can read sys.query_store_runtime_stats
-- (interval aggregate durations; never used for exact percentiles).
-- Idempotent: only alters the database when Query Store is not already on.
-- ==========================================================================

IF EXISTS (
    SELECT 1
    FROM sys.database_query_store_options
    WHERE actual_state_desc = N'OFF'
)
BEGIN
    ALTER DATABASE CURRENT SET QUERY_STORE = ON
    (
        OPERATION_MODE = READ_WRITE,
        INTERVAL_LENGTH_MINUTES = 1,
        QUERY_CAPTURE_MODE = AUTO,
        MAX_STORAGE_SIZE_MB = 100,
        WAIT_STATS_CAPTURE_MODE = ON
    );
END
