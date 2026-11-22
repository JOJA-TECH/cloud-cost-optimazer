package jobs

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/middleware"
	"github.com/google/uuid"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "message": message})
}

// enqueueRequest es el cuerpo de POST /api/v1/jobs.
type enqueueRequest struct {
	Type            string  `json:"type"`
	Payload         any     `json:"payload,omitempty"`
	OrganizationID  string  `json:"organization_id,omitempty"`
	ProjectID       string  `json:"project_id,omitempty"`
	IdempotencyKey  string  `json:"idempotency_key,omitempty"`
	MaxAttempts     int     `json:"max_attempts,omitempty"`
	ScheduleInSec   *int    `json:"schedule_in_sec,omitempty"`
}

// RegisterRoutes wires las rutas de jobs sobre el mux.
func RegisterRoutes(mux *http.ServeMux, svc Service, authMW *middleware.AuthMiddleware, perm *iam.PermissionEngine) {
	mux.Handle("POST /api/v1/jobs",
		authMW.Handler(middleware.RequirePermission(perm, "jobs", "write")(http.HandlerFunc(handleEnqueue(svc)))))
	mux.Handle("GET /api/v1/jobs",
		authMW.Handler(middleware.RequirePermission(perm, "jobs", "read")(http.HandlerFunc(handleList(svc)))))
	mux.Handle("GET /api/v1/jobs/{id}",
		authMW.Handler(middleware.RequirePermission(perm, "jobs", "read")(http.HandlerFunc(handleGet(svc)))))
	mux.Handle("DELETE /api/v1/jobs/{id}",
		authMW.Handler(middleware.RequirePermission(perm, "jobs", "write")(http.HandlerFunc(handleCancel(svc)))))
}

func handleEnqueue(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const maxBody = 1 << 20 // 1 MB
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		var req enqueueRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		if req.Type == "" {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "type is required")
			return
		}
		var payload []byte
		if req.Payload != nil {
			var err error
			payload, err = json.Marshal(req.Payload)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "payload not serializable")
				return
			}
		}
		var orgID, projectID *uuid.UUID
		if req.OrganizationID != "" {
			id, err := uuid.Parse(req.OrganizationID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid organization_id")
				return
			}
			orgID = &id
		}
		if req.ProjectID != "" {
			id, err := uuid.Parse(req.ProjectID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid project_id")
				return
			}
			projectID = &id
		}
		var idemKey *string
		if req.IdempotencyKey != "" {
			idemKey = &req.IdempotencyKey
		}
		var scheduleIn *time.Duration
		if req.ScheduleInSec != nil && *req.ScheduleInSec > 0 {
			d := time.Duration(*req.ScheduleInSec) * time.Second
			scheduleIn = &d
		}
		job, err := svc.Enqueue(r.Context(), EnqueueInput{
			Type:           req.Type,
			Payload:        payload,
			OrganizationID: orgID,
			ProjectID:      projectID,
			IdempotencyKey: idemKey,
			MaxAttempts:    req.MaxAttempts,
			ScheduleIn:     scheduleIn,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ENQUEUE_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, job.ToDTO())
	}
}

func handleGet(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseJobID(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		dto, err := svc.GetStatus(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrJobNotFound) {
				writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "job not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "GET_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, dto)
	}
}

func handleCancel(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseJobID(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		if err := svc.Cancel(r.Context(), id); err != nil {
			if errors.Is(err, ErrJobNotFound) {
				writeError(w, http.StatusNotFound, "JOB_NOT_FOUND", "job not found")
				return
			}
			if errors.Is(err, ErrJobTerminal) {
				writeError(w, http.StatusUnprocessableEntity, "JOB_TERMINAL", "job is already terminal")
				return
			}
			writeError(w, http.StatusInternalServerError, "CANCEL_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": JobStatusCancelled})
	}
}

func handleList(svc Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f := ListFilter{
			Status:      r.URL.Query().Get("status"),
			Type:        r.URL.Query().Get("type"),
			Limit:       50,
		}
		if v := r.URL.Query().Get("organization_id"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid organization_id")
				return
			}
			f.OrganizationID = &id
		}
		if v := r.URL.Query().Get("project_id"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid project_id")
				return
			}
			f.ProjectID = &id
		}
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || n > 500 {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid limit (max 500)")
				return
			}
			f.Limit = n
		}
		jobs, err := svc.List(r.Context(), f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "LIST_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, jobs)
	}
}

func parseJobID(v string) (uuid.UUID, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return uuid.Nil, errors.New("empty job id")
	}
	return uuid.Parse(v)
}
