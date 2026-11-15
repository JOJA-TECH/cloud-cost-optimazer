package recommendation

import (
	"context"
	"errors"
	"testing"
	"time"

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

type engineTestPolicyRepository struct {
	policy OptimizationPolicy
}

func (r engineTestPolicyRepository) CreatePolicy(context.Context, OptimizationPolicy) (OptimizationPolicy, error) {
	return OptimizationPolicy{}, errors.New("unexpected CreatePolicy call")
}
func (r engineTestPolicyRepository) GetPolicy(context.Context, uuid.UUID) (OptimizationPolicy, error) {
	return r.policy, nil
}
func (r engineTestPolicyRepository) GetLatestEnabledPolicy(context.Context, string) (OptimizationPolicy, error) {
	return r.policy, nil
}
func (r engineTestPolicyRepository) ListPolicies(context.Context, string, bool) ([]OptimizationPolicy, error) {
	return []OptimizationPolicy{r.policy}, nil
}

type engineTestRepository struct {
	RecommendationRepository
	latest       Recommendation
	persist      DecisionPersistence
	persistCalls int
}

func (r *engineTestRepository) GetLatestRecommendation(context.Context, uuid.UUID) (Recommendation, error) {
	if r.latest.UID == uuid.Nil {
		return Recommendation{}, ErrRecommendationNotFound
	}
	return r.latest, nil
}
func (r *engineTestRepository) PersistDecision(_ context.Context, decision DecisionPersistence) (Recommendation, error) {
	r.persistCalls++
	r.persist = decision
	decision.Recommendation.ScenarioUID = decision.Scenario.UID
	decision.Recommendation.ScenarioID = 101
	if decision.SelectedCandidateIndex != nil {
		id := int64(*decision.SelectedCandidateIndex + 1)
		decision.Recommendation.ChosenCandidateID = &id
	}
	return decision.Recommendation, nil
}

type engineTestAnalytics struct {
	AnalyticsReader
	snapshots   []FeatureSnapshot
	percentiles []PercentileStatistic
	trends      []Trend
	pattern     WorkloadPattern
	capacity    CapacityAnalysis
	cost        CostAnalysis
	des         DESComponents
}

func (a engineTestAnalytics) FeatureSnapshot(context.Context, uuid.UUID, string, time.Time, time.Time, int) ([]FeatureSnapshot, error) {
	return a.snapshots, nil
}
func (a engineTestAnalytics) PercentileStatistic(_ context.Context, _ uuid.UUID, metric, _ string, _, _ time.Time) ([]PercentileStatistic, error) {
	rows := make([]PercentileStatistic, 0)
	for _, row := range a.percentiles {
		if row.MetricCode == metric {
			rows = append(rows, row)
		}
	}
	return rows, nil
}
func (a engineTestAnalytics) Trend(context.Context, uuid.UUID, time.Time, time.Time) ([]Trend, error) {
	return a.trends, nil
}
func (a engineTestAnalytics) WorkloadPattern(context.Context, uuid.UUID, time.Time, time.Time) (WorkloadPattern, error) {
	return a.pattern, nil
}
func (a engineTestAnalytics) CapacityAnalysis(context.Context, uuid.UUID, string, time.Time) (CapacityAnalysis, error) {
	return a.capacity, nil
}
func (a engineTestAnalytics) CostAnalysis(context.Context, uuid.UUID, time.Time, time.Time) (CostAnalysis, error) {
	return a.cost, nil
}
func (a engineTestAnalytics) DESComponents(context.Context, uuid.UUID, string, time.Time) (DESComponents, error) {
	return a.des, nil
}

type engineTestCosts []CostRecord

func (c engineTestCosts) Costs(context.Context, uuid.UUID, time.Time, time.Time, int) ([]CostRecord, error) {
	return c, nil
}

type engineTestConfiguration struct {
	snapshot *ConfigurationSnapshot
}

func (c engineTestConfiguration) LatestConfigSnapshot(context.Context, uuid.UUID) (*ConfigurationSnapshot, error) {
	return c.snapshot, nil
}

type engineTestPricingContext struct{}

func (engineTestPricingContext) PricingContext(context.Context, uuid.UUID) (PricingContext, error) {
	return PricingContext{ProviderCode: "azure", RegionCode: "eastus"}, nil
}

type engineTestSKUs []SKU

func (s engineTestSKUs) ListSKUs(context.Context, ConfigurationSnapshot, bool) ([]SKU, error) {
	return s, nil
}

type engineTestPrices map[string]float64

func (p engineTestPrices) ListPrices(_ context.Context, query PriceQuery) ([]PriceRecord, error) {
	price, ok := p[query.SKUName]
	if !ok {
		return nil, nil
	}
	return []PriceRecord{{SKUName: &query.SKUName, UnitPrice: price, UnitOfMeasure: "1 Hour", Currency: query.Currency}}, nil
}

type engineTestForecasts []Forecast

func (f engineTestForecasts) Forecasts(context.Context, uuid.UUID, string, time.Time, time.Time) ([]Forecast, error) {
	return f, nil
}

func newTestEngine(t *testing.T, config *ConfigurationSnapshot) (*Service, *engineTestRepository, OptimizationPolicy, uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	resourceUID := uuid.New()
	policy := OptimizationPolicy{
		UID: uuid.New(), Name: "safe-cost-optimization", Version: 1, Enabled: true,
		ObjectiveWeights: map[string]float64{"cost": 0.6, "performance": 0.25, "risk": 0.15},
		Constraints: map[string]any{
			"min_confidence": 0.5, "max_risk_score": 0.9,
			"allow_scale_down": true, "allow_compute_model_change": false,
		},
	}
	cpu50, cpu95, cpu99 := 25.0, 30.0, 50.0
	io95, sessions95 := 40.0, 20.0
	family := "Gen5"
	analyticsReader := engineTestAnalytics{
		snapshots: []FeatureSnapshot{{
			ResourceUID: resourceUID, PeriodStartUTC: now.Add(-time.Hour), PeriodEndUTC: now,
			CPUAvg: 25, CPUP50: &cpu50, CPUP95: &cpu95, CPUP99: &cpu99, CapacityVCores: 8,
			ComputedAt: now,
		}},
		percentiles: []PercentileStatistic{
			{MetricCode: "cpu_percent", Percentile: 50, Value: &cpu50, PeriodStartUTC: now.Add(-time.Hour)},
			{MetricCode: "cpu_percent", Percentile: 95, Value: &cpu95, PeriodStartUTC: now.Add(-time.Hour)},
			{MetricCode: "cpu_percent", Percentile: 99, Value: &cpu99, PeriodStartUTC: now.Add(-time.Hour)},
			{MetricCode: "data_io_percent", Percentile: 95, Value: &io95, PeriodStartUTC: now.Add(-time.Hour)},
			{MetricCode: "sessions_percent", Percentile: 95, Value: &sessions95, PeriodStartUTC: now.Add(-time.Hour)},
		},
		trends:   []Trend{{MetricCode: "cpu_percent", Slope: 0.05, WindowStart: now.Add(-24 * time.Hour), WindowEnd: now}},
		pattern:  WorkloadPattern{PeakHours: []int{9, 10}, ComputedAt: now},
		capacity: CapacityAnalysis{HeadroomPct: 50, PeakUtilization: 50, ComputedAt: now},
		cost:     CostAnalysis{ActualMonthlyCost: 420, ComputedAt: now},
		des:      DESComponents{CapacityE: 0.8, CostE: 0.7, PerformanceE: 0.8, ComputedAt: now},
	}
	snapshot := config
	if snapshot == nil {
		skuName, tier, model, capacity := "GP_Gen5_8", "GeneralPurpose", "provisioned", 8.0
		snapshot = &ConfigurationSnapshot{
			ResourceUID: resourceUID, SKUName: &skuName, Tier: &tier, Family: &family,
			ComputeModel: &model, CapacityVCores: &capacity, ObservedAtUTC: now,
		}
	}
	uid := resourceUID
	repo := &engineTestRepository{}
	service, err := NewEngine(Dependencies{
		PolicyEvaluator:  adaptersPolicyEvaluator{},
		PolicyRepository: engineTestPolicyRepository{policy: policy},
		Repository:       repo, Analytics: analyticsReader,
		Forecasts: engineTestForecasts{{
			ForecastUID: uuid.New(), ResourceUID: uid, TargetMetric: "cpu_avg",
			PeriodStartUTC: now.Add(time.Hour), YPred: 30, YLower: floatPtr(20), YUpper: floatPtr(40),
		}},
		Costs: engineTestCosts{{
			ResourceUID: &uid, Currency: "USD", PreTaxCost: 420, ChargeCategory: stringPtr("usage"),
		}},
		Configurations:  engineTestConfiguration{snapshot: snapshot},
		ResourceContext: engineTestPricingContext{},
		Prices:          engineTestPrices{"GP_Gen5_8": 1, "GP_Gen5_6": 0.5},
		SKUs: engineTestSKUs{
			{ID: 8, Name: "GP_Gen5_8", Tier: "GeneralPurpose", Family: &family, ComputeModel: "provisioned", CapacityVCores: 8, InScope: true},
			{ID: 6, Name: "GP_Gen5_6", Tier: "GeneralPurpose", Family: &family, ComputeModel: "provisioned", CapacityVCores: 6, InScope: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service, repo, policy, resourceUID
}

type adaptersPolicyEvaluator struct{}

func (adaptersPolicyEvaluator) Validate(_ context.Context, policy OptimizationPolicy, input PolicyEvaluationInput) (PolicyEvaluation, error) {
	return EvaluatePolicy(policy, input), nil
}

func TestGenerateRecommendationPersistsPolicyApprovedCatalogCandidate(t *testing.T) {
	service, repository, policy, resourceUID := newTestEngine(t, nil)
	now := time.Now().UTC()
	result, err := service.GenerateRecommendation(context.Background(), GenerateRecommendationInput{
		ResourceUID: resourceUID, PolicyUID: policy.UID, From: now.Add(-30 * 24 * time.Hour),
		To: now, HorizonDays: 30, Currency: "USD", CreatedBy: uuid.NewString(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action.Type != ActionResizeDown {
		t.Fatalf("action = %q, want resize_down; recommendation: %+v", result.Action.Type, result)
	}
	if result.ProposedConfiguration.SKUName != "GP_Gen5_6" {
		t.Fatalf("proposed SKU = %q, want GP_Gen5_6", result.ProposedConfiguration.SKUName)
	}
	if len(repository.persist.Candidates) != 2 || len(repository.persist.Simulations) != 2 {
		t.Fatalf("persisted %d candidates and %d simulations, want 2 each", len(repository.persist.Candidates), len(repository.persist.Simulations))
	}
	if repository.persist.SelectedCandidateIndex == nil || *repository.persist.SelectedCandidateIndex != 1 {
		t.Fatalf("selected candidate index = %v, want 1", repository.persist.SelectedCandidateIndex)
	}
	if result.EstimatedSaving.MonthlyAmount <= 0 || result.EstimatedSaving.MonthlyPercentage <= 0 {
		t.Fatalf("unexpected saving estimate: %+v", result.EstimatedSaving)
	}
	if result.PolicyUID != policy.UID || result.PolicyVersion != policy.Version {
		t.Fatalf("policy provenance missing: %+v", result)
	}
}

func TestGenerateRecommendationReusesOnlyMatchingRequest(t *testing.T) {
	service, repository, policy, resourceUID := newTestEngine(t, nil)
	to := time.Now().UTC()
	from := to.Add(-30 * 24 * time.Hour)
	repository.latest = Recommendation{
		UID: uuid.New(), PolicyUID: policy.UID, Version: 2,
		ResourceUID: resourceUID, WindowStart: from, WindowEnd: to,
		HorizonDays: 30, EstimatedSaving: EstimatedSaving{Currency: "USD"},
		GenerationState: "no_action",
	}
	input := GenerateRecommendationInput{
		ResourceUID: resourceUID, PolicyUID: policy.UID,
		From: from, To: to, HorizonDays: 30, Currency: "USD",
	}

	reused, err := service.GenerateRecommendation(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if reused.UID != repository.latest.UID || repository.persistCalls != 0 {
		t.Fatalf("matching request was not reused: recommendation=%+v persist_calls=%d", reused, repository.persistCalls)
	}

	input.From = to.Add(-7 * 24 * time.Hour)
	regenerated, err := service.GenerateRecommendation(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if repository.persistCalls != 1 {
		t.Fatalf("different request persisted %d times, want 1", repository.persistCalls)
	}
	if regenerated.Version != repository.latest.Version+1 ||
		regenerated.SupersedesUID == nil || *regenerated.SupersedesUID != repository.latest.UID {
		t.Fatalf("different request did not create a superseding version: %+v", regenerated)
	}
}

func TestGenerateRecommendationMarksRequiredMissingForecastAsInsufficient(t *testing.T) {
	service, repository, policy, resourceUID := newTestEngine(t, nil)
	policy.Constraints["require_forecast"] = true
	service.policyRepository = engineTestPolicyRepository{policy: policy}
	service.forecasts = engineTestForecasts(nil)

	now := time.Now().UTC()
	result, err := service.GenerateRecommendation(context.Background(), GenerateRecommendationInput{
		ResourceUID: resourceUID, PolicyUID: policy.UID,
		From: now.Add(-30 * 24 * time.Hour), To: now,
		HorizonDays: 30, Currency: "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action.Type != ActionNoAction || result.GenerationState != "insufficient_information" {
		t.Fatalf("missing required forecast should produce explicit insufficient no-action, got action=%q state=%q",
			result.Action.Type, result.GenerationState)
	}
	if repository.persistCalls != 1 {
		t.Fatalf("insufficient decision persisted %d times, want 1 auditable decision", repository.persistCalls)
	}
}

func TestGenerateRecommendationFailsWhenConfigurationUnavailable(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	service.configurations = engineTestConfiguration{}
	_, err := service.GenerateRecommendation(context.Background(), GenerateRecommendationInput{
		ResourceUID: resourceUID, PolicyUID: policy.UID,
		From: time.Now().Add(-24 * time.Hour), To: time.Now(),
		HorizonDays: 30, Currency: "USD",
	})
	if !errors.Is(err, ErrConfigurationUnavailable) {
		t.Fatalf("GenerateRecommendation() error = %v, want ErrConfigurationUnavailable", err)
	}
}
