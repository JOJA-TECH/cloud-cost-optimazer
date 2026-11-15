package recommendation

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// RecommendationEngine coordinates evidence, evaluation, and persisted
// recommendations without exposing persistence or infrastructure details.
type RecommendationEngine interface {
	GenerateRecommendation(ctx context.Context, input GenerateRecommendationInput) (Recommendation, error)
	EvaluateOpportunity(ctx context.Context, input OpportunityInput) (OpportunityEvaluation, error)
	EstimateSaving(ctx context.Context, input SavingInput) (EstimatedSaving, error)
	EvaluateRisk(ctx context.Context, input RiskInput) (RecommendationRisk, error)
	ExplainRecommendation(ctx context.Context, recommendationUID uuid.UUID) (RecommendationRationale, error)
	GetRecommendation(ctx context.Context, recommendationUID uuid.UUID) (Recommendation, error)
	ListRecommendations(ctx context.Context, filter RecommendationFilter) ([]Recommendation, error)
}

// AnalyticsReader exposes analytical evidence needed to make a decision.
// Forecasts are intentionally kept behind MLPredictionReader.
type AnalyticsReader interface {
	FeatureSnapshot(ctx context.Context, resourceUID uuid.UUID, periodType string, from, to time.Time, limit int) ([]FeatureSnapshot, error)
	PercentileStatistic(ctx context.Context, resourceUID uuid.UUID, metricCode, periodType string, from, to time.Time) ([]PercentileStatistic, error)
	Trend(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) ([]Trend, error)
	WorkloadPattern(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (WorkloadPattern, error)
	CapacityAnalysis(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (CapacityAnalysis, error)
	CostAnalysis(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (CostAnalysis, error)
	DESComponents(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (DESComponents, error)
}

// MLPredictionReader reads forecasts without depending on the ML
// implementation or its storage models.
type MLPredictionReader interface {
	Forecasts(ctx context.Context, resourceUID uuid.UUID, targetMetric string, from, to time.Time) ([]Forecast, error)
}

// CostReader reads observed costs for a resource over a bounded period.
type CostReader interface {
	Costs(ctx context.Context, resourceUID uuid.UUID, from, to time.Time, limit int) ([]CostRecord, error)
}

// ConfigurationReader reads the latest observed configuration for a resource.
type ConfigurationReader interface {
	LatestConfigSnapshot(ctx context.Context, resourceUID uuid.UUID) (*ConfigurationSnapshot, error)
}

// PriceReader provides list prices used to simulate candidate configurations.
type PriceReader interface {
	ListPrices(ctx context.Context, query PriceQuery) ([]PriceRecord, error)
}

// PolicyEvaluator determines whether a candidate satisfies business and
// operational constraints.
type PolicyEvaluator interface {
	Validate(ctx context.Context, policy OptimizationPolicy, input PolicyEvaluationInput) (PolicyEvaluation, error)
}

// OptimizationPolicyRepository stores reusable, versioned policy records.
type OptimizationPolicyRepository interface {
	CreatePolicy(ctx context.Context, policy OptimizationPolicy) (OptimizationPolicy, error)
	GetPolicy(ctx context.Context, policyUID uuid.UUID) (OptimizationPolicy, error)
	GetLatestEnabledPolicy(ctx context.Context, name string) (OptimizationPolicy, error)
	ListPolicies(ctx context.Context, name string, enabledOnly bool) ([]OptimizationPolicy, error)
}

// RecommendationRepository persists recommendation-domain entities. Storage
// identifiers and database models remain an implementation detail of adapters.
type RecommendationRepository interface {
	CreateScenario(ctx context.Context, scenario Scenario) (Scenario, error)
	CreateCandidate(ctx context.Context, candidate CandidateConfiguration) (CandidateConfiguration, error)
	CreateSimulation(ctx context.Context, simulation ScenarioSimulation) (ScenarioSimulation, error)
	CreateRecommendation(ctx context.Context, recommendation Recommendation) (Recommendation, error)
	CreateEvidence(ctx context.Context, recommendationUID uuid.UUID, evidence []Evidence) error
	CreateRiskAssessment(ctx context.Context, recommendationUID uuid.UUID, assessment RiskAssessment) (RiskAssessment, error)
	GetRecommendation(ctx context.Context, recommendationUID uuid.UUID) (Recommendation, error)
	ListRecommendations(ctx context.Context, filter RecommendationFilter) ([]Recommendation, error)
	GetLatestRecommendation(ctx context.Context, resourceUID uuid.UUID) (Recommendation, error)
}

// AuditSink records the decision independently from recommendation storage.
type AuditSink interface {
	RecordDecision(ctx context.Context, event DecisionAuditEvent) error
}
