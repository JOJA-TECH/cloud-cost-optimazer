package recommendation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type statusRepositoryStub struct {
	RecommendationRepository
	value       Recommendation
	updateCalls int
	updateErr   error
}

func (r *statusRepositoryStub) GetRecommendation(context.Context, uuid.UUID) (Recommendation, error) {
	return r.value, nil
}

func (r *statusRepositoryStub) UpdateRecommendationStatus(
	_ context.Context,
	uid uuid.UUID,
	expected RecommendationStatus,
	status RecommendationStatus,
	actorID string,
	changedAt time.Time,
) (Recommendation, error) {
	r.updateCalls++
	if r.updateErr != nil {
		return Recommendation{}, r.updateErr
	}
	if uid != r.value.UID || expected != r.value.Status {
		return Recommendation{}, ErrInvalidStatusTransition
	}
	r.value.Status = status
	switch status {
	case RecommendationStatusAccepted, RecommendationStatusRejected, RecommendationStatusDismissed:
		r.value.DecidedAt = &changedAt
		r.value.DecidedBy = &actorID
	case RecommendationStatusApplied:
		r.value.AppliedAt = &changedAt
	}
	return r.value, nil
}

type statusAuditSink struct {
	events        []RecommendationAuditEvent
	recordErr     error
	decisionCalls int
}

func (s *statusAuditSink) RecordDecision(context.Context, DecisionAuditEvent) error {
	s.decisionCalls++
	return nil
}

func (s *statusAuditSink) RecordEvent(_ context.Context, event RecommendationAuditEvent) error {
	s.events = append(s.events, event)
	return s.recordErr
}

func TestUpdateRecommendationStatusRecordsDecisionLifecycle(t *testing.T) {
	tests := []struct {
		name   string
		from   RecommendationStatus
		to     RecommendationStatus
		action string
	}{
		{name: "accept", from: RecommendationStatusProposed, to: RecommendationStatusAccepted, action: "recommendation_accepted"},
		{name: "reject", from: RecommendationStatusProposed, to: RecommendationStatusRejected, action: "recommendation_rejected"},
		{name: "dismiss", from: RecommendationStatusUnderReview, to: RecommendationStatusDismissed, action: "recommendation_dismissed"},
		{name: "apply", from: RecommendationStatusAccepted, to: RecommendationStatusApplied, action: "recommendation_applied"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			uid, resourceUID := uuid.New(), uuid.New()
			repository := &statusRepositoryStub{value: Recommendation{
				UID: uid, ResourceUID: resourceUID, ScenarioUID: uuid.New(),
				PolicyUID: uuid.New(), Status: test.from,
			}}
			sink := &statusAuditSink{}
			service, err := NewEngine(Dependencies{
				PolicyEvaluator: policyEvaluatorFunc(func(context.Context, OptimizationPolicy, PolicyEvaluationInput) (PolicyEvaluation, error) {
					return PolicyEvaluation{}, nil
				}),
				Repository: repository,
				Audit:      sink,
			})
			if err != nil {
				t.Fatalf("create recommendation service: %v", err)
			}

			actorID := uuid.NewString()
			updated, err := service.UpdateRecommendationStatus(context.Background(), uid, test.to, actorID)
			if err != nil {
				t.Fatalf("update recommendation status: %v", err)
			}
			if updated.Status != test.to || repository.updateCalls != 1 {
				t.Fatalf("status update result=%+v calls=%d", updated, repository.updateCalls)
			}
			if len(sink.events) != 1 {
				t.Fatalf("recorded %d audit events, want 1", len(sink.events))
			}
			event := sink.events[0]
			if event.Action != test.action || event.RecommendationUID != uid ||
				event.ResourceUID != resourceUID ||
				event.Metadata["previous_status"] != string(test.from) ||
				event.Metadata["status"] != string(test.to) {
				t.Fatalf("incorrect status audit event: %#v", event)
			}
		})
	}
}

func TestUpdateRecommendationStatusRejectsInvalidTransitions(t *testing.T) {
	uid := uuid.New()
	repository := &statusRepositoryStub{value: Recommendation{
		UID: uid, Status: RecommendationStatusRejected,
	}}
	sink := &statusAuditSink{}
	service, err := NewEngine(Dependencies{
		PolicyEvaluator: policyEvaluatorFunc(func(context.Context, OptimizationPolicy, PolicyEvaluationInput) (PolicyEvaluation, error) {
			return PolicyEvaluation{}, nil
		}),
		Repository: repository,
		Audit:      sink,
	})
	if err != nil {
		t.Fatalf("create recommendation service: %v", err)
	}

	_, err = service.UpdateRecommendationStatus(context.Background(), uid, RecommendationStatusAccepted, uuid.NewString())
	if !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("got %v, want invalid status transition", err)
	}
	if repository.updateCalls != 0 || len(sink.events) != 0 {
		t.Fatalf("invalid transition wrote state or audit event: updates=%d events=%d",
			repository.updateCalls, len(sink.events))
	}
}

func TestUpdateRecommendationStatusMarksAuditPendingOnFailure(t *testing.T) {
	uid := uuid.New()
	repository := &statusRepositoryStub{value: Recommendation{
		UID: uid, ResourceUID: uuid.New(), ScenarioUID: uuid.New(),
		PolicyUID: uuid.New(), Status: RecommendationStatusProposed,
	}}
	sink := &statusAuditSink{recordErr: errors.New("audit storage unavailable")}
	service, err := NewEngine(Dependencies{
		PolicyEvaluator: policyEvaluatorFunc(func(context.Context, OptimizationPolicy, PolicyEvaluationInput) (PolicyEvaluation, error) {
			return PolicyEvaluation{}, nil
		}),
		Repository: repository,
		Audit:      sink,
	})
	if err != nil {
		t.Fatalf("create recommendation service: %v", err)
	}

	updated, err := service.UpdateRecommendationStatus(
		context.Background(), uid, RecommendationStatusAccepted, uuid.NewString(),
	)
	if err != nil {
		t.Fatalf("status update should remain successful when audit is pending: %v", err)
	}
	if updated.Status != RecommendationStatusAccepted || updated.AuditStatus != AuditStatusPending {
		t.Fatalf("updated recommendation was not marked audit_pending: %+v", updated)
	}
}
