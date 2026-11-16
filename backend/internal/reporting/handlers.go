package reporting

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/middleware"
	"github.com/google/uuid"
)

func RegisterRoutes(
	mux *http.ServeMux,
	svc *Service,
	authMiddleware *middleware.AuthMiddleware,
	permissions iam.PermissionChecker,
) {
	handler := middleware.RequirePermission(permissions, "reports", "write")(
		http.HandlerFunc(generateOptimizationHandler(svc, permissions)),
	)
	mux.Handle("POST /api/v1/reports/optimization", authMiddleware.Handler(handler))

	getHandler := middleware.RequirePermission(permissions, "reports", "read")(
		http.HandlerFunc(getReportHandler(svc, permissions)),
	)
	mux.Handle("GET /api/v1/reports/{uid}", authMiddleware.Handler(getHandler))

	queueHandler := middleware.RequirePermission(permissions, "reports", "write")(
		http.HandlerFunc(queueOptimizationReportHandler(svc, permissions)),
	)
	mux.Handle("POST /api/v1/reports", authMiddleware.Handler(queueHandler))

	statusHandler := middleware.RequirePermission(permissions, "reports", "read")(
		http.HandlerFunc(getJobStatusHandler(svc, permissions)),
	)
	mux.Handle("GET /api/v1/reports/{uid}/status", authMiddleware.Handler(statusHandler))

	cancelHandler := middleware.RequirePermission(permissions, "reports", "write")(
		http.HandlerFunc(cancelJobHandler(svc, permissions)),
	)
	mux.Handle("POST /api/v1/reports/{uid}/cancel", authMiddleware.Handler(cancelHandler))

	getArtifactHandler := middleware.RequirePermission(permissions, "reports", "export")(
		http.HandlerFunc(getReportArtifactHandler(svc, permissions)),
	)
	mux.Handle("GET /api/v1/reports/{uid}/artifact", authMiddleware.Handler(getArtifactHandler))
}

func GenerateOptimizationHandler(svc *Service) http.HandlerFunc {
	return generateOptimizationHandler(svc, nil)
}

func generateOptimizationHandler(svc *Service, permissions iam.PermissionChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeReportError(w, http.StatusServiceUnavailable, "REPORTING_UNAVAILABLE", "reporting service is not configured")
			return
		}
		var request optimizationReportRequest
		if err := decodeOptimizationReportRequest(w, r, &request); err != nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_REPORT_REQUEST", err.Error())
			return
		}
		identity, ok := iam.IdentityContextFromContext(r.Context())
		if !ok || identity.UserID == uuid.Nil {
			writeReportError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authenticated actor is unavailable")
			return
		}
		includeAudit, err := requestedAuditOption(request)
		if err != nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_REPORT_REQUEST", err.Error())
			return
		}
		if includeAudit && !canReadReportAudit(r, permissions, identity) {
			writeReportError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions to include audit trace")
			return
		}
		if identity.ProjectID == nil || *identity.ProjectID == uuid.Nil {
			writeReportError(w, http.StatusBadRequest, "PROJECT_REQUIRED", "select an authorized project using X-Project-ID")
			return
		}
		resourceUID, err := uuid.Parse(request.ResourceUID)
		if err != nil || resourceUID == uuid.Nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_RESOURCE_UID", "resource_uid must be a non-zero UUID")
			return
		}
		from, err := time.Parse(time.RFC3339, request.From)
		if err != nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_PERIOD", "from must be an RFC3339 timestamp")
			return
		}
		to, err := time.Parse(time.RFC3339, request.To)
		if err != nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_PERIOD", "to must be an RFC3339 timestamp")
			return
		}
		report, err := svc.GenerateOptimizationReport(r.Context(), OptimizationReportInput{
			ProjectID:          *identity.ProjectID,
			ResourceUID:        resourceUID,
			From:               from.UTC(),
			To:                 to.UTC(),
			Format:             ReportFormat(request.Format),
			IncludeAudit:       includeAudit,
			RecommendationUIDs: request.RecommendationUIDs,
			IncludeEvidence:    boolValueOrDefault(request.IncludeEvidence, true),
			IncludeRisks:       boolValueOrDefault(request.IncludeRisks, true),
			IncludeMetrics:     boolValueOrDefault(request.IncludeMetrics, true),
			Title:              request.Title,
			RequestedBy:        identity.UserID,
			Parameters:         request.Parameters,
		})
		if err != nil {
			writeReportDomainError(w, err)
			return
		}
		response, err := newReportResponse(report)
		if err != nil {
			writeReportDomainError(w, err)
			return
		}
		writeReportJSON(w, http.StatusCreated, response)
	}
}

func requestedAuditOption(request optimizationReportRequest) (bool, error) {
	if request.IncludeAudit != nil {
		if request.IncludeTrace != nil && *request.IncludeAudit != *request.IncludeTrace {
			return false, errors.New("include_audit and include_trace must match when both are provided")
		}
		return *request.IncludeAudit, nil
	}
	if request.IncludeTrace != nil && *request.IncludeTrace {
		return false, errors.New("include_audit must be true to include the audit trace")
	}
	return false, nil
}

func canReadReportAudit(
	r *http.Request,
	permissions iam.PermissionChecker,
	identity iam.IdentityContext,
) bool {
	if permissions == nil {
		return false
	}
	checks := [][2]string{{"audit", "read"}, {"audit:read", "own"}, {"audit:read", "organization"}}
	for _, check := range checks {
		allowed, err := permissions.Can(r.Context(), identity, check[0], check[1])
		if err == nil && allowed {
			return true
		}
	}
	return false
}

type optimizationReportRequest struct {
	ResourceUID        string         `json:"resource_uid"`
	From               string         `json:"from"`
	To                 string         `json:"to"`
	Format             string         `json:"format"`
	RecommendationUIDs []uuid.UUID    `json:"recommendation_uids,omitempty"`
	IncludeAudit       *bool          `json:"include_audit,omitempty"`
	IncludeTrace       *bool          `json:"include_trace,omitempty"`
	IncludeEvidence    *bool          `json:"include_evidence,omitempty"`
	IncludeRisks       *bool          `json:"include_risks,omitempty"`
	IncludeMetrics     *bool          `json:"include_metrics,omitempty"`
	Title              string         `json:"title,omitempty"`
	Parameters         map[string]any `json:"parameters,omitempty"`
}

func boolValueOrDefault(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func decodeOptimizationReportRequest(w http.ResponseWriter, r *http.Request, request *optimizationReportRequest) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		return errors.New("body must be a valid optimization report JSON object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON object")
	}
	return nil
}

func writeReportDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidReportRequest), errors.Is(err, ErrReportFormatUnsupported):
		writeReportError(w, http.StatusBadRequest, "INVALID_REPORT_REQUEST", err.Error())
	case errors.Is(err, ErrReportResourceNotFound), errors.Is(err, ErrReportNotFound):
		writeReportError(w, http.StatusNotFound, "REPORT_NOT_FOUND", "report resource or document was not found")
	case errors.Is(err, ErrReportDependencyUnavailable), errors.Is(err, ErrReportRepository):
		writeReportError(w, http.StatusServiceUnavailable, "REPORTING_UNAVAILABLE", "a reporting dependency is unavailable")
	default:
		writeReportError(w, http.StatusInternalServerError, "REPORT_GENERATION_FAILED", "the report could not be generated")
	}
}

func writeReportError(w http.ResponseWriter, status int, code, message string) {
	writeReportJSON(w, status, map[string]string{"code": code, "message": message})
}

func writeReportJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func getReportHandler(svc *Service, permissions iam.PermissionChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeReportError(w, http.StatusServiceUnavailable, "REPORTING_UNAVAILABLE", "reporting service is not configured")
			return
		}
		identity, ok := iam.IdentityContextFromContext(r.Context())
		if !ok || identity.UserID == uuid.Nil {
			writeReportError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authenticated actor is unavailable")
			return
		}
		uidStr := r.PathValue("uid")
		reportUID, err := uuid.Parse(uidStr)
		if err != nil || reportUID == uuid.Nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_REPORT_UID", "report uid must be a non-zero UUID")
			return
		}
		report, err := svc.GetReport(r.Context(), reportUID)
		if err != nil {
			if errors.Is(err, ErrReportNotFound) {
				writeReportError(w, http.StatusNotFound, "REPORT_NOT_FOUND", "report was not found")
				return
			}
			writeReportError(w, http.StatusInternalServerError, "REPORT_READ_FAILED", "the report could not be read")
			return
		}
		if !canAccessReport(identity, report) {
			writeReportError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions to access this report")
			return
		}
		writeReportJSON(w, http.StatusOK, report)
	}
}

func getReportArtifactHandler(svc *Service, permissions iam.PermissionChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeReportError(w, http.StatusServiceUnavailable, "REPORTING_UNAVAILABLE", "reporting service is not configured")
			return
		}
		identity, ok := iam.IdentityContextFromContext(r.Context())
		if !ok || identity.UserID == uuid.Nil {
			writeReportError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authenticated actor is unavailable")
			return
		}
		uidStr := r.PathValue("uid")
		reportUID, err := uuid.Parse(uidStr)
		if err != nil || reportUID == uuid.Nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_REPORT_UID", "report uid must be a non-zero UUID")
			return
		}
		report, err := svc.GetReport(r.Context(), reportUID)
		if err != nil {
			if errors.Is(err, ErrReportNotFound) {
				writeReportError(w, http.StatusNotFound, "REPORT_NOT_FOUND", "report was not found")
				return
			}
			writeReportError(w, http.StatusInternalServerError, "REPORT_READ_FAILED", "the report could not be read")
			return
		}
		if !canAccessReport(identity, report) {
			writeReportError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions to access this report")
			return
		}
		if report.Artifact == nil || report.Artifact.UID == uuid.Nil || report.Artifact.StorageURI == "" {
			writeReportError(w, http.StatusNotFound, "ARTIFACT_NOT_FOUND", "report artifact is not available")
			return
		}
		content, err := svc.DownloadArtifact(r.Context(), *report.Artifact)
		if err != nil {
			writeReportError(w, http.StatusInternalServerError, "ARTIFACT_READ_FAILED", "the artifact could not be read")
			return
		}
		if err := svc.RecordReportDownload(r.Context(), reportUID, *report.Artifact); err != nil {
			// No se bloquea la descarga por un fallo en la auditoría de descarga.
		}
		w.Header().Set("Content-Type", report.Artifact.MIMEType)
		if report.Artifact.FileSizeBytes != nil {
			w.Header().Set("Content-Length", strconv.FormatInt(*report.Artifact.FileSizeBytes, 10))
		}
		w.Header().Set("Content-Disposition", "attachment; filename=\""+report.Artifact.UID.String()+"\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}
}

func canAccessReport(identity iam.IdentityContext, report Report) bool {
	// platform_admin tiene acceso global a cualquier reporte.
	for _, role := range identity.SystemRoles {
		if role == "platform_admin" {
			return true
		}
	}
	// Miembros del proyecto: solo reportes de su proyecto activo.
	if identity.ProjectID != nil && report.ProjectID == *identity.ProjectID {
		return true
	}
	return false
}

func queueOptimizationReportHandler(svc *Service, permissions iam.PermissionChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeReportError(w, http.StatusServiceUnavailable, "REPORTING_UNAVAILABLE", "reporting service is not configured")
			return
		}
		identity, ok := iam.IdentityContextFromContext(r.Context())
		if !ok || identity.UserID == uuid.Nil {
			writeReportError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authenticated actor is unavailable")
			return
		}
		if identity.ProjectID == nil || *identity.ProjectID == uuid.Nil {
			writeReportError(w, http.StatusBadRequest, "PROJECT_REQUIRED", "select an authorized project using X-Project-ID")
			return
		}
		var request optimizationReportRequest
		if err := decodeOptimizationReportRequest(w, r, &request); err != nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_REPORT_REQUEST", err.Error())
			return
		}
		resourceUID, err := uuid.Parse(request.ResourceUID)
		if err != nil || resourceUID == uuid.Nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_RESOURCE_UID", "resource_uid must be a non-zero UUID")
			return
		}
		from, err := time.Parse(time.RFC3339, request.From)
		if err != nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_PERIOD", "from must be an RFC3339 timestamp")
			return
		}
		to, err := time.Parse(time.RFC3339, request.To)
		if err != nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_PERIOD", "to must be an RFC3339 timestamp")
			return
		}
		job, err := svc.QueueOptimizationReport(r.Context(), OptimizationReportInput{
			ProjectID:          *identity.ProjectID,
			ResourceUID:        resourceUID,
			From:               from.UTC(),
			To:                 to.UTC(),
			Format:             ReportFormat(request.Format),
			IncludeAudit:       requestedAuditOptionBool(request),
			RecommendationUIDs: request.RecommendationUIDs,
			IncludeEvidence:    boolValueOrDefault(request.IncludeEvidence, true),
			IncludeRisks:       boolValueOrDefault(request.IncludeRisks, true),
			IncludeMetrics:     boolValueOrDefault(request.IncludeMetrics, true),
			Title:              request.Title,
			RequestedBy:        identity.UserID,
			Parameters:         request.Parameters,
		})
		if err != nil {
			writeReportDomainError(w, err)
			return
		}
		writeReportJSON(w, http.StatusAccepted, map[string]any{
			"job_uid":     job.UID,
			"status":      job.Status,
			"report_uid":  job.ReportUID,
			"created_at":  job.CreatedAt,
			"report_type": job.ReportType,
		})
	}
}

func requestedAuditOptionBool(request optimizationReportRequest) bool {
	includeAudit, err := requestedAuditOption(request)
	if err != nil {
		return false
	}
	return includeAudit
}

func getJobStatusHandler(svc *Service, permissions iam.PermissionChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeReportError(w, http.StatusServiceUnavailable, "REPORTING_UNAVAILABLE", "reporting service is not configured")
			return
		}
		identity, ok := iam.IdentityContextFromContext(r.Context())
		if !ok || identity.UserID == uuid.Nil {
			writeReportError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authenticated actor is unavailable")
			return
		}
		uidStr := r.PathValue("uid")
		jobUID, err := uuid.Parse(uidStr)
		if err != nil || jobUID == uuid.Nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_REPORT_UID", "report uid must be a non-zero UUID")
			return
		}
		job, err := svc.GetJob(r.Context(), jobUID)
		if err != nil {
			if errors.Is(err, ErrReportNotFound) {
				writeReportError(w, http.StatusNotFound, "REPORT_NOT_FOUND", "report job was not found")
				return
			}
			writeReportError(w, http.StatusInternalServerError, "JOB_READ_FAILED", "the job could not be read")
			return
		}
		if identity.ProjectID == nil || *identity.ProjectID != job.ProjectID {
			if !isPlatformAdmin(identity) {
				writeReportError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions to access this job")
				return
			}
		}
		writeReportJSON(w, http.StatusOK, map[string]any{
			"job_uid":     job.UID,
			"status":      job.Status,
			"report_uid":  job.ReportUID,
			"artifact_uid": job.ArtifactUID,
			"created_at":  job.CreatedAt,
			"started_at":  job.StartedAt,
			"completed_at": job.CompletedAt,
			"error_message": job.ErrorMessage,
		})
	}
}

func cancelJobHandler(svc *Service, permissions iam.PermissionChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeReportError(w, http.StatusServiceUnavailable, "REPORTING_UNAVAILABLE", "reporting service is not configured")
			return
		}
		identity, ok := iam.IdentityContextFromContext(r.Context())
		if !ok || identity.UserID == uuid.Nil {
			writeReportError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authenticated actor is unavailable")
			return
		}
		uidStr := r.PathValue("uid")
		jobUID, err := uuid.Parse(uidStr)
		if err != nil || jobUID == uuid.Nil {
			writeReportError(w, http.StatusBadRequest, "INVALID_REPORT_UID", "report uid must be a non-zero UUID")
			return
		}
		job, err := svc.GetJob(r.Context(), jobUID)
		if err != nil {
			if errors.Is(err, ErrReportNotFound) {
				writeReportError(w, http.StatusNotFound, "REPORT_NOT_FOUND", "report job was not found")
				return
			}
			writeReportError(w, http.StatusInternalServerError, "JOB_READ_FAILED", "the job could not be read")
			return
		}
		if identity.ProjectID == nil || *identity.ProjectID != job.ProjectID {
			if !isPlatformAdmin(identity) {
				writeReportError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions to cancel this job")
				return
			}
		}
		if err := svc.CancelJob(r.Context(), jobUID); err != nil {
			writeReportDomainError(w, err)
			return
		}
		writeReportJSON(w, http.StatusOK, map[string]any{
			"job_uid": jobUID,
			"status":  ReportJobStatusCancelled,
		})
	}
}

func isPlatformAdmin(identity iam.IdentityContext) bool {
	for _, role := range identity.SystemRoles {
		if role == "platform_admin" {
			return true
		}
	}
	return false
}
