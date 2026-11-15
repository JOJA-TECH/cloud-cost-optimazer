package recommendation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type Service struct {
	policyEvaluator PolicyEvaluator
	repository      RecommendationRepository
}

func NewService(policyEvaluator PolicyEvaluator, repository RecommendationRepository) (*Service, error) {
	if policyEvaluator == nil {
		return nil, ErrPolicyEvaluator
	}
	if repository == nil {
		return nil, ErrRepository
	}
	return &Service{
		policyEvaluator: policyEvaluator,
		repository:      repository,
	}, nil
}

// PublishRecommendation evaluates the selected policy before writing anything.
// A rejected candidate or an evaluator error never reaches the repository.
func (s *Service) PublishRecommendation(ctx context.Context, input PublishRecommendationInput) (Recommendation, PolicyEvaluation, error) {
	if input.Policy.UID == uuid.Nil || input.Policy.Name == "" || input.Policy.Version < 1 {
		return Recommendation{}, PolicyEvaluation{}, ErrPolicyRequired
	}
	if !input.Policy.Enabled {
		return Recommendation{}, PolicyEvaluation{}, ErrPolicyDisabled
	}

	evaluation, err := s.policyEvaluator.Validate(ctx, input.Policy, PolicyEvaluationInput{
		Candidate:            input.Candidate,
		CurrentConfiguration: input.CurrentConfiguration,
		Simulation:           input.Simulation,
		Evidence:             input.Evidence,
		Confidence:           input.Recommendation.Confidence,
		Action:               input.Recommendation.Action,
	})
	if err != nil {
		return Recommendation{}, PolicyEvaluation{}, fmt.Errorf("evaluate optimization policy %q v%d: %w", input.Policy.Name, input.Policy.Version, err)
	}
	evaluation.PolicyUID = input.Policy.UID
	evaluation.PolicyVersion = input.Policy.Version
	if !evaluation.Allowed {
		return Recommendation{}, evaluation, ErrPolicyRejected
	}

	input.Recommendation.PolicyUID = input.Policy.UID
	input.Recommendation.PolicyVersion = input.Policy.Version
	persisted, err := s.repository.CreateRecommendation(ctx, input.Recommendation)
	if err != nil {
		return Recommendation{}, evaluation, fmt.Errorf("persist policy-approved recommendation: %w", err)
	}
	return persisted, evaluation, nil
}
