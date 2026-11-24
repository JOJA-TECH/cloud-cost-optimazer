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
// [From, To); nil bounds are unbounded.
type AuditFilter struct {
	OrganizationID *uuid.UUID
	UserID         *uuid.UUID
	Action         string
	EntityType     string
	EntityID       *uuid.UUID
	CorrelationID  *uuid.UUID
	OperationID    *uuid.UUID
	From           *time.Time
	To             *time.Time
	Limit          int
	Offset         int
}

// RecommendationTrace groups decision traces and audit events for a recommendation.
type RecommendationTrace struct {
	RecommendationUID uuid.UUID
	DecisionTraces    []DecisionTrace
	Events            []AuditEvent
}

type AuditEventResponse struct {
	EventID        int64               `json:"event_id"`
	OrganizationID *uuid.UUID          `json:"organization_id,omitempty"`
	Actor          ActorResponse       `json:"actor"`
	Action         string              `json:"action"`
	Resource       ResourceResponse    `json:"resource"`
	OccurredAt     time.Time           `json:"occurred_at"`
	Trace          CorrelationResponse `json:"trace"`
	Metadata       map[string]any      `json:"metadata"`
	IPAddress      *string             `json:"ip_address,omitempty"`
	UserAgent      *string             `json:"user_agent,omitempty"`
}

type ActorResponse struct {
	UserID      *uuid.UUID `json:"user_id,omitempty"`
	ActorType   string     `json:"actor_type"`
	AuthMethod  string     `json:"auth_method,omitempty"`
	PrincipalID *string    `json:"principal_id,omitempty"`
}

type ResourceResponse struct {
	Type       string     `json:"type,omitempty"`
	ID         *uuid.UUID `json:"id,omitempty"`
	ExternalID *string    `json:"external_id,omitempty"`
}

type CorrelationResponse struct {
	RequestID     uuid.UUID  `json:"request_id,omitempty"`
	CorrelationID uuid.UUID  `json:"correlation_id,omitempty"`
	OperationID   *uuid.UUID `json:"operation_id,omitempty"`
}

func EventResponse(event AuditEvent) AuditEventResponse {
	return AuditEventResponse{
		EventID:        event.EventID,
		OrganizationID: cloneAuditUUID(event.OrganizationID),
		Actor: ActorResponse{
			UserID:      cloneAuditUUID(event.Actor.UserID),
			ActorType:   event.Actor.ActorType,
			AuthMethod:  event.Actor.AuthMethod,
			PrincipalID: cloneAuditString(event.Actor.PrincipalID),
		},
		Action: event.Action,
		Resource: ResourceResponse{
			Type:       event.Resource.Type,
			ID:         cloneAuditUUID(event.Resource.ID),
			ExternalID: cloneAuditString(event.Resource.ExternalID),
		},
		OccurredAt: event.OccurredAt.UTC(),
		Trace: CorrelationResponse{
			RequestID:     event.Trace.RequestID,
			CorrelationID: event.Trace.CorrelationID,
			OperationID:   cloneAuditUUID(event.Trace.OperationID),
		},
		Metadata:  event.Metadata,
		IPAddress: cloneAuditString(event.IPAddress),
		UserAgent: cloneAuditString(event.UserAgent),
	}
}

func cloneAuditUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneAuditString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
