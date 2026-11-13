package iam

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/valkey-io/valkey-go"
	"golang.org/x/crypto/argon2"
	"gorm.io/gorm"
)

// ============================================================================
// JWT Token Management
// ============================================================================

// TokenConfig holds JWT signing configuration.
type TokenConfig struct {
	SigningKey      []byte // HS256 secret (rotate via env)
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	Issuer          string
	Audience        string
}

// DefaultTokenConfig returns sensible defaults.
func DefaultTokenConfig(signingKey string) TokenConfig {
	return TokenConfig{
		SigningKey:      []byte(signingKey),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		Issuer:          "cco-app",
		Audience:        "cco-api",
	}
}

// TokenPair contains access and refresh tokens.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64 // unix ms
}

// TokenManager handles JWT creation and validation. Refresh tokens are
// persisted hashed in the DB when one is provided; otherwise an in-memory
// store is used as fallback (single instance only).
type TokenManager struct {
	config        TokenConfig
	db            *gorm.DB
	refreshTokens sync.Map // token -> refreshTokenEntry (fallback when db == nil)
}

type refreshTokenEntry struct {
	userID    uuid.UUID
	expiresAt time.Time
}

// NewTokenManager creates a new TokenManager. db may be nil to keep refresh
// tokens in memory (dev/tests).
func NewTokenManager(config TokenConfig, db *gorm.DB) *TokenManager {
	return &TokenManager{config: config, db: db}
}

// GenerateAccessToken creates a short-lived access token.
func (m *TokenManager) GenerateAccessToken(ic IdentityContext) (string, error) {
	now := time.Now()
	expiresAt := now.Add(m.config.AccessTokenTTL)

	claims := jwt.MapClaims{
		"sub":   ic.UserID.String(),
		"email": ic.Email,
		"name":  ic.DisplayName,
		"amr":   ic.AuthMethod,
		"roles": ic.SystemRoles,
		"org":   ic.OrgID.String(),
		"org_r": ic.OrgRole,
		"prj":   ic.ProjectID,
		"prj_r": ic.ProjectRole,
		"perm":  ic.Permissions,
		"rid":   ic.RequestID,
		"cid":   ic.CorrelationID,
		"iat":   now.Unix(),
		"exp":   expiresAt.Unix(),
		"iss":   m.config.Issuer,
		"aud":   m.config.Audience,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.config.SigningKey)
}

// GenerateRefreshToken creates a long-lived opaque refresh token.
func (m *TokenManager) GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// AccessTokenTTL returns the access token TTL.
func (m *TokenManager) AccessTokenTTL() time.Duration {
	return m.config.AccessTokenTTL
}

// RefreshTokenTTL returns the refresh token TTL.
func (m *TokenManager) RefreshTokenTTL() time.Duration {
	return m.config.RefreshTokenTTL
}

// Validate implements iam.TokenValidator for TokenManager.
func (m *TokenManager) Validate(ctx context.Context, token string) (IdentityContext, error) {
	return m.ValidateAccessToken(token)
}

// GenerateTokenPair creates both access and refresh tokens.
func (m *TokenManager) GenerateTokenPair(ic IdentityContext) (TokenPair, error) {
	access, err := m.GenerateAccessToken(ic)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := m.GenerateRefreshToken()
	if err != nil {
		return TokenPair{}, err
	}
	expiresAt := time.Now().Add(m.config.AccessTokenTTL).UnixMilli()
	refreshExpiresAt := time.Now().Add(m.config.RefreshTokenTTL)
	m.persistRefreshToken(refresh, ic.UserID, refreshExpiresAt)
	return TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresAt:    expiresAt,
	}, nil
}

// StoreRefreshToken stores a refresh token with its expiry.
func (m *TokenManager) StoreRefreshToken(token string, userID uuid.UUID, expiresAt time.Time) {
	m.persistRefreshToken(token, userID, expiresAt)
}

// persistRefreshToken stores a refresh token: hashed in the DB when available,
// in memory otherwise. Persistence is best-effort — a failed write is logged
// but does not fail the login.
func (m *TokenManager) persistRefreshToken(token string, userID uuid.UUID, expiresAt time.Time) {
	if m.db == nil {
		m.refreshTokens.Store(token, refreshTokenEntry{userID: userID, expiresAt: expiresAt})
		return
	}
	row := database.RefreshTokenModel{TokenHash: hashToken(token), UserID: userID, ExpiresAt: expiresAt, CreatedAt: time.Now()}
	if err := m.db.Table(database.TableName(m.db, "iam", "refresh_tokens")).Create(&row).Error; err != nil {
		slog.Error("persist refresh token", "error", err)
	}
}

// hashToken returns the SHA-256 hex digest of a raw token.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ValidateRefreshToken validates a refresh token and returns the associated user ID.
// Consumes (deletes) the token on validation so it cannot be reused — this is the
// rotation mechanism: old token is invalidated when a new pair is issued.
// Returns false if token is missing, expired, or invalid.
// Refresh tokens are persisted hashed (SHA-256) and consumed on rotation.
func (m *TokenManager) ValidateRefreshToken(token string) (uuid.UUID, bool) {
	if m.db != nil {
		return m.validateDBRefreshToken(token)
	}
	val, ok := m.refreshTokens.LoadAndDelete(token)
	if !ok {
		return uuid.Nil, false
	}
	entry, ok := val.(refreshTokenEntry)
	if !ok || time.Now().After(entry.expiresAt) {
		return uuid.Nil, false
	}
	return entry.userID, true
}

// validateDBRefreshToken looks the token up by hash and deletes it in the same
// call (consume-on-use). RowsAffected == 0 on delete means another request
// already consumed it, so the token is rejected.
func (m *TokenManager) validateDBRefreshToken(token string) (uuid.UUID, bool) {
	table := database.TableName(m.db, "iam", "refresh_tokens")
	var row database.RefreshTokenModel
	if err := m.db.Table(table).Where("token_hash = ?", hashToken(token)).First(&row).Error; err != nil {
		return uuid.Nil, false
	}
	del := m.db.Table(table).Where("token_hash = ?", hashToken(token)).Delete(&database.RefreshTokenModel{})
	if del.Error != nil || del.RowsAffected == 0 {
		return uuid.Nil, false
	}
	if time.Now().After(row.ExpiresAt) {
		return uuid.Nil, false
	}
	return row.UserID, true
}

// RevokeRefreshToken removes a refresh token from the store.
func (m *TokenManager) RevokeRefreshToken(token string) {
	if m.db != nil {
		m.db.Table(database.TableName(m.db, "iam", "refresh_tokens")).Where("token_hash = ?", hashToken(token)).Delete(&database.RefreshTokenModel{})
		return
	}
	m.refreshTokens.Delete(token)
}

// ValidateAccessToken parses and validates an access token.
func (m *TokenManager) ValidateAccessToken(tokenString string) (IdentityContext, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return m.config.SigningKey, nil
	}, jwt.WithIssuer(m.config.Issuer), jwt.WithAudience(m.config.Audience))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return IdentityContext{}, ErrTokenExpired
		}
		return IdentityContext{}, ErrTokenInvalid
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return IdentityContext{}, ErrTokenInvalid
	}

	return m.claimsToIdentityContext(claims)
}

func (m *TokenManager) claimsToIdentityContext(claims jwt.MapClaims) (IdentityContext, error) {
	userIDStr, _ := claims["sub"].(string)
	userID, _ := uuid.Parse(userIDStr)

	email, _ := claims["email"].(string)
	displayName, _ := claims["name"].(string)
	authMethod, _ := claims["amr"].(string)

	var systemRoles []string
	if roles, ok := claims["roles"].([]interface{}); ok {
		for _, r := range roles {
			if s, ok := r.(string); ok {
				systemRoles = append(systemRoles, s)
			}
		}
	}

	orgIDStr, _ := claims["org"].(string)
	orgID, _ := uuid.Parse(orgIDStr)
	orgRole, _ := claims["org_r"].(string)

	var projectID *uuid.UUID
	if prj, ok := claims["prj"].(string); ok && prj != "" {
		pid, _ := uuid.Parse(prj)
		projectID = &pid
	}
	projectRole, _ := claims["prj_r"].(string)

	var permissions []string
	if perms, ok := claims["perm"].([]interface{}); ok {
		for _, p := range perms {
			if s, ok := p.(string); ok {
				permissions = append(permissions, s)
			}
		}
	}

	requestID, _ := claims["rid"].(string)
	correlationID, _ := claims["cid"].(string)

	exp, _ := claims["exp"].(float64)
	expiresAt := int64(exp * 1000)

	return IdentityContext{
		UserID:         userID,
		Email:          email,
		DisplayName:    displayName,
		AuthMethod:     authMethod,
		SystemRoles:    systemRoles,
		OrgID:          orgID,
		OrgRole:        orgRole,
		ProjectID:      projectID,
		ProjectRole:    &projectRole,
		Permissions:    permissions,
		RequestID:      requestID,
		CorrelationID:  correlationID,
		TokenExpiresAt: expiresAt,
	}, nil
}

// ============================================================================
// Password Hashing (Argon2id)
// ============================================================================

// argon2Params defines the Argon2id parameters used for password hashing.
// Memory: 64 MB, Iterations: 3, Parallelism: 4, Key length: 32 bytes.
// These values are a balance between security and performance for Report 1.0.
// Argon2id params are intentionally fixed: the custom "salt.hash" encoding does
// NOT embed the params (unlike the PHC string format), so changing them at
// runtime would silently invalidate every existing hash. Upgrade path: migrate
// to PHC-format hashes ($argon2id$...) with embedded params, then params can
// evolve per hash.
var argon2Params = struct {
	memory      uint32 // 64 MiB
	iterations  uint32 // 3 passes
	parallelism uint8  // 4 threads
	keyLength   uint32 // 32 bytes
	saltLength  uint32 // 16 bytes
}{
	memory:      64 * 1024,
	iterations:  3,
	parallelism: 4,
	keyLength:   32,
	saltLength:  16,
}

// HashPassword returns an Argon2id hash of password.
// Format: base64(salt) + "." + base64(hash)
func HashPassword(password string) (string, error) {
	salt := make([]byte, argon2Params.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argon2Params.iterations, argon2Params.memory, argon2Params.parallelism, argon2Params.keyLength)
	return base64.StdEncoding.EncodeToString(salt) + "." + base64.StdEncoding.EncodeToString(hash), nil
}

// VerifyPassword checks password against an Argon2id hash produced by HashPassword.
// Returns nil if the password matches, an error otherwise.
func VerifyPassword(hash, password string) error {
	parts := splitTwo(hash, ".")
	if len(parts) != 2 {
		return errors.New("invalid hash format")
	}
	salt, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}
	expectedHash, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	computedHash := argon2.IDKey([]byte(password), salt, argon2Params.iterations, argon2Params.memory, argon2Params.parallelism, argon2Params.keyLength)
	if subtle.ConstantTimeCompare(computedHash, expectedHash) != 1 {
		return errors.New("password does not match")
	}
	return nil
}

// splitTwo splits s by sep into at most two parts.
func splitTwo(s, sep string) []string {
	for i := 0; i < len(s); i++ {
		if string(s[i:i+len(sep)]) == sep {
			return []string{s[:i], s[i+len(sep):]}
		}
	}
	return []string{s}
}

// ============================================================================
// Account Lockout (brute-force protection)
// ============================================================================

// AccountLockout tracks failed login attempts per email to prevent brute-force.
// State lives in Valkey when a client is configured (shared across instances);
// otherwise it is kept in memory (single instance only, reset on restart).
type AccountLockout struct {
	mu              sync.Mutex
	attempts        map[string]int       // email (lowercase) -> failed count (in-memory mode only)
	lockedUntil     map[string]time.Time // email -> lockout expiry (in-memory mode only)
	maxAttempts     int
	lockoutDuration time.Duration
	valkey          valkey.Client
}

// NewAccountLockout creates an AccountLockout. vc may be nil to keep the state
// in memory (dev/tests/single instance).
func NewAccountLockout(maxAttempts int, lockoutDuration time.Duration, vc valkey.Client) *AccountLockout {
	return &AccountLockout{
		attempts:        make(map[string]int),
		lockedUntil:     make(map[string]time.Time),
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		valkey:          vc,
	}
}

// attemptsKey returns the Valkey key holding the failure counter for an email.
func (a *AccountLockout) attemptsKey(email string) string {
	return "cco:lock:attempts:" + email
}

// lockedKey returns the Valkey key holding the lockout marker for an email.
func (a *AccountLockout) lockedKey(email string) string {
	return "cco:lock:locked:" + email
}

// IsLocked reports whether the email is currently locked out.
func (a *AccountLockout) IsLocked(email string) bool {
	email = strings.ToLower(email)
	if a.valkey != nil {
		n, err := a.valkey.Do(context.Background(), a.valkey.B().Exists().Key(a.lockedKey(email)).Build()).ToInt64()
		if err != nil {
			// Prototype policy: fail open — never block logins because the cache is down.
			slog.Warn("lockout: valkey EXISTS failed, failing open", "error", err)
			return false
		}
		return n > 0
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	until, ok := a.lockedUntil[email]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(a.lockedUntil, email)
		delete(a.attempts, email)
		return false
	}
	return true
}

// RecordFailure increments the failure count for an email and locks it out
// when the threshold is reached.
// Note: in the Valkey mode attempts decay after lockoutDuration of inactivity,
// while the in-memory mode keeps them until a successful login — accepted
// difference between the two modes.
func (a *AccountLockout) RecordFailure(email string) {
	email = strings.ToLower(email)
	if a.valkey != nil {
		ctx := context.Background()
		n, err := a.valkey.Do(ctx, a.valkey.B().Incr().Key(a.attemptsKey(email)).Build()).ToInt64()
		if err != nil {
			slog.Warn("lockout: valkey INCR failed", "error", err)
			return
		}
		seconds := int64(a.lockoutDuration / time.Second)
		if n == 1 {
			// NX: set the decay TTL only when creating the counter, so repeated
			// failures don't extend the window.
			a.valkey.Do(ctx, a.valkey.B().Expire().Key(a.attemptsKey(email)).Seconds(seconds).Nx().Build())
		}
		if n >= int64(a.maxAttempts) {
			a.valkey.Do(ctx, a.valkey.B().Set().Key(a.lockedKey(email)).Value("1").ExSeconds(seconds).Build())
		}
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.attempts[email]++
	if a.attempts[email] >= a.maxAttempts {
		a.lockedUntil[email] = time.Now().Add(a.lockoutDuration)
	}
}

// RecordSuccess clears the failure count for an email (called on successful login).
func (a *AccountLockout) RecordSuccess(email string) {
	email = strings.ToLower(email)
	if a.valkey != nil {
		ctx := context.Background()
		// Two commands: a multi-key Del would cross hash slots and be rejected
		// by the valkey-go builder.
		a.valkey.Do(ctx, a.valkey.B().Del().Key(a.attemptsKey(email)).Build())
		a.valkey.Do(ctx, a.valkey.B().Del().Key(a.lockedKey(email)).Build())
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.attempts, email)
	delete(a.lockedUntil, email)
}

// RemainingAttempts returns the number of failed attempts remaining before lockout.
func (a *AccountLockout) RemainingAttempts(email string) int {
	email = strings.ToLower(email)
	if a.valkey != nil {
		n := int64(0)
		s, err := a.valkey.Do(context.Background(), a.valkey.B().Get().Key(a.attemptsKey(email)).Build()).ToString()
		if err != nil {
			// Miss (nil) or error → no recorded attempts. Parse errors are
			// impossible in practice: only INCR writes the counter.
			if !valkey.IsValkeyNil(err) {
				slog.Warn("lockout: valkey GET failed", "error", err)
			}
		} else if v, perr := strconv.ParseInt(s, 10, 64); perr == nil {
			n = v
		}
		remaining := a.maxAttempts - int(n)
		if remaining < 0 {
			return 0
		}
		return remaining
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	remaining := a.maxAttempts - a.attempts[email]
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ============================================================================
// Permission Engine (string-based RBAC)
// ============================================================================

// Permissions load from the DB at startup; the static map below is the
// fallback when there is no DB, the query fails, or the result is empty.

// rolePermissions maps system role_code -> permission_codes.
// Matches iam.roles + iam.permissions + iam.role_permissions in cco_app.
var rolePermissions = map[string][]string{
	"platform_admin": {"*:*"}, // wildcard handled in Can()
	"db_admin": {
		"cloud_accounts:*",
		"data_sources:*",
		"acquisitions:*",
		"resources:read",
		"data:read",
		"analytics:read",
		"recommendations:read",
		"reports:read",
		"organizations:read",
		"projects:read",
		"members:read",
	},
	"analyst": {
		"resources:read",
		"data:read",
		"analytics:read",
		"recommendations:read",
		"reports:read",
		"projects:read",
	},
	"viewer": {
		"resources:read",
		"data:read",
		"dashboards:read",
	},
	"collector": {
		"metrics:write",
		"acquisitions:run",
		"acquisitions:read",
		"config:read",
		"resources:read",
	},
	"ml_inference": {
		"features:read",
		"forecasts:write",
		"models:read",
	},
}

// membershipRolePermissions maps org membership role -> permission_codes.
var membershipRolePermissions = map[string][]string{
	"owner":  {"organizations:manage", "members:manage", "projects:manage", "cloud_accounts:*", "data_sources:*", "acquisitions:*", "resources:*", "data:read", "analytics:*", "recommendations:*", "reports:*"},
	"admin":  {"members:invite", "projects:manage", "cloud_accounts:*", "data_sources:read", "data_sources:write", "acquisitions:read", "acquisitions:run", "resources:read", "data:read", "analytics:read", "recommendations:read", "reports:read"},
	"member": {"projects:create", "data_sources:read", "acquisitions:read", "resources:read", "data:read", "analytics:read", "recommendations:read", "reports:read"},
	"viewer": {"resources:read", "acquisitions:read", "data:read", "dashboards:read"},
}

// PermissionEngine evaluates permissions for an IdentityContext.
type PermissionEngine struct {
	rolePermissions map[string][]string
}

// NewPermissionEngine creates a new PermissionEngine, loading system role
// permissions from iam.roles / iam.role_permissions / iam.permissions.
func NewPermissionEngine(db *gorm.DB) *PermissionEngine {
	return &PermissionEngine{rolePermissions: loadRolePermissions(db)}
}

// loadRolePermissions builds role_code -> permission_codes from the DB and
// falls back to the static rolePermissions map on any failure.
func loadRolePermissions(db *gorm.DB) map[string][]string {
	if db == nil {
		slog.Warn("permission engine: using static role map", "reason", "no database")
		return rolePermissions
	}
	var rows []struct {
		RoleCode       string
		PermissionCode string
	}
	err := db.WithContext(context.Background()).
		Table(database.TableName(db, "iam", "roles") + " r").
		Select("r.role_code, p.permission_code").
		Joins("JOIN " + database.TableName(db, "iam", "role_permissions") + " rp ON rp.role_id = r.role_id").
		Joins("JOIN " + database.TableName(db, "iam", "permissions") + " p ON p.permission_id = rp.permission_id").
		Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		reason := "query error"
		if err == nil {
			reason = "empty result"
		}
		slog.Warn("permission engine: using static role map", "reason", reason, "error", err)
		return rolePermissions
	}
	m := make(map[string][]string, len(rows))
	for _, r := range rows {
		m[r.RoleCode] = append(m[r.RoleCode], r.PermissionCode)
	}
	return m
}

// EffectivePermissions computes all permissions for an identity.
func (e *PermissionEngine) EffectivePermissions(ic IdentityContext) []string {
	permSet := make(map[string]struct{})

	// System roles
	for _, role := range ic.SystemRoles {
		if perms, ok := e.rolePermissions[role]; ok {
			for _, p := range perms {
				permSet[p] = struct{}{}
			}
		}
	}

	// Org membership role
	if perms, ok := membershipRolePermissions[ic.OrgRole]; ok {
		for _, p := range perms {
			permSet[p] = struct{}{}
		}
	}

	// Project role override (if set)
	if ic.ProjectRole != nil {
		if perms, ok := membershipRolePermissions[*ic.ProjectRole]; ok {
			for _, p := range perms {
				permSet[p] = struct{}{}
			}
		}
	}

	// Convert to slice
	perms := make([]string, 0, len(permSet))
	for p := range permSet {
		perms = append(perms, p)
	}
	return perms
}

// Can checks if identity has permission for resource:action.
// Supports wildcards: "resource:*", "*:*".
func (e *PermissionEngine) Can(ctx context.Context, ic IdentityContext, resource, action string) (bool, error) {
	required := resource + ":" + action
	requiredWildcard := resource + ":*"
	superWildcard := "*:*"

	perms := e.EffectivePermissions(ic)
	for _, p := range perms {
		if p == required || p == requiredWildcard || p == superWildcard {
			return true, nil
		}
	}
	return false, nil
}

// ============================================================================
// Membership Resolver
// ============================================================================

// MembershipResolver resolves org/project membership for an identity.
// Uses IdentityRepository to query cco_app.
type MembershipResolver struct {
	repo IdentityRepository
}

// NewMembershipResolver creates a new MembershipResolver.
func NewMembershipResolver(repo IdentityRepository) *MembershipResolver {
	return &MembershipResolver{repo: repo}
}

// ResolveOrgRole returns the user's membership role in an organization.
func (r *MembershipResolver) ResolveOrgRole(ctx context.Context, userID, orgID uuid.UUID) (string, error) {
	mem, err := r.repo.FindMembership(ctx, orgID, userID)
	if err != nil {
		if errors.Is(err, ErrNotMember) {
			return "", ErrNotMember
		}
		return "", err
	}
	if mem.Status != "active" {
		return "", ErrNotMember
	}
	return mem.MembershipRole, nil
}

// ResolveProjectRole returns the user's effective role in a project.
// Inherits from org membership unless project has explicit override (not modeled yet).
func (r *MembershipResolver) ResolveProjectRole(ctx context.Context, userID, projectID uuid.UUID) (string, error) {
	// Check project exists and user has access via org membership
	hasAccess, err := r.repo.CheckUserProjectAccess(ctx, userID, projectID)
	if err != nil {
		return "", err
	}
	if !hasAccess {
		return "", ErrNotProjectMember
	}

	// Get project to find org
	proj, err := r.repo.FindProjectByID(ctx, projectID)
	if err != nil {
		return "", err
	}

	// Inherit from org membership
	return r.ResolveOrgRole(ctx, userID, proj.OrganizationID)
}

// ResolvePermissions computes effective permissions for user in org/project context.
func (r *MembershipResolver) ResolvePermissions(ctx context.Context, userID uuid.UUID, orgID uuid.UUID, projectID *uuid.UUID) ([]string, error) {
	// Get system roles
	userRoles, err := r.repo.ListUserRolesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	var systemRoles []string
	for _, ur := range userRoles {
		role, err := r.repo.(RoleRepository).FindRoleByID(ctx, ur.RoleID)
		if err == nil {
			systemRoles = append(systemRoles, role.RoleCode)
		}
	}

	// Get org membership role
	orgRole := "viewer"
	if orgID != uuid.Nil {
		if role, err := r.ResolveOrgRole(ctx, userID, orgID); err == nil {
			orgRole = role
		}
	}

	// Get project role
	var projectRole *string
	if projectID != nil {
		if role, err := r.ResolveProjectRole(ctx, userID, *projectID); err == nil {
			projectRole = &role
		}
	}

	// Build temporary IdentityContext for PermissionEngine
	ic := IdentityContext{
		SystemRoles: systemRoles,
		OrgID:       orgID,
		OrgRole:     orgRole,
		ProjectID:   projectID,
		ProjectRole: projectRole,
		Permissions: []string{}, // will be computed
	}

	return NewPermissionEngine(nil).EffectivePermissions(ic), nil
}
