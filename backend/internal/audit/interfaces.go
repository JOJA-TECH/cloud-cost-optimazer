package audit

import (
	"context"

	"github.com/google/uuid"
)

type AuditLogger interface {
	Record(ctx context.Context, event AuditEvent) error
}

// AuditFailureLogger reports operational failures while persisting audit events.
type AuditFailureLogger interface {
	PersistenceFailed(ctx context.Context, event AuditEvent, err error)
}

type TraceContext interface {
	FromContext(ctx context.Context) (CorrelationContext, bool)
	WithContext(ctx context.Context, trace CorrelationContext) context.Context
}

type AuditQueryService interface {
	GetEvent(ctx context.Context, eventID int64) (AuditEvent, error)
	ListEvents(ctx context.Context, filter AuditFilter) ([]AuditEvent, error)
	GetResourceHistory(ctx context.Context, resource ResourceReference, filter AuditFilter) ([]AuditEvent, error)
	GetCorrelationHistory(ctx context.Context, correlationID uuid.UUID) ([]AuditEvent, error)
}

type DecisionTraceService interface {
	RecordDecision(ctx context.Context, trace DecisionTrace) error
	GetRecommendationTrace(ctx context.Context, recommendationUID uuid.UUID) (RecommendationTrace, error)
}

// AuditRepository is the domain persistence port; its implementation belongs
// in the repositories package and depends on this module's entities.
type AuditRepository interface {
	InsertEvent(ctx context.Context, event AuditEvent) (AuditEvent, error)
	GetEvent(ctx context.Context, eventID int64) (AuditEvent, error)
	ListEvents(ctx context.Context, filter AuditFilter) ([]AuditEvent, error)
	ListByCorrelationID(ctx context.Context, correlationID uuid.UUID) ([]AuditEvent, error)
	ListByResource(ctx context.Context, resource ResourceReference, filter AuditFilter) ([]AuditEvent, error)
}
