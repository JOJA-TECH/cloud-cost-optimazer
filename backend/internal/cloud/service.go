package cloud

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service coordinates cloud account, resource, and data source operations.
type Service struct {
	accounts AccountRepository
	resources ResourceRepository
	bindings DataSourceRepository
	validator ConnectionValidator
	provider ResourceProvider
}

// NewService constructs a Service.
func NewService(accounts AccountRepository, resources ResourceRepository, bindings DataSourceRepository, validator ConnectionValidator, provider ResourceProvider) *Service {
	return &Service{
		accounts: accounts,
		resources: resources,
		bindings: bindings,
		validator: validator,
		provider: provider,
	}
}

// RegisterAccount creates a cloud account in pending status.
func (s *Service) RegisterAccount(ctx context.Context, orgID, userID uuid.UUID, input CreateAccountInput) (AccountDTO, error) {
	if input.ProviderCode != ProviderAzure {
		return AccountDTO{}, NewError(CodeInvalidRequest, "provider_code must be azure", ErrInvalidInput)
	}
	if input.DisplayName == "" || input.SubscriptionID == "" || input.TenantID == "" {
		return AccountDTO{}, NewError(CodeInvalidRequest, "display_name, subscription_id, tenant_id are required", ErrInvalidInput)
	}
	input.OrgID = orgID
	input.UserID = userID
	return s.accounts.CreateAccount(ctx, input)
}

// ListAccounts returns all accounts for an organization.
func (s *Service) ListAccounts(ctx context.Context, orgID uuid.UUID) ([]AccountDTO, error) {
	return s.accounts.ListAccounts(ctx, orgID)
}

// GetAccount returns a single account. 404 semantics on org mismatch.
func (s *Service) GetAccount(ctx context.Context, orgID, id uuid.UUID) (AccountDTO, error) {
	acc, err := s.accounts.GetAccount(ctx, id)
	if err != nil {
		return AccountDTO{}, err
	}
	if acc.OrganizationID != orgID {
		return AccountDTO{}, ErrAccountNotFound
	}
	return acc, nil
}

// ValidateAccount validates account credentials against the provider and updates status.
// CU-01: never include the resolved secret in returned errors.
func (s *Service) ValidateAccount(ctx context.Context, orgID, id uuid.UUID) (AccountDTO, error) {
	acc, err := s.accounts.GetAccount(ctx, id)
	if err != nil {
		return AccountDTO{}, err
	}
	if acc.OrganizationID != orgID {
		return AccountDTO{}, ErrAccountNotFound
	}
	creds, err := resolveCredentials(acc)
	if err != nil {
		s.accounts.UpdateAccountStatus(ctx, id, "error", strPtr(err.Error()))
		return s.accounts.GetAccount(ctx, id)
	}
	result, err := s.validator.ValidateConnection(ctx, creds)
	if err != nil {
		s.accounts.UpdateAccountStatus(ctx, id, "error", strPtr(err.Error()))
		return s.accounts.GetAccount(ctx, id)
	}
	status := "active"
	var connErr *string
	if result.Status != "active" {
		status = "error"
		connErr = &result.Detail
	}
	if err := s.accounts.UpdateAccountStatus(ctx, id, status, connErr); err != nil {
		return AccountDTO{}, err
	}
	return s.accounts.GetAccount(ctx, id)
}

// SyncResources discovers resources from the provider and upserts them.
func (s *Service) SyncResources(ctx context.Context, orgID, id uuid.UUID) ([]ResourceDTO, error) {
	acc, err := s.accounts.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	if acc.OrganizationID != orgID {
		return nil, ErrAccountNotFound
	}
	creds, err := resolveCredentials(acc)
	if err != nil {
		return nil, err
	}
	discovered, err := s.provider.ListSQLDatabases(ctx, creds)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range discovered {
		discovered[i].CloudAccountID = acc.CloudAccountID
		discovered[i].ProviderCode = acc.ProviderCode
		discovered[i].SubscriptionID = acc.SubscriptionID
		discovered[i].LifecycleStatus = "active"
		discovered[i].LastSeenAt = now
	}
	return s.resources.UpsertResources(ctx, acc.CloudAccountID, discovered)
}

// ListResources returns resources for an account. 404 on org mismatch.
func (s *Service) ListResources(ctx context.Context, orgID, accountID uuid.UUID) ([]ResourceDTO, error) {
	acc, err := s.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if acc.OrganizationID != orgID {
		return nil, ErrAccountNotFound
	}
	return s.resources.ListResources(ctx, accountID)
}

// RegisterDataSource binds a discovered resource to a project.
func (s *Service) RegisterDataSource(ctx context.Context, orgID uuid.UUID, input CreateBindingInput) (BindingDTO, error) {
	acc, err := s.accounts.GetAccount(ctx, input.CloudAccountID)
	if err != nil {
		return BindingDTO{}, err
	}
	if acc.OrganizationID != orgID {
		return BindingDTO{}, ErrAccountNotFound
	}
	// The target project must belong to the same organization.
	if projOrgID, err := s.bindings.ProjectOrgID(ctx, input.ProjectID); err != nil {
		return BindingDTO{}, err
	} else if projOrgID != orgID {
		return BindingDTO{}, ErrProjectNotFound
	}
	if _, err := s.resources.GetResource(ctx, input.CloudAccountID, input.DataResourceUID); err != nil {
		return BindingDTO{}, err
	}
	return s.bindings.CreateBinding(ctx, input)
}

// ListDataSources returns bindings, optionally filtered by project, scoped to the caller's org.
func (s *Service) ListDataSources(ctx context.Context, orgID uuid.UUID, projectID *uuid.UUID) ([]BindingDTO, error) {
	if projectID != nil {
		if projOrgID, err := s.bindings.ProjectOrgID(ctx, *projectID); err != nil {
			return nil, err
		} else if projOrgID != orgID {
			return nil, ErrProjectNotFound
		}
		return s.bindings.ListBindings(ctx, projectID)
	}
	// No project filter: scope to the caller's organization via its accounts.
	accs, err := s.accounts.ListAccounts(ctx, orgID)
	if err != nil {
		return nil, err
	}
	accountIDs := make(map[uuid.UUID]bool, len(accs))
	for _, a := range accs {
		accountIDs[a.CloudAccountID] = true
	}
	bindings, err := s.bindings.ListBindings(ctx, nil)
	if err != nil {
		return nil, err
	}
	scoped := make([]BindingDTO, 0, len(bindings))
	for _, b := range bindings {
		if accountIDs[b.CloudAccountID] {
			scoped = append(scoped, b)
		}
	}
	return scoped, nil
}

// SetDataSourceMonitored toggles the monitored flag on a binding.
func (s *Service) SetDataSourceMonitored(ctx context.Context, orgID, id uuid.UUID, monitored bool) error {
	binding, err := s.bindings.GetBinding(ctx, id)
	if err != nil {
		return err
	}
	acc, err := s.accounts.GetAccount(ctx, binding.CloudAccountID)
	if err != nil {
		return err
	}
	if acc.OrganizationID != orgID {
		return ErrBindingNotFound
	}
	return s.bindings.SetBindingMonitored(ctx, id, monitored)
}

// ResolveSecretRef resolves a credential_reference ("env:VAR" or bare "VAR")
// to a secret taken exclusively from the environment. It never logs or
// persists the secret. Shared by the cloud and acquisition modules.
func ResolveSecretRef(ref string) (string, error) {
	var varName string
	if strings.HasPrefix(ref, "env:") {
		varName = ref[4:]
	} else {
		varName = ref
	}
	if varName == "" {
		return "", ErrCredentialsUnavailable
	}
	secret, ok := os.LookupEnv(varName)
	if !ok || secret == "" {
		return "", ErrCredentialsUnavailable
	}
	return secret, nil
}

// resolveCredentials resolves ConnectionCredentials from an account DTO.
// The secret comes only from the environment via credential_reference.
// shortcut: secrets only from env this iteration; upgrade: Key Vault / Managed Identity (RNF-02).
func resolveCredentials(a AccountDTO) (ConnectionCredentials, error) {
	creds := ConnectionCredentials{
		TenantID:       a.TenantID,
		ClientID:       a.ClientID,
		SubscriptionID: a.SubscriptionID,
	}
	if creds.TenantID == "" {
		return ConnectionCredentials{}, ErrCredentialsUnavailable
	}
	secret, err := ResolveSecretRef(a.CredentialReference)
	if err != nil {
		return ConnectionCredentials{}, err
	}
	creds.ClientSecret = secret
	return creds, nil
}
