package database

import (
	"fmt"

	"gorm.io/gorm"
)

// Migrate creates the cco_app persistence model through GORM. The PostgreSQL
// schema creation is the only dialect-specific operation and is isolated here;
// all tables and queries use the same models on both drivers.
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
		{"iam", []any{&UserModel{}, &ExternalIdentityModel{}, &RoleModel{}, &PermissionModel{}, &RolePermissionModel{}, &UserRoleModel{}}},
		{"organization", []any{&OrganizationModel{}, &MemberModel{}, &ProjectModel{}}},
		{"cloud", []any{&CloudAccountModel{}, &AccountProjectModel{}, &ResourceBindingModel{}}},
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
