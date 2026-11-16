package audit

import (
	"time"

	"github.com/google/uuid"
)

const (
	ActorTypeUser             = "user"
	ActorTypeServicePrincipal = "service_principal"
	ActorTypeManagedIdentity  = "managed_identity"
	ActorTypeSystem           = "system"
	ActorTypeAnonymous        = "anonymous"
)

// AuditEvent is an append-only record of an action performed in the system.
type AuditEvent struct {
	EventID        int64
	OrganizationID *uuid.UUID
	Actor          Actor
	Action         string
	Resource       ResourceReference
	OccurredAt     time.Time
	Trace          CorrelationContext
	Metadata       map[string]any
	IPAddress      *string
	UserAgent      *string
}

type Actor struct {
	UserID      *uuid.UUID
	ActorType   string
	AuthMethod  string
	PrincipalID *string
}

type ResourceReference struct {
	Type       string
	ID         *uuid.UUID
	ExternalID *string
}

type CorrelationContext struct {
	RequestID     uuid.UUID
	CorrelationID uuid.UUID
	OperationID   *uuid.UUID
}

type AuditIdentityContext struct {
	Actor          Actor
	OrganizationID *uuid.UUID
	Trace          CorrelationContext
}

// DecisionTrace links the inputs and outcome of a recommendation decision.
type DecisionTrace struct {
	TraceID            uuid.UUID
	RecommendationUID  uuid.UUID
	ScenarioUID        *uuid.UUID
	ResourceUID        uuid.UUID
	PolicyUID          *uuid.UUID
	PolicyVersion      *int
	AlgorithmVersion   string
	Action             string
	Status             string
	RiskScore          *float64
	RiskLevel          *string
	ConfidenceScore    *float64
	EstimatedSaving    *float64
	EvidenceCount      int
	EvidenceReferences []string
	Decision           string
	Rationale          string
	InputReferences    map[string]any
	OutputReferences   map[string]any
	Actor              Actor
	CreatedAt          time.Time
}

// AuditFilter selects audit events. Time bounds use the half-open interval
// [FromUTC, ToUTC); zero values leave that bound unbounded.
type AuditFilter struct {
	OrganizationID *uuid.UUID
	ActorUserID    *uuid.UUID
	ActorType      string
	Action         string
	Resource       *ResourceReference
	FromUTC        time.Time
	ToUTC          time.Time
	Limit          int
	Offset         int
}

// RecommendationTrace groups decision traces and audit events for a recommendation.
type RecommendationTrace struct {
	RecommendationUID uuid.UUID
	DecisionTraces    []DecisionTrace
	Events            []AuditEvent
}
