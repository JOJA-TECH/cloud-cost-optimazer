package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// OIDCAdapter implements AuthenticationProvider + TokenValidator for Entra ID / Keycloak / Google.
type OIDCAdapter struct {
	verifier      *oidc.IDTokenVerifier
	oauth2Config  *oauth2.Config
	provider      *oidc.Provider
	tokenManager  *iam.TokenManager
	repo          iam.IdentityRepository
	cookieDomain  string
	cookieSecure  bool
	issuerBaseURL string
	isEntra       bool
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

	baseURL, err := issuerBaseURL(cfg.IssuerURL)
	if err != nil {
		return nil, err
	}
	// Entra multi-tenant apps must use /common; a tenant-specific endpoint
	// rejects users homed in other tenants. The ID token issuer is checked
	// against the token's own tid below.
	isEntra := strings.HasSuffix(strings.Split(baseURL, "://")[1], "login.microsoftonline.com")
	verifyCfg := &oidc.Config{ClientID: cfg.ClientID}
	endpoint := provider.Endpoint()
	if isEntra {
		verifyCfg.SkipIssuerCheck = true
		endpoint = oauth2.Endpoint{
			AuthURL:  baseURL + "/common/oauth2/v2.0/authorize",
			TokenURL: baseURL + "/common/oauth2/v2.0/token",
		}
	}

	// Tenant-specific JWKS omits personal Microsoft account keys; /common has all.
	verifier := provider.Verifier(verifyCfg)
	if isEntra {
		verifier = oidc.NewVerifier(baseURL+"/common/v2.0", oidc.NewRemoteKeySet(ctx, baseURL+"/common/discovery/v2.0/keys"), verifyCfg)
	}

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     endpoint,
		Scopes:       append([]string{oidc.ScopeOpenID, "profile", "email"}, cfg.Scopes...),
	}

	return &OIDCAdapter{
		verifier:      verifier,
		oauth2Config:  oauth2Config,
		provider:      provider,
		tokenManager:  cfg.TokenManager,
		repo:          cfg.Repo,
		cookieDomain:  cfg.CookieDomain,
		cookieSecure:  cfg.CookieSecure,
		issuerBaseURL: baseURL,
		isEntra:       isEntra,
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
		Issuer            string `json:"iss"`
		Email             string `json:"email"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		TenantID          string `json:"tid"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return iam.IdentityContext{}, fmt.Errorf("parse claims: %w", err)
	}
	if a.isEntra {
		if claims.TenantID == "" {
			return iam.IdentityContext{}, fmt.Errorf("%w: missing tid claim", iam.ErrTokenInvalid)
		}
		if want := a.issuerBaseURL + "/" + claims.TenantID + "/v2.0"; claims.Issuer != want {
			return iam.IdentityContext{}, fmt.Errorf("%w: unexpected issuer %q", iam.ErrTokenInvalid, claims.Issuer)
		}
	}

	// Entra ID often omits `email` for work accounts; preferred_username is the
	// stable UPN in that case.
	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}
	// Find or create user
	user, err := a.findOrCreateUser(ctx, claims.Subject, email, claims.Name)
	if err != nil {
		return iam.IdentityContext{}, err
	}

	// Link external identity
	_, err = a.repo.LinkExternalIdentity(ctx, iam.CreateExternalIdentityInput{
		UserID:   user.ID,
		Provider: "entra_id",
		Subject:  claims.Subject,
	})
	if err != nil {
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
	if err == nil {
		// Return first active membership
		for _, m := range memberships {
			if m.Status == "active" {
				return m.OrgID, m.MembershipRole, nil
			}
		}
		if len(memberships) > 0 {
			return memberships[0].OrgID, memberships[0].MembershipRole, nil
		}
	}
	// shortcut: new Entra users get admin in the default org so the app can load
	// cloud accounts; upgrade: provision memberships from Entra groups.
	if id, err := uuid.Parse(os.Getenv("CCO_DEFAULT_ORG_ID")); err == nil {
		return id, "admin", nil
	}
	return uuid.Nil, "viewer", nil
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

// ExchangeCode exchanges an OAuth2 authorization code for tokens and returns
// the authenticated identity (callback half of the Authorization Code flow).
func (a *OIDCAdapter) ExchangeCode(ctx context.Context, code string) (iam.IdentityContext, error) {
	tok, err := a.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return iam.IdentityContext{}, fmt.Errorf("oidc code exchange: %w", err)
	}
	rawIDToken, _ := tok.Extra("id_token").(string)
	if rawIDToken == "" {
		return iam.IdentityContext{}, errors.New("no id_token in token response")
	}
	return a.Authenticate(ctx, iam.Credentials{Type: "oidc", IDToken: rawIDToken})
}

func issuerBaseURL(issuerURL string) (string, error) {
	u, err := url.Parse(issuerURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid OIDC issuer URL %q", issuerURL)
	}
	return u.Scheme + "://" + u.Host, nil
}
