package audit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const recommendationTraceEventLimit = 200

var _ DecisionTraceService = (*Service)(nil)
var _ ScopedDecisionTraceService = (*Service)(nil)

func (s *Service) RecordDecision(ctx context.Context, trace DecisionTrace) error {
	if trace.RecommendationUID == uuid.Nil || trace.ResourceUID == uuid.Nil {
		return fmt.Errorf("%w: recommendation and resource IDs are required", ErrInvalidAuditEvent)
	}
	if trace.Decision == "" {
		return fmt.Errorf("%w: decision is required", ErrInvalidAuditEvent)
	}
	if trace.TraceID == uuid.Nil {
		trace.TraceID = uuid.New()
	}
	trace.Action = strings.TrimSpace(trace.Action)
	if trace.Action == "" {
		trace.Action = "recommendation_generated"
	}
	if trace.Status == "" {
		trace.Status = "proposed"
	}
	if trace.CreatedAt.IsZero() {
		trace.CreatedAt = time.Now().UTC()
	} else {
		trace.CreatedAt = trace.CreatedAt.UTC()
	}

	actor := trace.Actor
	if actor.ActorType == "" {
		if _, err := AuditIdentityContextFromContext(ctx); err != nil {
			if !errors.Is(err, ErrIdentityContextMissing) {
				return err
			}
			actor.ActorType = ActorTypeSystem
		}
	}

	metadata := map[string]any{
		"decision_trace_id":   trace.TraceID.String(),
		"recommendation_uid":  trace.RecommendationUID.String(),
		"resource_uid":        trace.ResourceUID.String(),
		"algorithm_version":   trace.AlgorithmVersion,
		"action":              trace.Action,
		"status":              trace.Status,
		"evidence_count":      trace.EvidenceCount,
		"evidence_references": trace.EvidenceReferences,
		"decision":            trace.Decision,
		"rationale":           trace.Rationale,
		"input_references":    trace.InputReferences,
		"output_references":   trace.OutputReferences,
	}
	if trace.ScenarioUID != nil {
		metadata["scenario_uid"] = trace.ScenarioUID.String()
	}
	if trace.PolicyUID != nil {
		metadata["policy_uid"] = trace.PolicyUID.String()
	}
	if trace.PolicyVersion != nil {
		metadata["policy_version"] = *trace.PolicyVersion
	}
	if trace.RiskScore != nil {
		metadata["risk_score"] = *trace.RiskScore
	}
	if trace.RiskLevel != nil {
		metadata["risk_level"] = *trace.RiskLevel
	}
	if trace.ConfidenceScore != nil {
		metadata["confidence_score"] = *trace.ConfidenceScore
	}
	if trace.EstimatedSaving != nil {
		metadata["estimated_saving"] = *trace.EstimatedSaving
	}

	return s.Record(ctx, AuditEvent{
		Actor:  actor,
		Action: trace.Action,
		Resource: ResourceReference{
			Type: "recommendation",
			ID:   auditUUIDPointer(trace.RecommendationUID),
		},
		OccurredAt: trace.CreatedAt,
		Metadata:   metadata,
	})
}

func (s *Service) GetRecommendationTrace(
	ctx context.Context,
	recommendationUID uuid.UUID,
) (RecommendationTrace, error) {
	return s.GetRecommendationTraceScoped(ctx, recommendationUID, AuditFilter{
		Limit: recommendationTraceEventLimit,
	})
}

func (s *Service) GetRecommendationTraceScoped(
	ctx context.Context,
	recommendationUID uuid.UUID,
	filter AuditFilter,
) (RecommendationTrace, error) {
	if recommendationUID == uuid.Nil {
		return RecommendationTrace{}, fmt.Errorf("%w: recommendation ID must be a non-zero UUID", ErrInvalidAuditEvent)
	}
	if s == nil || s.repository == nil {
		return RecommendationTrace{}, ErrAuditRepository
	}
	events, err := s.repository.ListByResource(ctx, ResourceReference{
		Type: "recommendation",
		ID:   auditUUIDPointer(recommendationUID),
	}, filter)
	if err != nil {
		return RecommendationTrace{}, fmt.Errorf("list recommendation trace events: %w", err)
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].EventID < events[j].EventID
		}
		return events[i].OccurredAt.Before(events[j].OccurredAt)
	})
	result := RecommendationTrace{
		RecommendationUID: recommendationUID,
		DecisionTraces:    make([]DecisionTrace, 0),
		Events:            events,
	}
	for _, event := range events {
		if _, exists := event.Metadata["decision_trace_id"]; !exists {
			continue
		}
		trace, err := decisionTraceFromEvent(event, recommendationUID)
		if err != nil {
			return RecommendationTrace{}, err
		}
		result.DecisionTraces = append(result.DecisionTraces, trace)
	}
	return result, nil
}

func decisionTraceFromEvent(event AuditEvent, recommendationUID uuid.UUID) (DecisionTrace, error) {
	traceID, err := metadataUUID(event.Metadata, "decision_trace_id", true)
	if err != nil {
		return DecisionTrace{}, err
	}
	resourceID, err := metadataUUID(event.Metadata, "resource_uid", true)
	if err != nil {
		return DecisionTrace{}, err
	}
	scenarioID, err := metadataUUID(event.Metadata, "scenario_uid", false)
	if err != nil {
		return DecisionTrace{}, err
	}
	policyID, err := metadataUUID(event.Metadata, "policy_uid", false)
	if err != nil {
		return DecisionTrace{}, err
	}
	policyVersion, err := metadataInt(event.Metadata, "policy_version")
	if err != nil {
		return DecisionTrace{}, err
	}
	riskScore, err := metadataFloat(event.Metadata, "risk_score")
	if err != nil {
		return DecisionTrace{}, err
	}
	confidenceScore, err := metadataFloat(event.Metadata, "confidence_score")
	if err != nil {
		return DecisionTrace{}, err
	}
	estimatedSaving, err := metadataFloat(event.Metadata, "estimated_saving")
	if err != nil {
		return DecisionTrace{}, err
	}
	riskLevelValue, err := metadataString(event.Metadata, "risk_level")
	if err != nil {
		return DecisionTrace{}, err
	}
	var riskLevel *string
	if riskLevelValue != "" {
		riskLevel = &riskLevelValue
	}
	action, err := metadataString(event.Metadata, "action")
	if err != nil {
		return DecisionTrace{}, err
	}
	status, err := metadataString(event.Metadata, "status")
	if err != nil {
		return DecisionTrace{}, err
	}
	algorithmVersion, err := metadataString(event.Metadata, "algorithm_version")
	if err != nil {
		return DecisionTrace{}, err
	}
	decision, err := metadataString(event.Metadata, "decision")
	if err != nil {
		return DecisionTrace{}, err
	}
	rationale, err := metadataString(event.Metadata, "rationale")
	if err != nil {
		return DecisionTrace{}, err
	}
	evidenceCount, err := metadataInt(event.Metadata, "evidence_count")
	if err != nil {
		return DecisionTrace{}, err
	}
	evidenceReferences, err := metadataStrings(event.Metadata, "evidence_references")
	if err != nil {
		return DecisionTrace{}, err
	}
	inputReferences, err := metadataMap(event.Metadata, "input_references")
	if err != nil {
		return DecisionTrace{}, err
	}
	outputReferences, err := metadataMap(event.Metadata, "output_references")
	if err != nil {
		return DecisionTrace{}, err
	}
	if decision == "" {
		return DecisionTrace{}, fmt.Errorf("audit event %d has an empty decision", event.EventID)
	}
	var evidenceCountValue int
	if evidenceCount != nil {
		evidenceCountValue = *evidenceCount
	}

	return DecisionTrace{
		TraceID:            *traceID,
		RecommendationUID:  recommendationUID,
		ScenarioUID:        scenarioID,
		ResourceUID:        *resourceID,
		PolicyUID:          policyID,
		PolicyVersion:      policyVersion,
		AlgorithmVersion:   algorithmVersion,
		Action:             action,
		Status:             status,
		RiskScore:          riskScore,
		RiskLevel:          riskLevel,
		ConfidenceScore:    confidenceScore,
		EstimatedSaving:    estimatedSaving,
		EvidenceCount:      evidenceCountValue,
		EvidenceReferences: evidenceReferences,
		Decision:           decision,
		Rationale:          rationale,
		InputReferences:    inputReferences,
		OutputReferences:   outputReferences,
		Actor:              event.Actor,
		CreatedAt:          event.OccurredAt.UTC(),
	}, nil
}

func metadataUUID(metadata map[string]any, key string, required bool) (*uuid.UUID, error) {
	value, err := metadataString(metadata, key)
	if err != nil {
		return nil, err
	}
	if value == "" && !required {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil {
		return nil, fmt.Errorf("audit metadata %q is not a valid UUID", key)
	}
	return &id, nil
}

func metadataString(metadata map[string]any, key string) (string, error) {
	value, exists := metadata[key]
	if !exists || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("audit metadata %q is not a string", key)
	}
	return text, nil
}

func metadataInt(metadata map[string]any, key string) (*int, error) {
	value, exists := metadata[key]
	if !exists || value == nil {
		return nil, nil
	}
	var parsed int64
	switch typed := value.(type) {
	case int:
		return intPointer(typed), nil
	case int64:
		parsed = typed
	case float64:
		parsed = int64(typed)
	case fmt.Stringer:
		var err error
		parsed, err = strconv.ParseInt(typed.String(), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("audit metadata %q is not an integer", key)
		}
	default:
		return nil, fmt.Errorf("audit metadata %q is not an integer", key)
	}
	if strconv.IntSize == 32 && (parsed > int64(^uint32(0)>>1) || parsed < -int64(^uint32(0)>>1)-1) {
		return nil, fmt.Errorf("audit metadata %q is outside the int range", key)
	}
	return intPointer(int(parsed)), nil
}

func metadataFloat(metadata map[string]any, key string) (*float64, error) {
	value, exists := metadata[key]
	if !exists || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case float64:
		return floatPointer(typed), nil
	case float32:
		return floatPointer(float64(typed)), nil
	case int:
		return floatPointer(float64(typed)), nil
	case int64:
		return floatPointer(float64(typed)), nil
	case fmt.Stringer:
		parsed, err := strconv.ParseFloat(typed.String(), 64)
		if err != nil {
			return nil, fmt.Errorf("audit metadata %q is not numeric", key)
		}
		return floatPointer(parsed), nil
	default:
		return nil, fmt.Errorf("audit metadata %q is not numeric", key)
	}
}

func metadataStrings(metadata map[string]any, key string) ([]string, error) {
	value, exists := metadata[key]
	if !exists || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("audit metadata %q contains a non-string value", key)
			}
			values = append(values, text)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("audit metadata %q is not a string array", key)
	}
}

func metadataMap(metadata map[string]any, key string) (map[string]any, error) {
	value, exists := metadata[key]
	if !exists || value == nil {
		return map[string]any{}, nil
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("audit metadata %q is not an object", key)
	}
	return result, nil
}

func auditUUIDPointer(value uuid.UUID) *uuid.UUID {
	return &value
}

func intPointer(value int) *int {
	return &value
}

func floatPointer(value float64) *float64 {
	return &value
}
