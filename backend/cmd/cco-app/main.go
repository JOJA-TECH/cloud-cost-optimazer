package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	acqadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition/adapters"
	acqrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics"
	analyticsadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics/adapters"
	analyticsrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	auditadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit/adapters"
	auditrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	cloudadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud/adapters"
	cloudrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
	datarepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/adapters"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/middleware"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/jobs"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/valkey"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	recommendationadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation/adapters"
	recommendationrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	reportingadapters "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting/adapters"
	reportingrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting/repositories"
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
	return r.userRepo.FindUserByID(ctx, id)
}
func (r *compositeRepo) FindUserByEmail(ctx context.Context, email string) (iam.UserDTO, error) {
	return r.userRepo.FindUserByEmail(ctx, email)
}
func (r *compositeRepo) FindPasswordHash(ctx context.Context, userID uuid.UUID) (string, error) {
	return r.userRepo.FindPasswordHash(ctx, userID)
}
func (r *compositeRepo) UpdateUser(ctx context.Context, id uuid.UUID, input iam.UpdateUserInput) (iam.UserDTO, error) {
	return r.userRepo.UpdateUser(ctx, id, input)
}
func (r *compositeRepo) DeleteUser(ctx context.Context, id uuid.UUID) error {
	return r.userRepo.DeleteUser(ctx, id)
}
func (r *compositeRepo) ListUsers(ctx context.Context, filter iam.UserListFilter) ([]iam.UserDTO, error) {
	return r.userRepo.ListUsers(ctx, filter)
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
	if err := iam.SeedRecommendationPermissions(db); err != nil {
		log.Fatalf("seed recommendation permissions: %v", err)
	}
	if err := iam.SeedAuditPermissions(db); err != nil {
		log.Fatalf("seed audit permissions: %v", err)
	}
	if err := iam.SeedReportPermissions(db); err != nil {
		log.Fatalf("seed report permissions: %v", err)
	}
	if err := iam.SeedJobsPermissions(db); err != nil {
		log.Fatalf("seed jobs permissions: %v", err)
	}

	// ── Jobs (durable queue, Fase 3 del roadmap) ─────────────────────────
	jobRepo := jobs.NewRepository(db)
	jsvc := jobs.NewService(jobRepo)
	jw := jobs.NewWorker(jobRepo, 5*time.Second)

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
	auditRepo := auditrepos.NewAuditRepository(db)
	auditService, err := audit.NewService(auditRepo, auditadapters.NewSlogAuditLogger(slog.Default()))
	if err != nil {
		log.Fatalf("initialize audit service: %v", err)
	}

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
	var recommendationSvc *recommendation.Service
	var mlClient analytics.MLPort
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
		mlMode := os.Getenv("CCO_ML_MODE")
		switch mlMode {
		case "baseline":
			// Deterministic baseline (fake client) for local/dev validation.
			mlClient = analyticsadapters.NewFakeMLClient()
		case "not_available":
			// Explicitly not available: never present a fake forecast as productive.
			mlClient = analyticsadapters.NewNotAvailableMLClient()
		case "grpc":
			// Productive gRPC client to the external Python ML service (Module 6).
			mlEndpoint := os.Getenv("CCO_ML_ENDPOINT")
			mlTimeout := envSeconds("CCO_ML_REQUEST_TIMEOUT_SECONDS", 10*time.Second)
			grpcClient, err := analyticsadapters.NewGRPCMLClient(mlEndpoint, mlTimeout)
			if err != nil {
				log.Fatalf("connect ML endpoint: %v", err)
			}
			defer grpcClient.Close()
			mlClient = grpcClient
		default:
			// Default (dev/unset): fake client.
			mlClient = analyticsadapters.NewFakeMLClient()
		}
		analyticsSvc = analytics.NewService(analyticsRepo, dataRepo, dataRepo, dataRepo, mlClient, 4)

		// Recommendation Engine (Module 7): consumes Module 4/5 contracts and
		// persists decisions in cco_data. It never executes infrastructure changes.
		recommendationRepo := recommendationrepos.NewRecommendationRepository(dataDB)
		dataReaders := recommendationadapters.NewDataReaders(dataRepo, dataRepo, dataRepo, dataRepo, dataRepo)
		analyticsReader := recommendationadapters.NewAnalyticsReader(analyticsRepo)
		var mlReader recommendation.MLPredictionReader = analyticsReader
		var costReader recommendation.CostReader = dataReaders
		var configurationReader recommendation.ConfigurationReader = dataReaders
		var priceReader recommendation.PriceReader = dataReaders
		var skuReader recommendation.SKUReader = dataReaders
		var resourceContextReader recommendation.ResourceContextReader = dataReaders
		recommendationSvc, err = recommendation.NewEngine(recommendation.Dependencies{
			PolicyEvaluator:  recommendationadapters.NewPolicyEvaluator(),
			PolicyRepository: recommendationRepo,
			Repository:       recommendationRepo,
			Analytics:        analyticsReader,
			Forecasts:        mlReader,
			Costs:            costReader,
			Configurations:   configurationReader,
			ResourceContext:  resourceContextReader,
			Prices:           priceReader,
			SKUs:             skuReader,
			Audit: recommendationadapters.NewPersistentAuditSink(
				auditService, auditService, slog.Default(),
			),
		})
		if err != nil {
			log.Fatalf("initialize recommendation engine: %v", err)
		}
	}
	var reportService *reporting.Service
	if analyticsRepo != nil && recommendationSvc != nil {
		reportsDirectory := os.Getenv("CCO_REPORTS_DIR")
		if reportsDirectory == "" {
			reportsDirectory = "var/reports"
		}
		reportStorage, err := reportingadapters.NewLocalFilesystemObjectStorage(reportsDirectory)
		if err != nil {
			log.Fatalf("initialize report artifact storage: %v", err)
		}
		reportExporter := reportingadapters.NewObjectStorageExportAdapter(reportStorage)
		dataProvider := reportingadapters.NewCompositeReportDataProvider(
			reportingadapters.NewAnalyticsReportReader(analyticsRepo),
			reportingadapters.NewRecommendationReportReader(recommendationSvc),
			reportingadapters.NewAuditTraceReader(auditService),
		)
		reportService, err = reporting.NewService(reporting.Dependencies{
			DataProvider:  dataProvider,
			Repository:    reportingrepos.NewReportRepository(db),
			ResourceScope: reportingrepos.NewResourceProjectRepository(db),
			Formatters: []reporting.ReportFormatter{
				reportingadapters.NewJSONFormatter(),
				reportingadapters.NewCSVFormatter(),
				reportingadapters.NewHTMLFormatter(),
			},
			Exporter: reportExporter,
			Audit:    reportingadapters.NewAuditSink(auditService),
		})
		if err != nil {
			log.Fatalf("initialize reporting service: %v", err)
		}
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
	// Handler del worker para jobs de adquisición (tipo "acquisition:run").
	// Ejecuta RunCycle para la organización (y recurso opcional) del payload.
	jw.RegisterHandler("acquisition:run", func(ctx context.Context, payload []byte) error {
		if acqSvc == nil {
			return errors.New("acquisition service not configured (CCO_DATA_DSN missing)")
		}
		var parsed struct {
			OrgID       *string `json:"org_id"`
			ResourceUID *string `json:"resource_uid"`
		}
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return err
		}
		var orgID *uuid.UUID
		if parsed.OrgID != nil && *parsed.OrgID != "" {
			id, err := uuid.Parse(*parsed.OrgID)
			if err != nil {
				return err
			}
			orgID = &id
		}
		var resourceUID *uuid.UUID
		if parsed.ResourceUID != nil && *parsed.ResourceUID != "" {
			id, err := uuid.Parse(*parsed.ResourceUID)
			if err != nil {
				return err
			}
			resourceUID = &id
		}
		_, err := acqSvc.RunCycle(ctx, orgID, resourceUID)
		return err
	})

	// report:optimization ejecuta la generación de reporte ya encolada en
	// app.report_requests (vía QueueOptimizationReport) usando el worker
	// duradero de jobs.
	jw.RegisterHandler("report:optimization", func(ctx context.Context, payload []byte) error {
		if reportService == nil {
			return errors.New("reporting service not configured (CCO_DATA_DSN missing)")
		}
		var parsed struct {
			ReportJobUID string `json:"report_job_uid"`
		}
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return err
		}
		uid, err := uuid.Parse(parsed.ReportJobUID)
		if err != nil {
			return err
		}
		return reportService.ProcessJob(ctx, uid)
	})

	// analytics:compute ejecuta una operación analítica pesada de forma
	// asíncrona sin bloquear la API HTTP.
	jw.RegisterHandler("analytics:compute", func(ctx context.Context, payload []byte) error {
		if analyticsSvc == nil {
			return errors.New("analytics service not configured (CCO_DATA_DSN missing)")
		}
		var parsed struct {
			Operation    string `json:"operation"`
			ResourceUID  string `json:"resource_uid"`
			From         string `json:"from"`
			To           string `json:"to"`
			MetricCode   string `json:"metric_code,omitempty"`
			PeriodType   string `json:"period_type,omitempty"`
			HorizonSteps int    `json:"horizon_steps,omitempty"`
		}
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return err
		}
		resourceUID, err := uuid.Parse(parsed.ResourceUID)
		if err != nil {
			return err
		}
		from, err := time.Parse(time.RFC3339, parsed.From)
		if err != nil {
			return err
		}
		to, err := time.Parse(time.RFC3339, parsed.To)
		if err != nil {
			return err
		}
		switch parsed.Operation {
		case "percentiles":
			if parsed.MetricCode == "" {
				return errors.New("metric_code is required for percentiles")
			}
			_, err = analyticsSvc.ComputePercentiles(ctx, resourceUID, parsed.MetricCode, from, to, parsed.PeriodType)
		case "feature_snapshot":
			_, err = analyticsSvc.ComputeFeatureSnapshot(ctx, resourceUID, from, to)
		case "trends":
			_, err = analyticsSvc.ComputeTrends(ctx, resourceUID, from, to)
		case "patterns":
			_, err = analyticsSvc.DetectWorkloadPatterns(ctx, resourceUID, from, to)
		case "capacity":
			_, err = analyticsSvc.AnalyzeCapacity(ctx, resourceUID, parsed.PeriodType, from)
		case "costs":
			_, err = analyticsSvc.AnalyzeCosts(ctx, resourceUID, from, to)
		case "forecast":
			_, err = analyticsSvc.RequestForecast(ctx, analytics.ForecastRequest{
				ResourceUID:    resourceUID,
				TargetMetric:   parsed.MetricCode,
				HorizonSteps:   parsed.HorizonSteps,
				StepUnit:       "1h",
				PeriodStartUTC: from,
			})
		default:
			return errors.New("unknown analytics operation: " + parsed.Operation)
		}
		return err
	})

	// ml:train entrena un modelo candidato vía el servicio Python y lo
	// registra en ml.training_run / ml.model_version. Invocado por Jobs.
	jw.RegisterHandler("ml:train", func(ctx context.Context, payload []byte) error {
		if analyticsSvc == nil {
			return errors.New("analytics service not configured (CCO_DATA_DSN missing)")
		}
		mlMgmt, ok := mlClient.(*analyticsadapters.GrpcMLClient)
		if !ok {
			return errors.New("ML client is not a gRPC client; set CCO_ML_MODE=grpc")
		}
		var parsed struct {
			ResourceUID  string `json:"resource_uid"`
			TargetMetric string `json:"target_metric"`
			ModelName    string `json:"model_name"`
			ModelVersion string `json:"model_version"`
			Algorithm    string `json:"algorithm"`
			Hours        int    `json:"hours"`
		}
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return err
		}
		resourceUID, err := uuid.Parse(parsed.ResourceUID)
		if err != nil {
			return err
		}
		if parsed.ModelName == "" {
			return errors.New("model_name is required")
		}
		if parsed.ModelVersion == "" {
			return errors.New("model_version is required")
		}
		if parsed.Hours <= 0 {
			parsed.Hours = 720
		}
		if parsed.Algorithm == "" {
			parsed.Algorithm = "random_forest"
		}
		resp, err := mlMgmt.Train(ctx, resourceUID, parsed.TargetMetric, parsed.ModelName,
			parsed.ModelVersion, parsed.Algorithm, int32(parsed.Hours))
		if err != nil {
			return err
		}
		if resp.Status != "ok" {
			return fmt.Errorf("training failed: %s", resp.ErrorMessage)
		}
		slog.Info("ml:train completed", "resource_uid", resourceUID, "target", parsed.TargetMetric,
			"model", resp.ModelName, "version", resp.ModelVersion, "mae", resp.Mae)
		return nil
	})

	// recommendation:generate genera una recomendación pesada de forma
	// asíncrona y la persiste en cco_data.
	jw.RegisterHandler("recommendation:generate", func(ctx context.Context, payload []byte) error {
		if recommendationSvc == nil {
			return errors.New("recommendation service not configured (CCO_DATA_DSN missing)")
		}
		var parsed struct {
			ResourceUID  string `json:"resource_uid"`
			PolicyUID    string `json:"policy_uid"`
			From         string `json:"from"`
			To           string `json:"to"`
			HorizonDays  int    `json:"horizon_days"`
			Currency     string `json:"currency"`
			ForceRefresh bool   `json:"force_refresh"`
			CreatedBy    string `json:"created_by"`
		}
		if err := json.Unmarshal(payload, &parsed); err != nil {
			return err
		}
		resourceUID, err := uuid.Parse(parsed.ResourceUID)
		if err != nil {
			return err
		}
		policyUID, err := uuid.Parse(parsed.PolicyUID)
		if err != nil {
			return err
		}
		from, err := time.Parse(time.RFC3339, parsed.From)
		if err != nil {
			return err
		}
		to, err := time.Parse(time.RFC3339, parsed.To)
		if err != nil {
			return err
		}
		_, err = recommendationSvc.GenerateRecommendation(ctx, recommendation.GenerateRecommendationInput{
			ResourceUID: resourceUID, PolicyUID: policyUID,
			From: from, To: to, HorizonDays: parsed.HorizonDays,
			Currency: strings.ToUpper(parsed.Currency), ForceRefresh: parsed.ForceRefresh,
			CreatedBy: parsed.CreatedBy,
		})
		return err
	})

	// wg tracks background workers/schedulers so shutdown can wait for them
	// to finish (bounded by CCO_SHUTDOWN_TIMEOUT_SEC).
	var wg sync.WaitGroup

	// Shutdown context: cancelled on SIGINT/SIGTERM; shared by the
	// acquisition/cost schedulers, the jobs worker and the HTTP server for
	// orderly shutdown.
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	wg.Add(1)
	go func() {
		defer wg.Done()
		jw.Run(shutdownCtx)
	}()

	if acqWriter != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			acquisition.NewScheduler(acqSvc, envSeconds("CCO_ACQ_INTERVAL_SECONDS", 60*time.Second)).Run(shutdownCtx)
		}()

		// Cost scheduler (T5): separate ticker, runs per-subscription.
		wg.Add(1)
		go func() {
			defer wg.Done()
			ticker := time.NewTicker(envSeconds("CCO_ACQ_COST_INTERVAL_SECONDS", 6*time.Hour))
			defer ticker.Stop()
			slog.Info("cost scheduler started", "interval", envSeconds("CCO_ACQ_COST_INTERVAL_SECONDS", 6*time.Hour))
			for {
				select {
				case <-shutdownCtx.Done():
					slog.Info("cost scheduler stopped")
					return
				case <-ticker.C:
					_, err := acqSvc.RunCostCycle(shutdownCtx, nil)
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

	// M2M clients are loaded from CCO_M2M_CLIENTS:
	//   client_id:argon2id_hash:scope1,scope2|client_id2:hash2:scope3
	// Empty in production means the /auth/m2m/token endpoint fails closed
	// (no client can authenticate).
	spClients, spScopes := parseM2MClients(os.Getenv("CCO_M2M_CLIENTS"))
	if os.Getenv("CCO_ENV") == "prod" && len(spClients) == 0 {
		log.Fatalf("CCO_M2M_CLIENTS is required when CCO_ENV=prod")
	}
	// Production hardening: require secure cookies in prod.
	if os.Getenv("CCO_ENV") == "prod" && os.Getenv("CCO_COOKIE_SECURE") != "true" {
		log.Fatalf("CCO_COOKIE_SECURE=true is required when CCO_ENV=prod")
	}

	spCfg := adapters.SPConfig{
		TokenManager:  tokenMgr,
		Repo:          repo,
		Clients:       spClients,
		AllowedScopes: spScopes,
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
	mux.Handle("GET /readyz", iam.ReadyzHandler{
		DB:        db,
		MigrateOK: func() bool { return database.SchemaMigrationOK(db) },
	})

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

	// GET /api/v1/ml/models lists registered model versions (ML management).
	// Requires analytics:read.
	mux.Handle("GET /api/v1/ml/models",
		authMiddleware.Handler(middleware.RequirePermission(permEngine, "analytics", "read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if analyticsRepo == nil {
				writeError(w, http.StatusServiceUnavailable, "ML_UNAVAILABLE", "analytics repository not configured")
				return
			}
			models, err := analyticsRepo.ListModels(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "MODEL_LIST_FAILED", err.Error())
				return
			}
			out := make([]map[string]any, 0, len(models))
			for _, m := range models {
				out = append(out, map[string]any{
					"model_version_id":  m.ModelVersionID,
					"training_run_id":   m.TrainingRunID,
					"model_name":        m.ModelName,
					"version":           m.Version,
					"framework":         ptrStr(m.Framework),
					"stage":             m.Stage,
					"params":            ptrStr(m.Params),
					"metrics_summary":   ptrStr(m.MetricsSummary),
					"promoted_at":       ptrTime(m.PromotedAt),
					"created_at":        m.CreatedAt.UTC(),
					"algorithm":         ptrStr(m.Algorithm),
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": out, "count": len(out)})
		}))))

	// Recommendation routes (Module 7): nil service when CCO_DATA_DSN is not
	// configured → endpoints return 503.
	recommendation.RegisterRoutes(
		mux,
		recommendationSvc,
		authMiddleware,
		permEngine,
		recommendationadapters.NewAuditTraceReader(auditService, auditService),
	)
	reporting.RegisterRoutes(mux, reportService, authMiddleware, permEngine)
	audit.RegisterRoutes(mux, auditService, authMiddleware, permEngine)

	// Jobs (durable queue)
	jobs.RegisterRoutes(mux, jsvc, authMiddleware, permEngine)

	// Endpoint asíncrono de adquisición: encola un job y devuelve 202 con
	// job_id. El worker ejecuta el ciclo sin bloquear la respuesta HTTP.
	mux.Handle("POST /api/v1/jobs/acquisition",
		authMiddleware.Handler(middleware.RequirePermission(permEngine, "acquisitions", "run")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ic, ok := iam.IdentityContextFromContext(r.Context())
			if !ok || ic.OrgID == uuid.Nil {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
				return
			}
			const maxBody = 1 << 20 // 1 MB
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
			var req struct {
				ResourceUID string `json:"resource_uid"`
			}
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
				return
			}
			var resourceUID *uuid.UUID
			if req.ResourceUID != "" {
				uid, err := uuid.Parse(req.ResourceUID)
				if err != nil {
					writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid resource_uid")
					return
				}
				resourceUID = &uid
			}
			payload := map[string]any{"org_id": ic.OrgID.String()}
			if resourceUID != nil {
				payload["resource_uid"] = resourceUID.String()
			}
			b, err := json.Marshal(payload)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "ENQUEUE_FAILED", err.Error())
				return
			}
			job, err := jsvc.Enqueue(r.Context(), jobs.EnqueueInput{
				Type:           "acquisition:run",
				Payload:        b,
				OrganizationID: &ic.OrgID,
				MaxAttempts:    5,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "ENQUEUE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusAccepted, job.ToDTO())
		}))))

	// POST /api/v1/jobs/report encola una generación de reporte: primero crea
	// el ReportGenerationJob en app.report_requests (vía QueueOptimizationReport)
	// y luego encola un job de tipo report:optimization en app.jobs para que el
	// worker lo procese de forma asíncrona.
	mux.Handle("POST /api/v1/jobs/report",
		authMiddleware.Handler(middleware.RequirePermission(permEngine, "reports", "write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if reportService == nil {
				writeError(w, http.StatusServiceUnavailable, "REPORTING_UNAVAILABLE", "reporting service not configured (CCO_DATA_DSN missing)")
				return
			}
			ic, ok := iam.IdentityContextFromContext(r.Context())
			if !ok || ic.ProjectID == nil || *ic.ProjectID == uuid.Nil {
				writeError(w, http.StatusBadRequest, "PROJECT_REQUIRED", "select an authorized project using X-Project-ID")
				return
			}
			const maxBody = 1 << 20 // 1 MB
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
			var req struct {
				ResourceUID        string      `json:"resource_uid"`
				From               string      `json:"from"`
				To                 string      `json:"to"`
				Format             string      `json:"format"`
				IncludeAudit       bool        `json:"include_audit"`
				IncludeEvidence    bool        `json:"include_evidence"`
				IncludeRisks       bool        `json:"include_risks"`
				IncludeMetrics     bool        `json:"include_metrics"`
				Title              string      `json:"title"`
				RecommendationUIDs []uuid.UUID `json:"recommendation_uids,omitempty"`
			}
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
				return
			}
			resourceUID, err := uuid.Parse(req.ResourceUID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid resource_uid")
				return
			}
			from, err := time.Parse(time.RFC3339, req.From)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid from")
				return
			}
			to, err := time.Parse(time.RFC3339, req.To)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid to")
				return
			}
			reportJob, err := reportService.QueueOptimizationReport(r.Context(), reporting.OptimizationReportInput{
				ProjectID:          *ic.ProjectID,
				ResourceUID:        resourceUID,
				From:               from.UTC(),
				To:                 to.UTC(),
				Format:             reporting.ReportFormat(req.Format),
				IncludeAudit:       req.IncludeAudit,
				IncludeEvidence:    req.IncludeEvidence,
				IncludeRisks:       req.IncludeRisks,
				IncludeMetrics:     req.IncludeMetrics,
				Title:              req.Title,
				RequestedBy:        ic.UserID,
				RecommendationUIDs: req.RecommendationUIDs,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "REPORT_QUEUE_FAILED", err.Error())
				return
			}
			payload, _ := json.Marshal(map[string]string{"report_job_uid": reportJob.UID.String()})
			job, err := jsvc.Enqueue(r.Context(), jobs.EnqueueInput{
				Type:           "report:optimization",
				Payload:        payload,
				OrganizationID: &ic.OrgID,
				MaxAttempts:    5,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "ENQUEUE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusAccepted, job.ToDTO())
		}))))

	// POST /api/v1/jobs/analytics encola una operación analítica pesada
	// (percentiles, feature_snapshot, trends, patterns, capacity, costs,
	// forecast) para ejecución asíncrona por el worker de jobs.
	mux.Handle("POST /api/v1/jobs/analytics",
		authMiddleware.Handler(middleware.RequirePermission(permEngine, "analytics", "compute")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const maxBody = 1 << 20 // 1 MB
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
			var req struct {
				Operation    string `json:"operation"`
				ResourceUID  string `json:"resource_uid"`
				From         string `json:"from"`
				To           string `json:"to"`
				MetricCode   string `json:"metric_code,omitempty"`
				PeriodType   string `json:"period_type,omitempty"`
				HorizonSteps int    `json:"horizon_steps,omitempty"`
			}
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
				return
			}
			if req.Operation == "" || req.ResourceUID == "" || req.From == "" || req.To == "" {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "operation, resource_uid, from and to are required")
				return
			}
			_, err := uuid.Parse(req.ResourceUID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid resource_uid")
				return
			}
			from, err := time.Parse(time.RFC3339, req.From)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid from")
				return
			}
			to, err := time.Parse(time.RFC3339, req.To)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid to")
				return
			}
			ic, ok := iam.IdentityContextFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no identity context")
				return
			}
			payload, _ := json.Marshal(map[string]any{
				"operation":     req.Operation,
				"resource_uid":  req.ResourceUID,
				"from":          from.UTC().Format(time.RFC3339),
				"to":            to.UTC().Format(time.RFC3339),
				"metric_code":   req.MetricCode,
				"period_type":   req.PeriodType,
				"horizon_steps": req.HorizonSteps,
			})
			job, err := jsvc.Enqueue(r.Context(), jobs.EnqueueInput{
				Type:           "analytics:compute",
				Payload:        payload,
				OrganizationID: &ic.OrgID,
				MaxAttempts:    5,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "ENQUEUE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusAccepted, job.ToDTO())
		}))))

	// POST /api/v1/jobs/recommendation encola una generación de recomendación
	// pesada para ejecución asíncrona por el worker de jobs.
	mux.Handle("POST /api/v1/jobs/recommendation",
		authMiddleware.Handler(middleware.RequirePermission(permEngine, "recommendations", "write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const maxBody = 1 << 20 // 1 MB
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
			var req struct {
				ResourceUID  string `json:"resource_uid"`
				PolicyUID    string `json:"policy_uid"`
				From         string `json:"from"`
				To           string `json:"to"`
				HorizonDays  int    `json:"horizon_days"`
				Currency     string `json:"currency"`
				ForceRefresh bool   `json:"force_refresh"`
			}
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
				return
			}
			_, err := uuid.Parse(req.ResourceUID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid resource_uid")
				return
			}
			if _, err := uuid.Parse(req.PolicyUID); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid policy_uid")
				return
			}
			if len(req.Currency) != 3 || strings.ToUpper(req.Currency) != req.Currency {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "currency must be a three-letter uppercase code")
				return
			}
			from, err := time.Parse(time.RFC3339, req.From)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid from")
				return
			}
			to, err := time.Parse(time.RFC3339, req.To)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid to")
				return
			}
			if !from.Before(to) {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "from must be before to")
				return
			}
			if req.HorizonDays < 1 || req.HorizonDays > 365 {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "horizon_days must be between 1 and 365")
				return
			}
			ic, ok := iam.IdentityContextFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no identity context")
				return
			}
			payload, _ := json.Marshal(map[string]any{
				"resource_uid": req.ResourceUID, "policy_uid": req.PolicyUID,
				"from": from.UTC().Format(time.RFC3339), "to": to.UTC().Format(time.RFC3339),
				"horizon_days": req.HorizonDays, "currency": req.Currency,
				"force_refresh": req.ForceRefresh,
				"created_by":    ic.UserID.String(),
			})
			job, err := jsvc.Enqueue(r.Context(), jobs.EnqueueInput{
				Type:           "recommendation:generate",
				Payload:        payload,
				OrganizationID: &ic.OrgID,
				MaxAttempts:    5,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "ENQUEUE_FAILED", err.Error())
				return
			}
			writeJSON(w, http.StatusAccepted, job.ToDTO())
		}))))

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
	// Production hardening: refuse to start with an open CORS policy or without
	// fail-closed security, and never default to "*".
	hasStar := false
	for _, o := range allowedOrigins {
		if o == "*" {
			hasStar = true
		}
	}
	if os.Getenv("CCO_ENV") == "prod" {
		if len(allowedOrigins) == 0 || hasStar {
			log.Fatalf("CCO_ALLOWED_ORIGINS must be set to an explicit allowlist (no '*') when CCO_ENV=prod")
		}
	}
	handler := middleware.CORS(allowedOrigins, mux)
	handler = middleware.SecurityHeaders(os.Getenv("CCO_CSP"), handler)
	handler = middleware.RateLimitMiddleware(60, time.Minute, vc, handler)
	// CSRF protection: strict in prod, permissive otherwise (no cookie yet ->
	// not blocked). Clients must send X-CSRF-Token matching cco_csrf cookie
	// on state-changing requests once the cookie is issued.
	csrfStrict := os.Getenv("CCO_ENV") == "prod" || os.Getenv("CCO_CSRF_STRICT") == "true"
	handler = middleware.CSRFMiddleware(csrfStrict, handler)
	handler = audit.TraceMiddleware(handler, auditService)

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

	go func() {
		<-shutdownCtx.Done()
		slog.Info("shutdown signal received, stopping server")
		shutdownTimeout := envSeconds("CCO_SHUTDOWN_TIMEOUT_SEC", 30*time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(ctx)

		// Wait for background workers/schedulers to finish, bounded by the
		// same shutdown timeout so a stuck worker cannot block shutdown forever.
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
			slog.Info("background workers stopped")
		case <-ctx.Done():
			slog.Warn("shutdown timeout waiting for background workers")
		}
	}()

	log.Printf("cco-app listening on %s (driver=%s)", addr, driver)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

// parseM2MClients parses CCO_M2M_CLIENTS into client maps.
// Format: client_id:argon2id_hash:scope1,scope2|client_id2:hash2:scope3
// Hashes must be Argon2id (base64(salt).base64(hash)) per the repo standard.
func parseM2MClients(value string) (map[string]string, map[string][]string) {
	clients := make(map[string]string)
	scopes := make(map[string][]string)
	if value == "" {
		return clients, scopes
	}
	for _, entry := range strings.Split(value, "|") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 3)
		if len(parts) < 2 {
			continue
		}
		clientID := strings.TrimSpace(parts[0])
		secretHash := strings.TrimSpace(parts[1])
		var clientScopes []string
		if len(parts) == 3 {
			for _, s := range strings.Split(parts[2], ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					clientScopes = append(clientScopes, s)
				}
			}
		}
		if clientID != "" && secretHash != "" {
			clients[clientID] = secretHash
			scopes[clientID] = clientScopes
		}
	}
	return clients, scopes
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

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
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

func ptrTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
