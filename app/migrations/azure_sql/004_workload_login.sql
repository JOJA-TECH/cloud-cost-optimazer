-- ==========================================================================
-- 004_workload_login.sql
-- Least-privilege contained database user for the workload generator.
-- Placeholders __AZURE_SQL_WORKLOAD_USER__ / __AZURE_SQL_WORKLOAD_PASSWORD__
-- are substituted at runtime by src/migrations.py; never committed.
-- ==========================================================================

IF NOT EXISTS (
    SELECT 1 FROM sys.database_principals WHERE name = N'__AZURE_SQL_WORKLOAD_USER__'
)
BEGIN
    CREATE USER [__AZURE_SQL_WORKLOAD_USER__]
        WITH PASSWORD = N'__AZURE_SQL_WORKLOAD_PASSWORD__';
END

-- The workload user may only touch the loadtest schema. It cannot read other
-- user schemas, cannot manage the database, and has no DDL on anything outside
-- loadtest (a contained user has no rights beyond what is granted here).
GRANT SELECT, INSERT, UPDATE, DELETE, REFERENCES, EXECUTE
    ON SCHEMA::loadtest TO [__AZURE_SQL_WORKLOAD_USER__];

-- Belt and braces: deny broad catalog access that is not required.
DENY VIEW DEFINITION TO [__AZURE_SQL_WORKLOAD_USER__];