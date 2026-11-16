package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
)

type auditQueryFake struct {
	events []audit.AuditEvent
}

func (f auditQueryFake) GetEvent(context.Context, int64) (audit.AuditEvent, error) {
	return audit.AuditEvent{}, audit.ErrAuditEventNotFound
}

func (f auditQueryFake) ListEvents(_ context.Context, filter audit.AuditFilter) ([]audit.AuditEvent, error) {
	var matches []audit.AuditEvent
	for _, event := range f.events {
		if filter.OrganizationID != nil &&
			(event.OrganizationID == nil || *event.OrganizationID != *filter.OrganizationID) {
			continue
		}
		if filter.UserID != nil && (event.Actor.UserID == nil || *event.Actor.UserID != *filter.UserID) {
			continue
		}
		if filter.EntityType != "" && event.Resource.Type != filter.EntityType {
			continue
		}
		if filter.EntityID != nil &&
			(event.Resource.ID == nil || *event.Resource.ID != *filter.EntityID) {
			continue
		}
		if filter.CorrelationID != nil && event.Trace.CorrelationID != *filter.CorrelationID {
			continue
		}
		if filter.OperationID != nil &&
			(event.Trace.OperationID == nil || *event.Trace.OperationID != *filter.OperationID) {
			continue
		}
		matches = append(matches, event)
	}
	if len(matches) > filter.Limit {
		matches = matches[:filter.Limit]
	}
	return matches, nil
}

func (f auditQueryFake) GetResourceHistory(ctx context.Context, resource audit.ResourceReference, filter audit.AuditFilter) ([]audit.AuditEvent, error) {
	filter.EntityType = resource.Type
	filter.EntityID = resource.ID
	return f.ListEvents(ctx, filter)
}

func (f auditQueryFake) GetCorrelationHistory(context.Context, uuid.UUID) ([]audit.AuditEvent, error) {
	return nil, errors.New("not used")
}

func TestAuditTraceReaderCombinesScopedResourceCorrelationAndOperationEvents(t *testing.T) {
	organizationID := uuid.New()
	recommendationUID := uuid.New()
	correlationID, operationID := uuid.New(), uuid.New()
	timeBase := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	fake := auditQueryFake{events: []audit.AuditEvent{
		{
			EventID: 5, Action: "recommendation_generated", OccurredAt: timeBase.Add(2 * time.Second),
			OrganizationID: &organizationID,
			Resource:       audit.ResourceReference{Type: "recommendation", ID: &recommendationUID},
			Trace: audit.CorrelationContext{
				CorrelationID: correlationID, OperationID: &operationID,
			},
		},
		{
			EventID: 3, Action: "recommendation_evaluation_started", OccurredAt: timeBase,
			OrganizationID: &organizationID,
			Resource:       audit.ResourceReference{Type: "resource", ID: uuidPtrForTest(uuid.New())},
			Trace: audit.CorrelationContext{
				CorrelationID: correlationID,
			},
		},
		{
			EventID: 4, Action: "policy_evaluated", OccurredAt: timeBase.Add(time.Second),
			OrganizationID: &organizationID,
			Trace:          audit.CorrelationContext{OperationID: &operationID},
		},
		{
			EventID: 9, Action: "other_organization_event", OccurredAt: timeBase.Add(-time.Second),
			OrganizationID: uuidPtrForTest(uuid.New()),
			Trace:          audit.CorrelationContext{CorrelationID: correlationID},
		},
	}}
	reader := NewAuditTraceReader(fake)
	events, err := reader.ListRecommendationTraceEvents(context.Background(), recommendationUID,
		recommendation.RecommendationTraceFilter{OrganizationID: &organizationID})
	if err != nil {
		t.Fatalf("ListRecommendationTraceEvents() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	if events[0].EventID != 3 || events[1].EventID != 4 || events[2].EventID != 5 {
		t.Fatalf("events not sorted chronologically or incorrectly scoped: %+v", events)
	}
}

func uuidPtrForTest(value uuid.UUID) *uuid.UUID {
	return &value
}
