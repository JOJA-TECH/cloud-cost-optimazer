package adapters

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

// m2mMaxAttempts is the max failed M2M attempts per client_id before
// temporarily rejecting further attempts (simple in-memory brute-force guard).
const m2mMaxAttempts = 5

// m2mLockoutDuration is how long a client_id is blocked after exceeding
// m2mMaxAttempts failures.
const m2mLockoutDuration = 15 * time.Minute

// ServicePrincipalAdapter implements AuthenticationProvider for client_credentials flow.
// Used by collectors, ML service, and other machine-to-machine callers.
type ServicePrincipalAdapter struct {
	tokenManager       *iam.TokenManager
	repo               iam.IdentityRepository
	validClients       map[string]string    // client_id -> client_secret_hash (argon2id)
	allowedScopes      map[string][]string  // client_id -> allowed scopes
	m2mAttempts        map[string]int       // client_id -> failed attempt count (brute-force guard)
	m2mLockedUntil     map[string]time.Time // client_id -> lockout expiry
	m2mAttemptsMu      sync.Mutex
	m2mLockoutDuration time.Duration
}

// spArgon2Params mirrors the Argon2id parameters used for password hashing in
// internal/iam/auth.go so M2M client secrets follow the same standard:
//
//	argon2id, m=64MiB, t=3, p=4, kl=32, sl=16
//
// stored as base64(salt).base64(hash).
var spArgon2Params = struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	keyLength   uint32
	saltLength  uint32
}{
	memory:      64 * 1024,
	iterations:  3,
	parallelism: 4,
	keyLength:   32,
	saltLength:  16,
}

// SPConfig holds service principal configuration.
// Clients values must be Argon2id hashes (base64(salt).base64(hash)), never
// plaintext secrets.
type SPConfig struct {
	TokenManager  *iam.TokenManager
	Repo          iam.IdentityRepository
	Clients       map[string]string // client_id -> argon2id hash of client_secret
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
		tokenManager:       cfg.TokenManager,
		repo:               cfg.Repo,
		validClients:       cfg.Clients,
		allowedScopes:      cfg.AllowedScopes,
		m2mAttempts:        make(map[string]int),
		m2mLockedUntil:     make(map[string]time.Time),
		m2mLockoutDuration: m2mLockoutDuration,
	}
}

// verifyClientSecret compares a plaintext secret against its stored Argon2id
// hash using constant-time comparison (iam.VerifyPassword).
func verifyClientSecret(expectedHash, secret string) bool {
	if expectedHash == "" || secret == "" {
		return false
	}
	return argon2Verify(expectedHash, secret) == nil
}

func argon2Verify(hash, password string) error {
	parts := strings.SplitN(hash, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid hash format")
	}
	salt, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}
	expectedHash, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	computedHash := argon2.IDKey([]byte(password), salt, spArgon2Params.iterations, spArgon2Params.memory, spArgon2Params.parallelism, spArgon2Params.keyLength)
	if subtle.ConstantTimeCompare(computedHash, expectedHash) != 1 {
		return fmt.Errorf("secret does not match")
	}
	return nil
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

	// Per-client_id brute-force guard: reject if currently locked out, or if
	// too many recent failures (then lock out for m2mLockoutDuration).
	now := time.Now()
	a.m2mAttemptsMu.Lock()
	if until, ok := a.m2mLockedUntil[cred.ClientID]; ok && now.Before(until) {
		a.m2mAttemptsMu.Unlock()
		return iam.IdentityContext{}, iam.ErrSPValidationFailed
	}
	if a.m2mAttempts[cred.ClientID] >= m2mMaxAttempts {
		a.m2mLockedUntil[cred.ClientID] = now.Add(a.m2mLockoutDuration)
		delete(a.m2mAttempts, cred.ClientID)
		a.m2mAttemptsMu.Unlock()
		return iam.IdentityContext{}, iam.ErrSPValidationFailed
	}
	a.m2mAttemptsMu.Unlock()

	// Verify client secret against its Argon2id hash (repo standard).
	expectedHash, ok := a.validClients[cred.ClientID]
	if !ok {
		a.recordM2MFailure(cred.ClientID)
		return iam.IdentityContext{}, iam.ErrSPValidationFailed
	}
	if !verifyClientSecret(expectedHash, cred.ClientSecret) {
		a.recordM2MFailure(cred.ClientID)
		return iam.IdentityContext{}, iam.ErrSPValidationFailed
	}

	// Successful authentication clears the failure counter and lockout for this client.
	a.m2mAttemptsMu.Lock()
	delete(a.m2mAttempts, cred.ClientID)
	delete(a.m2mLockedUntil, cred.ClientID)
	a.m2mAttemptsMu.Unlock()

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

// recordM2MFailure increments the failure counter for a client_id and locks it
// out when the threshold is reached. Callers must not hold m2mAttemptsMu.
func (a *ServicePrincipalAdapter) recordM2MFailure(clientID string) {
	a.m2mAttemptsMu.Lock()
	defer a.m2mAttemptsMu.Unlock()
	a.m2mAttempts[clientID]++
	if a.m2mAttempts[clientID] >= m2mMaxAttempts {
		a.m2mLockedUntil[clientID] = time.Now().Add(a.m2mLockoutDuration)
		delete(a.m2mAttempts, clientID)
	}
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
