package analytics

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/middleware"
	"github.com/google/uuid"
)

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response with code and message.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}

// statusFromError maps domain errors to HTTP status codes.
func statusFromError(err error) int {
	switch {
	case errors.Is(err, ErrStorageUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrInvalidQuery):
		return http.StatusBadRequest
	case errors.Is(err, ErrMLUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, ErrContractMismatch):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// RegisterRoutes wires the Analytics endpoints onto the mux.
func RegisterRoutes(mux *http.ServeMux, svc *Service, authMW *middleware.AuthMiddleware, perm *iam.PermissionEngine) {
	// Write endpoints (analytics:write)
	mux.Handle("POST /api/v1/analytics/snapshots",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "write")(http.HandlerFunc(handleComputeSnapshots(svc)))))
	mux.Handle("POST /api/v1/analytics/percentiles",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "write")(http.HandlerFunc(handleComputePercentiles(svc)))))
	mux.Handle("POST /api/v1/analytics/trends",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "write")(http.HandlerFunc(handleComputeTrends(svc)))))
	mux.Handle("POST /api/v1/analytics/patterns",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "write")(http.HandlerFunc(handleDetectPatterns(svc)))))
	mux.Handle("POST /api/v1/analytics/capacity",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "write")(http.HandlerFunc(handleAnalyzeCapacity(svc)))))
	mux.Handle("POST /api/v1/analytics/costs",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "write")(http.HandlerFunc(handleAnalyzeCosts(svc)))))
	mux.Handle("POST /api/v1/analytics/des",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "write")(http.HandlerFunc(handleComputeDES(svc)))))
	mux.Handle("POST /api/v1/analytics/datasets",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "write")(http.HandlerFunc(handleBuildDataset(svc)))))
	mux.Handle("POST /api/v1/analytics/forecasts",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "write")(http.HandlerFunc(handleRequestForecast(svc)))))

	// Read endpoints (analytics:read)
	mux.Handle("GET /api/v1/analytics/snapshots",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "read")(http.HandlerFunc(handleListSnapshots(svc)))))
	mux.Handle("GET /api/v1/analytics/percentiles",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "read")(http.HandlerFunc(handleListPercentiles(svc)))))
	mux.Handle("GET /api/v1/analytics/trends",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "read")(http.HandlerFunc(handleListTrends(svc)))))
	mux.Handle("GET /api/v1/analytics/patterns",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "read")(http.HandlerFunc(handleGetPattern(svc)))))
	mux.Handle("GET /api/v1/analytics/capacity",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "read")(http.HandlerFunc(handleGetCapacity(svc)))))
	mux.Handle("GET /api/v1/analytics/costs",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "read")(http.HandlerFunc(handleGetCosts(svc)))))
	mux.Handle("GET /api/v1/analytics/des",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "read")(http.HandlerFunc(handleGetDES(svc)))))
	mux.Handle("GET /api/v1/analytics/datasets",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "read")(http.HandlerFunc(handleListDatasets(svc)))))
	mux.Handle("GET /api/v1/analytics/forecasts",
		authMW.Handler(middleware.RequirePermission(perm, "analytics", "read")(http.HandlerFunc(handleListForecasts(svc)))))
}

func guardStorage(svc *Service, w http.ResponseWriter) bool {
	if svc == nil {
		writeError(w, http.StatusServiceUnavailable, CodeStorageUnavailable, ErrStorageUnavailable.Error())
		return false
	}
	return true
}

func requireUUIDParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	v := r.URL.Query().Get(name)
	uid, err := uuid.Parse(v)
	if v == "" || err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid or missing "+name)
		return uuid.Nil, false
	}
	return uid, true
}

func requireTimeParam(w http.ResponseWriter, r *http.Request, name string) (time.Time, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		writeError(w, http.StatusBadRequest, CodeInvalidQuery, "missing "+name)
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339, v)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid "+name+" (want RFC3339 UTC)")
		return time.Time{}, false
	}
	return ts.UTC(), true
}

func optionalTimeParam(w http.ResponseWriter, r *http.Request, name string) (time.Time, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return time.Time{}, true
	}
	ts, err := time.Parse(time.RFC3339, v)
	if err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid "+name+" (want RFC3339 UTC)")
		return time.Time{}, false
	}
	return ts.UTC(), true
}

func limitParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return 0, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid limit")
		return 0, false
	}
	return n, true
}

// POST handlers (compute)

func handleComputeSnapshots(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		var req struct {
			ResourceUID string `json:"resource_uid"`
			From        string `json:"from"`
			To          string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid request body")
			return
		}
		resourceUID, err := uuid.Parse(req.ResourceUID)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid resource_uid")
			return
		}
		from, err := time.Parse(time.RFC3339, req.From)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid from (RFC3339)")
			return
		}
		to, err := time.Parse(time.RFC3339, req.To)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid to (RFC3339)")
			return
		}
		n, err := svc.ComputeFeatureSnapshot(r.Context(), resourceUID, from.UTC(), to.UTC())
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"computed": n, "resource_uid": req.ResourceUID})
	}
}

func handleComputePercentiles(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		var req struct {
			ResourceUID string `json:"resource_uid"`
			MetricCode  string `json:"metric_code"`
			From        string `json:"from"`
			To          string `json:"to"`
			PeriodType  string `json:"period_type"` // 1h, 1d, 15m
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid request body")
			return
		}
		resourceUID, err := uuid.Parse(req.ResourceUID)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid resource_uid")
			return
		}
		if req.MetricCode == "" {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "missing metric_code")
			return
		}
		from, err := time.Parse(time.RFC3339, req.From)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid from")
			return
		}
		to, err := time.Parse(time.RFC3339, req.To)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid to")
			return
		}
		periodType := req.PeriodType
		if periodType == "" {
			periodType = "1h"
		}
		res, err := svc.ComputePercentiles(r.Context(), resourceUID, req.MetricCode, from.UTC(), to.UTC(), periodType)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleComputeTrends(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		var req struct {
			ResourceUID string `json:"resource_uid"`
			From        string `json:"from"`
			To          string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid request body")
			return
		}
		resourceUID, err := uuid.Parse(req.ResourceUID)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid resource_uid")
			return
		}
		from, err := time.Parse(time.RFC3339, req.From)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid from")
			return
		}
		to, err := time.Parse(time.RFC3339, req.To)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid to")
			return
		}
		res, err := svc.ComputeTrends(r.Context(), resourceUID, from.UTC(), to.UTC())
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleDetectPatterns(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		var req struct {
			ResourceUID string `json:"resource_uid"`
			From        string `json:"from"`
			To          string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid request body")
			return
		}
		resourceUID, err := uuid.Parse(req.ResourceUID)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid resource_uid")
			return
		}
		from, err := time.Parse(time.RFC3339, req.From)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid from")
			return
		}
		to, err := time.Parse(time.RFC3339, req.To)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid to")
			return
		}
		res, err := svc.DetectWorkloadPatterns(r.Context(), resourceUID, from.UTC(), to.UTC())
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleAnalyzeCapacity(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		var req struct {
			ResourceUID string `json:"resource_uid"`
			PeriodType  string `json:"period_type"` // 1h, 1d
			PeriodStart string `json:"period_start"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid request body")
			return
		}
		resourceUID, err := uuid.Parse(req.ResourceUID)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid resource_uid")
			return
		}
		periodType := req.PeriodType
		if periodType == "" {
			periodType = "1d"
		}
		periodStart, err := time.Parse(time.RFC3339, req.PeriodStart)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid period_start")
			return
		}
		res, err := svc.AnalyzeCapacity(r.Context(), resourceUID, periodType, periodStart.UTC())
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleAnalyzeCosts(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		var req struct {
			ResourceUID string `json:"resource_uid"`
			From        string `json:"from"`
			To          string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid request body")
			return
		}
		resourceUID, err := uuid.Parse(req.ResourceUID)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid resource_uid")
			return
		}
		from, err := time.Parse(time.RFC3339, req.From)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid from")
			return
		}
		to, err := time.Parse(time.RFC3339, req.To)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid to")
			return
		}
		res, err := svc.AnalyzeCosts(r.Context(), resourceUID, from.UTC(), to.UTC())
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleComputeDES(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		var req struct {
			ResourceUID string `json:"resource_uid"`
			PeriodType  string `json:"period_type"`
			PeriodStart string `json:"period_start"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid request body")
			return
		}
		resourceUID, err := uuid.Parse(req.ResourceUID)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid resource_uid")
			return
		}
		periodType := req.PeriodType
		if periodType == "" {
			periodType = "1d"
		}
		periodStart, err := time.Parse(time.RFC3339, req.PeriodStart)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid period_start")
			return
		}
		res, err := svc.ComputeDESComponents(r.Context(), resourceUID, periodType, periodStart.UTC())
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleBuildDataset(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		var req TrainingDatasetSpec
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid request body")
			return
		}
		id, err := svc.BuildTrainingDataset(r.Context(), req)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"dataset_id": id, "name": req.Name, "version": req.Version})
	}
}

func handleRequestForecast(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		var req ForecastRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid request body")
			return
		}
		if req.ResourceUID == uuid.Nil || req.TargetMetric == "" || req.HorizonSteps <= 0 {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "resource_uid, target_metric, horizon_steps required")
			return
		}
		uid, err := svc.RequestForecast(r.Context(), req)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"forecast_uid": uid})
	}
}

func handleListSnapshots(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		resourceUID, ok := requireUUIDParam(w, r, "resource_uid")
		if !ok {
			return
		}
		periodType := r.URL.Query().Get("period_type")
		if periodType == "" {
			periodType = "1h"
		}
		from, ok := optionalTimeParam(w, r, "from")
		if !ok {
			return
		}
		to, ok := optionalTimeParam(w, r, "to")
		if !ok {
			return
		}
		limit, ok := limitParam(w, r)
		if !ok {
			return
		}
		res, err := svc.FeatureSnapshot(r.Context(), resourceUID, periodType, from, to, limit)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		if res == nil {
			res = []FeatureSnapshotRow{}
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleListPercentiles(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		resourceUID, ok := requireUUIDParam(w, r, "resource_uid")
		if !ok {
			return
		}
		metricCode := r.URL.Query().Get("metric_code")
		if metricCode == "" {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "missing metric_code")
			return
		}
		periodType := r.URL.Query().Get("period_type")
		if periodType == "" {
			periodType = "1h"
		}
		from, ok := optionalTimeParam(w, r, "from")
		if !ok {
			return
		}
		to, ok := optionalTimeParam(w, r, "to")
		if !ok {
			return
		}
		res, err := svc.PercentileStatistic(r.Context(), resourceUID, metricCode, periodType, from, to)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		if res == nil {
			res = []PercentileResult{}
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleListTrends(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		resourceUID, ok := requireUUIDParam(w, r, "resource_uid")
		if !ok {
			return
		}
		from, ok := optionalTimeParam(w, r, "from")
		if !ok {
			return
		}
		to, ok := optionalTimeParam(w, r, "to")
		if !ok {
			return
		}
		res, err := svc.Trend(r.Context(), resourceUID, from, to)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		if res == nil {
			res = []TrendResult{}
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleGetPattern(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		resourceUID, ok := requireUUIDParam(w, r, "resource_uid")
		if !ok {
			return
		}
		from, ok := optionalTimeParam(w, r, "from")
		if !ok {
			return
		}
		to, ok := optionalTimeParam(w, r, "to")
		if !ok {
			return
		}
		res, err := svc.WorkloadPattern(r.Context(), resourceUID, from, to)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleGetCapacity(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		resourceUID, ok := requireUUIDParam(w, r, "resource_uid")
		if !ok {
			return
		}
		periodType := r.URL.Query().Get("period_type")
		if periodType == "" {
			periodType = "1d"
		}
		periodStart, ok := requireTimeParam(w, r, "period_start")
		if !ok {
			return
		}
		res, err := svc.CapacityAnalysis(r.Context(), resourceUID, periodType, periodStart)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleGetCosts(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		resourceUID, ok := requireUUIDParam(w, r, "resource_uid")
		if !ok {
			return
		}
		from, ok := optionalTimeParam(w, r, "from")
		if !ok {
			return
		}
		to, ok := optionalTimeParam(w, r, "to")
		if !ok {
			return
		}
		res, err := svc.CostAnalysis(r.Context(), resourceUID, from, to)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleGetDES(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		resourceUID, ok := requireUUIDParam(w, r, "resource_uid")
		if !ok {
			return
		}
		periodType := r.URL.Query().Get("period_type")
		if periodType == "" {
			periodType = "1d"
		}
		periodStart, ok := requireTimeParam(w, r, "period_start")
		if !ok {
			return
		}
		res, err := svc.DESComponents(r.Context(), resourceUID, periodType, periodStart)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleListDatasets(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		name := r.URL.Query().Get("name")
		version := r.URL.Query().Get("version")
		if name == "" || version == "" {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "name and version required")
			return
		}
		v, err := strconv.Atoi(version)
		if err != nil {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid version")
			return
		}
		res, err := svc.TrainingDataset(r.Context(), name, v)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func handleListForecasts(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		resourceUID, ok := requireUUIDParam(w, r, "resource_uid")
		if !ok {
			return
		}
		targetMetric := r.URL.Query().Get("target_metric")
		from, ok := optionalTimeParam(w, r, "from")
		if !ok {
			return
		}
		to, ok := optionalTimeParam(w, r, "to")
		if !ok {
			return
		}
		res, err := svc.Forecasts(r.Context(), resourceUID, targetMetric, from, to)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		if res == nil {
			res = []ForecastResponse{}
		}
		writeJSON(w, http.StatusOK, res)
	}
}
