package acquisition

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

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
	case errors.Is(err, ErrCycleInProgress):
		return http.StatusUnprocessableEntity
	case errors.Is(err, ErrResourceNotFound), errors.Is(err, ErrRunNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrCredentialsUnavailable):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

// RegisterRoutes wires acquisition routes onto the mux.
func RegisterRoutes(mux *http.ServeMux, svc *Service, authMW *middleware.AuthMiddleware, perm *iam.PermissionEngine) {
	mux.Handle("POST /api/v1/acquisitions/run",
		authMW.Handler(middleware.RequirePermission(perm, "acquisitions", "run")(http.HandlerFunc(handleTriggerRun(svc)))))
	mux.Handle("GET /api/v1/acquisitions",
		authMW.Handler(middleware.RequirePermission(perm, "acquisitions", "read")(http.HandlerFunc(handleListRuns(svc)))))
	mux.Handle("GET /api/v1/acquisitions/{id}",
		authMW.Handler(middleware.RequirePermission(perm, "acquisitions", "read")(http.HandlerFunc(handleGetRun(svc)))))
}

// orgIDFromContext extracts the org ID from the request identity context.
func orgIDFromContext(r *http.Request) (uuid.UUID, bool) {
	ic, ok := iam.IdentityContextFromContext(r.Context())
	if !ok {
		return uuid.Nil, false
	}
	return ic.OrgID, ic.OrgID != uuid.Nil
}

// handleTriggerRun executes one acquisition cycle synchronously for the
// caller's organization (optionally one resource).
// shortcut: synchronous trigger bounded by the HTTP timeout; upgrade: async
// job returning 202 when cycles grow past the timeout (large fleets).
func handleTriggerRun(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		var req struct {
			ResourceUID string `json:"resource_uid"`
		}
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
				return
			}
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
		outcomes, err := svc.RunCycle(r.Context(), &orgID, resourceUID)
		if err != nil {
			slog.Warn("acquisition trigger failed", "error", err.Error())
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		if outcomes == nil {
			outcomes = []RunOutcome{}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"triggered": len(outcomes),
			"runs":      outcomes,
		})
	}
}

// handleListRuns lists collector runs scoped to the caller's organization.
func handleListRuns(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		filter := RunFilter{Source: r.URL.Query().Get("source"), Status: r.URL.Query().Get("status")}
		if v := r.URL.Query().Get("resource_uid"); v != "" {
			uid, err := uuid.Parse(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid resource_uid")
				return
			}
			filter.ResourceUID = &uid
		}
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid limit")
				return
			}
			filter.Limit = n
		}
		runs, err := svc.ListRuns(r.Context(), orgID, filter)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

// handleGetRun returns one collector run with org isolation (404 on mismatch).
func handleGetRun(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		v := r.PathValue("id")
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid run id")
			return
		}
		run, err := svc.GetRun(r.Context(), orgID, id)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}
