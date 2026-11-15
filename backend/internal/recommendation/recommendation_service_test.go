package recommendation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type policyEvaluatorFunc func(context.Context, OptimizationPolicy, PolicyEvaluationInput) (PolicyEvaluation, error)

func (f policyEvaluatorFunc) Validate(ctx context.Context, policy OptimizationPolicy, input PolicyEvaluationInput) (PolicyEvaluation, error) {
	return f(ctx, policy, input)
}

type recommendationRepositoryStub struct {
	RecommendationRepository
	created Recommendation
	calls   int
	err     error
}

func (r *recommendationRepositoryStub) CreateRecommendation(_ context.Context, recommendation Recommendation) (Recommendation, error) {
	r.calls++
	r.created = recommendation
	if r.err != nil {
		return Recommendation{}, r.err
	}
	return recommendation, nil
}

func TestPublishRecommendationEvaluatesPolicyBeforePersisting(t *testing.T) {
	policy := OptimizationPolicy{
		UID:     uuid.New(),
		Name:    "safe-cost-optimization",
		Version: 1,
		Enabled: true,
	}
	rejected := PolicyEvaluation{
		Allowed:             false,
		ViolatedConstraints: []string{"max_sla_violations"},
	}
	repository := &recommendationRepositoryStub{}
	service, err := NewService(policyEvaluatorFunc(func(_ context.Context, gotPolicy OptimizationPolicy, input PolicyEvaluationInput) (PolicyEvaluation, error) {
		if gotPolicy.UID != policy.UID || gotPolicy.Version != policy.Version {
			t.Fatalf("evaluator received policy %v v%d, want %v v%d", gotPolicy.UID, gotPolicy.Version, policy.UID, policy.Version)
		}
		if input.Confidence.Score != 0.8 {
			t.Fatalf("evaluator received confidence %v, want 0.8", input.Confidence.Score)
		}
		return rejected, nil
	}), repository)
	if err != nil {
		t.Fatal(err)
	}

	_, evaluation, err := service.PublishRecommendation(context.Background(), PublishRecommendationInput{
		Policy: policy,
		Recommendation: Recommendation{
			Confidence: RecommendationConfidence{Score: 0.8},
		},
	})
	if !errors.Is(err, ErrPolicyRejected) {
		t.Fatalf("PublishRecommendation() error = %v, want ErrPolicyRejected", err)
	}
	if evaluation.Allowed || evaluation.PolicyUID != policy.UID || evaluation.PolicyVersion != policy.Version {
		t.Fatalf("unexpected policy evaluation: %+v", evaluation)
	}
	if repository.calls != 0 {
		t.Fatalf("CreateRecommendation called %d times for rejected policy", repository.calls)
	}
}

func TestPublishRecommendationPersistsOnlyAfterPolicyApproval(t *testing.T) {
	policy := OptimizationPolicy{
		UID:     uuid.New(),
		Name:    "safe-cost-optimization",
		Version: 2,
		Enabled: true,
	}
	repository := &recommendationRepositoryStub{}
	service, err := NewService(policyEvaluatorFunc(func(context.Context, OptimizationPolicy, PolicyEvaluationInput) (PolicyEvaluation, error) {
		return PolicyEvaluation{Allowed: true}, nil
	}), repository)
	if err != nil {
		t.Fatal(err)
	}

	published, evaluation, err := service.PublishRecommendation(context.Background(), PublishRecommendationInput{
		Policy:         policy,
		Recommendation: Recommendation{UID: uuid.New()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !evaluation.Allowed {
		t.Fatal("policy evaluation should allow the recommendation")
	}
	if repository.calls != 1 {
		t.Fatalf("CreateRecommendation called %d times, want 1", repository.calls)
	}
	if repository.created.PolicyUID != policy.UID || repository.created.PolicyVersion != policy.Version {
		t.Fatalf("persisted recommendation lacks policy provenance: %+v", repository.created)
	}
	if published.PolicyUID != policy.UID || published.PolicyVersion != policy.Version {
		t.Fatalf("published recommendation lacks policy provenance: %+v", published)
	}
}

func TestPublishRecommendationDoesNotPersistWhenPolicyEvaluationFails(t *testing.T) {
	evaluationErr := errors.New("evaluator unavailable")
	repository := &recommendationRepositoryStub{}
	service, err := NewService(policyEvaluatorFunc(func(context.Context, OptimizationPolicy, PolicyEvaluationInput) (PolicyEvaluation, error) {
		return PolicyEvaluation{}, evaluationErr
	}), repository)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = service.PublishRecommendation(context.Background(), PublishRecommendationInput{
		Policy: OptimizationPolicy{
			UID:     uuid.New(),
			Name:    "safe-cost-optimization",
			Version: 1,
			Enabled: true,
		},
	})
	if !errors.Is(err, evaluationErr) {
		t.Fatalf("PublishRecommendation() error = %v, want wrapped evaluator error", err)
	}
	if repository.calls != 0 {
		t.Fatalf("CreateRecommendation called %d times after evaluator error", repository.calls)
	}
}

func TestPublishRecommendationRejectsDisabledPolicy(t *testing.T) {
	repository := &recommendationRepositoryStub{}
	service, err := NewService(policyEvaluatorFunc(func(context.Context, OptimizationPolicy, PolicyEvaluationInput) (PolicyEvaluation, error) {
		t.Fatal("disabled policy must not be evaluated")
		return PolicyEvaluation{}, nil
	}), repository)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = service.PublishRecommendation(context.Background(), PublishRecommendationInput{
		Policy: OptimizationPolicy{
			UID:     uuid.New(),
			Name:    "disabled-policy",
			Version: 1,
			Enabled: false,
		},
	})
	if !errors.Is(err, ErrPolicyDisabled) {
		t.Fatalf("PublishRecommendation() error = %v, want ErrPolicyDisabled", err)
	}
	if repository.calls != 0 {
		t.Fatalf("CreateRecommendation called %d times for disabled policy", repository.calls)
	}
}
