package audit

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam/middleware"
	"github.com/google/uuid"
)

const (
	defaultHTTPAuditLimit = 50
	maximumHTTPAuditLimit = 200
)

var auditResourceTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func RegisterRoutes(
	mux *http.ServeMux,
	query AuditQueryService,
	authMW *middleware.AuthMiddleware,
	permissions iam.PermissionChecker,
) {
	mux.Handle("GET /api/v1/audit/events",
		authMW.Handler(requireAuditRead(permissions, http.HandlerFunc(handleListEvents(query, permissions)))))
	mux.Handle("GET /api/v1/audit/events/{event_id}",
		authMW.Handler(requireAuditRead(permissions, http.HandlerFunc(handleGetEvent(query, permissions)))))
	mux.Handle("GET /api/v1/audit/correlations/{correlation_id}",
		authMW.Handler(requireAuditRead(permissions, http.HandlerFunc(handleCorrelationHistory(query, permissions)))))
	mux.Handle("GET /api/v1/audit/resources/{entity_type}/{entity_id}/history",
		authMW.Handler(requireAuditRead(permissions, http.HandlerFunc(handleResourceHistory(query, permissions)))))
}

func requireAuditRead(permissions iam.PermissionChecker, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := iam.IdentityContextFromContext(r.Context())
		if !ok {
			writeAuditError(w, http.StatusUnauthorized, "UNAUTHORIZED", "no identity context")
			return
		}
		if !canReadAudit(r, permissions, identity) {
			writeAuditError(w, http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func canReadAudit(r *http.Request, permissions iam.PermissionChecker, identity iam.IdentityContext) bool {
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

func handleListEvents(query AuditQueryService, permissions iam.PermissionChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if query == nil {
			writeAuditError(w, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "audit query service is not configured")
			return
		}
		identity, _ := iam.IdentityContextFromContext(r.Context())
		filter, ok := parseAuditFilter(w, r, identity, permissions)
		if !ok {
			return
		}
		events, err := query.ListEvents(r.Context(), filter)
		if err != nil {
			writeQueryError(w, err)
			return
		}
		writeAuditEvents(w, events)
	}
}

func handleGetEvent(query AuditQueryService, permissions iam.PermissionChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if query == nil {
			writeAuditError(w, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "audit query service is not configured")
			return
		}
		eventID, err := strconv.ParseInt(r.PathValue("event_id"), 10, 64)
		if err != nil || eventID <= 0 {
			writeAuditError(w, http.StatusBadRequest, "INVALID_EVENT_ID", "event_id must be a positive integer")
			return
		}
		event, err := query.GetEvent(r.Context(), eventID)
		if err != nil {
			writeQueryError(w, err)
			return
		}
		identity, _ := iam.IdentityContextFromContext(r.Context())
		if !eventVisible(r, permissions, identity, event) {
			writeAuditError(w, http.StatusNotFound, "AUDIT_EVENT_NOT_FOUND", "audit event not found")
			return
		}
		writeAuditJSON(w, http.StatusOK, EventResponse(event))
	}
}

func handleCorrelationHistory(query AuditQueryService, permissions iam.PermissionChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if query == nil {
			writeAuditError(w, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "audit query service is not configured")
			return
		}
		correlationID, err := parsePathUUID(r.PathValue("correlation_id"), "correlation_id")
		if err != nil {
			writeAuditError(w, http.StatusBadRequest, "INVALID_CORRELATION_ID", "invalid correlation_id")
			return
		}
		identity, _ := iam.IdentityContextFromContext(r.Context())
		filter, ok := parseAuditFilter(w, r, identity, permissions)
		if !ok {
			return
		}
		filter.CorrelationID = &correlationID
		events, err := query.ListEvents(r.Context(), filter)
		if err != nil {
			writeQueryError(w, err)
			return
		}
		writeAuditEvents(w, events)
	}
}

func handleResourceHistory(query AuditQueryService, permissions iam.PermissionChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if query == nil {
			writeAuditError(w, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "audit query service is not configured")
			return
		}
		entityType := r.PathValue("entity_type")
		if !auditResourceTypePattern.MatchString(entityType) {
			writeAuditError(w, http.StatusBadRequest, "INVALID_ENTITY_TYPE", "invalid entity_type")
			return
		}
		entityID, err := parsePathUUID(r.PathValue("entity_id"), "entity_id")
		if err != nil {
			writeAuditError(w, http.StatusBadRequest, "INVALID_ENTITY_ID", "invalid entity_id")
			return
		}
		identity, _ := iam.IdentityContextFromContext(r.Context())
		filter, ok := parseAuditFilter(w, r, identity, permissions)
		if !ok {
			return
		}
		events, err := query.GetResourceHistory(r.Context(), ResourceReference{
			Type: entityType,
			ID:   &entityID,
		}, filter)
		if err != nil {
			writeQueryError(w, err)
			return
		}
		writeAuditEvents(w, events)
	}
}

func parseAuditFilter(
	w http.ResponseWriter,
	r *http.Request,
	identity iam.IdentityContext,
	permissions iam.PermissionChecker,
) (AuditFilter, bool) {
	filter := AuditFilter{Limit: defaultHTTPAuditLimit}
	query := r.URL.Query()
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maximumHTTPAuditLimit {
			writeAuditError(w, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 200")
			return AuditFilter{}, false
		}
		filter.Limit = limit
	}
	if raw := query.Get("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeAuditError(w, http.StatusBadRequest, "INVALID_OFFSET", "offset must be zero or greater")
			return AuditFilter{}, false
		}
		filter.Offset = offset
	}
	if filter.Action = strings.TrimSpace(query.Get("action")); len(filter.Action) > 160 {
		writeAuditError(w, http.StatusBadRequest, "INVALID_ACTION", "action is too long")
		return AuditFilter{}, false
	}
	if filter.EntityType = strings.TrimSpace(query.Get("entity_type")); filter.EntityType != "" &&
		!auditResourceTypePattern.MatchString(filter.EntityType) {
		writeAuditError(w, http.StatusBadRequest, "INVALID_ENTITY_TYPE", "invalid entity_type")
		return AuditFilter{}, false
	}
	var ok bool
	if filter.UserID, ok = parseOptionalUUID(w, query.Get("user_id"), "user_id"); !ok {
		return AuditFilter{}, false
	}
	if filter.EntityID, ok = parseOptionalUUID(w, query.Get("entity_id"), "entity_id"); !ok {
		return AuditFilter{}, false
	}
	if filter.CorrelationID, ok = parseOptionalUUID(w, query.Get("correlation_id"), "correlation_id"); !ok {
		return AuditFilter{}, false
	}
	if filter.OperationID, ok = parseOptionalUUID(w, query.Get("operation_id"), "operation_id"); !ok {
		return AuditFilter{}, false
	}
	if raw := query.Get("from"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeAuditError(w, http.StatusBadRequest, "INVALID_FROM", "from must be RFC3339")
			return AuditFilter{}, false
		}
		value = value.UTC()
		filter.From = &value
	}
	if raw := query.Get("to"); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeAuditError(w, http.StatusBadRequest, "INVALID_TO", "to must be RFC3339")
			return AuditFilter{}, false
		}
		value = value.UTC()
		filter.To = &value
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		writeAuditError(w, http.StatusBadRequest, "INVALID_TIME_RANGE", "from must not be after to")
		return AuditFilter{}, false
	}
	if !applyReadScope(r, w, permissions, identity, &filter) {
		return AuditFilter{}, false
	}
	return filter, true
}

func applyReadScope(
	r *http.Request,
	w http.ResponseWriter,
	permissions iam.PermissionChecker,
	identity iam.IdentityContext,
	filter *AuditFilter,
) bool {
	if hasSystemRole(identity, "platform_admin") {
		if raw := r.URL.Query().Get("organization_id"); raw != "" {
			value, ok := parseOptionalUUID(w, raw, "organization_id")
			if !ok {
				return false
			}
			filter.OrganizationID = value
		}
		return true
	}

	canOwn := hasAuditPermission(r, permissions, identity, "own")
	canOrganization := hasAuditPermission(r, permissions, identity, "organization")
	if !canOwn && !canOrganization {
		writeAuditError(w, http.StatusForbidden, "FORBIDDEN", "audit access requires an organization or own-event scope")
		return false
	}
	if canOrganization {
		if identity.OrgID == uuid.Nil {
			writeAuditError(w, http.StatusForbidden, "FORBIDDEN", "an organization context is required")
			return false
		}
		if raw := r.URL.Query().Get("organization_id"); raw != "" {
			requested, err := uuid.Parse(raw)
			if err != nil || requested != identity.OrgID {
				writeAuditError(w, http.StatusForbidden, "FORBIDDEN", "audit access is limited to the active organization")
				return false
			}
		}
		filter.OrganizationID = uuidPointer(identity.OrgID)
	}
	if canOwn && !canOrganization {
		if raw := r.URL.Query().Get("user_id"); raw != "" {
			requested, err := uuid.Parse(raw)
			if err != nil || requested != identity.UserID {
				writeAuditError(w, http.StatusForbidden, "FORBIDDEN", "audit access is limited to your own events")
				return false
			}
		}
		filter.UserID = uuidPointer(identity.UserID)
	}
	return true
}

func eventVisible(
	r *http.Request,
	permissions iam.PermissionChecker,
	identity iam.IdentityContext,
	event AuditEvent,
) bool {
	if hasSystemRole(identity, "platform_admin") {
		return true
	}
	if hasAuditPermission(r, permissions, identity, "organization") &&
		identity.OrgID != uuid.Nil && event.OrganizationID != nil &&
		*event.OrganizationID == identity.OrgID {
		return true
	}
	return hasAuditPermission(r, permissions, identity, "own") &&
		event.Actor.UserID != nil && *event.Actor.UserID == identity.UserID
}

func hasAuditPermission(
	r *http.Request,
	permissions iam.PermissionChecker,
	identity iam.IdentityContext,
	scope string,
) bool {
	if permissions == nil {
		return false
	}
	allowed, err := permissions.Can(r.Context(), identity, "audit:read", scope)
	return err == nil && allowed
}

func hasSystemRole(identity iam.IdentityContext, role string) bool {
	for _, candidate := range identity.SystemRoles {
		if candidate == role {
			return true
		}
	}
	return false
}

func parseOptionalUUID(w http.ResponseWriter, raw, field string) (*uuid.UUID, bool) {
	if raw == "" {
		return nil, true
	}
	value, err := uuid.Parse(raw)
	if err != nil || value == uuid.Nil {
		writeAuditError(w, http.StatusBadRequest, "INVALID_"+strings.ToUpper(field), field+" must be a non-zero UUID")
		return nil, false
	}
	return &value, true
}

func parsePathUUID(raw, field string) (uuid.UUID, error) {
	value, err := uuid.Parse(raw)
	if err != nil || value == uuid.Nil {
		return uuid.Nil, errors.New("invalid " + field)
	}
	return value, nil
}

func writeAuditEvents(w http.ResponseWriter, events []AuditEvent) {
	items := make([]AuditEventResponse, 0, len(events))
	for _, event := range events {
		items = append(items, EventResponse(event))
	}
	writeAuditJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

func writeQueryError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrAuditEventNotFound) {
		writeAuditError(w, http.StatusNotFound, "AUDIT_EVENT_NOT_FOUND", "audit event not found")
		return
	}
	writeAuditError(w, http.StatusInternalServerError, "AUDIT_QUERY_FAILED", "audit query failed")
}

func writeAuditError(w http.ResponseWriter, status int, code, message string) {
	writeAuditJSON(w, status, map[string]string{"code": code, "message": message})
}

func writeAuditJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func uuidPointer(value uuid.UUID) *uuid.UUID {
	copy := value
	return &copy
}
