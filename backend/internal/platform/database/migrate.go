package database

import (
	"fmt"

	"gorm.io/gorm"
)

// Migrate creates the cco_app persistence model through GORM and applies
// PostgreSQL-specific audit constraints, indexes, policies, and triggers.
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
			if err := db.Table(TableName(db, group.schema, modelTable(model))).AutoMigrate(model); err != nil {
				return fmt.Errorf("migrate %s.%s: %w", group.schema, modelTable(model), err)
			}
		}
	}
	if db.Dialector.Name() == string(DriverPostgres) {
		if err := migrateAuditEvents(db); err != nil {
			return err
		}
		if err := migratePermissionIndexes(db); err != nil {
			return err
		}
	}
	return nil
}

// migratePermissionIndexes crea los índices únicos que requieren los seeds
// de permisos (ON CONFLICT permission_code). Es idempotente: seguro ejecutar
// sobre bases de datos ya desplegadas antes de que existiera el índice.
func migratePermissionIndexes(db *gorm.DB) error {
	statements := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS ix_iam_permissions_permission_code
			ON iam.permissions (permission_code)`,
	}
	for _, sql := range statements {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("create permission index: %w", err)
		}
	}
	return nil
}

func migrateAuditEvents(db *gorm.DB) error {
	statements := []struct {
		name string
		sql  string
	}{
		{
			name: "event action constraint",
			sql: `DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conname = 'ck_audit_action_not_blank'
			AND conrelid = 'audit.events'::regclass
	) THEN
		ALTER TABLE audit.events
			ADD CONSTRAINT ck_audit_action_not_blank
			CHECK (length(trim(action)) > 0);
	END IF;
END;
$$;`,
		},
		{
			name: "actor type constraint",
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
			name: "correlation index",
			sql: `CREATE INDEX IF NOT EXISTS ix_audit_correlation_time
				ON audit.events (correlation_id, occurred_at DESC)
				WHERE correlation_id IS NOT NULL`,
		},
		{
			name: "operation index",
			sql: `CREATE INDEX IF NOT EXISTS ix_audit_operation_time
				ON audit.events (operation_id, occurred_at DESC)
				WHERE operation_id IS NOT NULL`,
		},
		{
			name: "recommendation index",
			sql: `CREATE INDEX IF NOT EXISTS ix_audit_recommendation
				ON audit.events ((metadata->>'recommendation_uid'), occurred_at DESC)
				WHERE metadata ? 'recommendation_uid'`,
		},
		{
			name: "append-only trigger function",
			sql: `CREATE OR REPLACE FUNCTION audit.reject_event_mutation() RETURNS trigger
				LANGUAGE plpgsql
				AS $$
				BEGIN
					RAISE EXCEPTION 'audit.events is append-only';
					RETURN NULL;
				END;
				$$`,
		},
		{
			name: "append-only row triggers",
			sql: `DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_trigger
		WHERE tgname = 'trg_audit_events_no_row_mutation'
			AND tgrelid = 'audit.events'::regclass
			AND NOT tgisinternal
	) THEN
		CREATE TRIGGER trg_audit_events_no_row_mutation
			BEFORE UPDATE OR DELETE ON audit.events
			FOR EACH ROW EXECUTE FUNCTION audit.reject_event_mutation();
	END IF;
	IF NOT EXISTS (
		SELECT 1 FROM pg_trigger
		WHERE tgname = 'trg_audit_events_no_truncate'
			AND tgrelid = 'audit.events'::regclass
			AND NOT tgisinternal
	) THEN
		CREATE TRIGGER trg_audit_events_no_truncate
			BEFORE TRUNCATE ON audit.events
			FOR EACH STATEMENT EXECUTE FUNCTION audit.reject_event_mutation();
	END IF;
END;
$$;`,
		},
		{
			name: "system event insert policy",
			sql: `DO $$
BEGIN
	IF EXISTS (
		SELECT 1 FROM pg_policies
		WHERE schemaname = 'audit'
			AND tablename = 'events'
			AND policyname = 'audit_insert_own_context'
	) THEN
		ALTER POLICY audit_insert_own_context ON audit.events
			WITH CHECK (
				(
					actor_type = 'system'
					AND user_id IS NULL
					AND app.is_platform_admin()
				)
				OR (
					actor_type <> 'system'
					AND (
						app.is_platform_admin()
						OR (
							user_id = app.current_user_id()
							AND (organization_id IS NULL OR app.is_org_member(organization_id))
						)
					)
				)
			);
	END IF;
END;
$$;`,
		},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.sql).Error; err != nil {
			return fmt.Errorf("migrate audit.events %s: %w", statement.name, err)
		}
	}
	return nil
}

func modelTable(model any) string {
	switch model.(type) {
	case *UserModel:
		return "users"
	case *ExternalIdentityModel:
		return "external_identities"
	case *RoleModel:
		return "roles"
	case *PermissionModel:
		return "permissions"
	case *RolePermissionModel:
		return "role_permissions"
	case *UserRoleModel:
		return "user_roles"
	case *RefreshTokenModel:
		return "refresh_tokens"
	case *OrganizationModel:
		return "organizations"
	case *MemberModel:
		return "members"
	case *ProjectModel:
		return "projects"
	case *CloudAccountModel:
		return "accounts"
	case *AccountProjectModel:
		return "account_projects"
	case *ResourceBindingModel:
		return "resource_bindings"
	case *ResourceModel:
		return "resources"
	case *DashboardPreferenceModel:
		return "dashboard_preferences"
	case *NotificationModel:
		return "notifications"
	case *NotificationDeliveryModel:
		return "notification_deliveries"
	case *SavedViewModel:
		return "saved_views"
	case *UserPreferenceModel:
		return "user_preferences"
	case *RecommendationInteractionModel:
		return "recommendation_interactions"
	case *ReportRequestModel:
		return "report_requests"
	case *ReportArtifactModel:
		return "report_artifacts"
	case *AuditEventModel:
		return "events"
	default:
		panic("unknown persistence model")
	}
}
