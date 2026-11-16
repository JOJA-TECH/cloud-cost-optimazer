package recommendation

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/middleware"
	"github.com/google/uuid"
)

func RegisterRoutes(
	mux *http.ServeMux,
	svc *Service,
	authMW *middleware.AuthMiddleware,
	perm *iam.PermissionEngine,
	traceReaders ...RecommendationTraceReader,
) {
	mux.Handle("POST /api/v1/recommendations",
		authMW.Handler(middleware.RequirePermission(perm, "recommendations", "write")(http.HandlerFunc(handleGenerate(svc)))))
	mux.Handle("POST /api/v1/recommendations/evaluate",
		authMW.Handler(middleware.RequirePermission(perm, "recommendations", "evaluate")(http.HandlerFunc(handleEvaluate(svc)))))
	mux.Handle("GET /api/v1/recommendations",
		authMW.Handler(middleware.RequirePermission(perm, "recommendations", "read")(http.HandlerFunc(handleList(svc)))))
	mux.Handle("GET /api/v1/recommendations/{uid}",
		authMW.Handler(middleware.RequirePermission(perm, "recommendations", "read")(http.HandlerFunc(handleGet(svc)))))
	mux.Handle("GET /api/v1/recommendations/{uid}/evidence",
		authMW.Handler(middleware.RequirePermission(perm, "recommendations", "read")(http.HandlerFunc(handleEvidence(svc)))))
	mux.Handle("POST /api/v1/recommendations/{uid}/evaluate",
		authMW.Handler(middleware.RequirePermission(perm, "recommendations", "evaluate")(http.HandlerFunc(handleEvaluateExisting(svc)))))
	for _, action := range []struct {
		path   string
		status RecommendationStatus
	}{
		{path: "accept", status: RecommendationStatusAccepted},
		{path: "reject", status: RecommendationStatusRejected},
		{path: "dismiss", status: RecommendationStatusDismissed},
		{path: "apply", status: RecommendationStatusApplied},
	} {
		mux.Handle("POST /api/v1/recommendations/{uid}/"+action.path,
			authMW.Handler(middleware.RequirePermission(perm, "recommendations", "write")(
				http.HandlerFunc(handleStatusAction(svc, action.status)))))
	}
	if len(traceReaders) > 0 {
		mux.Handle("GET /api/v1/recommendations/{uid}/trace",
			authMW.Handler(middleware.RequirePermission(perm, "recommendations", "read")(
				http.HandlerFunc(handleTrace(svc, traceReaders[0])))))
	}
}

func handleStatusAction(svc *Service, status RecommendationStatus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardService(svc, w) {
			return
		}
		uid, err := uuid.Parse(r.PathValue("uid"))
		if err != nil || uid == uuid.Nil {
			writeError(w, http.StatusBadRequest, "INVALID_RECOMMENDATION_UID", "invalid recommendation uid")
			return
		}
		identity, ok := iam.IdentityContextFromContext(r.Context())
		if !ok || identity.UserID == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "authenticated user identity is unavailable")
			return
		}
		updated, err := svc.UpdateRecommendationStatus(r.Context(), uid, status, identity.UserID.String())
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, recommendationResponse(updated))
	}
}

type generateRequest struct {
	ResourceUID  string `json:"resource_uid"`
	PolicyUID    string `json:"policy_uid"`
	HorizonDays  int    `json:"horizon_days"`
	From         string `json:"from"`
	To           string `json:"to"`
	Currency     string `json:"currency"`
	ForceRefresh bool   `json:"force_refresh"`
}

func handleGenerate(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardService(svc, w) {
			return
		}
		request, ok := decodeGenerateRequest(w, r)
		if !ok {
			return
		}
		input, ok := generateInput(w, r, request)
		if !ok {
			return
		}
		result, err := svc.GenerateRecommendation(r.Context(), input)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, recommendationResponse(result))
	}
}

func handleEvaluate(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardService(svc, w) {
			return
		}
		request, ok := decodeGenerateRequest(w, r)
		if !ok {
			return
		}
		input, ok := generateInput(w, r, request)
		if !ok {
			return
		}
		input.ForceRefresh = true
		result, err := svc.EvaluateRecommendation(r.Context(), input)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, evaluationResponse(result))
	}
}

func handleEvaluateExisting(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardService(svc, w) {
			return
		}
		uid, err := uuid.Parse(r.PathValue("uid"))
		if err != nil || uid == uuid.Nil {
			writeError(w, http.StatusBadRequest, "INVALID_RECOMMENDATION_UID", "invalid recommendation uid")
			return
		}
		existing, err := svc.GetRecommendation(r.Context(), uid)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		input := GenerateRecommendationInput{
			ResourceUID: existing.ResourceUID, PolicyUID: existing.PolicyUID,
			From: existing.WindowStart, To: existing.WindowEnd,
			HorizonDays:   existing.HorizonDays,
			Currency:      existing.EstimatedSaving.Currency,
			ForceRefresh:  true,
			CreatedBy:     actorFromRequest(r),
			RequestID:     r.Header.Get("X-Request-ID"),
			CorrelationID: r.Header.Get("X-Correlation-ID"),
			OperationID:   r.Header.Get("X-Operation-ID"),
		}
		result, err := svc.EvaluateRecommendation(r.Context(), input)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		svc.recordAuditEvent(r.Context(), RecommendationAuditEvent{
			Action: "recommendation_re_evaluated", RecommendationUID: existing.UID,
			ResourceUID: existing.ResourceUID, ScenarioUID: existing.ScenarioUID, PolicyUID: existing.PolicyUID,
			RequestID: input.RequestID, CorrelationID: input.CorrelationID, OperationID: input.OperationID,
			OccurredAt: time.Now().UTC(),
		})
		writeJSON(w, http.StatusOK, evaluationResponse(result))
	}
}

func handleList(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardService(svc, w) {
			return
		}
		filter := RecommendationFilter{}
		if raw := r.URL.Query().Get("resource_uid"); raw != "" {
			uid, err := uuid.Parse(raw)
			if err != nil || uid == uuid.Nil {
				writeError(w, http.StatusBadRequest, "INVALID_RESOURCE_UID", "invalid resource_uid")
				return
			}
			filter.ResourceUID = &uid
		}
		if raw := r.URL.Query().Get("status"); raw != "" {
			filter.Status = RecommendationStatus(raw)
			if !validRecommendationStatus(filter.Status) {
				writeError(w, http.StatusBadRequest, "INVALID_STATUS", "invalid recommendation status")
				return
			}
		}
		var err error
		if raw := r.URL.Query().Get("limit"); raw != "" {
			filter.Limit, err = strconv.Atoi(raw)
			if err != nil || filter.Limit < 1 || filter.Limit > 500 {
				writeError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 500")
				return
			}
		}
		if raw := r.URL.Query().Get("offset"); raw != "" {
			filter.Offset, err = strconv.Atoi(raw)
			if err != nil || filter.Offset < 0 {
				writeError(w, http.StatusBadRequest, "INVALID_OFFSET", "offset must be zero or greater")
				return
			}
		}
		recommendations, err := svc.ListRecommendations(r.Context(), filter)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		response := make([]map[string]any, 0, len(recommendations))
		for _, recommendation := range recommendations {
			response = append(response, recommendationResponse(recommendation))
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": response, "count": len(response)})
	}
}

func handleGet(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardService(svc, w) {
			return
		}
		uid, err := uuid.Parse(r.PathValue("uid"))
		if err != nil || uid == uuid.Nil {
			writeError(w, http.StatusBadRequest, "INVALID_RECOMMENDATION_UID", "invalid recommendation uid")
			return
		}
		value, err := svc.GetRecommendation(r.Context(), uid)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, recommendationResponse(value))
	}
}

func handleEvidence(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardService(svc, w) {
			return
		}
		uid, err := uuid.Parse(r.PathValue("uid"))
		if err != nil || uid == uuid.Nil {
			writeError(w, http.StatusBadRequest, "INVALID_RECOMMENDATION_UID", "invalid recommendation uid")
			return
		}
		value, err := svc.GetRecommendation(r.Context(), uid)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"recommendation_uid": value.UID,
			"evidence":           value.Evidence,
			"count":              len(value.Evidence),
		})
	}
}

func handleTrace(svc *Service, reader RecommendationTraceReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardService(svc, w) {
			return
		}
		if reader == nil {
			writeError(w, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "recommendation trace is not configured")
			return
		}
		uid, err := uuid.Parse(r.PathValue("uid"))
		if err != nil || uid == uuid.Nil {
			writeError(w, http.StatusBadRequest, "INVALID_RECOMMENDATION_UID", "invalid recommendation uid")
			return
		}
		value, err := svc.GetRecommendation(r.Context(), uid)
		if err != nil {
			writeDomainError(w, err)
			return
		}
		identity, _ := iam.IdentityContextFromContext(r.Context())
		filter := RecommendationTraceFilter{}
		if !hasSystemRole(identity, "platform_admin") {
			if identity.OrgID != uuid.Nil {
				filter.OrganizationID = &identity.OrgID
			} else if identity.UserID != uuid.Nil {
				filter.UserID = &identity.UserID
			} else {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "trace access requires an organization or user scope")
				return
			}
		}
		events, err := reader.ListRecommendationTraceEvents(r.Context(), value.UID, filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "AUDIT_QUERY_FAILED", "recommendation trace query failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"recommendation_uid": value.UID,
			"resource_uid":       value.ResourceUID,
			"scenario_uid":       value.ScenarioUID,
			"policy_uid":         value.PolicyUID,
			"decision":           "recommendation_generated",
			"status":             value.Status,
			"risk": map[string]any{
				"score": value.Risk.Score,
				"level": value.Risk.Level,
			},
			"confidence": map[string]any{"score": value.Confidence.Score},
			"estimated_saving": map[string]any{
				"monthly_amount": value.EstimatedSaving.MonthlyAmount,
				"currency":       value.EstimatedSaving.Currency,
			},
			"events": events,
		})
	}
}

func hasSystemRole(identity iam.IdentityContext, role string) bool {
	for _, candidate := range identity.SystemRoles {
		if candidate == role {
			return true
		}
	}
	return false
}

func decodeGenerateRequest(w http.ResponseWriter, r *http.Request) (generateRequest, bool) {
	var request generateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid request body")
		return generateRequest{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "request body must contain a single JSON object")
		return generateRequest{}, false
	}
	return request, true
}

func generateInput(w http.ResponseWriter, r *http.Request, request generateRequest) (GenerateRecommendationInput, bool) {
	resourceUID, err := uuid.Parse(request.ResourceUID)
	if err != nil || resourceUID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "INVALID_RESOURCE_UID", "invalid resource_uid")
		return GenerateRecommendationInput{}, false
	}
	policyUID, err := uuid.Parse(request.PolicyUID)
	if err != nil || policyUID == uuid.Nil {
		writeError(w, http.StatusBadRequest, "INVALID_POLICY_UID", "invalid policy_uid")
		return GenerateRecommendationInput{}, false
	}
	from, err := time.Parse(time.RFC3339, request.From)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_FROM", "from must be RFC3339")
		return GenerateRecommendationInput{}, false
	}
	to, err := time.Parse(time.RFC3339, request.To)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TO", "to must be RFC3339")
		return GenerateRecommendationInput{}, false
	}
	return GenerateRecommendationInput{
		ResourceUID: resourceUID, PolicyUID: policyUID, From: from.UTC(), To: to.UTC(),
		HorizonDays: request.HorizonDays, Currency: strings.ToUpper(request.Currency),
		ForceRefresh: request.ForceRefresh, CreatedBy: actorFromRequest(r),
		RequestID: r.Header.Get("X-Request-ID"), CorrelationID: r.Header.Get("X-Correlation-ID"),
		OperationID: r.Header.Get("X-Operation-ID"),
	}, true
}

func actorFromRequest(r *http.Request) string {
	if identity, ok := iam.IdentityContextFromContext(r.Context()); ok {
		return identity.UserID.String()
	}
	return "unknown"
}

func recommendationResponse(value Recommendation) map[string]any {
	factors := make([]string, 0, len(value.Rationale.Evidence))
	for _, item := range value.Rationale.Evidence {
		switch item.Type {
		case "cpu_p95", "cpu_p99", "headroom":
			if item.Value != nil && item.Unit != nil {
				factors = append(factors, fmt.Sprintf("%s: %.2f %s", item.Type, *item.Value, *item.Unit))
			}
		case "policy_constraint":
			if item.Context != nil {
				factors = append(factors, fmt.Sprintf("Restricción de política: %v", item.Context["constraint"]))
			}
		}
	}
	response := map[string]any{
		"recommendation_uid": value.UID,
		"scenario_uid":       value.ScenarioUID,
		"policy_uid":         value.PolicyUID,
		"policy_version":     value.PolicyVersion,
		"resource_uid":       value.ResourceUID,
		"version":            value.Version,
		"supersedes_uid":     value.SupersedesUID,
		"status":             value.Status,
		"generation_state":   value.GenerationState,
		"action": map[string]any{
			"type": value.Action.Type, "description": value.Action.Description,
			"parameters": value.Action.Parameters,
		},
		"current_configuration":  value.CurrentConfiguration,
		"proposed_configuration": value.ProposedConfiguration,
		"estimated_saving": map[string]any{
			"current_monthly_cost":  value.EstimatedSaving.CurrentMonthlyCost,
			"proposed_monthly_cost": value.EstimatedSaving.ProposedMonthlyCost,
			"monthly_amount":        value.EstimatedSaving.MonthlyAmount,
			"monthly_percentage":    value.EstimatedSaving.MonthlyPercentage * 100,
			"currency":              value.EstimatedSaving.Currency,
			"period_start":          value.EstimatedSaving.PeriodStart,
			"period_end":            value.EstimatedSaving.PeriodEnd,
			"method":                value.EstimatedSaving.Method,
		},
		"risk": map[string]any{"score": value.Risk.Score, "level": value.Risk.Level},
		"confidence": map[string]any{
			"score":                        value.Confidence.Score,
			"data_quality_score":           value.Confidence.DataQualityScore,
			"evidence_completeness_score":  value.Confidence.EvidenceCompletenessScore,
			"forecast_quality_score":       value.Confidence.ForecastQualityScore,
			"simulation_feasibility_score": value.Confidence.SimulationFeasibilityScore,
			"policy_match_score":           value.Confidence.PolicyMatchScore,
			"method":                       value.Confidence.Method,
		},
		"rationale":      map[string]any{"summary": value.Rationale.Summary, "factors": factors},
		"evidence_count": len(value.Evidence),
		"created_at":     value.CreatedAt,
		"decided_at":     value.DecidedAt,
		"decided_by":     value.DecidedBy,
	}
	if value.AuditStatus != "" {
		response["audit_status"] = value.AuditStatus
	}
	return response
}

func evaluationResponse(value RecommendationEvaluation) map[string]any {
	candidates := make([]map[string]any, 0, len(value.CandidateResults))
	for _, result := range value.CandidateResults {
		candidates = append(candidates, map[string]any{
			"sku_name": result.Candidate.SKUName, "selected": result.Selected,
			"feasible": result.Simulation.Feasible, "discard_reason": result.DiscardReason,
			"projected_monthly_cost": result.Saving.ProposedMonthlyCost,
			"estimated_savings":      result.Saving.MonthlyAmount,
			"risk_score":             result.Risk.Score, "confidence_score": result.Confidence.Score,
		})
	}
	return map[string]any{
		"resource_uid": value.ResourceUID, "policy_uid": value.PolicyUID,
		"from": value.From, "to": value.To, "horizon_days": value.HorizonDays,
		"currency": value.Currency, "forecast_available": value.ForecastAvailable,
		"degraded": value.Degraded, "state": value.State,
		"evidence_count": value.EvidenceCount,
		"confidence":     value.Confidence, "candidates": candidates,
	}
}

func guardService(svc *Service, w http.ResponseWriter) bool {
	if svc == nil {
		writeError(w, http.StatusServiceUnavailable, "RECOMMENDATION_STORAGE_UNAVAILABLE", "recommendation service is not configured")
		return false
	}
	return true
}

func writeDomainError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "INTERNAL_ERROR"
	switch {
	case errors.Is(err, ErrInvalidRequest):
		status, code = http.StatusBadRequest, "RECOMMENDATION_INVALID_REQUEST"
	case errors.Is(err, ErrPolicyRequired):
		status, code = http.StatusBadRequest, "RECOMMENDATION_POLICY_REQUIRED"
	case errors.Is(err, ErrResourceNotFound), errors.Is(err, ErrPolicyNotFound), errors.Is(err, ErrRecommendationNotFound):
		status, code = http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, ErrConfigurationUnavailable):
		status, code = http.StatusUnprocessableEntity, "RECOMMENDATION_CONFIGURATION_UNAVAILABLE"
	case errors.Is(err, ErrInsufficientEvidence):
		status, code = http.StatusUnprocessableEntity, "RECOMMENDATION_INSUFFICIENT_EVIDENCE"
	case errors.Is(err, ErrPolicyDisabled), errors.Is(err, ErrConcurrentOperation):
		status, code = http.StatusConflict, "RECOMMENDATION_CONFLICT"
	case errors.Is(err, ErrPolicyRejected):
		status, code = http.StatusUnprocessableEntity, "RECOMMENDATION_POLICY_REJECTED"
	case errors.Is(err, ErrDependenciesUnavailable), errors.Is(err, ErrMLUnavailable):
		status, code = http.StatusServiceUnavailable, "RECOMMENDATION_DEPENDENCY_UNAVAILABLE"
	case errors.Is(err, ErrInvalidStatusTransition):
		status, code = http.StatusConflict, "RECOMMENDATION_STATUS_CONFLICT"
	}
	writeError(w, status, code, err.Error())
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func validRecommendationStatus(status RecommendationStatus) bool {
	switch status {
	case RecommendationStatusProposed, RecommendationStatusUnderReview,
		RecommendationStatusAccepted, RecommendationStatusRejected,
		RecommendationStatusApplied, RecommendationStatusDismissed:
		return true
	default:
		return false
	}
}
