package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Models are persistence-only types. Domain objects and API DTOs must not
// embed them; this keeps GORM and database types at the infrastructure edge.
type UserModel struct {
	UserID        uuid.UUID  `gorm:"column:user_id;type:uuid;primaryKey"`
	Email         string     `gorm:"column:email;not null"`
	DisplayName   string     `gorm:"column:display_name;not null"`
	Status        string     `gorm:"column:status;not null;default:active"`
	EmailVerified bool       `gorm:"column:email_verified;not null;default:false"`
	LastLoginAt   *time.Time `gorm:"column:last_login_at"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;not null"`
}

func (UserModel) TableName() string { return "users" }

type ExternalIdentityModel struct {
	ExternalIdentityID uuid.UUID  `gorm:"column:external_identity_id;type:uuid;primaryKey"`
	UserID             uuid.UUID  `gorm:"column:user_id;type:uuid;not null"`
	Provider           string     `gorm:"column:provider;not null"`
	Subject            string     `gorm:"column:subject;not null"`
	CreatedAt          time.Time  `gorm:"column:created_at;not null"`
	LastSeenAt         *time.Time `gorm:"column:last_seen_at"`
}

func (ExternalIdentityModel) TableName() string { return "external_identities" }

type RoleModel struct {
	RoleID       int16     `gorm:"column:role_id;primaryKey"`
	RoleCode     string    `gorm:"column:role_code;not null"`
	Description  *string   `gorm:"column:description"`
	IsSystemRole bool      `gorm:"column:is_system_role;not null;default:true"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
}

func (RoleModel) TableName() string { return "roles" }

type PermissionModel struct {
	PermissionID   int16     `gorm:"column:permission_id;primaryKey"`
	PermissionCode string    `gorm:"column:permission_code;not null"`
	Description    *string   `gorm:"column:description"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (PermissionModel) TableName() string { return "permissions" }

type RolePermissionModel struct {
	RoleID       int16 `gorm:"column:role_id;primaryKey"`
	PermissionID int16 `gorm:"column:permission_id;primaryKey"`
}

func (RolePermissionModel) TableName() string { return "role_permissions" }

type UserRoleModel struct {
	UserID     uuid.UUID  `gorm:"column:user_id;type:uuid;primaryKey"`
	RoleID     int16      `gorm:"column:role_id;primaryKey"`
	AssignedAt time.Time  `gorm:"column:assigned_at;not null"`
	AssignedBy *uuid.UUID `gorm:"column:assigned_by;type:uuid"`
}

func (UserRoleModel) TableName() string { return "user_roles" }

type OrganizationModel struct {
	OrganizationID uuid.UUID `gorm:"column:organization_id;type:uuid;primaryKey"`
	Name           string    `gorm:"column:name;not null"`
	Slug           string    `gorm:"column:slug;not null"`
	Status         string    `gorm:"column:status;not null;default:active"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null"`
}

func (OrganizationModel) TableName() string { return "organizations" }

type MemberModel struct {
	OrganizationID uuid.UUID  `gorm:"column:organization_id;type:uuid;primaryKey"`
	UserID         uuid.UUID  `gorm:"column:user_id;type:uuid;primaryKey"`
	MembershipRole string     `gorm:"column:membership_role;not null;default:member"`
	Status         string     `gorm:"column:status;not null;default:active"`
	JoinedAt       time.Time  `gorm:"column:joined_at;not null"`
	InvitedBy      *uuid.UUID `gorm:"column:invited_by;type:uuid"`
}

func (MemberModel) TableName() string { return "members" }

type ProjectModel struct {
	ProjectID      uuid.UUID  `gorm:"column:project_id;type:uuid;primaryKey"`
	OrganizationID uuid.UUID  `gorm:"column:organization_id;type:uuid;not null"`
	Name           string     `gorm:"column:name;not null"`
	Description    *string    `gorm:"column:description"`
	Status         string     `gorm:"column:status;not null;default:active"`
	CreatedBy      *uuid.UUID `gorm:"column:created_by;type:uuid"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null"`
}

func (ProjectModel) TableName() string { return "projects" }

type CloudAccountModel struct {
	CloudAccountID      uuid.UUID  `gorm:"column:cloud_account_id;type:uuid;primaryKey"`
	OrganizationID      uuid.UUID  `gorm:"column:organization_id;type:uuid;not null"`
	ProviderCode        string     `gorm:"column:provider_code;not null"`
	DisplayName         string     `gorm:"column:display_name;not null"`
	SubscriptionID      string     `gorm:"column:subscription_id;not null"`
	TenantID            *string    `gorm:"column:tenant_id"`
	CredentialReference *string    `gorm:"column:credential_reference"`
	Status              string     `gorm:"column:status;not null;default:pending"`
	CreatedBy           *uuid.UUID `gorm:"column:created_by;type:uuid"`
	CreatedAt           time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;not null"`
}

func (CloudAccountModel) TableName() string { return "accounts" }

type AccountProjectModel struct {
	CloudAccountID uuid.UUID `gorm:"column:cloud_account_id;type:uuid;primaryKey"`
	ProjectID      uuid.UUID `gorm:"column:project_id;type:uuid;primaryKey"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (AccountProjectModel) TableName() string { return "account_projects" }

type ResourceBindingModel struct {
	ResourceBindingID uuid.UUID `gorm:"column:resource_binding_id;type:uuid;primaryKey"`
	ProjectID         uuid.UUID `gorm:"column:project_id;type:uuid;not null"`
	CloudAccountID    uuid.UUID `gorm:"column:cloud_account_id;type:uuid;not null"`
	DataResourceUID   uuid.UUID `gorm:"column:data_resource_uid;type:uuid;not null"`
	DisplayName       *string   `gorm:"column:display_name"`
	IsMonitored       bool      `gorm:"column:is_monitored;not null;default:true"`
	CreatedAt         time.Time `gorm:"column:created_at;not null"`
	UpdatedAt         time.Time `gorm:"column:updated_at;not null"`
}

func (ResourceBindingModel) TableName() string { return "resource_bindings" }

type DashboardPreferenceModel struct {
	DashboardPreferenceID uuid.UUID      `gorm:"column:dashboard_preference_id;type:uuid;primaryKey"`
	UserID                uuid.UUID      `gorm:"column:user_id;type:uuid;not null"`
	ProjectID             *uuid.UUID     `gorm:"column:project_id;type:uuid"`
	Layout                datatypes.JSON `gorm:"column:layout;not null"`
	Widgets               datatypes.JSON `gorm:"column:widgets;not null"`
	DefaultPeriod         string         `gorm:"column:default_period;not null;default:30d"`
	CreatedAt             time.Time      `gorm:"column:created_at;not null"`
	UpdatedAt             time.Time      `gorm:"column:updated_at;not null"`
}

func (DashboardPreferenceModel) TableName() string { return "dashboard_preferences" }

type NotificationModel struct {
	NotificationID uuid.UUID  `gorm:"column:notification_id;type:uuid;primaryKey"`
	UserID         uuid.UUID  `gorm:"column:user_id;type:uuid;not null"`
	OrganizationID *uuid.UUID `gorm:"column:organization_id;type:uuid"`
	Type           string     `gorm:"column:type;not null"`
	Title          string     `gorm:"column:title;not null"`
	Message        string     `gorm:"column:message;not null"`
	Severity       string     `gorm:"column:severity;not null;default:info"`
	ReferenceType  *string    `gorm:"column:reference_type"`
	ReferenceID    *uuid.UUID `gorm:"column:reference_id;type:uuid"`
	ReadAt         *time.Time `gorm:"column:read_at"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	ExpiresAt      *time.Time `gorm:"column:expires_at"`
}

func (NotificationModel) TableName() string { return "notifications" }

type NotificationDeliveryModel struct {
	NotificationDeliveryID uuid.UUID  `gorm:"column:notification_delivery_id;type:uuid;primaryKey"`
	NotificationID         uuid.UUID  `gorm:"column:notification_id;type:uuid;not null"`
	Channel                string     `gorm:"column:channel;not null"`
	Status                 string     `gorm:"column:status;not null;default:pending"`
	ProviderMessageID      *string    `gorm:"column:provider_message_id"`
	Attempts               int16      `gorm:"column:attempts;not null;default:0"`
	LastAttemptAt          *time.Time `gorm:"column:last_attempt_at"`
	DeliveredAt            *time.Time `gorm:"column:delivered_at"`
	FailureReason          *string    `gorm:"column:failure_reason"`
}

func (NotificationDeliveryModel) TableName() string { return "notification_deliveries" }

type SavedViewModel struct {
	SavedViewID     uuid.UUID      `gorm:"column:saved_view_id;type:uuid;primaryKey"`
	UserID          uuid.UUID      `gorm:"column:user_id;type:uuid;not null"`
	ProjectID       *uuid.UUID     `gorm:"column:project_id;type:uuid"`
	Name            string         `gorm:"column:name;not null"`
	ResourceFilters datatypes.JSON `gorm:"column:resource_filters;not null"`
	MetricFilters   datatypes.JSON `gorm:"column:metric_filters;not null"`
	CreatedAt       time.Time      `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;not null"`
}

func (SavedViewModel) TableName() string { return "saved_views" }

type UserPreferenceModel struct {
	UserID               uuid.UUID      `gorm:"column:user_id;type:uuid;primaryKey"`
	Timezone             string         `gorm:"column:timezone;not null;default:UTC"`
	Locale               string         `gorm:"column:locale;not null;default:es-CO"`
	Currency             string         `gorm:"column:currency;not null;default:USD"`
	Theme                string         `gorm:"column:theme;not null;default:system"`
	NotificationSettings datatypes.JSON `gorm:"column:notification_settings;not null"`
	CreatedAt            time.Time      `gorm:"column:created_at;not null"`
	UpdatedAt            time.Time      `gorm:"column:updated_at;not null"`
}

func (UserPreferenceModel) TableName() string { return "user_preferences" }

type RecommendationInteractionModel struct {
	RecommendationInteractionID uuid.UUID `gorm:"column:recommendation_interaction_id;type:uuid;primaryKey"`
	UserID                      uuid.UUID `gorm:"column:user_id;type:uuid;not null"`
	ProjectID                   uuid.UUID `gorm:"column:project_id;type:uuid;not null"`
	RecommendationUID           uuid.UUID `gorm:"column:recommendation_uid;type:uuid;not null"`
	Action                      string    `gorm:"column:action;not null"`
	Comment                     *string   `gorm:"column:comment"`
	CreatedAt                   time.Time `gorm:"column:created_at;not null"`
}

func (RecommendationInteractionModel) TableName() string { return "recommendation_interactions" }

type ReportRequestModel struct {
	ReportRequestID uuid.UUID      `gorm:"column:report_request_id;type:uuid;primaryKey"`
	ProjectID       uuid.UUID      `gorm:"column:project_id;type:uuid;not null"`
	RequestedBy     uuid.UUID      `gorm:"column:requested_by;type:uuid;not null"`
	ReportType      string         `gorm:"column:report_type;not null"`
	Parameters      datatypes.JSON `gorm:"column:parameters;not null"`
	Status          string         `gorm:"column:status;not null;default:queued"`
	CreatedAt       time.Time      `gorm:"column:created_at;not null"`
	StartedAt       *time.Time     `gorm:"column:started_at"`
	CompletedAt     *time.Time     `gorm:"column:completed_at"`
	ErrorMessage    *string        `gorm:"column:error_message"`
}

func (ReportRequestModel) TableName() string { return "report_requests" }

type ReportArtifactModel struct {
	ReportArtifactID uuid.UUID  `gorm:"column:report_artifact_id;type:uuid;primaryKey"`
	ReportRequestID  uuid.UUID  `gorm:"column:report_request_id;type:uuid;not null"`
	StorageURI       string     `gorm:"column:storage_uri;not null"`
	MimeType         string     `gorm:"column:mime_type;not null"`
	FileSizeBytes    *int64     `gorm:"column:file_size_bytes"`
	ChecksumSHA256   *string    `gorm:"column:checksum_sha256"`
	CreatedAt        time.Time  `gorm:"column:created_at;not null"`
	ExpiresAt        *time.Time `gorm:"column:expires_at"`
}

func (ReportArtifactModel) TableName() string { return "report_artifacts" }

type AuditEventModel struct {
	EventID        uint64         `gorm:"column:event_id;primaryKey;autoIncrement"`
	OrganizationID *uuid.UUID     `gorm:"column:organization_id;type:uuid"`
	UserID         *uuid.UUID     `gorm:"column:user_id;type:uuid"`
	Action         string         `gorm:"column:action;not null"`
	EntityType     *string        `gorm:"column:entity_type"`
	EntityID       *uuid.UUID     `gorm:"column:entity_id;type:uuid"`
	OccurredAt     time.Time      `gorm:"column:occurred_at;not null"`
	RequestID      *uuid.UUID     `gorm:"column:request_id;type:uuid"`
	IPAddress      *string        `gorm:"column:ip_address"`
	UserAgent      *string        `gorm:"column:user_agent"`
	Metadata       datatypes.JSON `gorm:"column:metadata;not null"`
}

func (AuditEventModel) TableName() string { return "events" }
