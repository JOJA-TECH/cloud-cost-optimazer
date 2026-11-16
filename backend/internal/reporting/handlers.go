package reporting

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
		http.HandlerFunc(GenerateOptimizationHandler(svc)),
	)
	mux.Handle("POST /api/v1/reports/optimization", authMiddleware.Handler(handler))
}

func GenerateOptimizationHandler(svc *Service) http.HandlerFunc {
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
			IncludeTrace:       boolValueOrDefault(request.IncludeTrace, true),
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

type optimizationReportRequest struct {
	ResourceUID        string         `json:"resource_uid"`
	From               string         `json:"from"`
	To                 string         `json:"to"`
	Format             string         `json:"format"`
	RecommendationUIDs []uuid.UUID    `json:"recommendation_uids,omitempty"`
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
