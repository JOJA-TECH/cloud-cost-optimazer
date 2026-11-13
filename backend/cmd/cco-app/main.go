package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	acqadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition/adapters"
	acqrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics"
	analyticsadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics/adapters"
	analyticsrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	cloudadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud/adapters"
	cloudrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
	datarepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/adapters"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/middleware"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/valkey"
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
	// Decisión: un solo handler global a stdout basta a escala prototipo;
	// handler por módulo solo si el volumen de logs lo justifica.
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
		if os.Getenv("CCO_ENV") == "prod" {
			log.Fatalf("CCO_JWT_SIGNING_KEY is required when CCO_ENV=prod")
		}
		// Dev-only default so local runs without .env still boot.
		signingKey = []byte("dev-secret-change-in-production")
	}
	tokenConfig := iam.TokenConfig{
		SigningKey:      signingKey,
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		Issuer:          "cco-app",
		Audience:        "cco-api",
	}
	tokenMgr := iam.NewTokenManager(tokenConfig, db)

	// ── Valkey (shared state for rate limiting + lockout) ────────────────
	// nil client = single-instance dev mode; each component falls back to memory.
	vc, err := valkey.NewFromEnv()
	if err != nil {
		log.Fatalf("valkey: %v", err)
	}
	lockout := iam.NewAccountLockout(5, 15*time.Minute, vc)

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
	permEngine := iam.NewPermissionEngine(db)

	// ── Cloud module (Module 2) ──────────────────────────────────────────
	accountRepo := cloudrepos.NewAccountRepository(db)
	resourceRepo := cloudrepos.NewResourceRepository(db)
	bindingRepo := cloudrepos.NewDataSourceRepository(db)

	// Adapter selection: default to the fake adapter; the real Azure adapter
	// is added once credentials are available.
	var validator cloud.ConnectionValidator
	var provider cloud.ResourceProvider
	mode := os.Getenv("CCO_CLOUD_PROVIDER_MODE")
	if mode == "azure" {
		validator = cloudadapters.NewAzureAdapter()
		provider = cloudadapters.NewAzureAdapter()
	} else {
		validator = cloudadapters.NewFakeAdapter()
		provider = cloudadapters.NewFakeAdapter()
	}
	cloudSvc := cloud.NewService(accountRepo, resourceRepo, bindingRepo, validator, provider)

	// ── Acquisition module (Module 3) ────────────────────────────────────
	// Targets and credentials come from cco_app; telemetry lands in cco_data
	// through the Gestión de Datos contracts (internal/data — ADR-002: no
	// cross-database FKs).
	var acqWriter data.DataWriter
	var acqRuns data.RunReader
	var dataSvc *data.Service
	var analyticsRepo *analyticsrepos.AnalyticsRepository
	var analyticsSvc *analytics.Service
	if dataDSN := os.Getenv("CCO_DATA_DSN"); dataDSN != "" {
		dataDB, err := database.Open(database.Config{Driver: database.DriverPostgres, DSN: dataDSN})
		if err != nil {
			log.Fatalf("open cco_data database: %v", err)
		}
		dataRepo := datarepos.NewDataRepository(dataDB)
		acqWriter = dataRepo
		acqRuns = dataRepo
		dataSvc = data.NewService(dataRepo)

		// Analytics module (Module 5)
		analyticsRepo = analyticsrepos.NewAnalyticsRepository(dataDB)
		var mlClient analytics.MLPort
		if mode == "azure" {
			// Real ML client would go here; for now fake
			mlClient = analyticsadapters.NewFakeMLClient()
		} else {
			mlClient = analyticsadapters.NewFakeMLClient()
		}
		analyticsSvc = analytics.NewService(analyticsRepo, dataRepo, dataRepo, dataRepo, mlClient, 4)
	}
	acqTargets := acqrepos.NewTargetRepository(db)

	var metricsProvider acquisition.MetricsProvider
	var diagProvider acquisition.DiagnosticsProvider
	var qstoreProvider acquisition.QueryStoreProvider
	var configProvider acquisition.ConfigProvider
	var costProvider acquisition.CostProvider
	var sampleProvider acquisition.SampleProvider
	if mode == "azure" {
		metricsProvider = acqadapters.NewAzureMonitorProvider()
		sqlDiag := acqadapters.NewSQLServerDiagnostics()
		diagProvider = sqlDiag
		qstoreProvider = sqlDiag
		configProvider = acqadapters.NewAzureConfigProvider()
		costProvider = acqadapters.NewAzureCostProvider()
		// Azure Monitor PT1M does not expose individual samples; SampleProvider
		// would be a no-op. Query Store samples come via FakeSample in dev.
		sampleProvider = nil
	} else {
		metricsProvider = acqadapters.NewFakeMonitor()
		diagProvider = acqadapters.NewFakeDiagnostics()
		qstoreProvider = acqadapters.NewFakeQueryStore()
		configProvider = acqadapters.NewFakeConfig()
		costProvider = acqadapters.NewFakeCost()
		sampleProvider = acqadapters.NewFakeSample()
	}
	acqSvc := acquisition.NewService(acqTargets, acqWriter, acqRuns, metricsProvider, diagProvider, qstoreProvider, configProvider, costProvider, sampleProvider, acquisition.Config{
		MonitorDelay:     envSeconds("CCO_ACQ_MONITOR_DELAY_SECONDS", 90*time.Second),
		MonitorLookback:  envSeconds("CCO_ACQ_MONITOR_LOOKBACK_SECONDS", 300*time.Second),
		CostLookbackDays: envDays("CCO_ACQ_COST_LOOKBACK_DAYS", 3*24*time.Hour),
		SQLUser:          os.Getenv("CCO_ACQ_SQL_USER"),
		SQLPassword:      os.Getenv("CCO_ACQ_SQL_PASSWORD"),
	})
	if acqWriter != nil {
		go acquisition.NewScheduler(acqSvc, envSeconds("CCO_ACQ_INTERVAL_SECONDS", 60*time.Second)).Run(context.Background())

		// Cost scheduler (T5): separate ticker, runs per-subscription.
		go func() {
			ticker := time.NewTicker(envSeconds("CCO_ACQ_COST_INTERVAL_SECONDS", 6*time.Hour))
			defer ticker.Stop()
			slog.Info("cost scheduler started", "interval", envSeconds("CCO_ACQ_COST_INTERVAL_SECONDS", 6*time.Hour))
			for {
				select {
				case <-ctx.Done():
					slog.Info("cost scheduler stopped")
					return
				case <-ticker.C:
					_, err := acqSvc.RunCostCycle(ctx, nil)
					if err != nil {
						if err == acquisition.ErrStorageUnavailable {
							slog.Warn("cost cycle skipped: cco_data storage not configured")
							continue
						}
						slog.Warn("cost cycle failed", "error", err.Error())
					}
				}
			}
		}()
	}

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
		lockout,
	))
	if oidcAdapter != nil {
		mux.Handle("GET /auth/oidc/login", iam.NewOIDCLoginHandler(oidcAdapter, oidcCfg.CookieDomain, oidcCfg.CookieSecure))
		mux.Handle("GET /auth/oidc/callback", iam.NewOIDCCallbackHandler(oidcAdapter, tokenMgr, oidcCfg.CookieDomain, oidcCfg.CookieSecure))
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

	// Cloud routes (Module 2)
	cloud.RegisterRoutes(mux, cloudSvc, authMiddleware, permEngine)

	// Acquisition routes (Module 3)
	acquisition.RegisterRoutes(mux, acqSvc, authMiddleware, permEngine)

	// Data routes (Module 4): read-only queries over cco_data. nil service =
	// CCO_DATA_DSN not configured → endpoints answer 503.
	data.RegisterRoutes(mux, dataSvc, authMiddleware, permEngine)

	// Analytics routes (Module 5): analytical evidence endpoints. nil service =
	// CCO_DATA_DSN not configured → endpoints answer 503.
	analytics.RegisterRoutes(mux, analyticsSvc, authMiddleware, permEngine)

	// ── Security middleware ────────────────────────────────────────────────
	// CORS wraps all routes; rate limiting and lockout share the Valkey client
	// (in-memory fallbacks when CCO_VALKEY_ADDR is unset).
	var allowedOrigins []string
	if v := os.Getenv("CCO_ALLOWED_ORIGINS"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}
	handler := middleware.CORS(allowedOrigins, mux)
	handler = middleware.SecurityHeaders(os.Getenv("CCO_CSP"), handler)
	handler = middleware.RateLimitMiddleware(60, time.Minute, vc, handler)

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
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    iam.CodeUnauthorized,
				"message": "no identity context",
			})
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
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"code":    iam.CodeUnauthorized,
				"message": "no identity context",
			})
			return
		}
		allowed, err := engine.Can(r.Context(), ic, "resources", "read")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"code":    "INTERNAL_ERROR",
				"message": "permission check failed",
			})
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

// envDays parses a positive integer of days from the environment falling
// back to def on missing or invalid values.
func envDays(name string, def time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * 24 * time.Hour
		}
	}
	return def
}

// envSeconds parses a positive integer of seconds from the environment
// falling back to def on missing or invalid values.
func envSeconds(name string, def time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return def
}
