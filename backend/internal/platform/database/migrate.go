package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Migrate creates the cco_app persistence model through GORM and applies
// PostgreSQL-specific audit constraints, indexes, policies, and triggers.
// Versioned schema migrations are applied idempotently through
// runSchemaMigrations, which records each applied migration and its SHA-256
// checksum in app.schema_migrations so drift is detectable on every startup.
func Migrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database handle is nil")
	}
	if db.Dialector.Name() == string(DriverPostgres) {
		for _, schema := range []string{"iam", "organization", "cloud", "app", "audit"} {
			if err := db.Exec("CREATE SCHEMA IF NOT EXISTS " + schema).Error; err != nil {
				return fmt.Errorf("create schema %s: %w", schema, err)
			}
		}
	}
	groups := []struct {
		schema string
		models []any
	}{
		{"iam", []any{&UserModel{}, &ExternalIdentityModel{}, &RoleModel{}, &PermissionModel{}, &RolePermissionModel{}, &UserRoleModel{}, &RefreshTokenModel{}}},
		{"organization", []any{&OrganizationModel{}, &MemberModel{}, &ProjectModel{}}},
		{"cloud", []any{&CloudAccountModel{}, &AccountProjectModel{}, &ResourceBindingModel{}, &ResourceModel{}}},
		{"app", []any{&DashboardPreferenceModel{}, &NotificationModel{}, &NotificationDeliveryModel{}, &SavedViewModel{}, &UserPreferenceModel{}, &RecommendationInteractionModel{}, &ReportRequestModel{}, &ReportArtifactModel{}}},
		{"audit", []any{&AuditEventModel{}}},
	}
	for _, group := range groups {
		for _, model := range group.models {
			tableName := TableName(db, group.schema, tablerName(model))
			if err := db.Table(tableName).AutoMigrate(model); err != nil {
				return fmt.Errorf("migrate %s.%s: %w", group.schema, tableName, err)
			}
		}
	}
	if db.Dialector.Name() == string(DriverPostgres) {
		if err := runSchemaMigrations(db); err != nil {
			return err
		}
		if err := migrateAuditEvents(db); err != nil {
			return err
		}
		if err := migratePermissionIndexes(db); err != nil {
			return err
		}
		if err := migrateJobsTable(db); err != nil {
			return err
		}
	}
	return nil
}

// schemaMigrationEntry records one applied migration in the schema_migrations table.
type schemaMigrationEntry struct {
	Version   string    `gorm:"column:version;primaryKey"`
	Checksum  string    `gorm:"column:checksum"`
	AppliedAt time.Time `gorm:"column:applied_at"`
}

// schemaMigrationTableName returns the qualified table name for schema
// migrations (app schema on Postgres, plain on SQLite).
func schemaMigrationTableName(db *gorm.DB) string {
	if db.Dialector.Name() == string(DriverPostgres) {
		return "app.schema_migrations"
	}
	return "schema_migrations"
}

// schemaMigrations is the registry of versioned, idempotent schema migrations.
// Each entry has a unique version, the SQL to apply, and its expected SHA-256
// checksum (computed at registration). Re-applying a migration with a
// different checksum indicates drift and is rejected.
var schemaMigrations = []struct {
	version  string
	checksum string
	sql      string
}{
	{
		version: "001_permission_index",
		checksum: func() string {
			sum := sha256.Sum256([]byte(permissionIndexSQL))
			return hex.EncodeToString(sum[:])
		}(),
		sql: permissionIndexSQL,
	},
	{
		version: "002_jobs_table",
		checksum: func() string {
			sum := sha256.Sum256([]byte(jobsTableSQL))
			return hex.EncodeToString(sum[:])
		}(),
		sql: jobsTableSQL,
	},
}

// permissionIndexSQL is the unique index required by permission seeds.
const permissionIndexSQL = `CREATE UNIQUE INDEX IF NOT EXISTS ix_iam_permissions_permission_code
	ON iam.permissions (permission_code)`

// jobsTableSQL is the durable job queue table (app.jobs on cco_app).
const jobsTableSQL = `CREATE TABLE IF NOT EXISTS app.jobs (
	id uuid PRIMARY KEY,
	job_type text NOT NULL,
	organization_id uuid,
	project_id uuid,
	status text NOT NULL DEFAULT 'queued',
	attempts int NOT NULL DEFAULT 0,
	max_attempts int NOT NULL DEFAULT 5,
	payload text,
	idempotency_key text UNIQUE,
	scheduled_at timestamptz,
	started_at timestamptz,
	finished_at timestamptz,
	last_error text,
	duration_ms bigint,
	created_at timestamptz NOT NULL DEFAULT now(),
	updated_at timestamptz NOT NULL DEFAULT NOW()
)`

// runSchemaMigrations applies versioned migrations idempotently. It creates
// the schema_migrations table, skips already-applied migrations whose checksum
// matches, and returns an error if a previously applied migration's checksum
// no longer matches (drift detection).
func runSchemaMigrations(db *gorm.DB) error {
	if err := db.Exec(fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			version    varchar(64) PRIMARY KEY,
			checksum   varchar(64) NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`,
		schemaMigrationTableName(db),
	)).Error; err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// Sort by version for deterministic application order.
	ordered := make([]struct {
		version  string
		checksum string
		sql      string
	}, len(schemaMigrations))
	copy(ordered, schemaMigrations)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].version < ordered[j].version
	})

	for _, m := range ordered {
		var existing string
		err := db.Raw(fmt.Sprintf(
			"SELECT checksum FROM %s WHERE version = ?",
			schemaMigrationTableName(db),
		), m.version).Scan(&existing).Error
		if err != nil {
			return fmt.Errorf("check migration %s: %w", m.version, err)
		}
		if existing != "" {
			if existing != m.checksum {
				return fmt.Errorf("schema drift detected: migration %s checksum mismatch (stored %q, expected %q)",
					m.version, existing, m.checksum)
			}
			continue // already applied, checksum matches
		}
		if err := db.Exec(m.sql).Error; err != nil {
			return fmt.Errorf("apply migration %s: %w", m.version, err)
		}
		if err := db.Exec(fmt.Sprintf(
			"INSERT INTO %s (version, checksum, applied_at) VALUES (?, ?, ?)",
			schemaMigrationTableName(db),
		), m.version, m.checksum, time.Now().UTC()).Error; err != nil {
			return fmt.Errorf("record migration %s: %w", m.version, err)
		}
	}
	return nil
}

// migrateAuditEvents creates audit-specific indexes and constraints.
func migrateAuditEvents(db *gorm.DB) error {
	statements := []struct {
		name string
		sql  string
	}{
		{
			name: "audit_actor_type_constraint",
			sql: `DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conname = 'ck_audit_actor_type'
			AND conrelid = 'audit.events'::regclass
	) THEN
		ALTER TABLE audit.events
			ADD CONSTRAINT ck_audit_actor_type
			CHECK (
				actor_type IN ('user', 'service_principal', 'managed_identity', 'system', 'anonymous')
				AND (actor_type <> 'system' OR user_id IS NULL)
			);
	END IF;
END;
$$;`,
		},
		{
			name: "audit_idx_correlation",
			sql: `CREATE INDEX IF NOT EXISTS idx_audit_correlation_time
				ON audit.events (correlation_id, occurred_at DESC)
				WHERE correlation_id IS NOT NULL`,
		},
		{
			name: "audit_idx_operation",
			sql: `CREATE INDEX IF NOT EXISTS idx_audit_operation_time
				ON audit.events (operation_id, occurred_at DESC)
				WHERE operation_id IS NOT NULL`,
		},
		{
			name: "audit_idx_recommendation",
			sql: `CREATE INDEX IF NOT EXISTS idx_audit_recommendation
				ON audit.events ((metadata->>'recommendation_uid'), occurred_at DESC)
				WHERE metadata ? 'recommendation_uid'`,
		},
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt.sql).Error; err != nil {
			return fmt.Errorf("migrate audit.events %s: %w", stmt.name, err)
		}
	}
	return nil
}

// migratePermissionIndexes creates idempotent permission indexes.
func migratePermissionIndexes(db *gorm.DB) error {
	statements := []struct {
		name string
		sql  string
	}{
		{
			name: "ix_iam_permissions_permission_code",
			sql: `CREATE UNIQUE INDEX IF NOT EXISTS ix_iam_permissions_permission_code
				ON iam.permissions (permission_code)`,
		},
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt.sql).Error; err != nil {
			return fmt.Errorf("migrate permission index %s: %w", stmt.name, err)
		}
	}
	return nil
}

// migrateJobsTable creates the durable job queue table (app.jobs on cco_app).
func migrateJobsTable(db *gorm.DB) error {
	statements := []struct {
		name string
		sql  string
	}{
		{
			name: "jobs_table",
			sql: `CREATE TABLE IF NOT EXISTS app.jobs (
				id uuid PRIMARY KEY,
				job_type text NOT NULL,
				organization_id uuid,
				project_id uuid,
				status text NOT NULL DEFAULT 'queued',
				attempts int NOT NULL DEFAULT 0,
				max_attempts int NOT NULL DEFAULT 5,
				payload text,
				idempotency_key text UNIQUE,
				scheduled_at timestamptz,
				started_at timestamptz,
				finished_at timestamptz,
				last_error text,
				duration_ms bigint,
				created_at timestamptz NOT NULL DEFAULT now(),
				updated_at timestamptz NOT NULL DEFAULT NOW()
			)`,
		},
		{
			name: "ix_jobs_status_scheduled",
			sql: `CREATE INDEX IF NOT EXISTS ix_app_jobs_status_scheduled
				ON app.jobs (status, scheduled_at) WHERE status = 'queued'`,
		},
		{
			name: "ix_jobs_type",
			sql: `CREATE INDEX IF NOT EXISTS ix_app_jobs_job_type
				ON app.jobs (job_type)`,
		},
		{
			name: "ix_jobs_org",
			sql: `CREATE INDEX IF NOT EXISTS ix_app_jobs_org
				ON app.jobs (organization_id) WHERE organization_id IS NOT NULL`,
		},
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt.sql).Error; err != nil {
			return fmt.Errorf("migrate jobs table %s: %w", stmt.name, err)
		}
	}
	return nil
}

// tablerName returns the table name for a GORM model, using the Tabler
// interface if the model implements it, otherwise falling back to the
// struct name.
func tablerName(model any) string {
	if tabler, ok := any(model).(schema.Tabler); ok {
		return tabler.TableName()
	}
	// Fallback: use the type name
	return fmt.Sprintf("%T", model)
}
