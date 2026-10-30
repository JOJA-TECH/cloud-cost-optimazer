package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/adapters"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/middleware"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
)

// compositeRepo satisfies iam.IdentityRepository by delegating to each sub-repo.
type compositeRepo struct {
	userRepo       iam.UserRepository
	roleRepo       iam.RoleRepository
	permRepo       iam.PermissionRepository
	userRoleRepo   iam.UserRoleRepository
	extIDRepo      iam.ExternalIdentityRepository
	membershipRepo iam.MembershipRepository
	projectRepo    iam.ProjectRepository
}

var _ iam.IdentityRepository = (*compositeRepo)(nil)

func (r *compositeRepo) CreateUser(ctx context.Context, input iam.CreateUserInput) (iam.UserDTO, error) {
	return r.userRepo.CreateUser(ctx, input)
}
func (r *compositeRepo) FindUserByID(ctx context.Context, id uuid.UUID) (iam.UserDTO, error) {
	return iam.UserDTO{}, nil
}
func (r *compositeRepo) FindUserByEmail(ctx context.Context, email string) (iam.UserDTO, error) {
	return r.userRepo.FindUserByEmail(ctx, email)
}
func (r *compositeRepo) FindPasswordHash(ctx context.Context, userID uuid.UUID) (string, error) {
	return r.userRepo.FindPasswordHash(ctx, userID)
}
func (r *compositeRepo) UpdateUser(ctx context.Context, id uuid.UUID, input iam.UpdateUserInput) (iam.UserDTO, error) {
	return iam.UserDTO{}, nil
}
func (r *compositeRepo) DeleteUser(ctx context.Context, id uuid.UUID) error { return nil }
func (r *compositeRepo) ListUsers(ctx context.Context, filter iam.UserListFilter) ([]iam.UserDTO, error) {
	return nil, nil
}
func (r *compositeRepo) FindRoleByCode(ctx context.Context, code string) (iam.RoleDTO, error) {
	return r.roleRepo.FindRoleByCode(ctx, code)
}
func (r *compositeRepo) FindRoleByID(ctx context.Context, id int16) (iam.RoleDTO, error) {
	return r.roleRepo.FindRoleByID(ctx, id)
}
func (r *compositeRepo) ListRoles(ctx context.Context) ([]iam.RoleDTO, error) {
	return r.roleRepo.ListRoles(ctx)
}
func (r *compositeRepo) FindPermissionByCode(ctx context.Context, code string) (iam.PermissionDTO, error) {
	return r.permRepo.FindPermissionByCode(ctx, code)
}
func (r *compositeRepo) FindPermissionByID(ctx context.Context, id int16) (iam.PermissionDTO, error) {
	return r.permRepo.FindPermissionByID(ctx, id)
}
func (r *compositeRepo) ListPermissions(ctx context.Context) ([]iam.PermissionDTO, error) {
	return r.permRepo.ListPermissions(ctx)
}
func (r *compositeRepo) ListPermissionsByRole(ctx context.Context, roleID int16) ([]iam.PermissionDTO, error) {
	return r.permRepo.ListPermissionsByRole(ctx, roleID)
}
func (r *compositeRepo) AssignRole(ctx context.Context, userID uuid.UUID, roleID int16, assignedBy *uuid.UUID) error {
	return r.userRoleRepo.AssignRole(ctx, userID, roleID, assignedBy)
}
func (r *compositeRepo) RevokeRole(ctx context.Context, userID uuid.UUID, roleID int16) error {
	return r.userRoleRepo.RevokeRole(ctx, userID, roleID)
}
func (r *compositeRepo) ListUserRolesByUser(ctx context.Context, userID uuid.UUID) ([]iam.UserRoleDTO, error) {
	return r.userRoleRepo.ListUserRolesByUser(ctx, userID)
}
func (r *compositeRepo) ListUserRolesByRole(ctx context.Context, roleID int16) ([]iam.UserRoleDTO, error) {
	return r.userRoleRepo.ListUserRolesByRole(ctx, roleID)
}
func (r *compositeRepo) LinkExternalIdentity(ctx context.Context, input iam.CreateExternalIdentityInput) (iam.ExternalIdentityDTO, error) {
	return r.extIDRepo.LinkExternalIdentity(ctx, input)
}
func (r *compositeRepo) FindExternalIdentityByProviderSubject(ctx context.Context, provider, subject string) (iam.ExternalIdentityDTO, error) {
	return r.extIDRepo.FindExternalIdentityByProviderSubject(ctx, provider, subject)
}
func (r *compositeRepo) FindExternalIdentitiesByUser(ctx context.Context, userID uuid.UUID) ([]iam.ExternalIdentityDTO, error) {
	return r.extIDRepo.FindExternalIdentitiesByUser(ctx, userID)
}
func (r *compositeRepo) UpdateExternalIdentityLastSeen(ctx context.Context, id uuid.UUID) error {
	return r.extIDRepo.UpdateExternalIdentityLastSeen(ctx, id)
}
func (r *compositeRepo) AddMember(ctx context.Context, input iam.AddMemberInput) (iam.MembershipDTO, error) {
	return r.membershipRepo.AddMember(ctx, input)
}
func (r *compositeRepo) UpdateMember(ctx context.Context, orgID, userID uuid.UUID, input iam.UpdateMemberInput) (iam.MembershipDTO, error) {
	return r.membershipRepo.UpdateMember(ctx, orgID, userID, input)
}
func (r *compositeRepo) RemoveMember(ctx context.Context, orgID, userID uuid.UUID) error {
	return r.membershipRepo.RemoveMember(ctx, orgID, userID)
}
func (r *compositeRepo) ListMembershipsByOrg(ctx context.Context, orgID uuid.UUID) ([]iam.MembershipDTO, error) {
	return r.membershipRepo.ListMembershipsByOrg(ctx, orgID)
}
func (r *compositeRepo) ListMembershipsByUser(ctx context.Context, userID uuid.UUID) ([]iam.MembershipDTO, error) {
	return r.membershipRepo.ListMembershipsByUser(ctx, userID)
}
func (r *compositeRepo) FindMembership(ctx context.Context, orgID, userID uuid.UUID) (iam.MembershipDTO, error) {
	return r.membershipRepo.FindMembership(ctx, orgID, userID)
}
func (r *compositeRepo) FindProjectByID(ctx context.Context, id uuid.UUID) (iam.ProjectDTO, error) {
	return r.projectRepo.FindProjectByID(ctx, id)
}
func (r *compositeRepo) ListProjectsByOrg(ctx context.Context, orgID uuid.UUID) ([]iam.ProjectDTO, error) {
	return r.projectRepo.ListProjectsByOrg(ctx, orgID)
}
func (r *compositeRepo) CheckUserProjectAccess(ctx context.Context, userID, projectID uuid.UUID) (bool, error) {
	return r.projectRepo.CheckUserProjectAccess(ctx, userID, projectID)
}

func main() {
	ctx := context.Background()

	// ── Database ──────────────────────────────────────────────────────────
	dsn := os.Getenv("CCO_DB_DSN")
	if dsn == "" {
		dsn = "file::memory:?cache=shared" // dev: SQLite in-memory
	}
	driver := database.Driver(os.Getenv("CCO_DB_DRIVER"))
	if driver == "" {
		driver = database.DriverSQLite
	}

	db, err := database.Open(database.Config{
		Driver:   driver,
		DSN:      dsn,
		LogLevel: 0,
	})
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	// ── Logging ───────────────────────────────────────────────────────────
	// Stdout es la interfaz hacia cualquier colector externo (Loki, ELK,
	// Fluentd). No hay dependencia extra; log/slog es estándar desde Go 1.21.
	// shortcut: un solo handler global; upgrade: handler por módulo con
	// niveles diferentes si el volumen de logs lo justifica.
	var h slog.Handler
	if os.Getenv("CCO_LOG_FORMAT") == "json" {
		h = slog.NewJSONHandler(os.Stdout, nil)
	} else {
		h = slog.NewTextHandler(os.Stdout, nil)
	}
	slog.SetDefault(slog.New(h))
	slog.Info("cco-app logging initialized", "format", os.Getenv("CCO_LOG_FORMAT"))

	// ── Token manager ────────────────────────────────────────────────────
	signingKey := []byte(os.Getenv("CCO_JWT_SIGNING_KEY"))
	if len(signingKey) == 0 {
		signingKey = []byte("dev-secret-change-in-production") // shortcut: dev-only default; upgrade: require env var in prod
	}
	tokenConfig := iam.TokenConfig{
		SigningKey:      signingKey,
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		Issuer:          "cco-app",
		Audience:        "cco-api",
	}
	tokenMgr := iam.NewTokenManager(tokenConfig)

	// ── Repositories ─────────────────────────────────────────────────────
	userRepo := iam.NewUserRepository(db)
	repo := &compositeRepo{
		userRepo:       userRepo,
		roleRepo:       repositories.NewRoleRepository(db),
		permRepo:       repositories.NewPermissionRepository(db),
		userRoleRepo:   repositories.NewUserRoleRepository(db),
		extIDRepo:      repositories.NewExternalIdentityRepository(db),
		membershipRepo: repositories.NewMembershipRepository(db),
		projectRepo:    repositories.NewProjectRepository(db),
	}

	// ── Authorization engine ─────────────────────────────────────────────
	permEngine := iam.NewPermissionEngine()

	// ── Adapters ─────────────────────────────────────────────────────────
	oidcCfg := adapters.OIDCConfig{
		IssuerURL:    os.Getenv("CCO_OIDC_ISSUER_URL"),
		ClientID:     os.Getenv("CCO_OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("CCO_OIDC_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("CCO_OIDC_REDIRECT_URL"),
		TokenManager: tokenMgr,
		Repo:         repo,
		CookieDomain: os.Getenv("CCO_COOKIE_DOMAIN"),
		CookieSecure: os.Getenv("CCO_COOKIE_SECURE") == "true",
	}
	var oidcAdapter *adapters.OIDCAdapter
	if oidcCfg.IssuerURL != "" && oidcCfg.ClientID != "" {
		var err error
		oidcAdapter, err = adapters.NewOIDCAdapter(ctx, oidcCfg)
		if err != nil {
			log.Printf("oidc adapter disabled: %v", err)
		}
	}

	spCfg := adapters.SPConfig{
		TokenManager:  tokenMgr,
		Repo:          repo,
		Clients:       map[string]string{}, // populated from env in production
		AllowedScopes: map[string][]string{},
	}
	spAdapter := adapters.NewServicePrincipalAdapter(spCfg)

	// ── Auth middleware ──────────────────────────────────────────────────
	authMiddleware := middleware.NewAuthMiddleware(tokenMgr, repo)
	authMiddleware.AllowAnonymous("/healthz", "/ready")

	// ── Router ───────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Public
	mux.Handle("GET /", iam.RootHandler{})
	mux.Handle("GET /healthz", iam.HealthHandler{})

	// Auth (password dev, OIDC, M2M)
	mux.Handle("POST /auth/register", iam.NewRegisterHandler(userRepo, tokenMgr))
	mux.Handle("POST /auth/login", iam.NewLoginHandler(
		adapters.NewPasswordAdapter(repo, tokenMgr),
		tokenMgr,
	))
	if oidcAdapter != nil {
		mux.Handle("GET /auth/oidc/login", iam.NewOIDCLoginHandler(oidcAdapter, "dev-state"))
		mux.Handle("GET /auth/oidc/callback", &iam.OIDCCallbackHandler{})
	}
	mux.Handle("POST /auth/m2m/token", iam.NewM2MTokenHandler(spAdapter, tokenMgr))
	mux.Handle("POST /auth/token/refresh", iam.NewTokenRefreshHandler(tokenMgr, userRepo))

	// Protected example: list users (requires platform_admin)
	mux.Handle("GET /api/v1/users",
		authMiddleware.Handler(
			middleware.RequireRole("platform_admin")(
				http.HandlerFunc(listUsers(iam.NewUserRepository(db))),
			),
		),
	)

	// Protected example: check permission (requires any authenticated identity)
	mux.Handle("POST /api/v1/can",
		authMiddleware.Handler(
			middleware.RequirePermission(permEngine, "resources", "read")(
				http.HandlerFunc(canHandler(permEngine)),
			),
		),
	)

	// ── Security middleware ────────────────────────────────────────────────
	// CORS wraps all routes; rate limiting applies globally (simple, dev-ready).
	// shortcut: in-memory rate limiter resets on restart; upgrade to Redis-backed
	// limiter when running multiple instances.
	handler := middleware.CORS(mux)
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.RateLimitMiddleware(60, time.Minute, handler)

	// ── Server ───────────────────────────────────────────────────────────
	addr := os.Getenv("CCO_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("cco-app listening on %s (driver=%s)", addr, driver)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

func listUsers(repo iam.UserRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ic, ok := iam.IdentityContextFromContext(r.Context())
		if !ok {
			http.Error(w, `{"code":"UNAUTHORIZED","message":"no identity context"}`, http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"message":     fmt.Sprintf("authenticated as %s (org_role=%s)", ic.Email, ic.OrgRole),
			"user_id":     ic.UserID,
			"org_id":      ic.OrgID,
			"org_role":    ic.OrgRole,
			"permissions": ic.Permissions,
		})
	}
}

func canHandler(engine *iam.PermissionEngine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ic, ok := iam.IdentityContextFromContext(r.Context())
		if !ok {
			http.Error(w, `{"code":"UNAUTHORIZED","message":"no identity context"}`, http.StatusUnauthorized)
			return
		}
		allowed, err := engine.Can(r.Context(), ic, "resources", "read")
		if err != nil {
			http.Error(w, `{"code":"INTERNAL_ERROR","message":"permission check failed"}`, http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"allowed":  allowed,
			"user_id":  ic.UserID,
			"resource": "resources",
			"action":   "read",
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
