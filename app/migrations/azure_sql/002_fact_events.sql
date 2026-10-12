-- ==========================================================================
-- 002_fact_events.sql
-- Append-only event ledger for the simulated workload plus an optional
-- payload table (details off the hot path).
-- ==========================================================================

IF OBJECT_ID(N'loadtest.fact_events', N'U') IS NULL
BEGIN
    CREATE TABLE loadtest.fact_events (
        event_id            uniqueidentifier NOT NULL CONSTRAINT DF_fe_id
                                DEFAULT NEWID(),
        experiment_uid      uniqueidentifier NOT NULL,
        created_at_utc      datetime2        NOT NULL CONSTRAINT DF_fe_created
                                DEFAULT SYSUTCDATETIME(),
        operation_type      nvarchar(16)     NOT NULL,
        payload_size_bytes  int              NOT NULL CONSTRAINT DF_fe_size
                                DEFAULT 0,
        batch_id            bigint           NOT NULL,
        worker_id           int              NOT NULL,
        CONSTRAINT PK_fact_events PRIMARY KEY (event_id)
    );
END

IF OBJECT_ID(N'loadtest.fact_events_payload', N'U') IS NULL
BEGIN
    CREATE TABLE loadtest.fact_events_payload (
        event_id        uniqueidentifier NOT NULL,
        payload         nvarchar(max)    NULL,
        payload_sha256  char(64)         NOT NULL,
        CONSTRAINT PK_fact_events_payload PRIMARY KEY (event_id),
        CONSTRAINT FK_fact_events_payload_event
            FOREIGN KEY (event_id) REFERENCES loadtest.fact_events (event_id)
    );
END

IF NOT EXISTS (
    SELECT 1 FROM sys.indexes
    WHERE name = N'IX_fe_experiment_time'
      AND object_id = OBJECT_ID(N'loadtest.fact_events')
)
BEGIN
    CREATE INDEX IX_fe_experiment_time
        ON loadtest.fact_events (experiment_uid, created_at_utc);
END