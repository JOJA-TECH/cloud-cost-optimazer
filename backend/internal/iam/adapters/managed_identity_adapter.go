package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ManagedIdentityAdapter implements AuthenticationProvider for Azure Managed Identity.
// Validates MI tokens from Azure IMDS or Azure SDK.
type ManagedIdentityAdapter struct {
	tokenManager     *iam.TokenManager
	repo             iam.IdentityRepository
	tenantID         string
	allowedClientIDs map[string][]string // client_id -> allowed roles
	httpClient       *http.Client
}

// MIConfig holds managed identity configuration.
type MIConfig struct {
	TokenManager     *iam.TokenManager
	Repo             iam.IdentityRepository
	TenantID         string
	AllowedClientIDs map[string][]string // client_id -> roles
	HTTPClient       *http.Client
}

// NewManagedIdentityAdapter creates a new ManagedIdentityAdapter.
func NewManagedIdentityAdapter(cfg MIConfig) *ManagedIdentityAdapter {
	if cfg.TokenManager == nil {
		panic("token manager required")
	}
	if cfg.Repo == nil {
		panic("identity repository required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &ManagedIdentityAdapter{
		tokenManager:     cfg.TokenManager,
		repo:             cfg.Repo,
		tenantID:         cfg.TenantID,
		allowedClientIDs: cfg.AllowedClientIDs,
		httpClient:       client,
	}
}

// Authenticate validates a managed identity token and returns IdentityContext.
// Implements AuthenticationProvider for "managed_identity" credential type.
func (a *ManagedIdentityAdapter) Authenticate(ctx context.Context, cred iam.Credentials) (iam.IdentityContext, error) {
	if cred.Type != "managed_identity" {
		return iam.IdentityContext{}, iam.ErrProviderNotSupported
	}
	if cred.MIToken == "" {
		return iam.IdentityContext{}, iam.ErrInvalidCredentials
	}

	claims, err := a.validateMIToken(ctx, cred.MIToken, cred.MIClientID)
	if err != nil {
		return iam.IdentityContext{}, fmt.Errorf("%w: %v", iam.ErrMIValidationFailed, err)
	}

	// Extract client_id from claims
	clientID := claims["client_id"]
	if clientID == nil {
		clientID = claims["appid"]
	}
	clientIDStr, _ := clientID.(string)

	// Check if client_id is allowed
	allowedRoles, ok := a.allowedClientIDs[clientIDStr]
	if !ok {
		return iam.IdentityContext{}, iam.ErrMIValidationFailed
	}

	// Extract object_id (principal ID)
	objectID := claims["oid"]
	objectIDStr, _ := objectID.(string)

	// Create or get MI identity user
	user, err := a.getOrCreateMIUser(ctx, clientIDStr, objectIDStr, allowedRoles)
	if err != nil {
		return iam.IdentityContext{}, err
	}

	ic := iam.IdentityContext{
		UserID:         user.ID,
		Email:          fmt.Sprintf("%s@mi.cco", clientIDStr),
		DisplayName:    fmt.Sprintf("Managed Identity: %s", clientIDStr),
		AuthMethod:     "managed_identity",
		SystemRoles:    user.SystemRoles,
		OrgID:          uuid.Nil,
		OrgRole:        "viewer",
		ProjectID:      nil,
		ProjectRole:    nil,
		Permissions:    user.Permissions,
		RequestID:      uuid.New().String(),
		CorrelationID:  uuid.New().String(),
		TokenExpiresAt: time.Now().Add(a.tokenManager.AccessTokenTTL()).UnixMilli(),
	}

	return ic, nil
}

// Validate validates an access token.
// Implements TokenValidator (delegates to token manager).
func (a *ManagedIdentityAdapter) Validate(ctx context.Context, token string) (iam.IdentityContext, error) {
	return a.tokenManager.ValidateAccessToken(token)
}

// validateMIToken validates an Azure MI token.
// Supports tokens from IMDS (http://169.254.169.254) or Azure SDK.
// shortcut: signature verification is not implemented; accepts JWT structure + claims only.
// upgrade: use go-jose or Azure SDK to verify RS256 signature against Microsoft JWKS endpoint.
func (a *ManagedIdentityAdapter) validateMIToken(ctx context.Context, token, expectedClientID string) (jwt.MapClaims, error) {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	parsed, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}

	// Verify issuer
	iss, _ := claims["iss"].(string)
	if iss != "" && a.tenantID != "" {
		expectedIss := fmt.Sprintf("https://sts.windows.net/%s/", a.tenantID)
		if iss != expectedIss {
			return nil, fmt.Errorf("invalid issuer: %s", iss)
		}
	}

	// Verify audience matches expected client_id (if provided)
	if expectedClientID != "" {
		aud, _ := claims["aud"].(string)
		if aud != expectedClientID {
			// Also check "appid" claim as audience
			appid, _ := claims["appid"].(string)
			if appid != expectedClientID {
				return nil, fmt.Errorf("audience mismatch: expected %s, got %s", expectedClientID, aud)
			}
		}
	}

	return claims, nil
}

func (a *ManagedIdentityAdapter) getOrCreateMIUser(ctx context.Context, clientID, objectID string, roles []string) (*miUser, error) {
	email := fmt.Sprintf("%s@mi.cco", clientID)
	user, err := a.repo.FindUserByEmail(ctx, email)
	if err == nil {
		return &miUser{ID: user.ID, SystemRoles: roles, Permissions: permissionsForRoles(roles)}, nil
	}

	created, err := a.repo.CreateUser(ctx, iam.CreateUserInput{
		Email:        email,
		DisplayName:  fmt.Sprintf("Managed Identity: %s (%s)", clientID, objectID[:min(8, len(objectID))]),
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

	return &miUser{ID: created.ID, SystemRoles: roles, Permissions: permissionsForRoles(roles)}, nil
}

type miUser struct {
	ID          uuid.UUID
	SystemRoles []string
	Permissions []string
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
