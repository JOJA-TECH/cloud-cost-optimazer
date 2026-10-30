package adapters

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

// ServicePrincipalAdapter implements AuthenticationProvider for client_credentials flow.
// Used by collectors, ML service, and other machine-to-machine callers.
type ServicePrincipalAdapter struct {
	tokenManager  *iam.TokenManager
	repo          iam.IdentityRepository
	validClients  map[string]string   // client_id -> client_secret_hash (argon2id)
	allowedScopes map[string][]string // client_id -> allowed scopes
}

// SPConfig holds service principal configuration.
type SPConfig struct {
	TokenManager  *iam.TokenManager
	Repo          iam.IdentityRepository
	Clients       map[string]string // client_id -> client_secret_hash
	AllowedScopes map[string][]string
}

// NewServicePrincipalAdapter creates a new ServicePrincipalAdapter.
func NewServicePrincipalAdapter(cfg SPConfig) *ServicePrincipalAdapter {
	if cfg.TokenManager == nil {
		panic("token manager required")
	}
	if cfg.Repo == nil {
		panic("identity repository required")
	}
	return &ServicePrincipalAdapter{
		tokenManager:  cfg.TokenManager,
		repo:          cfg.Repo,
		validClients:  cfg.Clients,
		allowedScopes: cfg.AllowedScopes,
	}
}

// Authenticate validates client_credentials and returns IdentityContext.
// Implements AuthenticationProvider for "client_credentials" credential type.
func (a *ServicePrincipalAdapter) Authenticate(ctx context.Context, cred iam.Credentials) (iam.IdentityContext, error) {
	if cred.Type != "client_credentials" {
		return iam.IdentityContext{}, iam.ErrProviderNotSupported
	}
	if cred.ClientID == "" || cred.ClientSecret == "" {
		return iam.IdentityContext{}, iam.ErrInvalidCredentials
	}

	// Verify client secret
	expectedHash, ok := a.validClients[cred.ClientID]
	if !ok {
		return iam.IdentityContext{}, iam.ErrSPValidationFailed
	}

	// Direct compare (secrets should be hashed in config for production)
	if expectedHash != cred.ClientSecret {
		return iam.IdentityContext{}, iam.ErrSPValidationFailed
	}

	// Validate scope
	if cred.Scope != "" {
		allowed, ok := a.allowedScopes[cred.ClientID]
		if ok {
			requested := strings.Split(cred.Scope, " ")
			for _, r := range requested {
				found := false
				for _, s := range allowed {
					if s == r || s == "*" {
						found = true
						break
					}
				}
				if !found {
					return iam.IdentityContext{}, iam.ErrInsufficientScope
				}
			}
		}
	}

	// Determine system role from client_id or scope
	systemRoles := rolesForClient(cred.ClientID, cred.Scope)

	// Create or get service identity user
	user, err := a.getOrCreateServiceUser(ctx, cred.ClientID, systemRoles)
	if err != nil {
		return iam.IdentityContext{}, err
	}

	ic := iam.IdentityContext{
		UserID:         user.ID,
		Email:          fmt.Sprintf("%s@service.cco", cred.ClientID),
		DisplayName:    fmt.Sprintf("Service Principal: %s", cred.ClientID),
		AuthMethod:     "service_principal",
		SystemRoles:    systemRoles,
		OrgID:          uuid.Nil,
		OrgRole:        "viewer",
		ProjectID:      nil,
		ProjectRole:    nil,
		Permissions:    permissionsForRoles(systemRoles),
		RequestID:      uuid.New().String(),
		CorrelationID:  uuid.New().String(),
		TokenExpiresAt: time.Now().Add(a.tokenManager.AccessTokenTTL()).UnixMilli(),
	}

	return ic, nil
}

// Validate validates an access token.
// Implements TokenValidator (delegates to token manager).
func (a *ServicePrincipalAdapter) Validate(ctx context.Context, token string) (iam.IdentityContext, error) {
	return a.tokenManager.ValidateAccessToken(token)
}

func rolesForClient(clientID, scope string) []string {
	switch clientID {
	case "cco-collector":
		return []string{"collector"}
	case "cco-ml-inference":
		return []string{"ml_inference"}
	default:
		if strings.Contains(scope, "metrics:write") || strings.Contains(scope, "config:read") {
			return []string{"collector"}
		}
		if strings.Contains(scope, "features:read") || strings.Contains(scope, "forecasts:write") {
			return []string{"ml_inference"}
		}
		return []string{"viewer"}
	}
}

func permissionsForRoles(roles []string) []string {
	var perms []string
	for _, role := range roles {
		switch role {
		case "collector":
			perms = append(perms, "metrics:write", "config:read", "resources:read")
		case "ml_inference":
			perms = append(perms, "features:read", "forecasts:write", "models:read")
		}
	}
	return perms
}

func (a *ServicePrincipalAdapter) getOrCreateServiceUser(ctx context.Context, clientID string, roles []string) (*serviceUser, error) {
	email := fmt.Sprintf("%s@service.cco", clientID)
	user, err := a.repo.FindUserByEmail(ctx, email)
	if err == nil {
		return &serviceUser{ID: user.ID, SystemRoles: roles}, nil
	}

	created, err := a.repo.CreateUser(ctx, iam.CreateUserInput{
		Email:        email,
		DisplayName:  fmt.Sprintf("Service Principal: %s", clientID),
		PasswordHash: nil,
	})
	if err != nil {
		return nil, err
	}

	for _, roleCode := range roles {
		role, err := a.repo.FindRoleByCode(ctx, roleCode)
		if err == nil {
			_ = a.repo.AssignRole(ctx, created.ID, role.RoleID, nil)
		}
	}

	return &serviceUser{ID: created.ID, SystemRoles: roles}, nil
}

type serviceUser struct {
	ID          uuid.UUID
	SystemRoles []string
}
