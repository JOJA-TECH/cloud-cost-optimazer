package cloud

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

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
	case errors.Is(err, ErrAccountNotFound), errors.Is(err, ErrResourceNotFound), errors.Is(err, ErrBindingNotFound), errors.Is(err, ErrProjectNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrCredentialsUnavailable):
		return http.StatusBadRequest
	case errors.Is(err, ErrAccountExists), errors.Is(err, ErrBindingExists):
		return http.StatusConflict
	case errors.Is(err, ErrConnectionFailed), errors.Is(err, ErrProviderUnavailable):
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// RegisterRoutes wires cloud routes onto the mux.
func RegisterRoutes(mux *http.ServeMux, svc *Service, authMW *middleware.AuthMiddleware, perm *iam.PermissionEngine) {
	// Cloud accounts
	mux.Handle("POST /api/v1/cloud-accounts",
		authMW.Handler(middleware.RequirePermission(perm, "cloud_accounts", "write")(http.HandlerFunc(handleRegisterAccount(svc)))))
	mux.Handle("GET /api/v1/cloud-accounts",
		authMW.Handler(middleware.RequirePermission(perm, "cloud_accounts", "read")(http.HandlerFunc(handleListAccounts(svc)))))
	mux.Handle("GET /api/v1/cloud-accounts/{id}",
		authMW.Handler(middleware.RequirePermission(perm, "cloud_accounts", "read")(http.HandlerFunc(handleGetAccount(svc)))))
	mux.Handle("POST /api/v1/cloud-accounts/{id}/validate",
		authMW.Handler(middleware.RequirePermission(perm, "cloud_accounts", "write")(http.HandlerFunc(handleValidateAccount(svc)))))
	mux.Handle("POST /api/v1/cloud-accounts/{id}/sync",
		authMW.Handler(middleware.RequirePermission(perm, "cloud_accounts", "write")(http.HandlerFunc(handleSyncResources(svc)))))
	mux.Handle("GET /api/v1/cloud-accounts/{id}/resources",
		authMW.Handler(middleware.RequirePermission(perm, "resources", "read")(http.HandlerFunc(handleListResources(svc)))))

	// Resource bindings / data sources
	mux.Handle("POST /api/v1/resource-bindings",
		authMW.Handler(middleware.RequirePermission(perm, "data_sources", "write")(http.HandlerFunc(handleRegisterDataSource(svc)))))
	mux.Handle("GET /api/v1/resource-bindings",
		authMW.Handler(middleware.RequirePermission(perm, "data_sources", "read")(http.HandlerFunc(handleListDataSources(svc)))))
	mux.Handle("PATCH /api/v1/resource-bindings/{id}",
		authMW.Handler(middleware.RequirePermission(perm, "data_sources", "write")(http.HandlerFunc(handleSetDataSourceMonitored(svc)))))
}

// orgIDFromContext extracts the org ID from the request identity context.
func orgIDFromContext(r *http.Request) (uuid.UUID, bool) {
	ic, ok := iam.IdentityContextFromContext(r.Context())
	if !ok {
		return uuid.Nil, false
	}
	return ic.OrgID, ic.OrgID != uuid.Nil
}

// ---------- account handlers ----------

func handleRegisterAccount(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		ic, _ := iam.IdentityContextFromContext(r.Context())
		var req struct {
			ProviderCode       string `json:"provider_code"`
			DisplayName        string `json:"display_name"`
			SubscriptionID     string `json:"subscription_id"`
			TenantID           string `json:"tenant_id"`
			ClientID           *string `json:"client_id,omitempty"`
			CredentialReference string `json:"credential_reference"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		acc, err := svc.RegisterAccount(r.Context(), orgID, ic.UserID, CreateAccountInput{
			ProviderCode:        req.ProviderCode,
			DisplayName:         req.DisplayName,
			SubscriptionID:      req.SubscriptionID,
			TenantID:            req.TenantID,
			ClientID:            req.ClientID,
			CredentialReference: req.CredentialReference,
		})
		if err != nil {
			slog.Warn("register account failed", "error", err.Error())
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, acc)
	}
}

func handleListAccounts(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		accs, err := svc.ListAccounts(r.Context(), orgID)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, accs)
	}
}

func handleGetAccount(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := accountIDFromPath(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid account id")
			return
		}
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		acc, err := svc.GetAccount(r.Context(), orgID, id)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, acc)
	}
}

func handleValidateAccount(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := accountIDFromPath(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid account id")
			return
		}
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		acc, err := svc.ValidateAccount(r.Context(), orgID, id)
		if err != nil {
			slog.Warn("validate account failed", "account_id", id, "error", err.Error())
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, acc)
	}
}

func handleSyncResources(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := accountIDFromPath(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid account id")
			return
		}
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		resources, err := svc.SyncResources(r.Context(), orgID, id)
		if err != nil {
			slog.Warn("sync resources failed", "account_id", id, "error", err.Error())
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"synced":   len(resources),
			"resources": toResourceAPI(resources),
		})
	}
}

func handleListResources(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := accountIDFromPath(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid account id")
			return
		}
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		resources, err := svc.ListResources(r.Context(), orgID, id)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, toResourceAPI(resources))
	}
}

// toResourceAPI converts internal ResourceDTOs to the snake_case, unix-millis
// API-facing representation.
func toResourceAPI(resources []ResourceDTO) []ResourceAPIDTO {
	out := make([]ResourceAPIDTO, len(resources))
	for i, r := range resources {
		var kind *string
		if r.Kind != nil {
			k := *r.Kind
			kind = &k
		}
		out[i] = ResourceAPIDTO{
			ResourceUID:     r.ResourceUID,
			CloudAccountID:  r.CloudAccountID,
			ProviderCode:    r.ProviderCode,
			SubscriptionID:  r.SubscriptionID,
			ResourceGroup:   r.ResourceGroup,
			ServerName:      r.ServerName,
			DatabaseName:    r.DatabaseName,
			AzureResourceID: r.AzureResourceID,
			RegionCode:      r.RegionCode,
			Kind:            kind,
			LifecycleStatus: r.LifecycleStatus,
			LastSeenAt:      r.LastSeenAt.UnixMilli(),
		}
	}
	return out
}

// ---------- binding handlers ----------

func handleRegisterDataSource(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ProjectID       string `json:"project_id"`
			CloudAccountID  string `json:"cloud_account_id"`
			DataResourceUID string `json:"data_resource_uid"`
			DisplayName     string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		pid, err := uuid.Parse(req.ProjectID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid project_id")
			return
		}
		aid, err := uuid.Parse(req.CloudAccountID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid cloud_account_id")
			return
		}
		druid, err := uuid.Parse(req.DataResourceUID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid data_resource_uid")
			return
		}
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		binding, err := svc.RegisterDataSource(r.Context(), orgID, CreateBindingInput{
			ProjectID:       pid,
			CloudAccountID:  aid,
			DataResourceUID: druid,
			DisplayName:     req.DisplayName,
		})
		if err != nil {
			slog.Warn("register data source failed", "error", err.Error())
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, binding)
	}
}

func handleListDataSources(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var projectID *uuid.UUID
		if v := r.URL.Query().Get("project_id"); v != "" {
			pid, err := uuid.Parse(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid project_id")
				return
			}
			projectID = &pid
		}
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		bindings, err := svc.ListDataSources(r.Context(), orgID, projectID)
		if err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, bindings)
	}
}

func handleSetDataSourceMonitored(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := bindingIDFromPath(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid binding id")
			return
		}
		orgID, ok := orgIDFromContext(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no org context")
			return
		}
		var req struct {
			IsMonitored bool `json:"is_monitored"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
			return
		}
		if err := svc.SetDataSourceMonitored(r.Context(), orgID, id, req.IsMonitored); err != nil {
			writeError(w, statusFromError(err), MapErrorToCode(err), err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// ---------- path helpers ----------

func accountIDFromPath(r *http.Request) (uuid.UUID, bool) {
	return idFromPath(r, "id")
}

func bindingIDFromPath(r *http.Request) (uuid.UUID, bool) {
	return idFromPath(r, "id")
}

func idFromPath(r *http.Request, name string) (uuid.UUID, bool) {
	// Go 1.22 path patterns bind {id}; extract via r.PathValue.
	if v := r.PathValue(name); v != "" {
		id, err := uuid.Parse(v)
		return id, err == nil
	}
	return uuid.Nil, false
}

func strPtr(s string) *string { return &s }
