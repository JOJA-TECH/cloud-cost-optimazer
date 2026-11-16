package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
)

type auditLoggerFunc func(context.Context, audit.AuditEvent) error

func (f auditLoggerFunc) Record(ctx context.Context, event audit.AuditEvent) error {
	return f(ctx, event)
}

func TestPersistentAuditSinkMapsDecisionAndContext(t *testing.T) {
	userID := uuid.New()
	organizationID := uuid.New()
	requestID := uuid.New()
	correlationID := uuid.New()
	operationID := uuid.New()
	recommendationID := uuid.New()
	resourceID := uuid.New()
	policyID := uuid.New()
	scenarioID := uuid.New()
	selectedCandidate := "sku-small"
	occurredAt := time.Now().UTC()

	ctx := audit.WithCorrelationContext(context.Background(), audit.CorrelationContext{
		RequestID: requestID, CorrelationID: correlationID, OperationID: &operationID,
	})
	ctx = iam.WithIdentityContext(ctx, iam.IdentityContext{
		UserID: userID, OrgID: organizationID, AuthMethod: "oidc",
		RequestID: requestID.String(), CorrelationID: correlationID.String(),
	})

	var recorded audit.AuditEvent
	sink := NewPersistentAuditSink(auditLoggerFunc(func(_ context.Context, event audit.AuditEvent) error {
		recorded = event
		return nil
	}), nil)
	event := recommendation.DecisionAuditEvent{
		RecommendationUID: recommendationID,
		ResourceUID:       resourceID,
		Decision:          "resize_down",
		Actor:             userID.String(),
		OccurredAt:        occurredAt,
		RequestID:         "invalid-header-value-is-ignored-with-context",
		CorrelationID:     "invalid-header-value-is-ignored-with-context",
		PolicyUID:         policyID,
		PolicyVersion:     3,
		AlgorithmVersion:  "rule_v1",
		ScenarioUID:       scenarioID,
		SelectedCandidate: &selectedCandidate,
		RiskScore:         0.21,
		RiskLevel:         "low",
		ConfidenceScore:   0.87,
		EstimatedSaving:   250.5,
		EvidenceCount:     4,
		CandidateResults:  []map[string]any{{"sku_name": "sku-small", "selected": true}},
		Duration:          1250 * time.Millisecond,
		Errors:            []string{},
	}

	if err := sink.RecordDecision(ctx, event); err != nil {
		t.Fatalf("record decision: %v", err)
	}
	if recorded.Action != "recommendation.decision_recorded" ||
		recorded.Resource.Type != "recommendation" ||
		recorded.Resource.ID == nil || *recorded.Resource.ID != recommendationID {
		t.Fatalf("audit event does not identify the recommendation: %#v", recorded)
	}
	if recorded.OrganizationID == nil || *recorded.OrganizationID != organizationID ||
		recorded.Actor.UserID == nil || *recorded.Actor.UserID != userID ||
		recorded.Actor.ActorType != audit.ActorTypeUser {
		t.Fatalf("identity was not mapped from IAM: %#v", recorded)
	}
	if recorded.Trace.RequestID != requestID || recorded.Trace.CorrelationID != correlationID ||
		recorded.Trace.OperationID == nil || *recorded.Trace.OperationID != operationID {
		t.Fatalf("trace context was not mapped from request context: %#v", recorded.Trace)
	}
	for key, want := range map[string]any{
		"recommendation_uid": recommendationID.String(),
		"scenario_uid":       scenarioID.String(),
		"resource_uid":       resourceID.String(),
		"policy_uid":         policyID.String(),
		"risk_score":         0.21,
		"risk_level":         "low",
		"confidence_score":   0.87,
		"estimated_saving":   250.5,
		"evidence_count":     4,
		"selected_candidate": "sku-small",
		"duration_ms":        int64(1250),
	} {
		if recorded.Metadata[key] != want {
			t.Errorf("metadata[%q] = %#v, want %#v", key, recorded.Metadata[key], want)
		}
	}
	if !recorded.OccurredAt.Equal(occurredAt) {
		t.Fatalf("occurred_at = %v, want %v", recorded.OccurredAt, occurredAt)
	}
}

func TestPersistentAuditSinkRejectsInvalidTraceIDsWithoutContext(t *testing.T) {
	var called bool
	sink := NewPersistentAuditSink(auditLoggerFunc(func(context.Context, audit.AuditEvent) error {
		called = true
		return nil
	}), nil)
	err := sink.RecordDecision(context.Background(), recommendation.DecisionAuditEvent{
		RecommendationUID: uuid.New(),
		ResourceUID:       uuid.New(),
		PolicyUID:         uuid.New(),
		ScenarioUID:       uuid.New(),
		RequestID:         "not-a-uuid",
	})
	if !errors.Is(err, audit.ErrInvalidTraceContext) {
		t.Fatalf("got %v, want ErrInvalidTraceContext", err)
	}
	if called {
		t.Fatal("invalid trace ID reached the audit logger")
	}
}

func TestPersistentAuditSinkPropagatesLoggerFailure(t *testing.T) {
	wantErr := errors.New("audit storage unavailable")
	var logBuffer bytes.Buffer
	structuredLogger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
	sink := NewPersistentAuditSink(auditLoggerFunc(func(context.Context, audit.AuditEvent) error {
		return wantErr
	}), structuredLogger)
	correlationID := uuid.New()
	operationID := uuid.New()
	ctx := audit.WithCorrelationContext(context.Background(), audit.CorrelationContext{
		CorrelationID: correlationID, OperationID: &operationID,
	})
	recommendationID := uuid.New()
	event := recommendation.DecisionAuditEvent{
		RecommendationUID: recommendationID,
		ResourceUID:       uuid.New(),
		PolicyUID:         uuid.New(),
		ScenarioUID:       uuid.New(),
	}
	err := sink.RecordDecision(ctx, event)
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want wrapped logger error", err)
	}
	var logged map[string]any
	if err := json.Unmarshal(logBuffer.Bytes(), &logged); err != nil {
		t.Fatalf("decode structured failure log: %v", err)
	}
	for key, want := range map[string]string{
		"recommendation_uid": recommendationID.String(),
		"correlation_id":     correlationID.String(),
		"operation_id":       operationID.String(),
	} {
		if logged[key] != want {
			t.Errorf("log field %q = %#v, want %q", key, logged[key], want)
		}
	}
}
