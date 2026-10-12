-- ==========================================================================
-- 003_workload_control.sql
-- Controlled state machine for the workload plus the seed query templates.
-- The only legal way to change status is a Step A -> Step B transition listed
-- in the trigger; anything else raises and the audit trail always records.
-- ==========================================================================

IF OBJECT_ID(N'loadtest.workload_control', N'U') IS NULL
BEGIN
    CREATE TABLE loadtest.workload_control (
        control_uid     uniqueidentifier NOT NULL CONSTRAINT DF_wc_uid DEFAULT NEWID(),
        experiment_uid  uniqueidentifier NOT NULL,
        status          nvarchar(24)     NOT NULL CONSTRAINT DF_wc_status
                            DEFAULT N'CREATED',
        updated_at_utc  datetime2        NOT NULL CONSTRAINT DF_wc_updated
                            DEFAULT SYSUTCDATETIME(),
        requested_by    nvarchar(32)     NULL,
        CONSTRAINT PK_workload_control PRIMARY KEY (experiment_uid),
        CONSTRAINT CHK_wc_status CHECK (
            status IN (
                N'CREATED', N'RUNNING', N'PAUSING', N'PAUSED',
                N'STOPPING', N'STOPPED', N'FAILED'
            )
        )
    );
END

IF OBJECT_ID(N'loadtest.control_audit', N'U') IS NULL
BEGIN
    CREATE TABLE loadtest.control_audit (
        audit_uid       uniqueidentifier NOT NULL CONSTRAINT DF_ca_uid DEFAULT NEWID(),
        experiment_uid  uniqueidentifier NOT NULL,
        from_status     nvarchar(24)     NULL,
        to_status       nvarchar(24)     NOT NULL,
        changed_at_utc  datetime2        NOT NULL CONSTRAINT DF_ca_changed
                            DEFAULT SYSUTCDATETIME(),
        changed_by      nvarchar(128)    NOT NULL,
        CONSTRAINT PK_control_audit PRIMARY KEY (audit_uid)
    );
END

IF EXISTS (SELECT 1 FROM sys.triggers WHERE name = N'trg_workload_control_guard')
BEGIN
    DROP TRIGGER loadtest.trg_workload_control_guard;
END

CREATE TRIGGER loadtest.trg_workload_control_guard
ON loadtest.workload_control
INSTEAD OF UPDATE
AS
BEGIN
    SET NOCOUNT ON;

    DECLARE @invalid nvarchar(max) = N'';

    SELECT @invalid = @invalid + N' ' + CONVERT(nvarchar(36), i.experiment_uid)
    FROM inserted i
    JOIN deleted d ON d.experiment_uid = i.experiment_uid
    LEFT JOIN (
        SELECT N'CREATED'  AS [from], N'RUNNING'  AS [to], 1 AS ok UNION ALL
        SELECT N'RUNNING'  AS [from], N'PAUSING'  AS [to], 1 UNION ALL
        SELECT N'RUNNING'  AS [from], N'STOPPING' AS [to], 1 UNION ALL
        SELECT N'RUNNING'  AS [from], N'FAILED'   AS [to], 1 UNION ALL
        SELECT N'PAUSING'  AS [from], N'PAUSED'   AS [to], 1 UNION ALL
        SELECT N'PAUSING'  AS [from], N'RUNNING'  AS [to], 1 UNION ALL
        SELECT N'PAUSED'   AS [from], N'RUNNING'  AS [to], 1 UNION ALL
        SELECT N'PAUSED'   AS [from], N'STOPPING' AS [to], 1 UNION ALL
        SELECT N'STOPPING' AS [from], N'STOPPED'  AS [to], 1 UNION ALL
        SELECT N'STOPPING' AS [from], N'FAILED'   AS [to], 1 UNION ALL
        SELECT N'FAILED'   AS [from], N'RUNNING'  AS [to], 1
    ) t ON t.[from] = d.status AND t.[to] = i.status
    WHERE i.status <> d.status
      AND t.ok IS NULL;

    IF @invalid <> N''
    BEGIN
        RAISERROR(N'Invalid workload_control transition for experiment(s): %s', 16, 1, @invalid);
        RETURN;
    END

    UPDATE wc
    SET status         = i.status,
        updated_at_utc = SYSUTCDATETIME(),
        requested_by   = i.requested_by
    FROM loadtest.workload_control wc
    JOIN inserted i ON i.experiment_uid = wc.experiment_uid;

    INSERT INTO loadtest.control_audit (
        experiment_uid, from_status, to_status, changed_at_utc, changed_by
    )
    SELECT i.experiment_uid, d.status, i.status, SYSUTCDATETIME(),
           ISNULL(i.requested_by, N'unknown')
    FROM inserted i
    JOIN deleted d ON d.experiment_uid = i.experiment_uid;
END

-- --------------------------------------------------------------------------
-- Seed query templates (append-only, keyed).
-- --------------------------------------------------------------------------
IF OBJECT_ID(N'loadtest.query_templates', N'U') IS NULL
BEGIN
    CREATE TABLE loadtest.query_templates (
        template_key    nvarchar(40)    NOT NULL,
        operation_type  nvarchar(16)    NOT NULL,
        sql_template    nvarchar(max)   NOT NULL,
        comment_text    nvarchar(255)   NULL,
        created_at_utc  datetime2       NOT NULL CONSTRAINT DF_qt_created
                            DEFAULT SYSUTCDATETIME(),
        CONSTRAINT PK_query_templates PRIMARY KEY (template_key)
    );
END

IF NOT EXISTS (
    SELECT 1 FROM loadtest.query_templates WHERE template_key = N'point_lookup'
)
BEGIN
    INSERT INTO loadtest.query_templates (template_key, operation_type, sql_template, comment_text) VALUES
    (N'point_lookup', N'SELECT',
     N'SELECT TOP (1) fe.event_id, fe.created_at_utc, fe.operation_type '
     N'FROM loadtest.fact_events fe WHERE fe.experiment_uid = @experiment_uid '
     N'ORDER BY fe.created_at_utc DESC',
     N'Cheap point lookup refreshing a hot page.'),
    (N'range_scan', N'SELECT',
     N'SELECT COUNT_BIG(*) FROM loadtest.fact_events fe '
     N'WHERE fe.experiment_uid = @experiment_uid '
     N'AND fe.created_at_utc BETWEEN @start_utc AND @end_utc',
     N'Indexed range scan over the experiment window.'),
    (N'insert_event', N'INSERT',
     N'INSERT INTO loadtest.fact_events (experiment_uid, operation_type, '
     N'payload_size_bytes, batch_id, worker_id) '
     N'VALUES (@experiment_uid, @operation_type, @payload_size_bytes, @batch_id, @worker_id)',
     N'Transactional insert driving log buffer writes.'),
    (N'update_row', N'UPDATE',
     N'UPDATE TOP (1) loadtest.fact_events '
     N'SET payload_size_bytes = payload_size_bytes + 1 '
     N'WHERE experiment_uid = @experiment_uid',
     N'Point update generating log buffer traffic.'),
    (N'aggregate', N'SELECT',
     N'SELECT experiment_uid, COUNT_BIG(*) AS n, AVG(payload_size_bytes) AS avg_size '
     N'FROM loadtest.fact_events GROUP BY experiment_uid',
     N'Small aggregate used by cpu_intensive runs.');
END