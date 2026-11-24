package cloud

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const ProviderAzure = "azure"

// ConnectionCredentials holds resolved credentials for talking to a provider.
// The secret is resolved from the environment via the account's
// credential_reference (format "env:VAR_NAME" or bare "VAR_NAME").
type ConnectionCredentials struct {
	TenantID       string
	ClientID       string
	ClientSecret   string
	SubscriptionID string
}

// ConnectionResult reports the outcome of a connection validation.
type ConnectionResult struct {
	Status string // "active" | "error"
	Detail string // human-readable detail on failure; must never contain the secret
}

// ResourceDTO is a normalized cloud resource discovered by a provider.
// Columns mirror databases/cco_data/cco_data_schema.sql core.resource so the
// Python analytical module can map rows 1:1 via resource_uid (contract UUID).
type ResourceDTO struct {
	ResourceUID     uuid.UUID
	CloudAccountID  uuid.UUID
	ProviderCode    string
	SubscriptionID  string
	ResourceGroup   string
	ServerName      string
	DatabaseName    string
	AzureResourceID string // full ARM resource ID, natural identity
	RegionCode      string
	Kind            *string // e.g. "v12.0,user,vcore,serverless,freelimit"
	LifecycleStatus string  // "active"
	LastSeenAt      time.Time
}

// ConnectionValidator verifies stored credentials against the provider.
type ConnectionValidator interface {
	ValidateConnection(ctx context.Context, creds ConnectionCredentials) (ConnectionResult, error)
}

// ResourceProvider discovers normalized resources. First iteration: Azure SQL only.
type ResourceProvider interface {
	ListSQLDatabases(ctx context.Context, creds ConnectionCredentials) ([]ResourceDTO, error)
}

// AccountDTO mirrors cloud.accounts.
type AccountDTO struct {
	CloudAccountID      uuid.UUID `json:"cloud_account_id"`
	OrganizationID      uuid.UUID `json:"organization_id"`
	ProviderCode        string    `json:"provider_code"`
	DisplayName         string    `json:"display_name"`
	SubscriptionID      string    `json:"subscription_id"`
	TenantID            string    `json:"tenant_id"`
	ClientID            string    `json:"client_id"`
	CredentialReference string    `json:"credential_reference"`
	Status              string    `json:"status"`
	LastConnectionError *string   `json:"last_connection_error,omitempty"`
	CreatedAt           int64     `json:"created_at"` // unix ms
	UpdatedAt           int64     `json:"updated_at"` // unix ms
}

// ResourceDTO json tags for API responses.
type ResourceAPIDTO struct {
	ResourceUID     uuid.UUID `json:"resource_uid"`
	CloudAccountID  uuid.UUID `json:"cloud_account_id"`
	ProviderCode    string    `json:"provider_code"`
	SubscriptionID  string    `json:"subscription_id"`
	ResourceGroup   string    `json:"resource_group"`
	ServerName      string    `json:"server_name"`
	DatabaseName    string    `json:"database_name"`
	AzureResourceID string    `json:"azure_resource_id"`
	RegionCode      string    `json:"region_code"`
	Kind            *string   `json:"kind"`
	LifecycleStatus string    `json:"lifecycle_status"`
	LastSeenAt      int64     `json:"last_seen_at"` // unix ms
}

// BindingDTO mirrors cloud.resource_bindings.
type BindingDTO struct {
	ResourceBindingID uuid.UUID `json:"resource_binding_id"`
	ProjectID         uuid.UUID `json:"project_id"`
	CloudAccountID    uuid.UUID `json:"cloud_account_id"`
	DataResourceUID   uuid.UUID `json:"data_resource_uid"`
	DisplayName       string    `json:"display_name"`
	IsMonitored       bool      `json:"is_monitored"`
	CreatedAt         int64     `json:"created_at"` // unix ms
	UpdatedAt         int64     `json:"updated_at"` // unix ms
}

type CreateAccountInput struct {
	OrgID               uuid.UUID
	UserID              uuid.UUID
	ProviderCode        string
	DisplayName         string
	SubscriptionID      string
	TenantID            string
	ClientID            *string
	CredentialReference string
}

type CreateBindingInput struct {
	ProjectID       uuid.UUID
	CloudAccountID  uuid.UUID
	DataResourceUID uuid.UUID
	DisplayName     string
}

type AccountRepository interface {
	CreateAccount(ctx context.Context, input CreateAccountInput) (AccountDTO, error)
	ListAccounts(ctx context.Context, orgID uuid.UUID) ([]AccountDTO, error)
	GetAccount(ctx context.Context, id uuid.UUID) (AccountDTO, error)
	UpdateAccountStatus(ctx context.Context, id uuid.UUID, status string, connectionError *string) error
}

type ResourceRepository interface {
	UpsertResources(ctx context.Context, accountID uuid.UUID, resources []ResourceDTO) ([]ResourceDTO, error)
	ListResources(ctx context.Context, accountID uuid.UUID) ([]ResourceDTO, error)
	GetResource(ctx context.Context, accountID, resourceUID uuid.UUID) (ResourceDTO, error)
}

type DataSourceRepository interface {
	CreateBinding(ctx context.Context, input CreateBindingInput) (BindingDTO, error)
	GetBinding(ctx context.Context, id uuid.UUID) (BindingDTO, error)
	ListBindings(ctx context.Context, projectID *uuid.UUID) ([]BindingDTO, error)
	SetBindingMonitored(ctx context.Context, id uuid.UUID, monitored bool) error
	ProjectOrgID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error)
}
