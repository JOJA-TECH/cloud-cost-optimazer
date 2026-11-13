package data

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
	case errors.Is(err, ErrInvalidQuery), errors.Is(err, ErrInvalidCostRecord):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// RegisterRoutes wires the Gestión de Datos read API onto the mux. All
// endpoints are read-only and require the data:read permission.
func RegisterRoutes(mux *http.ServeMux, svc *Service, authMW *middleware.AuthMiddleware, perm *iam.PermissionEngine) {
	mux.Handle("GET /api/v1/data/metrics",
		authMW.Handler(middleware.RequirePermission(perm, "data", "read")(http.HandlerFunc(handleMetricSeries(svc)))))
	mux.Handle("GET /api/v1/data/costs",
		authMW.Handler(middleware.RequirePermission(perm, "data", "read")(http.HandlerFunc(handleCosts(svc)))))
	mux.Handle("GET /api/v1/data/config-snapshots",
		authMW.Handler(middleware.RequirePermission(perm, "data", "read")(http.HandlerFunc(handleConfigSnapshots(svc)))))
	mux.Handle("GET /api/v1/data/coverage",
		authMW.Handler(middleware.RequirePermission(perm, "data", "read")(http.HandlerFunc(handleCoverage(svc)))))
}

// guardStorage rejects requests when cco_data is not configured.
func guardStorage(svc *Service, w http.ResponseWriter) bool {
	if svc == nil {
		writeError(w, http.StatusServiceUnavailable, CodeStorageUnavailable, ErrStorageUnavailable.Error())
		return false
	}
	return true
}

// requireUUIDParam parses a required uuid query parameter.
func requireUUIDParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	v := r.URL.Query().Get(name)
	uid, err := uuid.Parse(v)
	if v == "" || err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid or missing "+name)
		return uuid.Nil, false
	}
	return uid, true
}

// requireTimeParam parses a required RFC3339 UTC timestamp parameter.
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

// optionalTimeParam parses an optional RFC3339 UTC timestamp parameter.
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

// limitParam parses an optional non-negative limit.
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

// handleMetricSeries serves GET /api/v1/data/metrics.
func handleMetricSeries(svc *Service) http.HandlerFunc {
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
		from, ok := requireTimeParam(w, r, "from")
		if !ok {
			return
		}
		to, ok := requireTimeParam(w, r, "to")
		if !ok {
			return
		}
		limit, ok := limitParam(w, r)
		if !ok {
			return
		}
		points, err := svc.MetricSeries(r.Context(), resourceUID, metricCode, from, to, limit)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		if points == nil {
			points = []HistoricalMetric{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"resource_uid": resourceUID,
			"metric_code":  metricCode,
			"points":       points,
		})
	}
}

// handleCosts serves GET /api/v1/data/costs (by resource_uid or scope).
func handleCosts(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !guardStorage(svc, w) {
			return
		}
		var resourceUID *uuid.UUID
		if v := r.URL.Query().Get("resource_uid"); v != "" {
			uid, err := uuid.Parse(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid resource_uid")
				return
			}
			resourceUID = &uid
		}
		scopeType := r.URL.Query().Get("scope_type")
		scopeName := r.URL.Query().Get("scope_name")
		if resourceUID == nil && (scopeType == "" || scopeName == "") {
			writeError(w, http.StatusBadRequest, CodeInvalidQuery, "either resource_uid or scope_type+scope_name is required")
			return
		}
		from, ok := requireTimeParam(w, r, "from")
		if !ok {
			return
		}
		to, ok := requireTimeParam(w, r, "to")
		if !ok {
			return
		}
		limit, ok := limitParam(w, r)
		if !ok {
			return
		}
		records, err := svc.Costs(r.Context(), resourceUID, scopeType, scopeName, from, to, limit)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		if records == nil {
			records = []CostRecord{}
		}
		writeJSON(w, http.StatusOK, records)
	}
}

// handleConfigSnapshots serves GET /api/v1/data/config-snapshots.
func handleConfigSnapshots(svc *Service) http.HandlerFunc {
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
		limit, ok := limitParam(w, r)
		if !ok {
			return
		}
		snaps, err := svc.ConfigSnapshots(r.Context(), resourceUID, from, to, limit)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		if snaps == nil {
			snaps = []ConfigurationSnapshot{}
		}
		writeJSON(w, http.StatusOK, snaps)
	}
}

// handleCoverage serves GET /api/v1/data/coverage (HU-05).
func handleCoverage(svc *Service) http.HandlerFunc {
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
		from, ok := requireTimeParam(w, r, "from")
		if !ok {
			return
		}
		to, ok := requireTimeParam(w, r, "to")
		if !ok {
			return
		}
		bucket := r.URL.Query().Get("bucket")
		if bucket == "" {
			bucket = BucketHour
		}
		minCoverage := 0.8
		if v := r.URL.Query().Get("min_coverage"); v != "" {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, CodeInvalidQuery, "invalid min_coverage")
				return
			}
			minCoverage = f
		}
		report, err := svc.Coverage(r.Context(), resourceUID, metricCode, from, to, bucket, minCoverage)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, report)
	}
}
