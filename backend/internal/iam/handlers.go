package iam

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// oidcAuthURLProvider is implemented by *adapters.OIDCAdapter.
type oidcAuthURLProvider interface {
	AuthURL(state string) string
}

// HealthHandler returns 200 OK for liveness/readiness checks.
type HealthHandler struct{}

func (h HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// RootHandler returns basic API information for the root path "/".
type RootHandler struct{}

func (h RootHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"name":    "Cloud Cost Optimizer",
		"version": "1.0.0",
		"endpoints": []map[string]string{
			{"method": "GET", "path": "/", "description": "API information"},
			{"method": "GET", "path": "/healthz", "description": "Health check"},
			{"method": "POST", "path": "/auth/register", "description": "Register a new user"},
			{"method": "POST", "path": "/auth/login", "description": "Login with email and password"},
			{"method": "GET", "path": "/auth/oidc/login", "description": "OIDC login redirect"},
			{"method": "GET", "path": "/auth/oidc/callback", "description": "OIDC callback"},
			{"method": "POST", "path": "/auth/m2m/token", "description": "Service principal token"},
			{"method": "POST", "path": "/auth/token/refresh", "description": "Refresh token"},
			{"method": "GET", "path": "/api/v1/users", "description": "List users (platform_admin)"},
			{"method": "POST", "path": "/api/v1/can", "description": "Check permission"},
		},
		"documentation": "See API.md",
	})
}

// LoginRequest is the password login payload.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// NewLoginHandler creates a LoginHandler.
func NewLoginHandler(adapter AuthenticationProvider, tokenMgr *TokenManager, lockout *AccountLockout) *LoginHandler {
	return &LoginHandler{adapter: adapter, tokenMgr: tokenMgr, lockout: lockout}
}

// LoginHandler handles POST /auth/login (password auth, dev/local only).
type LoginHandler struct {
	adapter  AuthenticationProvider
	tokenMgr *TokenManager
	lockout  *AccountLockout
}

func (h *LoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidToken, "invalid request body")
		return
	}

	// Account lockout: reject if email is currently locked out
	if h.lockout.IsLocked(req.Email) {
		writeError(w, http.StatusTooManyRequests, CodeInvalidCredentials,
			"account temporarily locked due to too many failed login attempts")
		return
	}

	ic, err := h.adapter.Authenticate(r.Context(), Credentials{
		Type:     "password",
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		h.lockout.RecordFailure(req.Email)
		slog.Warn("login failed", "email", req.Email, "error", err.Error())
		code := MapErrorToCode(err)
		status := http.StatusUnauthorized
		if code == "FORBIDDEN" || code == "INSUFFICIENT_SCOPE" {
			status = http.StatusForbidden
		}
		writeError(w, status, code, err.Error())
		return
	}

	h.lockout.RecordSuccess(req.Email)

	pair, err := h.tokenMgr.GenerateTokenPair(ic)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	slog.Info("user logged in",
		"user_id", ic.UserID,
		"email", ic.Email,
		"auth_method", ic.AuthMethod,
		"org_id", ic.OrgID,
		"org_role", ic.OrgRole,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_at":    pair.ExpiresAt,
		"user_id":       ic.UserID,
		"email":         ic.Email,
		"org_id":        ic.OrgID,
		"org_role":      ic.OrgRole,
	})
}

// M2MTokenRequest is the client_credentials payload.
type M2MTokenRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Scope        string `json:"scope,omitempty"`
}

// NewM2MTokenHandler creates an M2MTokenHandler.
func NewM2MTokenHandler(adapter AuthenticationProvider, tokenMgr *TokenManager) *M2MTokenHandler {
	return &M2MTokenHandler{adapter: adapter, tokenMgr: tokenMgr}
}

// M2MTokenHandler handles POST /auth/m2m/token (service principal).
type M2MTokenHandler struct {
	adapter  AuthenticationProvider
	tokenMgr *TokenManager
}

func (h *M2MTokenHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req M2MTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidToken, "invalid request body")
		return
	}

	if req.ClientID == "" || req.ClientSecret == "" {
		writeError(w, http.StatusBadRequest, CodeInvalidCredentials, "client_id and client_secret are required")
		return
	}

	ic, err := h.adapter.Authenticate(r.Context(), Credentials{
		Type:         "client_credentials",
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		Scope:        req.Scope,
	})
	if err != nil {
		slog.Warn("m2m token failed", "client_id", req.ClientID, "error", err.Error())
		code := MapErrorToCode(err)
		status := http.StatusUnauthorized
		if code == "FORBIDDEN" || code == "INSUFFICIENT_SCOPE" {
			status = http.StatusForbidden
		}
		writeError(w, status, code, err.Error())
		return
	}

	pair, err := h.tokenMgr.GenerateTokenPair(ic)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	slog.Info("m2m token issued",
		"user_id", ic.UserID,
		"auth_method", ic.AuthMethod,
		"client_id", req.ClientID,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_at":    pair.ExpiresAt,
		"auth_method":   ic.AuthMethod,
		"user_id":       ic.UserID,
	})
}

// oidcCodeExchanger is implemented by *adapters.OIDCAdapter.
type oidcCodeExchanger interface {
	ExchangeCode(ctx context.Context, code string) (IdentityContext, error)
}

// NewOIDCLoginHandler creates an OIDCLoginHandler.
func NewOIDCLoginHandler(adapter oidcAuthURLProvider, cookieDomain string, cookieSecure bool) *OIDCLoginHandler {
	return &OIDCLoginHandler{adapter: adapter, cookieDomain: cookieDomain, cookieSecure: cookieSecure}
}

// OIDCLoginHandler redirects to the OIDC provider authorization endpoint,
// generating a per-request CSRF state bound to an HttpOnly cookie.
type OIDCLoginHandler struct {
	adapter      oidcAuthURLProvider
	cookieDomain string
	cookieSecure bool
}

func (h *OIDCLoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	state := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     "cco_oidc_state",
		Value:    state,
		Path:     "/auth/oidc/callback",
		Domain:   h.cookieDomain,
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, h.adapter.AuthURL(state), http.StatusFound)
}

// OIDCCallbackHandler exchanges the authorization code for tokens and sets
// session cookies after verifying the CSRF state.
type OIDCCallbackHandler struct {
	exchanger    oidcCodeExchanger
	tokenMgr     *TokenManager
	cookieDomain string
	cookieSecure bool
}

// NewOIDCCallbackHandler creates an OIDCCallbackHandler.
func NewOIDCCallbackHandler(exchanger oidcCodeExchanger, tokenMgr *TokenManager, cookieDomain string, cookieSecure bool) *OIDCCallbackHandler {
	return &OIDCCallbackHandler{exchanger: exchanger, tokenMgr: tokenMgr, cookieDomain: cookieDomain, cookieSecure: cookieSecure}
}

func (h *OIDCCallbackHandler) clearStateCookie(w http.ResponseWriter, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cco_oidc_state",
		Value:    "",
		Path:     "/auth/oidc/callback",
		Domain:   h.cookieDomain,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *OIDCCallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	expected, err := r.Cookie("cco_oidc_state")
	if code == "" || err != nil || expected.Value == "" ||
		subtle.ConstantTimeCompare([]byte(state), []byte(expected.Value)) != 1 {
		h.clearStateCookie(w, -1)
		writeError(w, http.StatusBadRequest, CodeInvalidToken, "missing or invalid code/state")
		return
	}
	h.clearStateCookie(w, -1)

	ic, err := h.exchanger.ExchangeCode(r.Context(), code)
	if err != nil {
		slog.Warn("oidc code exchange failed", "error", err.Error())
		writeError(w, http.StatusUnauthorized, CodeInvalidToken, err.Error())
		return
	}

	pair, err := h.tokenMgr.GenerateTokenPair(ic)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "cco_access",
		Value:    pair.AccessToken,
		Path:     "/",
		Domain:   h.cookieDomain,
		MaxAge:   int(h.tokenMgr.AccessTokenTTL().Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "cco_refresh",
		Value:    pair.RefreshToken,
		Path:     "/",
		Domain:   h.cookieDomain,
		MaxAge:   int(h.tokenMgr.RefreshTokenTTL().Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	slog.Info("oidc login completed", "user_id", ic.UserID, "email", ic.Email)
	http.Redirect(w, r, "/", http.StatusFound)
}

// TokenRefreshHandler rotates access/refresh tokens.
// Refresh tokens are persisted (hashed) and consumed on rotation by TokenManager.
type TokenRefreshHandler struct {
	tokenMgr *TokenManager
	userRepo UserRepository
}

// NewTokenRefreshHandler creates a TokenRefreshHandler.
func NewTokenRefreshHandler(tokenMgr *TokenManager, userRepo UserRepository) *TokenRefreshHandler {
	return &TokenRefreshHandler{tokenMgr: tokenMgr, userRepo: userRepo}
}

func (h *TokenRefreshHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidToken, "invalid request body")
		return
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, CodeInvalidToken, "refresh_token is required")
		return
	}

	userID, ok := h.tokenMgr.ValidateRefreshToken(req.RefreshToken)
	if !ok {
		slog.Warn("token refresh failed: invalid refresh token", "token_len", len(req.RefreshToken))
		writeError(w, http.StatusUnauthorized, CodeInvalidToken, "invalid or expired refresh token")
		return
	}

	// Look up user to rebuild identity context
	user, err := h.userRepo.FindUserByID(r.Context(), userID)
	if err != nil {
		slog.Warn("token refresh failed: user not found", "user_id", userID, "error", err.Error())
		writeError(w, http.StatusUnauthorized, CodeUserNotFound, "user not found")
		return
	}

	ic := IdentityContext{
		UserID:         user.ID,
		Email:          user.Email,
		DisplayName:    user.DisplayName,
		AuthMethod:     "refresh_token",
		OrgID:          uuid.Nil,
		OrgRole:        "viewer",
		RequestID:      uuid.New().String(),
		CorrelationID:  uuid.New().String(),
		TokenExpiresAt: time.Now().Add(h.tokenMgr.config.AccessTokenTTL).UnixMilli(),
	}

	// Token already consumed by ValidateRefreshToken (rotation). Revoke is a
	// no-op safety net; then issue a new pair.
	h.tokenMgr.RevokeRefreshToken(req.RefreshToken)
	pair, err := h.tokenMgr.GenerateTokenPair(ic)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	slog.Info("token refreshed", "user_id", userID, "request_id", ic.RequestID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_at":    pair.ExpiresAt,
		"user_id":       ic.UserID,
	})
}

// RegisterRequest is the user registration payload.
// Password is optional: omit it for OIDC-federated users; provide it for local password users.
type RegisterRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password,omitempty"`
}

// NewRegisterHandler creates a RegisterHandler.
func NewRegisterHandler(repo UserRepository, tokenMgr *TokenManager) *RegisterHandler {
	return &RegisterHandler{repo: repo, tokenMgr: tokenMgr}
}

// RegisterHandler handles POST /auth/register (public endpoint to create the first user).
type RegisterHandler struct {
	repo     UserRepository
	tokenMgr *TokenManager
}

func (h *RegisterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidToken, "invalid request body")
		return
	}

	if req.Email == "" || req.DisplayName == "" {
		slog.Warn("register failed: missing required fields", "email", req.Email, "display_name", req.DisplayName)
		writeError(w, http.StatusBadRequest, CodeInvalidCredentials, "email and display_name are required")
		return
	}

	// Verify email is not already registered
	if _, err := h.repo.FindUserByEmail(r.Context(), req.Email); err == nil {
		slog.Warn("register failed: email already exists", "email", req.Email)
		writeError(w, http.StatusConflict, CodeUserExists, "user already exists")
		return
	}

	var passwordHash *string
	if req.Password != "" {
		hash, err := HashPassword(req.Password)
		if err != nil {
			slog.Warn("register failed: password hashing error", "email", req.Email, "error", err.Error())
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		passwordHash = &hash
	}

	created, err := h.repo.CreateUser(r.Context(), CreateUserInput{
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PasswordHash: passwordHash,
	})
	if err != nil {
		slog.Warn("register failed: create user error", "email", req.Email, "error", err.Error())
		code := MapErrorToCode(err)
		status := http.StatusBadRequest
		if code == CodeUserExists {
			status = http.StatusConflict
		}
		writeError(w, status, code, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	slog.Info("user registered",
		"user_id", created.ID,
		"email", created.Email,
		"display_name", created.DisplayName,
		"status", created.Status,
	)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":        created.ID,
		"email":          created.Email,
		"display_name":   created.DisplayName,
		"status":         created.Status,
		"email_verified": created.EmailVerified,
		"created_at":     created.CreatedAt,
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
