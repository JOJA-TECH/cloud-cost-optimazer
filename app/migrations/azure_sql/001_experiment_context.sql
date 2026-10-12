-- ==========================================================================
-- 001_experiment_context.sql
-- Experiment context kept inside the system under test for correlation.
-- The authoritative experiment record lives in Supabase (experiment.experiment_run).
-- ==========================================================================

IF OBJECT_ID(N'loadtest.experiment_context', N'U') IS NULL
BEGIN
    CREATE TABLE loadtest.experiment_context (
        experiment_uid  uniqueidentifier NOT NULL,
        resource_uid    uniqueidentifier NOT NULL,
        started_at_utc  datetime2        NOT NULL CONSTRAINT DF_expctx_start
                            DEFAULT SYSUTCDATETIME(),
        finished_at_utc datetime2        NULL,
        workload_class  nvarchar(32)     NOT NULL,
        random_seed     int              NULL,
        status          nvarchar(24)     NOT NULL CONSTRAINT DF_expctx_status
                            DEFAULT N'CREATED',
        stop_reason     nvarchar(255)    NULL,
        created_at_utc  datetime2        NOT NULL CONSTRAINT DF_expctx_created
                            DEFAULT SYSUTCDATETIME(),
        CONSTRAINT PK_experiment_context PRIMARY KEY (experiment_uid),
        CONSTRAINT CHK_expctx_status CHECK (
            status IN (
                N'CREATED', N'RUNNING', N'PAUSING', N'PAUSED',
                N'STOPPING', N'STOPPED', N'FAILED'
            )
        )
    );
END