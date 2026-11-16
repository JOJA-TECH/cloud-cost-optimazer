package adapters

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
)

type SlogAuditLogger struct {
	logger *slog.Logger
}

func NewSlogAuditLogger(logger *slog.Logger) *SlogAuditLogger {
	if logger == nil {
		logger = slog.Default()
	}
	return &SlogAuditLogger{logger: logger}
}

func (l *SlogAuditLogger) Record(ctx context.Context, event audit.AuditEvent) error {
	l.logger.InfoContext(ctx, "audit event recorded",
		"event_id", event.EventID,
		"organization_id", event.OrganizationID,
		"user_id", event.Actor.UserID,
		"principal_id", event.Actor.PrincipalID,
		"actor_type", event.Actor.ActorType,
		"auth_method", event.Actor.AuthMethod,
		"action", event.Action,
		"resource_type", event.Resource.Type,
		"resource_id", event.Resource.ID,
		"resource_external_id", event.Resource.ExternalID,
		"occurred_at", event.OccurredAt,
		"metadata", event.Metadata,
		"ip_address", event.IPAddress,
		"user_agent", event.UserAgent,
		"request_id", event.Trace.RequestID,
		"correlation_id", event.Trace.CorrelationID,
		"operation_id", event.Trace.OperationID,
	)
	return nil
}

func (l *SlogAuditLogger) PersistenceFailed(ctx context.Context, event audit.AuditEvent, err error) {
	l.logger.ErrorContext(ctx, "audit event persistence failed",
		"organization_id", event.OrganizationID,
		"user_id", event.Actor.UserID,
		"action", event.Action,
		"actor_type", event.Actor.ActorType,
		"resource_type", event.Resource.Type,
		"resource_id", event.Resource.ID,
		"request_id", event.Trace.RequestID,
		"correlation_id", event.Trace.CorrelationID,
		"operation_id", event.Trace.OperationID,
		"error", "audit event persistence failed",
		"error_type", fmt.Sprintf("%T", err),
	)
}

var _ audit.AuditLogger = (*SlogAuditLogger)(nil)
var _ audit.AuditFailureLogger = (*SlogAuditLogger)(nil)
