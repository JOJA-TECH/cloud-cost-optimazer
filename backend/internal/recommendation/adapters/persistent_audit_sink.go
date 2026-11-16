package adapters

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
)

type PersistentAuditSink struct {
	auditLogger          audit.AuditLogger
	decisionTraceService audit.DecisionTraceService
	logger               *slog.Logger
}

func NewPersistentAuditSink(
	auditLogger audit.AuditLogger,
	decisionTraceService audit.DecisionTraceService,
	logger *slog.Logger,
) *PersistentAuditSink {
	if logger == nil {
		logger = slog.Default()
	}
	return &PersistentAuditSink{
		auditLogger:          auditLogger,
		decisionTraceService: decisionTraceService,
		logger:               logger,
	}
}

func (s *PersistentAuditSink) RecordDecision(ctx context.Context, event recommendation.DecisionAuditEvent) error {
	if s == nil {
		return logAuditPending(nil, ctx, event, audit.ErrAuditLogger)
	}
	if s.decisionTraceService == nil {
		return s.logAuditPending(ctx, event, audit.ErrAuditLogger)
	}
	if event.RecommendationUID == uuid.Nil || event.ResourceUID == uuid.Nil ||
		event.PolicyUID == uuid.Nil || event.ScenarioUID == uuid.Nil {
		err := fmt.Errorf("%w: recommendation, resource, policy, and scenario IDs are required",
			audit.ErrInvalidAuditEvent)
		return s.logAuditPending(ctx, event, err)
	}

	actor, _, trace, hasIdentity, err := auditContextFromDecision(ctx, event)
	if err != nil {
		return s.logAuditPending(ctx, event, err)
	}
	ctx = audit.WithCorrelationContext(ctx, trace)
	policyVersion := event.PolicyVersion
	riskScore := event.RiskScore
	confidenceScore := event.ConfidenceScore
	estimatedSaving := event.EstimatedSaving
	inputReferences := map[string]any{
		"details":      event.Details,
		"resource_uid": event.ResourceUID.String(),
		"scenario_uid": event.ScenarioUID.String(),
		"policy_uid":   event.PolicyUID.String(),
	}
	outputReferences := map[string]any{
		"recommendation_uid": event.RecommendationUID.String(),
		"candidate_results":  event.CandidateResults,
		"duration_ms":        event.Duration.Milliseconds(),
		"error_count":        len(event.Errors),
	}
	if event.SelectedCandidate != nil {
		outputReferences["selected_candidate"] = *event.SelectedCandidate
	}
	if !hasIdentity && strings.TrimSpace(event.Actor) != "" {
		inputReferences["actor_reference"] = strings.TrimSpace(event.Actor)
	}
	var riskLevel *string
	if event.RiskLevel != "" {
		riskLevel = &event.RiskLevel
	}
	traceRecord := audit.DecisionTrace{
		TraceID:            uuid.New(),
		RecommendationUID:  event.RecommendationUID,
		ScenarioUID:        uuidPointer(event.ScenarioUID),
		ResourceUID:        event.ResourceUID,
		PolicyUID:          uuidPointer(event.PolicyUID),
		PolicyVersion:      &policyVersion,
		AlgorithmVersion:   event.AlgorithmVersion,
		Action:             "recommendation_generated",
		Status:             event.Status,
		RiskScore:          &riskScore,
		RiskLevel:          riskLevel,
		ConfidenceScore:    &confidenceScore,
		EstimatedSaving:    &estimatedSaving,
		EvidenceCount:      event.EvidenceCount,
		EvidenceReferences: append([]string(nil), event.EvidenceReferences...),
		Decision:           event.Decision,
		Rationale:          event.Rationale,
		InputReferences:    inputReferences,
		OutputReferences:   outputReferences,
		Actor:              actor,
		CreatedAt:          event.OccurredAt,
	}
	if err := s.decisionTraceService.RecordDecision(ctx, traceRecord); err != nil {
		return s.logAuditPending(ctx, event, err)
	}
	return nil
}

func (s *PersistentAuditSink) RecordEvent(ctx context.Context, event recommendation.RecommendationAuditEvent) error {
	contextEvent := decisionEventForAuditContext(event)
	if s == nil {
		return logAuditPending(nil, ctx, contextEvent, audit.ErrAuditLogger)
	}
	if s.auditLogger == nil {
		return s.logAuditPending(ctx, contextEvent, audit.ErrAuditLogger)
	}
	if strings.TrimSpace(event.Action) == "" {
		err := fmt.Errorf("%w: action is required", audit.ErrInvalidAuditEvent)
		return s.logAuditPending(ctx, contextEvent, err)
	}
	actor, organizationID, trace, _, err := auditContextFromDecision(ctx, contextEvent)
	if err != nil {
		return s.logAuditPending(ctx, contextEvent, err)
	}
	metadata := make(map[string]any, len(event.Metadata)+4)
	for key, value := range event.Metadata {
		metadata[key] = value
	}
	if event.RecommendationUID != uuid.Nil {
		metadata["recommendation_uid"] = event.RecommendationUID.String()
	}
	if event.ScenarioUID != uuid.Nil {
		metadata["scenario_uid"] = event.ScenarioUID.String()
	}
	if event.ResourceUID != uuid.Nil {
		metadata["resource_uid"] = event.ResourceUID.String()
	}
	if event.PolicyUID != uuid.Nil {
		metadata["policy_uid"] = event.PolicyUID.String()
	}
	if event.ResourceType != "" {
		metadata["entity_type"] = event.ResourceType
	}
	resource := audit.ResourceReference{}
	if event.RecommendationUID != uuid.Nil {
		resource = audit.ResourceReference{Type: "recommendation", ID: uuidPointer(event.RecommendationUID)}
	} else if event.ResourceUID != uuid.Nil {
		resource = audit.ResourceReference{Type: "resource", ID: uuidPointer(event.ResourceUID)}
	}
	auditEvent := audit.AuditEvent{
		OrganizationID: organizationID,
		Actor:          actor,
		Action:         strings.TrimSpace(event.Action),
		Resource:       resource,
		OccurredAt:     event.OccurredAt,
		Trace:          trace,
		Metadata:       metadata,
	}
	if err := s.auditLogger.Record(ctx, auditEvent); err != nil {
		return s.logAuditPending(ctx, contextEvent, err)
	}
	return nil
}

func decisionEventForAuditContext(event recommendation.RecommendationAuditEvent) recommendation.DecisionAuditEvent {
	return recommendation.DecisionAuditEvent{
		RecommendationUID: event.RecommendationUID,
		ResourceUID:       event.ResourceUID,
		ScenarioUID:       event.ScenarioUID,
		PolicyUID:         event.PolicyUID,
		RequestID:         event.RequestID,
		CorrelationID:     event.CorrelationID,
		OperationID:       event.OperationID,
	}
}

func (s *PersistentAuditSink) logAuditPending(
	ctx context.Context,
	event recommendation.DecisionAuditEvent,
	err error,
) error {
	if s == nil {
		return logAuditPending(nil, ctx, event, err)
	}
	return logAuditPending(s.logger, ctx, event, err)
}

func logAuditPending(
	logger *slog.Logger,
	ctx context.Context,
	event recommendation.DecisionAuditEvent,
	err error,
) error {
	if logger == nil {
		logger = slog.Default()
	}
	correlationID := safeTraceID(event.CorrelationID)
	operationID := safeTraceID(event.OperationID)
	if trace, ok := audit.CorrelationContextFromContext(ctx); ok {
		if trace.CorrelationID != uuid.Nil {
			correlationID = trace.CorrelationID.String()
		}
		if trace.OperationID != nil {
			operationID = trace.OperationID.String()
		}
	}
	logger.ErrorContext(ctx, "recommendation audit pending",
		"recommendation_uid", event.RecommendationUID.String(),
		"correlation_id", correlationID,
		"operation_id", operationID,
		"error", "recommendation audit persistence failed",
		"error_type", fmt.Sprintf("%T", err),
	)
	return fmt.Errorf("record recommendation decision audit: %w", err)
}

func safeTraceID(value string) string {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || id == uuid.Nil {
		return ""
	}
	return id.String()
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
