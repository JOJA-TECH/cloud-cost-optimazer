package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// OIDCAdapter implements AuthenticationProvider + TokenValidator for Entra ID / Keycloak / Google.
type OIDCAdapter struct {
	verifier     *oidc.IDTokenVerifier
	oauth2Config *oauth2.Config
	provider     *oidc.Provider
	tokenManager *iam.TokenManager
	repo         iam.IdentityRepository
	cookieDomain string
	cookieSecure bool
}

// OIDCConfig holds OIDC provider configuration.
type OIDCConfig struct {
	IssuerURL    string // e.g., "https://login.microsoftonline.com/{tenant}/v2.0"
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	TokenManager *iam.TokenManager
	Repo         iam.IdentityRepository
	CookieDomain string
	CookieSecure bool
}

// NewOIDCAdapter creates a new OIDCAdapter.
// Requires network access to issuer for discovery on startup.
func NewOIDCAdapter(ctx context.Context, cfg OIDCConfig) (*OIDCAdapter, error) {
	if cfg.TokenManager == nil {
		return nil, errors.New("token manager required")
	}
	if cfg.Repo == nil {
		return nil, errors.New("identity repository required")
	}

	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc provider discovery: %w", err)
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       append([]string{oidc.ScopeOpenID, "profile", "email"}, cfg.Scopes...),
	}

	return &OIDCAdapter{
		verifier:     verifier,
		oauth2Config: oauth2Config,
		provider:     provider,
		tokenManager: cfg.TokenManager,
		repo:         cfg.Repo,
		cookieDomain: cfg.CookieDomain,
		cookieSecure: cfg.CookieSecure,
	}, nil
}

// AuthURL returns the authorization URL for login initiation.
// state should be a random CSRF token stored in session/cookie.
func (a *OIDCAdapter) AuthURL(state string) string {
	return a.oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
}

// Authenticate exchanges authorization code for tokens and returns IdentityContext.
// Implements AuthenticationProvider for "oidc" credential type.
func (a *OIDCAdapter) Authenticate(ctx context.Context, cred iam.Credentials) (iam.IdentityContext, error) {
	if cred.Type != "oidc" {
		return iam.IdentityContext{}, iam.ErrProviderNotSupported
	}
	if cred.IDToken == "" {
		return iam.IdentityContext{}, iam.ErrInvalidCredentials
	}

	// Verify ID token
	idToken, err := a.verifier.Verify(ctx, cred.IDToken)
	if err != nil {
		return iam.IdentityContext{}, fmt.Errorf("%w: %v", iam.ErrTokenInvalid, err)
	}

	// Extract claims
	var claims struct {
		Subject           string `json:"sub"`
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		TenantID          string `json:"tid"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return iam.IdentityContext{}, fmt.Errorf("parse claims: %w", err)
	}

	// Find or create user
	user, err := a.findOrCreateUser(ctx, claims.Subject, claims.Email, claims.Name)
	if err != nil {
		return iam.IdentityContext{}, err
	}

	// Link external identity
	_, err = a.repo.LinkExternalIdentity(ctx, iam.CreateExternalIdentityInput{
		UserID:   user.ID,
		Provider: "entra_id",
		Subject:  claims.Subject,
	})
	if err != nil && !errors.Is(err, iam.ErrExternalIdentityExists) {
		return iam.IdentityContext{}, err
	}

	// Resolve org/project context (default to first org)
	orgID, orgRole, err := a.resolveDefaultOrg(ctx, user.ID)
	if err != nil {
		orgID = uuid.Nil
		orgRole = "viewer"
	}

	// Build permissions
	perms, err := iam.NewMembershipResolver(a.repo).ResolvePermissions(ctx, user.ID, orgID, nil)
	if err != nil {
		perms = []string{}
	}

	ic := iam.IdentityContext{
		UserID:         user.ID,
		Email:          user.Email,
		DisplayName:    user.DisplayName,
		AuthMethod:     "oidc",
		SystemRoles:    user.SystemRoles,
		OrgID:          orgID,
		OrgRole:        orgRole,
		ProjectID:      nil,
		ProjectRole:    nil,
		Permissions:    perms,
		RequestID:      uuid.New().String(),
		CorrelationID:  uuid.New().String(),
		TokenExpiresAt: time.Now().Add(a.tokenManager.AccessTokenTTL()).UnixMilli(),
	}

	return ic, nil
}

// Validate validates an access token and returns IdentityContext.
// Implements TokenValidator for middleware.
func (a *OIDCAdapter) Validate(ctx context.Context, token string) (iam.IdentityContext, error) {
	return a.tokenManager.ValidateAccessToken(token)
}

// findOrCreateUser finds user by external identity or creates new.
func (a *OIDCAdapter) findOrCreateUser(ctx context.Context, subject, email, name string) (*userInfo, error) {
	// Try find by external identity
	ext, err := a.repo.FindExternalIdentityByProviderSubject(ctx, "entra_id", subject)
	if err == nil {
		// Update last seen
		_ = a.repo.UpdateExternalIdentityLastSeen(ctx, ext.ExternalIdentityID)
		return a.getUserInfo(ctx, ext.UserID)
	}

	// Try find by email
	if email != "" {
		user, err := a.repo.FindUserByEmail(ctx, email)
		if err == nil {
			// Link external identity
			_, _ = a.repo.LinkExternalIdentity(ctx, iam.CreateExternalIdentityInput{
				UserID:   user.ID,
				Provider: "entra_id",
				Subject:  subject,
			})
			return a.getUserInfo(ctx, user.ID)
		}
	}

	// Create new user
	displayName := name
	if displayName == "" {
		displayName = email
	}
	if displayName == "" {
		displayName = subject[:min(20, len(subject))]
	}

	created, err := a.repo.CreateUser(ctx, iam.CreateUserInput{
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: nil, // OIDC user
	})
	if err != nil {
		return nil, err
	}

	// Link external identity
	_, _ = a.repo.LinkExternalIdentity(ctx, iam.CreateExternalIdentityInput{
		UserID:   created.ID,
		Provider: "entra_id",
		Subject:  subject,
	})

	return a.getUserInfo(ctx, created.ID)
}

type userInfo struct {
	ID          uuid.UUID
	Email       string
	DisplayName string
	SystemRoles []string
}

func (a *OIDCAdapter) getUserInfo(ctx context.Context, userID uuid.UUID) (*userInfo, error) {
	user, err := a.repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	userRoles, err := a.repo.ListUserRolesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	var systemRoles []string
	for _, ur := range userRoles {
		role, err := a.repo.FindRoleByID(ctx, ur.RoleID)
		if err == nil {
			systemRoles = append(systemRoles, role.RoleCode)
		}
	}
	return &userInfo{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		SystemRoles: systemRoles,
	}, nil
}

func (a *OIDCAdapter) resolveDefaultOrg(ctx context.Context, userID uuid.UUID) (uuid.UUID, string, error) {
	memberships, err := a.repo.ListMembershipsByUser(ctx, userID)
	if err != nil || len(memberships) == 0 {
		return uuid.Nil, "viewer", nil
	}
	// Return first active membership
	for _, m := range memberships {
		if m.Status == "active" {
			return m.OrgID, m.MembershipRole, nil
		}
	}
	return memberships[0].OrgID, memberships[0].MembershipRole, nil
}

// SetAuthCookies sets access and refresh tokens as httpOnly cookies.
func (a *OIDCAdapter) SetAuthCookies(w http.ResponseWriter, accessToken, refreshToken string, accessTTL, refreshTTL time.Duration) {
	domain := a.cookieDomain
	secure := a.cookieSecure

	http.SetCookie(w, &http.Cookie{
		Name:     "cco_access",
		Value:    accessToken,
		Path:     "/",
		Domain:   domain,
		MaxAge:   int(accessTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "cco_refresh",
		Value:    refreshToken,
		Path:     "/",
		Domain:   domain,
		MaxAge:   int(refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearAuthCookies clears auth cookies.
func (a *OIDCAdapter) ClearAuthCookies(w http.ResponseWriter) {
	domain := a.cookieDomain
	http.SetCookie(w, &http.Cookie{
		Name:     "cco_access",
		Value:    "",
		Path:     "/",
		Domain:   domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "cco_refresh",
		Value:    "",
		Path:     "/",
		Domain:   domain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// shortcut: refresh token rotation not implemented; upgrade: store hashed refresh tokens in DB with expiry.
