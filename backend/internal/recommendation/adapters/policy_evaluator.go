package adapters

import (
	"context"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
)

type PolicyEvaluator struct{}

func NewPolicyEvaluator() *PolicyEvaluator { return &PolicyEvaluator{} }

func (*PolicyEvaluator) Validate(_ context.Context, policy recommendation.OptimizationPolicy, input recommendation.PolicyEvaluationInput) (recommendation.PolicyEvaluation, error) {
	return recommendation.EvaluatePolicy(policy, input), nil
}
