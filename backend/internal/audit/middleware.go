package audit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

func TraceMiddleware(next http.Handler, recorders ...AuditLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestValue := r.Header.Get("X-Request-ID")
		requestID, invalidRequestID := iam.NormalizeTraceID(requestValue)
		if invalidRequestID {
			logInvalidTraceID("X-Request-ID", requestValue, requestID)
		}

		correlationValue := r.Header.Get("X-Correlation-ID")
		correlationID := requestID
		if correlationValue != "" {
			var invalidCorrelationID bool
			correlationID, invalidCorrelationID = iam.NormalizeTraceID(correlationValue)
			if invalidCorrelationID {
				logInvalidTraceID("X-Correlation-ID", correlationValue, correlationID)
			}
		}

		operationID := uuid.New()
		trace := CorrelationContext{
			RequestID:     requestID,
			CorrelationID: correlationID,
			OperationID:   &operationID,
		}
		r.Header.Set("X-Request-ID", requestID.String())
		r.Header.Set("X-Correlation-ID", correlationID.String())
		r.Header.Set("X-Operation-ID", operationID.String())
		w.Header().Set("X-Request-ID", requestID.String())
		w.Header().Set("X-Correlation-ID", correlationID.String())
		w.Header().Set("X-Operation-ID", operationID.String())

		ctx := WithCorrelationContext(r.Context(), trace)
		identityCapture := iam.NewIdentityCapture()
		ctx = iam.WithIdentityCapture(ctx, identityCapture)
		request := r.WithContext(ctx)
		plan := requestAuditPlan(request.Method, request.URL.Path)
		startedAt := time.Now().UTC()

		captured := &statusCaptureWriter{ResponseWriter: w}
		next.ServeHTTP(captured, request)
		status := captured.status
		if status == 0 {
			status = http.StatusOK
		}
		authRoute := isAuthenticationPath(request.URL.Path)
		if status == http.StatusForbidden && !authRoute {
			denialPlan := plan
			denialPlan.startAction = ""
			recordHTTPRequest(ctx, request, identityCapture, recorders, denialPlan,
				"authorization_denied", "failed", status, time.Now().UTC())
			return
		}
		if status == http.StatusUnauthorized && !authRoute {
			return
		}
		if status == http.StatusTooManyRequests && !authRoute {
			return
		}
		if plan.startAction != "" {
			recordHTTPRequest(ctx, request, identityCapture, recorders, plan, plan.startAction, "started", 0, startedAt)
		}
		action := plan.failureAction
		phase := "failed"
		if status < http.StatusBadRequest {
			action = plan.successAction
			phase = "completed"
		}
		if action != "" {
			recordHTTPRequest(ctx, request, identityCapture, recorders, plan, action, phase, status, time.Now().UTC())
		}
	})
}

type statusCaptureWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCaptureWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCaptureWriter) Write(value []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(value)
}

func (w *statusCaptureWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusCaptureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *statusCaptureWriter) Push(target string, options *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, options)
}

func (w *statusCaptureWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

type auditRequestPlan struct {
	startAction   string
	successAction string
	failureAction string
	resourceType  string
	resourceID    *uuid.UUID
}

func requestAuditPlan(method, path string) auditRequestPlan {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	plan := auditRequestPlan{}
	switch {
	case method == http.MethodPost && path == "/auth/register":
		plan = auditRequestPlan{successAction: "user_registered", failureAction: "user_registration_failed", resourceType: "user"}
	case method == http.MethodPost && path == "/auth/login":
		plan = auditRequestPlan{successAction: "user_login_succeeded", failureAction: "user_login_failed", resourceType: "user"}
	case method == http.MethodPost && path == "/auth/token/refresh":
		plan = auditRequestPlan{successAction: "token_refreshed", failureAction: "token_refresh_failed", resourceType: "user"}
	case method == http.MethodGet && path == "/auth/oidc/callback":
		plan = auditRequestPlan{successAction: "user_login_succeeded", failureAction: "user_login_failed", resourceType: "user"}
	case method == http.MethodPost && path == "/api/v1/cloud-accounts":
		plan = auditRequestPlan{
			successAction: "cloud_account_registered", failureAction: "cloud_account_registration_failed",
			resourceType: "cloud_account",
		}
	case method == http.MethodPost && len(segments) == 5 &&
		segments[0] == "api" && segments[1] == "v1" && segments[2] == "cloud-accounts" &&
		segments[4] == "validate":
		plan = auditRequestPlan{
			successAction: "cloud_account_validated", failureAction: "cloud_account_validation_failed",
			resourceType: "cloud_account", resourceID: pathUUID(segments[3]),
		}
	case method == http.MethodPost && len(segments) == 5 &&
		segments[0] == "api" && segments[1] == "v1" && segments[2] == "cloud-accounts" &&
		segments[4] == "sync":
		plan = auditRequestPlan{
			startAction: "resource_sync_started", successAction: "resource_sync_completed",
			failureAction: "resource_sync_failed", resourceType: "cloud_account",
			resourceID: pathUUID(segments[3]),
		}
	case method == http.MethodPost && path == "/api/v1/acquisitions/run":
		plan = auditRequestPlan{
			startAction: "acquisition_started", successAction: "acquisition_completed",
			failureAction: "acquisition_failed", resourceType: "acquisition",
		}
	}
	return plan
}

func recordHTTPRequest(
	ctx context.Context,
	r *http.Request,
	capture *iam.IdentityCapture,
	recorders []AuditLogger,
	plan auditRequestPlan,
	action, phase string,
	status int,
	occurredAt time.Time,
) {
	if len(recorders) == 0 || recorders[0] == nil {
		return
	}
	trace, _ := CorrelationContextFromContext(ctx)
	eventContext := ctx
	actor := Actor{ActorType: ActorTypeAnonymous}
	var organizationID *uuid.UUID
	resourceID := cloneAuditUUID(plan.resourceID)
	if identity, ok := capture.Identity(); ok {
		identity.RequestID = trace.RequestID.String()
		identity.CorrelationID = trace.CorrelationID.String()
		eventContext = iam.WithIdentityContext(eventContext, identity)
		identityAudit, err := AuditIdentityContextFromContext(eventContext)
		if err == nil {
			actor = identityAudit.Actor
			organizationID = identityAudit.OrganizationID
			if plan.resourceType == "user" && resourceID == nil {
				resourceID = cloneAuditUUID(actor.UserID)
			}
		}
	}
	metadata := map[string]any{
		"phase":  phase,
		"method": r.Method,
	}
	if status != 0 {
		metadata["http_status"] = status
	}
	if plan.resourceType != "" {
		metadata["entity_type"] = plan.resourceType
	}
	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteIP); err == nil {
		remoteIP = host
	}
	var ipAddress *string
	if remoteIP != "" {
		ipAddress = &remoteIP
	}
	event := AuditEvent{
		OrganizationID: organizationID,
		Actor:          actor,
		Action:         action,
		OccurredAt:     occurredAt,
		Trace:          trace,
		Metadata:       metadata,
		IPAddress:      ipAddress,
	}
	if r.UserAgent() != "" {
		userAgent := r.UserAgent()
		event.UserAgent = &userAgent
	}
	if plan.resourceType != "" && resourceID != nil {
		event.Resource = ResourceReference{Type: plan.resourceType, ID: resourceID}
	} else if plan.resourceType == "user" && actor.UserID != nil {
		event.Resource = ResourceReference{Type: "user", ID: cloneAuditUUID(actor.UserID)}
	}
	if err := recorders[0].Record(eventContext, event); err != nil {
		slog.ErrorContext(eventContext, "record HTTP audit event failed",
			"action", action,
			"correlation_id", trace.CorrelationID,
			"operation_id", trace.OperationID,
			"error", "audit event record failed",
			"error_type", fmt.Sprintf("%T", err),
		)
	}
}

func isAuthenticationPath(path string) bool {
	return path == "/auth/register" || path == "/auth/login" ||
		path == "/auth/oidc/callback" || path == "/auth/token/refresh"
}

func pathUUID(value string) *uuid.UUID {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return nil
	}
	return &id
}

func logInvalidTraceID(header, value string, replacement uuid.UUID) {
	const maxLoggedValueLength = 256
	valueLength := len(value)
	truncated := valueLength > maxLoggedValueLength
	slog.Warn("invalid trace identifier replaced",
		"header", header,
		"invalid_value_length", valueLength,
		"value_truncated", truncated,
		"replacement", replacement.String(),
	)
}
