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

func testGenerationInput(resourceUID, policyUID uuid.UUID, from, to time.Time) GenerateRecommendationInput {
	return GenerateRecommendationInput{
		ResourceUID: resourceUID, PolicyUID: policyUID,
		From: from, To: to, HorizonDays: 30, Currency: "USD",
	}
}

func TestGenerateRecommendationRejectsInvalidWindows(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	now := time.Now().UTC()
	tests := []struct {
		name string
		from time.Time
		to   time.Time
	}{
		{"zero start", time.Time{}, now},
		{"zero end", now.Add(-time.Hour), time.Time{}},
		{"equal bounds", now, now},
		{"reversed bounds", now, now.Add(-time.Hour)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.GenerateRecommendation(context.Background(),
				testGenerationInput(resourceUID, policy.UID, test.from, test.to))
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("GenerateRecommendation() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestGenerateRecommendationFailsForMissingResource(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	service.configurations = engineTestConfiguration{err: ErrResourceNotFound}
	now := time.Now().UTC()
	_, err := service.GenerateRecommendation(context.Background(),
		testGenerationInput(resourceUID, policy.UID, now.Add(-24*time.Hour), now))
	if !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("GenerateRecommendation() error = %v, want ErrResourceNotFound", err)
	}
}

func TestGenerateRecommendationReturnsNoActionWithoutCosts(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	service.costs = engineTestCosts(nil)
	analyticsReader := service.analytics.(engineTestAnalytics)
	analyticsReader.cost.ActualMonthlyCost = 0
	service.analytics = analyticsReader
	now := time.Now().UTC()

	result, err := service.GenerateRecommendation(context.Background(),
		testGenerationInput(resourceUID, policy.UID, now.Add(-30*24*time.Hour), now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Action.Type != ActionNoAction || result.GenerationState != "insufficient_information" {
		t.Fatalf("without costs got action=%q state=%q, want no_action/insufficient_information",
			result.Action.Type, result.GenerationState)
	}
}

func TestGenerateRecommendationAuditsPersistedDecisionFields(t *testing.T) {
	service, repository, policy, resourceUID := newTestEngine(t, nil)
	sink := &engineTestAuditSink{repository: repository}
	service.audit = sink
	now := time.Now().UTC()
	input := testGenerationInput(resourceUID, policy.UID, now.Add(-30*24*time.Hour), now)
	input.RequestID = uuid.NewString()
	input.CorrelationID = uuid.NewString()
	input.OperationID = uuid.NewString()

	result, err := service.GenerateRecommendation(context.Background(), input)
	if err != nil {
		t.Fatalf("generate recommendation: %v", err)
	}
	if repository.persistCalls != 1 || len(sink.events) != 1 {
		t.Fatalf("persist calls=%d, audit events=%d; want one of each", repository.persistCalls, len(sink.events))
	}
	event := sink.events[0]
	if event.RecommendationUID != result.UID || event.PolicyUID != policy.UID ||
		event.ResourceUID != resourceUID || event.RequestID != input.RequestID ||
		event.CorrelationID != input.CorrelationID || event.OperationID != input.OperationID {
		t.Fatalf("audit event identifiers do not match persisted decision: %+v", event)
	}
	if event.RiskScore != repository.persist.Recommendation.Risk.Score ||
		event.RiskLevel != string(repository.persist.Recommendation.Risk.Level) ||
		event.ConfidenceScore != repository.persist.Recommendation.Confidence.Score ||
		event.EstimatedSaving != repository.persist.Recommendation.EstimatedSaving.MonthlyAmount {
		t.Fatalf("audit scores do not match persisted decision: %+v", event)
	}
	if event.EvidenceCount != len(repository.persist.Recommendation.Evidence) {
		t.Fatalf("evidence count=%d, want %d", event.EvidenceCount, len(repository.persist.Recommendation.Evidence))
	}
}

func TestGenerateRecommendationDoesNotAuditFailedPersistence(t *testing.T) {
	service, repository, policy, resourceUID := newTestEngine(t, nil)
	persistErr := errors.New("cco_data unavailable")
	repository.persistErr = persistErr
	sink := &engineTestAuditSink{repository: repository}
	service.audit = sink
	now := time.Now().UTC()

	_, err := service.GenerateRecommendation(context.Background(),
		testGenerationInput(resourceUID, policy.UID, now.Add(-30*24*time.Hour), now))
	if !errors.Is(err, persistErr) {
		t.Fatalf("got %v, want persistence error", err)
	}
	if len(sink.events) != 0 {
		t.Fatalf("audited %d decisions after persistence failed", len(sink.events))
	}
}

func TestGenerateRecommendationReturnsNoActionWithoutAnalyticsEvidence(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	analyticsReader := service.analytics.(engineTestAnalytics)
	analyticsReader.snapshots = nil
	analyticsReader.percentiles = nil
	analyticsReader.trends = nil
	analyticsReader.capacity = CapacityAnalysis{}
	analyticsReader.cost = CostAnalysis{}
	analyticsReader.des = DESComponents{}
	service.analytics = analyticsReader
	now := time.Now().UTC()

	result, err := service.GenerateRecommendation(context.Background(),
		testGenerationInput(resourceUID, policy.UID, now.Add(-30*24*time.Hour), now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Action.Type != ActionNoAction || result.GenerationState != "insufficient_information" {
		t.Fatalf("without analytics evidence got action=%q state=%q",
			result.Action.Type, result.GenerationState)
	}
}

func TestGenerateRecommendationReturnsNoActionForNegativeSavings(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	service.prices = engineTestPrices{"GP_Gen5_8": 100, "GP_Gen5_6": 100}
	now := time.Now().UTC()

	result, err := service.GenerateRecommendation(context.Background(),
		testGenerationInput(resourceUID, policy.UID, now.Add(-30*24*time.Hour), now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Action.Type != ActionNoAction || result.EstimatedSaving.MonthlyAmount != 0 {
		t.Fatalf("negative savings should produce no_action with zeroed decision savings: %+v", result)
	}
}

func TestGenerateRecommendationRejectsInfeasibleCandidate(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	service.forecasts = engineTestForecasts{{
		ForecastUID: uuid.New(), ResourceUID: resourceUID, TargetMetric: "cpu_avg",
		PeriodStartUTC: time.Now().UTC(), YPred: 100, YUpper: floatPtr(100),
	}}
	now := time.Now().UTC()

	result, err := service.GenerateRecommendation(context.Background(),
		testGenerationInput(resourceUID, policy.UID, now.Add(-30*24*time.Hour), now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Action.Type != ActionNoAction {
		t.Fatalf("infeasible candidate produced action %q, want no_action", result.Action.Type)
	}
	foundFeasible := false
	for _, simulation := range service.repository.(*engineTestRepository).persist.Simulations {
		if simulation.Feasible != nil && *simulation.Feasible {
			foundFeasible = true
		}
	}
	if foundFeasible {
		t.Fatal("at least one candidate simulation was feasible despite over-capacity forecasts")
	}
}

func TestGenerateRecommendationReturnsNoActionBelowConfidenceThreshold(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	policy.Constraints["min_confidence"] = 1.01
	service.policyRepository = engineTestPolicyRepository{policy: policy}
	now := time.Now().UTC()

	result, err := service.GenerateRecommendation(context.Background(),
		testGenerationInput(resourceUID, policy.UID, now.Add(-30*24*time.Hour), now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Action.Type != ActionNoAction {
		t.Fatalf("below-policy confidence produced action %q, want no_action", result.Action.Type)
	}
}

func TestEvaluatePolicyRejectsMissingLatencyAndSLAPredictions(t *testing.T) {
	tests := []struct {
		name        string
		constraints map[string]any
		want        string
	}{
		{"latency", map[string]any{"max_latency_increase_pct": 10.0}, "latency_evidence_unavailable"},
		{"sla", map[string]any{"max_sla_violations": 0.0}, "sla_evidence_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := EvaluatePolicy(
				OptimizationPolicy{UID: uuid.New(), Version: 1, Enabled: true, Constraints: test.constraints},
				PolicyEvaluationInput{
					Action: RecommendationAction{Type: ActionResizeDown},
					Simulation: ScenarioSimulation{
						Feasible: boolPtr(true),
					},
					Confidence: RecommendationConfidence{Score: 0.9},
				},
			)
			if result.Allowed || !containsString(result.ViolatedConstraints, test.want) {
				t.Fatalf("EvaluatePolicy() = %+v, want rejection for %s", result, test.want)
			}
		})
	}
}

func TestCalculateRiskCanReachCriticalLevel(t *testing.T) {
	p50, p99 := 0.0, 100.0
	evidence := generationEvidence{EvidenceBundle: EvidenceBundle{
		PercentileStatistics: []PercentileStatistic{
			{MetricCode: "cpu_percent", Percentile: 50, Value: &p50},
			{MetricCode: "cpu_percent", Percentile: 99, Value: &p99},
		},
		Trends:           []Trend{{MetricCode: "cpu_percent", Slope: 100}},
		CapacityAnalysis: &CapacityAnalysis{PeakUtilization: 100, HeadroomPct: 0},
	}}
	risk := calculateRisk(evidence, ScenarioSimulation{}, CandidateConfiguration{
		CapacityVCores: floatPtr(1),
	}, Configuration{CapacityVCores: floatPtr(8)})
	if risk.Level != RiskLevelCritical || risk.Score < 0.8 {
		t.Fatalf("risk = %+v, want critical score >= 0.8", risk)
	}
}

func TestGenerateRecommendationSupportsScaleUp(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	family := "Gen5"
	sku := "GP_Gen5_6"
	model := "provisioned"
	capacity := 6.0
	service.configurations = engineTestConfiguration{snapshot: &ConfigurationSnapshot{
		ResourceUID: resourceUID, SKUName: &sku, Tier: stringPtr("GeneralPurpose"),
		Family: &family, ComputeModel: &model, CapacityVCores: &capacity,
		ObservedAtUTC: time.Now().UTC(),
	}}
	analyticsReader := service.analytics.(engineTestAnalytics)
	cpu95, cpu99 := 85.0, 95.0
	for i := range analyticsReader.percentiles {
		if analyticsReader.percentiles[i].MetricCode != "cpu_percent" {
			continue
		}
		switch analyticsReader.percentiles[i].Percentile {
		case 95:
			analyticsReader.percentiles[i].Value = &cpu95
		case 99:
			analyticsReader.percentiles[i].Value = &cpu99
		}
	}
	service.analytics = analyticsReader
	service.skus = engineTestSKUs{
		{ID: 4, Name: "GP_Gen5_4", Tier: "GeneralPurpose", Family: &family, ComputeModel: "provisioned", CapacityVCores: 4, InScope: true},
		{ID: 6, Name: "GP_Gen5_6", Tier: "GeneralPurpose", Family: &family, ComputeModel: "provisioned", CapacityVCores: 6, InScope: true},
		{ID: 8, Name: "GP_Gen5_8", Tier: "GeneralPurpose", Family: &family, ComputeModel: "provisioned", CapacityVCores: 8, InScope: true},
	}
	service.prices = engineTestPrices{"GP_Gen5_6": 0.4, "GP_Gen5_8": 0.5}
	now := time.Now().UTC()
	result, err := service.GenerateRecommendation(context.Background(),
		testGenerationInput(resourceUID, policy.UID, now.Add(-30*24*time.Hour), now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Action.Type != ActionResizeUp || result.ProposedConfiguration.SKUName != "GP_Gen5_8" {
		t.Fatalf("got action=%q SKU=%q, want resize_up GP_Gen5_8", result.Action.Type, result.ProposedConfiguration.SKUName)
	}
}

func TestGenerateRecommendationSupportsComputeModelChange(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	policy.Constraints["allow_compute_model_change"] = true
	service.policyRepository = engineTestPolicyRepository{policy: policy}
	analyticsReader := service.analytics.(engineTestAnalytics)
	analyticsReader.pattern.IntensityBand = "low"
	analyticsReader.pattern.PeakFrequency = 1
	analyticsReader.snapshots[0].CPUAvg = 10
	cpu50, cpu95, cpu99 := 10.0, 15.0, 20.0
	for i := range analyticsReader.percentiles {
		switch analyticsReader.percentiles[i].Percentile {
		case 50:
			analyticsReader.percentiles[i].Value = &cpu50
		case 95:
			analyticsReader.percentiles[i].Value = &cpu95
		case 99:
			analyticsReader.percentiles[i].Value = &cpu99
		}
	}
	service.analytics = analyticsReader
	family := "Gen5"
	service.skus = engineTestSKUs{
		{ID: 8, Name: "GP_Gen5_8", Tier: "GeneralPurpose", Family: &family, ComputeModel: "provisioned", CapacityVCores: 8, InScope: true},
		{ID: 9, Name: "GP_S_Gen5_8", Tier: "GeneralPurpose", Family: &family, ComputeModel: "serverless", CapacityVCores: 8, InScope: true},
	}
	service.prices = engineTestPrices{"GP_Gen5_8": 0.5, "GP_S_Gen5_8": 0.4}
	now := time.Now().UTC()
	result, err := service.GenerateRecommendation(context.Background(),
		testGenerationInput(resourceUID, policy.UID, now.Add(-30*24*time.Hour), now))
	if err != nil {
		t.Fatal(err)
	}
	if result.Action.Type != ActionChangeComputeModel || result.ProposedConfiguration.ComputeModel != "serverless" {
		t.Fatalf("got action=%q model=%q, want change_compute_model/serverless",
			result.Action.Type, result.ProposedConfiguration.ComputeModel)
	}
}

func TestGenerateRecommendationIncludesRequiredEvidence(t *testing.T) {
	service, _, policy, resourceUID := newTestEngine(t, nil)
	now := time.Now().UTC()
	result, err := service.GenerateRecommendation(context.Background(),
		testGenerationInput(resourceUID, policy.UID, now.Add(-30*24*time.Hour), now))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"cpu_p95", "cpu_p99", "headroom", "projected_monthly_cost", "estimated_savings"} {
		if !containsEvidence(result.Evidence, required) {
			t.Errorf("evidence missing required type %q: %+v", required, result.Evidence)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsEvidence(values []RecommendationEvidence, target string) bool {
	for _, value := range values {
		if value.Type == target {
			return true
		}
	}
	return false
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
	persistErr   error
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
	if r.persistErr != nil {
		return Recommendation{}, r.persistErr
	}
	decision.Recommendation.ScenarioUID = decision.Scenario.UID
	decision.Recommendation.ScenarioID = 101
	if decision.SelectedCandidateIndex != nil {
		id := int64(*decision.SelectedCandidateIndex + 1)
		decision.Recommendation.ChosenCandidateID = &id
	}
	return decision.Recommendation, nil
}

type engineTestAuditSink struct {
	repository *engineTestRepository
	events     []DecisionAuditEvent
	err        error
}

func (s *engineTestAuditSink) RecordDecision(_ context.Context, event DecisionAuditEvent) error {
	if s.repository.persistCalls == 0 {
		return errors.New("audit called before recommendation persistence")
	}
	s.events = append(s.events, event)
	return s.err
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
	err      error
}

func (c engineTestConfiguration) LatestConfigSnapshot(context.Context, uuid.UUID) (*ConfigurationSnapshot, error) {
	return c.snapshot, c.err
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
