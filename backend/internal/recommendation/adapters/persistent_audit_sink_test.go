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

type decisionTraceServiceFunc func(context.Context, audit.DecisionTrace) error

func (f decisionTraceServiceFunc) RecordDecision(ctx context.Context, trace audit.DecisionTrace) error {
	return f(ctx, trace)
}

func (f decisionTraceServiceFunc) GetRecommendationTrace(context.Context, uuid.UUID) (audit.RecommendationTrace, error) {
	return audit.RecommendationTrace{}, nil
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

	var recorded audit.DecisionTrace
	sink := NewPersistentAuditSink(auditLoggerFunc(func(context.Context, audit.AuditEvent) error {
		t.Fatal("decision was written through the event logger instead of DecisionTraceService")
		return nil
	}), decisionTraceServiceFunc(func(ctx context.Context, trace audit.DecisionTrace) error {
		recorded = trace
		gotContext, ok := audit.CorrelationContextFromContext(ctx)
		if !ok || gotContext.RequestID != requestID || gotContext.CorrelationID != correlationID ||
			gotContext.OperationID == nil || *gotContext.OperationID != operationID {
			t.Fatalf("trace context was not forwarded: %#v", gotContext)
		}
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
		Status:            "proposed",
		Rationale:         "Evidence supports a smaller SKU.",
		EvidenceReferences: []string{
			"cpu_p95", "estimated_savings",
		},
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
	if recorded.Action != "recommendation_generated" || recorded.Status != "proposed" ||
		recorded.RecommendationUID != recommendationID || recorded.ResourceUID != resourceID ||
		recorded.ScenarioUID == nil || *recorded.ScenarioUID != scenarioID ||
		recorded.PolicyUID == nil || *recorded.PolicyUID != policyID {
		t.Fatalf("decision trace does not identify the recommendation: %#v", recorded)
	}
	if recorded.Actor.UserID == nil || *recorded.Actor.UserID != userID ||
		recorded.Actor.ActorType != audit.ActorTypeUser {
		t.Fatalf("actor was not mapped from IAM: %#v", recorded.Actor)
	}
	if recorded.PolicyVersion == nil || *recorded.PolicyVersion != 3 ||
		recorded.AlgorithmVersion != "rule_v1" ||
		recorded.RiskScore == nil || *recorded.RiskScore != 0.21 ||
		recorded.RiskLevel == nil || *recorded.RiskLevel != "low" ||
		recorded.ConfidenceScore == nil || *recorded.ConfidenceScore != 0.87 ||
		recorded.EstimatedSaving == nil || *recorded.EstimatedSaving != 250.5 ||
		recorded.EvidenceCount != 4 || recorded.Decision != "resize_down" ||
		recorded.Rationale != event.Rationale ||
		len(recorded.EvidenceReferences) != 2 {
		t.Fatalf("decision measurements and rationale were not mapped: %#v", recorded)
	}
	if recorded.OutputReferences["selected_candidate"] != "sku-small" {
		t.Fatalf("selected candidate missing from output references: %#v", recorded.OutputReferences)
	}
	if !recorded.CreatedAt.Equal(occurredAt) {
		t.Fatalf("created_at = %v, want %v", recorded.CreatedAt, occurredAt)
	}
}

func TestPersistentAuditSinkRecordsGenericLifecycleEvent(t *testing.T) {
	requestID, correlationID, operationID := uuid.New(), uuid.New(), uuid.New()
	resourceUID := uuid.New()
	ctx := audit.WithCorrelationContext(context.Background(), audit.CorrelationContext{
		RequestID: requestID, CorrelationID: correlationID, OperationID: &operationID,
	})
	var recorded audit.AuditEvent
	sink := NewPersistentAuditSink(auditLoggerFunc(func(_ context.Context, event audit.AuditEvent) error {
		recorded = event
		return nil
	}), nil, nil)

	err := sink.RecordEvent(ctx, recommendation.RecommendationAuditEvent{
		Action: "recommendation_evaluation_started", ResourceUID: resourceUID,
		RequestID: requestID.String(), CorrelationID: correlationID.String(),
		OperationID: operationID.String(), Metadata: map[string]any{"phase": "started"},
	})
	if err != nil {
		t.Fatalf("record lifecycle event: %v", err)
	}
	if recorded.Action != "recommendation_evaluation_started" ||
		recorded.Resource.Type != "resource" || recorded.Resource.ID == nil ||
		*recorded.Resource.ID != resourceUID {
		t.Fatalf("lifecycle event resource/action mismatch: %#v", recorded)
	}
	if recorded.Actor.ActorType != audit.ActorTypeSystem {
		t.Fatalf("anonymous background event actor = %q, want system", recorded.Actor.ActorType)
	}
	if recorded.Trace.RequestID != requestID || recorded.Trace.CorrelationID != correlationID ||
		recorded.Trace.OperationID == nil || *recorded.Trace.OperationID != operationID {
		t.Fatalf("lifecycle trace context mismatch: %#v", recorded.Trace)
	}
}

func TestPersistentAuditSinkRejectsInvalidTraceIDsWithoutContext(t *testing.T) {
	var called bool
	sink := NewPersistentAuditSink(auditLoggerFunc(func(context.Context, audit.AuditEvent) error {
		called = true
		return nil
	}), decisionTraceServiceFunc(func(context.Context, audit.DecisionTrace) error {
		t.Fatal("invalid event reached decision trace service")
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
	}), decisionTraceServiceFunc(func(context.Context, audit.DecisionTrace) error {
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
