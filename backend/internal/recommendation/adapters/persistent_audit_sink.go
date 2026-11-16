package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
)

type PersistentAuditSink struct {
	logger audit.AuditLogger
}

func NewPersistentAuditSink(logger audit.AuditLogger) *PersistentAuditSink {
	return &PersistentAuditSink{logger: logger}
}

func (s *PersistentAuditSink) RecordDecision(ctx context.Context, event recommendation.DecisionAuditEvent) error {
	if s == nil || s.logger == nil {
		return audit.ErrAuditLogger
	}
	if event.RecommendationUID == uuid.Nil || event.ResourceUID == uuid.Nil ||
		event.PolicyUID == uuid.Nil || event.ScenarioUID == uuid.Nil {
		return fmt.Errorf("%w: recommendation, resource, policy, and scenario IDs are required",
			audit.ErrInvalidAuditEvent)
	}

	actor, organizationID, trace, hasIdentity, err := auditContextFromDecision(ctx, event)
	if err != nil {
		return err
	}
	metadata := map[string]any{
		"recommendation_uid": event.RecommendationUID.String(),
		"scenario_uid":       event.ScenarioUID.String(),
		"resource_uid":       event.ResourceUID.String(),
		"policy_uid":         event.PolicyUID.String(),
		"policy_version":     event.PolicyVersion,
		"algorithm_version":  event.AlgorithmVersion,
		"decision":           event.Decision,
		"risk_score":         event.RiskScore,
		"risk_level":         event.RiskLevel,
		"confidence_score":   event.ConfidenceScore,
		"estimated_saving":   event.EstimatedSaving,
		"evidence_count":     event.EvidenceCount,
		"candidate_results":  event.CandidateResults,
		"duration_ms":        event.Duration.Milliseconds(),
		"errors":             event.Errors,
	}
	if event.SelectedCandidate != nil {
		metadata["selected_candidate"] = *event.SelectedCandidate
	}
	if event.Details != nil {
		metadata["details"] = event.Details
	}
	if !hasIdentity && strings.TrimSpace(event.Actor) != "" {
		metadata["actor_reference"] = strings.TrimSpace(event.Actor)
	}

	auditEvent := audit.AuditEvent{
		OrganizationID: organizationID,
		Actor:          actor,
		Action:         "recommendation.decision_recorded",
		Resource: audit.ResourceReference{
			Type: "recommendation",
			ID:   uuidPointer(event.RecommendationUID),
		},
		OccurredAt: event.OccurredAt,
		Trace:      trace,
		Metadata:   metadata,
	}
	if err := s.logger.Record(ctx, auditEvent); err != nil {
		return fmt.Errorf("persist recommendation decision audit: %w", err)
	}
	return nil
}

func auditContextFromDecision(
	ctx context.Context,
	event recommendation.DecisionAuditEvent,
) (audit.Actor, *uuid.UUID, audit.CorrelationContext, bool, error) {
	identity, err := audit.AuditIdentityContextFromContext(ctx)
	if err == nil {
		return identity.Actor, identity.OrganizationID, identity.Trace, true, nil
	}
	if !errors.Is(err, audit.ErrIdentityContextMissing) {
		return audit.Actor{}, nil, audit.CorrelationContext{}, false,
			fmt.Errorf("resolve recommendation audit identity: %w", err)
	}

	actor := audit.Actor{ActorType: audit.ActorTypeSystem}
	if actorID, parseErr := uuid.Parse(strings.TrimSpace(event.Actor)); parseErr == nil && actorID != uuid.Nil {
		actor.ActorType = audit.ActorTypeUser
		actor.UserID = &actorID
	}
	trace, _ := audit.CorrelationContextFromContext(ctx)
	if trace.RequestID == uuid.Nil {
		trace.RequestID, err = parseOptionalTraceID("request ID", event.RequestID)
		if err != nil {
			return audit.Actor{}, nil, audit.CorrelationContext{}, false, err
		}
	}
	if trace.CorrelationID == uuid.Nil {
		trace.CorrelationID, err = parseOptionalTraceID("correlation ID", event.CorrelationID)
		if err != nil {
			return audit.Actor{}, nil, audit.CorrelationContext{}, false, err
		}
	}
	if trace.OperationID == nil {
		operationID, parseErr := parseOptionalTraceID("operation ID", event.OperationID)
		if parseErr != nil {
			return audit.Actor{}, nil, audit.CorrelationContext{}, false, parseErr
		}
		if operationID != uuid.Nil {
			trace.OperationID = &operationID
		}
	}
	if trace.CorrelationID == uuid.Nil {
		trace.CorrelationID = trace.RequestID
	}
	return actor, nil, trace, false, nil
}

func parseOptionalTraceID(name, value string) (uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: invalid %s", audit.ErrInvalidTraceContext, name)
	}
	return id, nil
}

func uuidPointer(value uuid.UUID) *uuid.UUID {
	copy := value
	return &copy
}

var _ recommendation.AuditSink = (*PersistentAuditSink)(nil)
