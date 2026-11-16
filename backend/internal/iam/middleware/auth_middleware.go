package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

// writeJSONError devuelve un error JSON con Content-Type application/json y
// escapa correctamente el mensaje (evita JSON roto por comillas o saltos de
// línea en err.Error()). Todos los errores de la API deben ser JSON.
func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}

// AuthMiddleware extracts Bearer token, validates it, resolves org/project context,
// and injects IdentityContext into request context.
type AuthMiddleware struct {
	validator      iam.TokenValidator
	repo           iam.IdentityRepository
	allowAnonymous bool // for health checks, etc.
}

// NewAuthMiddleware creates a new AuthMiddleware.
func NewAuthMiddleware(validator iam.TokenValidator, repo iam.IdentityRepository) *AuthMiddleware {
	return &AuthMiddleware{
		validator: validator,
		repo:      repo,
	}
}

// AllowAnonymous marks paths that don't require authentication.
func (m *AuthMiddleware) AllowAnonymous(paths ...string) *AuthMiddleware {
	m.allowAnonymous = true
	return m
}

// Handler returns the middleware handler function.
func (m *AuthMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health checks
		if m.allowAnonymous && (r.URL.Path == "/healthz" || r.URL.Path == "/ready") {
			next.ServeHTTP(w, r)
			return
		}

		// Extract Bearer token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// Try cookie
			if cookie, err := r.Cookie("cco_access"); err == nil {
				authHeader = "Bearer " + cookie.Value
			}
		}

		if authHeader == "" {
			slog.Warn("auth failed: missing authorization header", "path", r.URL.Path, "remote", r.RemoteAddr)
			writeJSONError(w, http.StatusUnauthorized, iam.CodeUnauthorized, "missing authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			slog.Warn("auth failed: invalid authorization format", "path", r.URL.Path, "remote", r.RemoteAddr)
			writeJSONError(w, http.StatusUnauthorized, iam.CodeUnauthorized, "invalid authorization format")
			return
		}

		token := parts[1]

		// Validate token
		ic, err := m.validator.Validate(r.Context(), token)
		if err != nil {
			code := iam.MapErrorToCode(err)
			status := http.StatusUnauthorized
			if code == "FORBIDDEN" || code == "INSUFFICIENT_SCOPE" {
				status = http.StatusForbidden
			}
			slog.Warn("auth failed: token validation error",
				"path", r.URL.Path, "remote", r.RemoteAddr, "error", err.Error(), "code", code)
			writeJSONError(w, status, code, err.Error())
			return
		}

		// Check token expiry (defense in depth)
		if iam.IsTokenExpired(ic.TokenExpiresAt) {
			slog.Warn("auth failed: token expired", "path", r.URL.Path, "user_id", ic.UserID)
			writeJSONError(w, http.StatusUnauthorized, iam.CodeTokenExpired, "token expired")
			return
		}

		// Resolve Org/Project from headers (optional override)
		// Headers are validated against membership in repo
		resolver := iam.NewMembershipResolver(m.repo)

		if orgIDStr := r.Header.Get("X-Org-ID"); orgIDStr != "" {
			if orgID, err := uuid.Parse(orgIDStr); err == nil {
				if m.validateOrgAccess(r.Context(), ic.UserID, orgID) {
					ic.OrgID = orgID
					if role, err := resolver.ResolveOrgRole(r.Context(), ic.UserID, orgID); err == nil {
						ic.OrgRole = role
					}
				}
			}
		}

		if projectIDStr := r.Header.Get("X-Project-ID"); projectIDStr != "" {
			if projectID, err := uuid.Parse(projectIDStr); err == nil {
				if m.validateProjectAccess(r.Context(), ic.UserID, projectID) {
					ic.ProjectID = &projectID
					if role, err := resolver.ResolveProjectRole(r.Context(), ic.UserID, projectID); err == nil {
						ic.ProjectRole = &role
					}
				}
			}
		}

		// Recompute permissions with resolved org/project
		perms, err := resolver.ResolvePermissions(r.Context(), ic.UserID, ic.OrgID, ic.ProjectID)
		if err == nil {
			ic.Permissions = perms
		}

		// Inject request ID for tracing
		requestID, correlationID := normalizeTraceIDs(r)
		ic.RequestID = requestID.String()
		ic.CorrelationID = correlationID.String()

		// Add to context
		ctx := iam.WithIdentityContext(r.Context(), ic)
		ctx = iam.WithRequestID(ctx, ic.RequestID)

		slog.Info("authenticated request",
			"request_id", ic.RequestID,
			"correlation_id", ic.CorrelationID,
			"user_id", ic.UserID,
			"email", ic.Email,
			"auth_method", ic.AuthMethod,
			"org_id", ic.OrgID,
			"org_role", ic.OrgRole,
			"project_id", ic.ProjectID,
			"path", r.URL.Path,
			"method", r.Method,
		)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// validateOrgAccess checks if user is active member of organization.
func (m *AuthMiddleware) validateOrgAccess(ctx context.Context, userID, orgID uuid.UUID) bool {
	mem, err := m.repo.FindMembership(ctx, orgID, userID)
	return err == nil && mem.Status == "active"
}

func normalizeTraceID(value string) (uuid.UUID, bool) {
	if value == "" {
		return uuid.New(), false
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.New(), true
	}
	return id, false
}

func normalizeTraceIDs(r *http.Request) (uuid.UUID, uuid.UUID) {
	requestValue := r.Header.Get("X-Request-ID")
	requestID, invalidRequestID := normalizeTraceID(requestValue)
	if invalidRequestID {
		logInvalidTraceID("X-Request-ID", requestValue, requestID)
	}
	r.Header.Set("X-Request-ID", requestID.String())

	correlationValue := r.Header.Get("X-Correlation-ID")
	correlationID := requestID
	if correlationValue != "" {
		var invalidCorrelationID bool
		correlationID, invalidCorrelationID = normalizeTraceID(correlationValue)
		if invalidCorrelationID {
			logInvalidTraceID("X-Correlation-ID", correlationValue, correlationID)
		}
	}
	r.Header.Set("X-Correlation-ID", correlationID.String())
	return requestID, correlationID
}

func logInvalidTraceID(header, value string, replacement uuid.UUID) {
	const maxLoggedValueLength = 256
	truncated := len(value) > maxLoggedValueLength
	if truncated {
		value = value[:maxLoggedValueLength]
	}
	slog.Warn("invalid trace identifier replaced",
		"header", header,
		"invalid_value", value,
		"value_truncated", truncated,
		"replacement", replacement.String(),
	)
}

// validateProjectAccess checks if user has access to project via org membership.
func (m *AuthMiddleware) validateProjectAccess(ctx context.Context, userID, projectID uuid.UUID) bool {
	hasAccess, err := m.repo.CheckUserProjectAccess(ctx, userID, projectID)
	return err == nil && hasAccess
}

// RequirePermission returns middleware that checks permission before calling next.
func RequirePermission(checker iam.PermissionChecker, resource, action string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ic, ok := iam.IdentityContextFromContext(r.Context())
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, iam.CodeUnauthorized, "no identity context")
				return
			}

			allowed, err := checker.Can(r.Context(), ic, resource, action)
			if err != nil || !allowed {
				code := iam.CodeForbidden
				if err != nil {
					code = iam.MapErrorToCode(err)
				}
				writeJSONError(w, http.StatusForbidden, code, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole returns middleware that checks for a system role.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ic, ok := iam.IdentityContextFromContext(r.Context())
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, iam.CodeUnauthorized, "no identity context")
				return
			}

			hasRole := false
			for _, r := range ic.SystemRoles {
				if r == role {
					hasRole = true
					break
				}
			}

			if !hasRole {
				writeJSONError(w, http.StatusForbidden, iam.CodeForbidden, "requires role "+role)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireOrgRole returns middleware that checks for org membership role.
func RequireOrgRole(roles ...string) func(http.Handler) http.Handler {
	roleSet := make(map[string]bool)
	for _, r := range roles {
		roleSet[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ic, ok := iam.IdentityContextFromContext(r.Context())
			if !ok {
				writeJSONError(w, http.StatusUnauthorized, iam.CodeUnauthorized, "no identity context")
				return
			}

			if !roleSet[ic.OrgRole] {
				writeJSONError(w, http.StatusForbidden, iam.CodeForbidden, "requires org role")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
