package audit

import (
	"context"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/iam"
	"github.com/google/uuid"
)

type decisionTraceRepositoryStub struct {
	AuditRepository
	events     []AuditEvent
	lastFilter AuditFilter
}

func (r *decisionTraceRepositoryStub) InsertEvent(_ context.Context, event AuditEvent) (AuditEvent, error) {
	event.EventID = int64(len(r.events) + 1)
	r.events = append(r.events, event)
	return event, nil
}

func (r *decisionTraceRepositoryStub) ListByResource(
	_ context.Context,
	resource ResourceReference,
	filter AuditFilter,
) ([]AuditEvent, error) {
	r.lastFilter = filter
	matches := make([]AuditEvent, 0)
	for _, event := range r.events {
		if event.Resource.Type == resource.Type && event.Resource.ID != nil &&
			resource.ID != nil && *event.Resource.ID == *resource.ID {
			if filter.OrganizationID != nil &&
				(event.OrganizationID == nil || *event.OrganizationID != *filter.OrganizationID) {
				continue
			}
			if filter.UserID != nil &&
				(event.Actor.UserID == nil || *event.Actor.UserID != *filter.UserID) {
				continue
			}
			matches = append(matches, event)
		}
	}
	return matches, nil
}

func TestServiceRecordsAndReconstructsRecommendationDecisionTrace(t *testing.T) {
	recommendationID := uuid.New()
	resourceID := uuid.New()
	scenarioID := uuid.New()
	policyID := uuid.New()
	userID := uuid.New()
	organizationID := uuid.New()
	requestID := uuid.New()
	correlationID := uuid.New()
	operationID := uuid.New()
	policyVersion := 4
	riskScore := 0.21
	riskLevel := "low"
	confidence := 0.87
	saving := 125.5
	createdAt := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)

	ctx := WithCorrelationContext(context.Background(), CorrelationContext{
		RequestID: requestID, CorrelationID: correlationID, OperationID: &operationID,
	})
	ctx = iam.WithIdentityContext(ctx, iam.IdentityContext{
		UserID: userID, OrgID: organizationID, AuthMethod: "oidc",
		RequestID: requestID.String(), CorrelationID: correlationID.String(),
	})

	repository := &decisionTraceRepositoryStub{}
	logger := &auditLoggerStub{}
	service, err := NewService(repository, logger)
	if err != nil {
		t.Fatalf("create audit service: %v", err)
	}
	trace := DecisionTrace{
		RecommendationUID: recommendationID,
		ScenarioUID:       &scenarioID,
		ResourceUID:       resourceID,
		PolicyUID:         &policyID,
		PolicyVersion:     &policyVersion,
		AlgorithmVersion:  "rule_v1",
		Action:            "recommendation_generated",
		Status:            "proposed",
		RiskScore:         &riskScore,
		RiskLevel:         &riskLevel,
		ConfidenceScore:   &confidence,
		EstimatedSaving:   &saving,
		EvidenceCount:     2,
		EvidenceReferences: []string{
			"cpu_p95", "estimated_savings",
		},
		Decision:  "resize_down",
		Rationale: "Observed capacity supports a lower-cost configuration.",
		InputReferences: map[string]any{
			"window_days": 30,
		},
		OutputReferences: map[string]any{
			"candidate": "sku-small",
		},
		CreatedAt: createdAt,
	}

	if err := service.RecordDecision(ctx, trace); err != nil {
		t.Fatalf("record recommendation decision trace: %v", err)
	}
	if len(repository.events) != 1 {
		t.Fatalf("persisted %d audit events, want 1", len(repository.events))
	}
	event := repository.events[0]
	if event.EventID != 1 || event.Resource.Type != "recommendation" ||
		event.Resource.ID == nil || *event.Resource.ID != recommendationID {
		t.Fatalf("decision audit event identity mismatch: %#v", event)
	}
	if event.Actor.ActorType != ActorTypeUser || event.Actor.UserID == nil ||
		*event.Actor.UserID != userID || event.OrganizationID == nil ||
		*event.OrganizationID != organizationID {
		t.Fatalf("decision actor was not enriched from IAM: %#v", event)
	}
	if event.Trace.RequestID != requestID || event.Trace.CorrelationID != correlationID ||
		event.Trace.OperationID == nil || *event.Trace.OperationID != operationID {
		t.Fatalf("decision trace context was not persisted: %#v", event.Trace)
	}
	if event.Metadata["decision_trace_id"] == nil {
		t.Fatal("decision trace ID was not included in persisted metadata")
	}

	result, err := service.GetRecommendationTraceScoped(ctx, recommendationID, AuditFilter{
		OrganizationID: &organizationID,
		Limit:          recommendationTraceEventLimit,
	})
	if err != nil {
		t.Fatalf("get recommendation trace: %v", err)
	}
	if repository.lastFilter.OrganizationID == nil ||
		*repository.lastFilter.OrganizationID != organizationID {
		t.Fatalf("recommendation trace scope was not passed to repository: %#v", repository.lastFilter)
	}
	if result.RecommendationUID != recommendationID || len(result.Events) != 1 ||
		len(result.DecisionTraces) != 1 {
		t.Fatalf("unexpected recommendation trace: %#v", result)
	}
	got := result.DecisionTraces[0]
	if got.TraceID == uuid.Nil || got.RecommendationUID != recommendationID ||
		got.ResourceUID != resourceID || got.ScenarioUID == nil ||
		*got.ScenarioUID != scenarioID || got.PolicyUID == nil || *got.PolicyUID != policyID {
		t.Fatalf("decision trace identifiers did not round-trip: %#v", got)
	}
	if got.PolicyVersion == nil || *got.PolicyVersion != policyVersion ||
		got.AlgorithmVersion != "rule_v1" || got.Action != "recommendation_generated" ||
		got.Status != "proposed" || got.Decision != "resize_down" ||
		got.EvidenceCount != 2 || got.Rationale != trace.Rationale {
		t.Fatalf("decision trace fields did not round-trip: %#v", got)
	}
	if got.RiskScore == nil || *got.RiskScore != riskScore ||
		got.RiskLevel == nil || *got.RiskLevel != riskLevel ||
		got.ConfidenceScore == nil || *got.ConfidenceScore != confidence ||
		got.EstimatedSaving == nil || *got.EstimatedSaving != saving {
		t.Fatalf("decision measurements did not round-trip: %#v", got)
	}
	if len(got.EvidenceReferences) != 2 ||
		got.InputReferences["window_days"] == nil ||
		got.OutputReferences["candidate"] != "sku-small" ||
		got.Actor.UserID == nil || *got.Actor.UserID != userID ||
		!got.CreatedAt.Equal(createdAt) {
		t.Fatalf("decision trace references or provenance did not round-trip: %#v", got)
	}
}

func TestServiceRecordDecisionRejectsMissingIdentifiers(t *testing.T) {
	service, err := NewService(&decisionTraceRepositoryStub{}, &auditLoggerStub{})
	if err != nil {
		t.Fatalf("create audit service: %v", err)
	}
	err = service.RecordDecision(context.Background(), DecisionTrace{Decision: "resize_down"})
	if err == nil {
		t.Fatal("expected invalid decision trace to fail")
	}
}
