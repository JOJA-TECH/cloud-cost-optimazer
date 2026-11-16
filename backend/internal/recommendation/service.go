package recommendation

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultPeriodType     = "1h"
	defaultSnapshotLimit  = 1000
	defaultCostLimit      = 10000
	defaultAlgorithm      = "rule_v1"
	hoursPerAverageMonth  = 730.0
	daysPerAverageMonth   = 365.2425 / 12
	maxRecommendationDays = 365
)

type Service struct {
	policyEvaluator  PolicyEvaluator
	policyRepository OptimizationPolicyRepository
	repository       RecommendationRepository
	analytics        AnalyticsReader
	forecasts        MLPredictionReader
	costs            CostReader
	configurations   ConfigurationReader
	resourceContext  ResourceContextReader
	prices           PriceReader
	skus             SKUReader
	audit            AuditSink

	lockMu sync.Mutex
	locks  map[uuid.UUID]*resourceLock
}

type resourceLock struct {
	mu    sync.Mutex
	users int
}

type Dependencies struct {
	PolicyEvaluator  PolicyEvaluator
	PolicyRepository OptimizationPolicyRepository
	Repository       RecommendationRepository
	Analytics        AnalyticsReader
	Forecasts        MLPredictionReader
	Costs            CostReader
	Configurations   ConfigurationReader
	ResourceContext  ResourceContextReader
	Prices           PriceReader
	SKUs             SKUReader
	Audit            AuditSink
}

func NewService(policyEvaluator PolicyEvaluator, repository RecommendationRepository) (*Service, error) {
	return NewEngine(Dependencies{PolicyEvaluator: policyEvaluator, Repository: repository})
}

func NewEngine(deps Dependencies) (*Service, error) {
	if deps.PolicyEvaluator == nil {
		return nil, ErrPolicyEvaluator
	}
	if deps.Repository == nil {
		return nil, ErrRepository
	}
	return &Service{
		policyEvaluator:  deps.PolicyEvaluator,
		policyRepository: deps.PolicyRepository,
		repository:       deps.Repository,
		analytics:        deps.Analytics,
		forecasts:        deps.Forecasts,
		costs:            deps.Costs,
		configurations:   deps.Configurations,
		resourceContext:  deps.ResourceContext,
		prices:           deps.Prices,
		skus:             deps.SKUs,
		audit:            deps.Audit,
		locks:            make(map[uuid.UUID]*resourceLock),
	}, nil
}

func (s *Service) acquireResourceLock(resourceUID uuid.UUID) (func(), error) {
	s.lockMu.Lock()
	entry := s.locks[resourceUID]
	if entry == nil {
		entry = &resourceLock{}
		s.locks[resourceUID] = entry
	}
	entry.users++
	s.lockMu.Unlock()

	if !entry.mu.TryLock() {
		s.lockMu.Lock()
		entry.users--
		if entry.users == 0 {
			delete(s.locks, resourceUID)
		}
		s.lockMu.Unlock()
		return nil, ErrConcurrentOperation
	}

	return func() {
		entry.mu.Unlock()
		s.lockMu.Lock()
		entry.users--
		if entry.users == 0 {
			delete(s.locks, resourceUID)
		}
		s.lockMu.Unlock()
	}, nil
}

func (s *Service) validateRequest(input GenerateRecommendationInput) error {
	if input.ResourceUID == uuid.Nil {
		return fmt.Errorf("%w: resource_uid is required", ErrInvalidRequest)
	}
	if input.PolicyUID == uuid.Nil {
		return fmt.Errorf("%w: policy_uid is required", ErrPolicyRequired)
	}
	if input.From.IsZero() || input.To.IsZero() || !input.From.Before(input.To) {
		return fmt.Errorf("%w: from must be before to", ErrInvalidRequest)
	}
	if input.HorizonDays < 1 || input.HorizonDays > maxRecommendationDays {
		return fmt.Errorf("%w: horizon_days must be between 1 and %d", ErrInvalidRequest, maxRecommendationDays)
	}
	if len(input.Currency) != 3 || input.Currency != strings.ToUpper(input.Currency) {
		return fmt.Errorf("%w: currency must be a three-letter uppercase code", ErrInvalidRequest)
	}
	return nil
}

// GenerateRecommendation loads evidence, evaluates catalog-backed candidates,
// persists a versioned decision atomically, and records the audit event.
func (s *Service) GenerateRecommendation(ctx context.Context, input GenerateRecommendationInput) (Recommendation, error) {
	evaluation, err := s.generate(ctx, input, true)
	if err != nil {
		return Recommendation{}, err
	}
	return evaluation.recommendation, nil
}

type generationOutcome struct {
	recommendation Recommendation
	result         RecommendationEvaluation
}

func (s *Service) EvaluateRecommendation(ctx context.Context, input GenerateRecommendationInput) (RecommendationEvaluation, error) {
	outcome, err := s.generate(ctx, input, false)
	if err != nil {
		return RecommendationEvaluation{}, err
	}
	return outcome.result, nil
}

func (s *Service) generate(ctx context.Context, input GenerateRecommendationInput, persist bool) (generationOutcome, error) {
	startedAt := time.Now()
	if err := s.validateRequest(input); err != nil {
		return generationOutcome{}, err
	}
	unlock, err := s.acquireResourceLock(input.ResourceUID)
	if err != nil {
		return generationOutcome{}, err
	}
	defer unlock()

	if s.policyRepository == nil || s.configurations == nil || s.analytics == nil ||
		s.resourceContext == nil || s.costs == nil || s.prices == nil || s.skus == nil {
		return generationOutcome{}, ErrDependenciesUnavailable
	}

	policy, err := s.policyRepository.GetPolicy(ctx, input.PolicyUID)
	if err != nil {
		if errors.Is(err, ErrPolicyNotFound) {
			return generationOutcome{}, err
		}
		return generationOutcome{}, fmt.Errorf("load optimization policy: %w", err)
	}
	if policy.UID != input.PolicyUID {
		return generationOutcome{}, ErrPolicyNotFound
	}
	if !policy.Enabled {
		return generationOutcome{}, ErrPolicyDisabled
	}

	current, err := s.configurations.LatestConfigSnapshot(ctx, input.ResourceUID)
	if err != nil {
		if errors.Is(err, ErrResourceNotFound) {
			return generationOutcome{}, err
		}
		return generationOutcome{}, fmt.Errorf("load current resource configuration: %w", err)
	}
	if current == nil || current.SKUName == nil || strings.TrimSpace(*current.SKUName) == "" {
		return generationOutcome{}, ErrConfigurationUnavailable
	}

	var previous *Recommendation
	if !input.ForceRefresh {
		latest, latestErr := s.repository.GetLatestRecommendation(ctx, input.ResourceUID)
		if latestErr == nil {
			previous = &latest
			if latest.PolicyUID == policy.UID &&
				latest.WindowStart.Equal(input.From) && latest.WindowEnd.Equal(input.To) &&
				latest.HorizonDays == input.HorizonDays &&
				latest.EstimatedSaving.Currency == input.Currency {
				return generationOutcome{
					recommendation: latest,
					result: RecommendationEvaluation{
						ResourceUID: input.ResourceUID, PolicyUID: policy.UID,
						From: input.From, To: input.To, HorizonDays: input.HorizonDays,
						Currency: input.Currency, CandidateResults: nil,
						State: latest.GenerationState,
					},
				}, nil
			}
		} else if !errors.Is(latestErr, ErrRecommendationNotFound) {
			return generationOutcome{}, fmt.Errorf("load latest recommendation: %w", latestErr)
		}
	} else {
		latest, latestErr := s.repository.GetLatestRecommendation(ctx, input.ResourceUID)
		if latestErr == nil {
			previous = &latest
		} else if !errors.Is(latestErr, ErrRecommendationNotFound) {
			return generationOutcome{}, fmt.Errorf("load latest recommendation: %w", latestErr)
		}
	}

	evidence, err := s.loadEvidence(ctx, input, current, policy)
	if err != nil {
		return generationOutcome{}, err
	}
	costRows, err := s.costs.Costs(ctx, input.ResourceUID, input.From, input.To, defaultCostLimit)
	if err != nil {
		return generationOutcome{}, fmt.Errorf("read observed costs: %w", err)
	}
	evidence.Costs = costRows
	actualCost, costCurrency := observedMonthlyCost(costRows, input.From, input.To)
	if len(costRows) == 0 && evidence.CostAnalysis != nil && evidence.CostAnalysis.ActualMonthlyCost > 0 {
		actualCost = evidence.CostAnalysis.ActualMonthlyCost
	}
	if actualCost <= 0 {
		evidence.InsufficientReasons = append(evidence.InsufficientReasons, "observed_cost_unavailable")
	}
	if costCurrency != "" && costCurrency != input.Currency {
		return generationOutcome{}, fmt.Errorf("%w: observed costs use %s, requested currency is %s", ErrInvalidRequest, costCurrency, input.Currency)
	}

	requireForecast := policyBool(policy.Constraints, "require_forecast") || policyBool(policy.Constraints, "forecast_required")
	if requireForecast && len(evidence.Forecasts) == 0 {
		evidence.Degraded = true
		evidence.InsufficientReasons = append(evidence.InsufficientReasons, "required_forecast_unavailable")
	}
	allowServerless := policyBool(policy.Constraints, "allow_compute_model_change") ||
		strings.EqualFold(stringValue(policy.Constraints["allow_compute_model_change"]), "true")
	priceContext, err := s.resourceContext.PricingContext(ctx, input.ResourceUID)
	if err != nil {
		return generationOutcome{}, fmt.Errorf("load resource pricing context: %w", err)
	}
	evidence.ProviderCode, evidence.RegionCode, evidence.MeterID =
		priceContext.ProviderCode, priceContext.RegionCode, priceContext.MeterID
	skus, err := s.skus.ListSKUs(ctx, *current, allowServerless)
	if err != nil {
		return generationOutcome{}, fmt.Errorf("load candidate SKUs: %w", err)
	}
	candidateSKUs := adjacentSKUs(skus, *current, allowServerless)

	scenario := Scenario{
		UID:              uuid.New(),
		ResourceUID:      input.ResourceUID,
		PolicyUID:        policy.UID,
		CreatedBy:        input.CreatedBy,
		HorizonDays:      input.HorizonDays,
		ObjectiveWeights: policy.ObjectiveWeights,
		Constraints:      policy.Constraints,
		Status:           "completed",
		CreatedAt:        time.Now().UTC(),
		CompletedAt:      timePtr(time.Now().UTC()),
	}
	if scenario.CreatedBy == "" {
		scenario.CreatedBy = "system"
	}

	currentConfig := configurationFromSnapshot(*current)
	candidates := make([]CandidateConfiguration, 0, len(candidateSKUs))
	simulations := make([]ScenarioSimulation, 0, len(candidateSKUs))
	candidateResults := make([]CandidateEvaluation, 0, len(candidateSKUs))
	selectedIndex := -1
	bestCost, bestRisk, bestConfidence := math.Inf(1), math.Inf(1), math.Inf(-1)
	var chosen *CandidateConfiguration
	var chosenSaving EstimatedSaving
	var chosenRisk RecommendationRisk
	var chosenConfidence RecommendationConfidence
	var rationale RecommendationRationale

	for _, sku := range candidateSKUs {
		candidate := candidateFromSKU(sku)
		priceQuery := PriceQuery{
			ProviderCode: evidence.ProviderCode,
			RegionCode:   evidence.RegionCode,
			MeterID:      evidence.MeterID,
			SKUName:      sku.Name,
			ARMSKUName:   derefString(sku.ARMSKUName),
			ComputeModel: sku.ComputeModel,
			Currency:     input.Currency,
			EffectiveAt:  input.To,
		}
		priceRows, priceErr := s.prices.ListPrices(ctx, priceQuery)
		if priceErr != nil {
			return generationOutcome{}, fmt.Errorf("read list price for SKU %q: %w", sku.Name, priceErr)
		}
		proposedMonthly, basis, priceFound := monthlyListPrice(priceRows)
		if !priceFound {
			candidateResults = append(candidateResults, CandidateEvaluation{
				Candidate: candidate, DiscardReason: "list_price_unavailable",
			})
			continue
		}
		candidate.EstimatedMonthlyCost = floatPtr(proposedMonthly)
		priceBasis := string(basis)
		candidate.PriceBasis = &priceBasis
		candidates = append(candidates, candidate)

		simulation := simulateCandidate(candidate, currentConfig, evidence, actualCost, proposedMonthly, input)
		risk := calculateRisk(evidence, simulation, candidate, currentConfig)
		confidence := calculateConfidence(evidence, simulation, true, !evidence.Degraded)
		policyInput := PolicyEvaluationInput{
			Candidate: candidate, CurrentConfiguration: currentConfig,
			Simulation: simulation, Evidence: evidence.EvidenceBundle,
			Confidence: confidence, Risk: risk,
			Action: actionForConfigurations(currentConfig, configurationFromSKU(sku), proposedMonthly >= actualCost),
		}
		policyResult, policyErr := s.policyEvaluator.Validate(ctx, policy, policyInput)
		if policyErr != nil {
			return generationOutcome{}, fmt.Errorf("evaluate policy for candidate %q: %w", sku.Name, policyErr)
		}
		policyResult.PolicyUID = policy.UID
		policyResult.PolicyVersion = policy.Version
		saving := savingEstimate(actualCost, proposedMonthly, costCurrencyOrRequested(costCurrency, input.Currency),
			CostBasisActualCost, basis, input.From, input.To, "actual_cost_vs_retail_list_price")
		ruleAllowed, ruleReason := evaluateInitialOpportunity(candidate, currentConfig, evidence, simulation, saving, policy)
		allowed := simulation.Feasible != nil && *simulation.Feasible && policyResult.Allowed &&
			risk.Score <= maxRisk(policy.Constraints) && confidence.Score >= minConfidence(policy.Constraints) &&
			ruleAllowed
		discardReason := ""
		if !allowed {
			discardReason = ruleReason
			if discardReason == "" {
				discardReason = candidateDiscardReason(simulation, policyResult, risk, confidence, policy)
			}
		}
		simulations = append(simulations, simulation)
		result := CandidateEvaluation{
			Candidate: candidate, Simulation: simulation,
			Saving: saving,
			Risk:   risk, Confidence: confidence, Policy: policyResult, DiscardReason: discardReason,
		}
		candidateResults = append(candidateResults, result)
		if allowed && betterCandidate(proposedMonthly, risk.Score, confidence.Score, bestCost, bestRisk, bestConfidence) {
			selectedIndex = len(candidates) - 1
			bestCost, bestRisk, bestConfidence = proposedMonthly, risk.Score, confidence.Score
			candidateCopy := candidate
			chosen = &candidateCopy
			chosenSaving = result.Saving
			chosenRisk = risk
			chosenConfidence = confidence
			rationale = rationaleFromEvidence(evidence, simulation, result.Saving, policyResult)
		}
	}

	isInsufficient := len(evidence.InsufficientReasons) > 0 || !evidence.HasMinimumEvidence ||
		(actualCost <= 0) || len(candidateSKUs) == 0 || len(candidates) == 0
	if isInsufficient || chosen == nil {
		selectedIndex = -1
		chosenSaving = savingEstimate(actualCost, actualCost, costCurrencyOrRequested(costCurrency, input.Currency),
			CostBasisActualCost, CostBasisActualCost, input.From, input.To, "no_action")
		chosenRisk = calculateRisk(evidence, ScenarioSimulation{}, CandidateConfiguration{}, currentConfig)
		chosenConfidence = calculateConfidence(evidence, ScenarioSimulation{}, false, false)
		reasons := append([]string(nil), evidence.InsufficientReasons...)
		if actualCost <= 0 {
			reasons = append(reasons, "observed_cost_unavailable")
		}
		if chosen == nil && !isInsufficient {
			reasons = append(reasons, "no_candidate_satisfies_policy")
		}
		rationale = RecommendationRationale{
			Summary: "No se propone un cambio porque la evidencia es insuficiente o ningún candidato cumple la política.",
		}
		for _, reason := range reasons {
			rationale.Evidence = append(rationale.Evidence, Evidence{Type: "decision_reason", Context: map[string]any{"reason": reason}, Method: defaultAlgorithm})
		}
	} else {
		for i := range candidateResults {
			if candidateResults[i].Candidate.SKUName == chosen.SKUName {
				candidateResults[i].Selected = true
			}
		}
	}

	action := RecommendationAction{Type: ActionNoAction}
	proposedConfig := currentConfig
	var chosenCandidateID *int64
	if chosen == nil {
		noActionPolicy, policyErr := s.policyEvaluator.Validate(ctx, policy, PolicyEvaluationInput{
			CurrentConfiguration: currentConfig,
			Evidence:             evidence.EvidenceBundle,
			Confidence:           chosenConfidence,
			Risk:                 chosenRisk,
			Action:               action,
		})
		if policyErr != nil {
			return generationOutcome{}, fmt.Errorf("evaluate no-action decision against policy: %w", policyErr)
		}
		noActionPolicy.PolicyUID, noActionPolicy.PolicyVersion = policy.UID, policy.Version
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "policy_evaluation", Context: map[string]any{
				"allowed":              noActionPolicy.Allowed,
				"warnings":             noActionPolicy.Warnings,
				"violated_constraints": noActionPolicy.ViolatedConstraints,
			},
			Method: "policy_evaluator",
		})
	}
	if chosen != nil && selectedIndex >= 0 {
		proposedConfig = configurationForCandidate(*chosen)
		action = actionForConfigurations(currentConfig, proposedConfig, chosenSaving.MonthlyAmount >= 0)
	}
	decision := Recommendation{
		UID: uuid.New(), ScenarioUID: scenario.UID, PolicyUID: policy.UID, PolicyVersion: policy.Version,
		Version: 1, ResourceUID: input.ResourceUID, WindowStart: input.From,
		WindowEnd: input.To, HorizonDays: input.HorizonDays,
		ChosenCandidateID: chosenCandidateID,
		Action:            action, CurrentConfiguration: currentConfig, ProposedConfiguration: proposedConfig,
		EstimatedSaving: chosenSaving, Risk: chosenRisk, Confidence: chosenConfidence,
		Rationale: rationale, Evidence: recommendationEvidence(evidence, rationale),
		Status: RecommendationStatusProposed, GeneratedBy: defaultAlgorithm, CreatedAt: time.Now().UTC(),
		GenerationState: "complete",
	}
	if previous != nil {
		decision.Version = previous.Version + 1
		supersedes := previous.UID
		decision.SupersedesUID = &supersedes
	}
	if action.Type == ActionNoAction {
		decision.GenerationState = "no_action"
		if isInsufficient {
			decision.GenerationState = "insufficient_information"
		}
	}

	result := RecommendationEvaluation{
		ResourceUID: input.ResourceUID, PolicyUID: policy.UID, From: input.From, To: input.To,
		HorizonDays: input.HorizonDays, Currency: input.Currency,
		Confidence: chosenConfidence, EvidenceCount: len(decision.Evidence),
		ForecastAvailable: len(evidence.Forecasts) > 0, Degraded: evidence.Degraded,
		State: decision.GenerationState, CandidateResults: candidateResults,
	}
	if !persist {
		return generationOutcome{recommendation: decision, result: result}, nil
	}

	riskAssessment := riskAssessmentFromRisk(chosenRisk)
	persisted, err := s.repository.PersistDecision(ctx, DecisionPersistence{
		Scenario: scenario, Candidates: candidates, Simulations: simulations,
		Recommendation: decision, Evidence: rationale.Evidence, RiskAssessment: riskAssessment,
		SelectedCandidateIndex: selectedCandidateIndex(selectedIndex, candidates),
	})
	if err != nil {
		return generationOutcome{}, fmt.Errorf("persist recommendation decision: %w", err)
	}
	if s.audit != nil {
		var selectedCandidate *string
		if chosen != nil {
			selectedName := skuName(chosen)
			selectedCandidate = &selectedName
		}
		auditEvent := DecisionAuditEvent{
			RecommendationUID: persisted.UID, ResourceUID: input.ResourceUID,
			Decision: string(action.Type), Actor: input.CreatedBy, OccurredAt: time.Now().UTC(),
			RequestID: input.RequestID, CorrelationID: input.CorrelationID, OperationID: input.OperationID,
			PolicyUID: policy.UID, PolicyVersion: policy.Version,
			AlgorithmVersion: defaultAlgorithm, ScenarioUID: scenario.UID,
			SelectedCandidate: selectedCandidate, CandidateResults: candidateAuditResults(candidateResults),
			RiskScore: decision.Risk.Score, RiskLevel: string(decision.Risk.Level),
			ConfidenceScore: decision.Confidence.Score, EstimatedSaving: decision.EstimatedSaving.MonthlyAmount,
			EvidenceCount: len(decision.Evidence),
			Duration:      time.Since(startedAt),
			Errors:        evidence.PartialErrors,
		}
		if err := s.audit.RecordDecision(ctx, auditEvent); err != nil {
			return generationOutcome{recommendation: persisted}, fmt.Errorf("record recommendation decision audit: %w", err)
		}
	}
	resultRecommendation := persisted
	return generationOutcome{recommendation: resultRecommendation, result: result}, nil
}

func (s *Service) loadEvidence(ctx context.Context, input GenerateRecommendationInput, current *ConfigurationSnapshot, policy OptimizationPolicy) (generationEvidence, error) {
	result := generationEvidence{EvidenceBundle: EvidenceBundle{Configuration: current}}
	from, to := input.From.UTC(), input.To.UTC()
	snapshots, err := s.analytics.FeatureSnapshot(ctx, input.ResourceUID, defaultPeriodType, from, to, defaultSnapshotLimit)
	if err != nil {
		return result, fmt.Errorf("read feature snapshots: %w", err)
	}
	result.FeatureSnapshots = snapshots
	result.HasMinimumEvidence = len(snapshots) > 0
	for _, metric := range []string{"cpu_percent", "data_io_percent", "sessions_percent"} {
		values, readErr := s.analytics.PercentileStatistic(ctx, input.ResourceUID, metric, defaultPeriodType, from, to)
		if readErr != nil {
			return result, fmt.Errorf("read %s percentiles: %w", metric, readErr)
		}
		result.PercentileStatistics = append(result.PercentileStatistics, values...)
	}
	if !hasPercentile(result.PercentileStatistics, "cpu_percent", 95) ||
		!hasPercentile(result.PercentileStatistics, "cpu_percent", 99) {
		result.InsufficientReasons = append(result.InsufficientReasons, "cpu_percentiles_unavailable")
	}
	trends, err := s.analytics.Trend(ctx, input.ResourceUID, from, to)
	if err != nil {
		return result, fmt.Errorf("read trends: %w", err)
	}
	result.Trends = trends
	pattern, err := s.analytics.WorkloadPattern(ctx, input.ResourceUID, from, to)
	if err != nil {
		return result, fmt.Errorf("read workload pattern: %w", err)
	}
	result.WorkloadPattern = &pattern
	periodStart := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	capacity, err := s.analytics.CapacityAnalysis(ctx, input.ResourceUID, "1d", periodStart)
	if err != nil {
		return result, fmt.Errorf("read capacity analysis: %w", err)
	}
	result.CapacityAnalysis = &capacity
	costAnalysis, err := s.analytics.CostAnalysis(ctx, input.ResourceUID, from, to)
	if err != nil {
		return result, fmt.Errorf("read cost analysis: %w", err)
	}
	result.CostAnalysis = &costAnalysis
	des, err := s.analytics.DESComponents(ctx, input.ResourceUID, "1d", periodStart)
	if err != nil {
		return result, fmt.Errorf("read DES components: %w", err)
	}
	result.DESComponents = &des

	if s.forecasts != nil {
		forecastStart := to
		forecastEnd := to.Add(time.Duration(input.HorizonDays) * 24 * time.Hour)
		forecasts, forecastErr := s.forecasts.Forecasts(ctx, input.ResourceUID, "cpu_avg", forecastStart, forecastEnd)
		if forecastErr != nil {
			if errors.Is(forecastErr, ErrMLUnavailable) {
				result.Degraded = true
				result.PartialErrors = append(result.PartialErrors, forecastErr.Error())
			} else {
				return result, fmt.Errorf("read ML forecasts: %w", forecastErr)
			}
		} else {
			for _, forecast := range forecasts {
				if forecast.ResourceUID == input.ResourceUID && forecast.TargetMetric == "cpu_avg" &&
					!forecast.PeriodStartUTC.Before(forecastStart) &&
					!forecast.PeriodStartUTC.After(forecastEnd) {
					result.Forecasts = append(result.Forecasts, forecast)
				}
			}
		}
	}
	if len(result.Forecasts) == 0 {
		result.Degraded = true
		if policyBool(policy.Constraints, "require_forecast") || policyBool(policy.Constraints, "forecast_required") {
			result.InsufficientReasons = append(result.InsufficientReasons, "required_forecast_unavailable")
		}
	}
	result.HasMinimumEvidence = result.HasMinimumEvidence &&
		result.CapacityAnalysis != nil && !result.CapacityAnalysis.ComputedAt.IsZero() &&
		result.CostAnalysis != nil && !result.CostAnalysis.ComputedAt.IsZero() &&
		result.DESComponents != nil && !result.DESComponents.ComputedAt.IsZero() &&
		len(result.Trends) > 0
	if !result.HasMinimumEvidence {
		result.InsufficientReasons = append(result.InsufficientReasons, "minimum_analytics_evidence_unavailable")
	}

	return result, nil
}

type generationEvidence struct {
	EvidenceBundle
	ProviderCode        string
	RegionCode          string
	MeterID             string
	HasMinimumEvidence  bool
	Degraded            bool
	InsufficientReasons []string
	PartialErrors       []string
}

func (s *Service) EstimateSaving(_ context.Context, input SavingInput) (EstimatedSaving, error) {
	if input.CurrentMonthlyCost < 0 || input.ProposedMonthlyCost < 0 ||
		input.Currency == "" || !input.PeriodStart.Before(input.PeriodEnd) {
		return EstimatedSaving{}, ErrInvalidRequest
	}
	return savingEstimate(input.CurrentMonthlyCost, input.ProposedMonthlyCost, input.Currency,
		input.CurrentCostBasis, input.ProposedCostBasis, input.PeriodStart, input.PeriodEnd, input.Method), nil
}

func (s *Service) EvaluateRisk(_ context.Context, input RiskInput) (RecommendationRisk, error) {
	if input.Candidate.SKUName == "" {
		return RecommendationRisk{}, ErrInvalidRequest
	}
	return calculateRisk(generationEvidence{EvidenceBundle: input.Evidence}, input.Simulation, input.Candidate, Configuration{}), nil
}

func (s *Service) EvaluateOpportunity(ctx context.Context, input OpportunityInput) (OpportunityEvaluation, error) {
	if input.Policy.UID == uuid.Nil || !input.Policy.Enabled {
		return OpportunityEvaluation{}, ErrPolicyRequired
	}
	risk := calculateRisk(generationEvidence{EvidenceBundle: input.Evidence}, ScenarioSimulation{}, input.Candidate, Configuration{})
	confidence := calculateConfidence(generationEvidence{EvidenceBundle: input.Evidence}, ScenarioSimulation{}, false, true)
	action := actionForConfigurations(Configuration{}, configurationForCandidate(input.Candidate), true)
	evaluation, err := s.policyEvaluator.Validate(ctx, input.Policy, PolicyEvaluationInput{
		Candidate: input.Candidate, Evidence: input.Evidence, Confidence: confidence, Risk: risk, Action: action,
	})
	if err != nil {
		return OpportunityEvaluation{}, fmt.Errorf("evaluate opportunity policy: %w", err)
	}
	return OpportunityEvaluation{
		Candidate: input.Candidate, Feasible: evaluation.Allowed && risk.Score <= maxRisk(input.Policy.Constraints),
		Policy: evaluation, Risk: risk, Rationale: RecommendationRationale{},
	}, nil
}

func (s *Service) ExplainRecommendation(ctx context.Context, recommendationUID uuid.UUID) (RecommendationRationale, error) {
	rec, err := s.repository.GetRecommendation(ctx, recommendationUID)
	if err != nil {
		return RecommendationRationale{}, err
	}
	return rec.Rationale, nil
}

func (s *Service) GetRecommendation(ctx context.Context, recommendationUID uuid.UUID) (Recommendation, error) {
	if recommendationUID == uuid.Nil {
		return Recommendation{}, ErrInvalidRequest
	}
	return s.repository.GetRecommendation(ctx, recommendationUID)
}

func (s *Service) ListRecommendations(ctx context.Context, filter RecommendationFilter) ([]Recommendation, error) {
	if filter.Limit < 0 || filter.Offset < 0 {
		return nil, ErrInvalidRequest
	}
	return s.repository.ListRecommendations(ctx, filter)
}

// PublishRecommendation is retained for focused callers; new generation
// should use GenerateRecommendation so all evidence and related rows commit
// atomically through PersistDecision.
func (s *Service) PublishRecommendation(ctx context.Context, input PublishRecommendationInput) (Recommendation, PolicyEvaluation, error) {
	if input.Policy.UID == uuid.Nil || input.Policy.Name == "" || input.Policy.Version < 1 {
		return Recommendation{}, PolicyEvaluation{}, ErrPolicyRequired
	}
	if !input.Policy.Enabled {
		return Recommendation{}, PolicyEvaluation{}, ErrPolicyDisabled
	}
	evaluation, err := s.policyEvaluator.Validate(ctx, input.Policy, PolicyEvaluationInput{
		Candidate: input.Candidate, CurrentConfiguration: input.CurrentConfiguration,
		Simulation: input.Simulation, Evidence: input.Evidence,
		Confidence: input.Recommendation.Confidence, Risk: input.Recommendation.Risk, Action: input.Recommendation.Action,
	})
	if err != nil {
		return Recommendation{}, PolicyEvaluation{}, fmt.Errorf("evaluate optimization policy %q v%d: %w", input.Policy.Name, input.Policy.Version, err)
	}
	evaluation.PolicyUID, evaluation.PolicyVersion = input.Policy.UID, input.Policy.Version
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

func observedMonthlyCost(records []CostRecord, from, to time.Time) (float64, string) {
	var total float64
	currency := ""
	for _, record := range records {
		if record.ChargeCategory != nil && *record.ChargeCategory != "usage" {
			continue
		}
		if currency == "" {
			currency = record.Currency
		}
		if record.Currency != currency {
			return 0, "MIXED"
		}
		total += record.PreTaxCost
	}
	if total <= 0 || !from.Before(to) {
		return 0, currency
	}
	days := to.Sub(from).Hours() / 24
	return total * daysPerAverageMonth / days, currency
}

func monthlyListPrice(records []PriceRecord) (float64, CostBasis, bool) {
	if len(records) == 0 {
		return 0, CostBasisListPrice, false
	}
	price := records[0]
	switch {
	case strings.Contains(strings.ToLower(price.UnitOfMeasure), "hour"):
		return price.UnitPrice * hoursPerAverageMonth, CostBasisListPrice, true
	case strings.Contains(strings.ToLower(price.UnitOfMeasure), "month"):
		return price.UnitPrice, CostBasisListPrice, true
	default:
		return 0, CostBasisListPrice, false
	}
}

func adjacentSKUs(skus []SKU, current ConfigurationSnapshot, includeServerless bool) []SKU {
	if current.SKUName == nil {
		return nil
	}
	var sameModel []SKU
	var currentSKU *SKU
	for i := range skus {
		sku := skus[i]
		if !sku.InScope {
			continue
		}
		if sku.Name == *current.SKUName {
			currentSKU = &sku
		}
		if strings.EqualFold(sku.Tier, derefString(current.Tier)) &&
			strings.EqualFold(derefString(sku.Family), derefString(current.Family)) &&
			strings.EqualFold(sku.ComputeModel, derefString(current.ComputeModel)) {
			sameModel = append(sameModel, sku)
		}
	}
	sort.Slice(sameModel, func(i, j int) bool { return sameModel[i].CapacityVCores < sameModel[j].CapacityVCores })
	out := make([]SKU, 0, 4)
	if currentSKU != nil {
		out = append(out, *currentSKU)
	} else {
		return nil
	}
	currentIndex := -1
	for i := range sameModel {
		if sameModel[i].Name == currentSKU.Name {
			currentIndex = i
			break
		}
	}
	if currentIndex > 0 {
		out = append(out, sameModel[currentIndex-1])
	}
	if currentIndex >= 0 && currentIndex+1 < len(sameModel) {
		out = append(out, sameModel[currentIndex+1])
	}
	if includeServerless {
		for _, sku := range skus {
			if sku.InScope && strings.EqualFold(sku.Tier, derefString(current.Tier)) &&
				strings.EqualFold(derefString(sku.Family), derefString(current.Family)) &&
				strings.EqualFold(sku.ComputeModel, "serverless") && sku.Name != currentSKU.Name {
				out = append(out, sku)
				break
			}
		}
	}
	return deduplicateSKUs(out)
}

func deduplicateSKUs(skus []SKU) []SKU {
	seen := make(map[string]bool, len(skus))
	out := make([]SKU, 0, len(skus))
	for _, sku := range skus {
		if !seen[sku.Name] {
			seen[sku.Name] = true
			out = append(out, sku)
		}
	}
	return out
}

func candidateFromSKU(sku SKU) CandidateConfiguration {
	id := sku.ID
	return CandidateConfiguration{
		SKUID: &id, SKUName: sku.Name, Tier: sku.Tier, Family: sku.Family,
		ComputeModel: stringPtr(sku.ComputeModel), CapacityVCores: floatPtr(sku.CapacityVCores),
		MinCapacityVCores: sku.MinCapacityVCores, GeneratedBy: "catalog_adjacent",
	}
}

func configurationFromSnapshot(snapshot ConfigurationSnapshot) Configuration {
	return Configuration{
		SKUName: derefString(snapshot.SKUName), ComputeModel: derefString(snapshot.ComputeModel),
		CapacityVCores: snapshot.CapacityVCores,
		Parameters: map[string]any{
			"tier": snapshot.Tier, "family": snapshot.Family, "max_size_bytes": snapshot.MaxSizeBytes,
			"min_capacity_vcores": snapshot.MinCapacityVCores, "auto_pause_delay_minutes": snapshot.AutoPauseDelayMin,
			"observed_at_utc": snapshot.ObservedAtUTC,
		},
	}
}

func configurationFromSKU(sku SKU) Configuration {
	return Configuration{SKUName: sku.Name, ComputeModel: sku.ComputeModel, CapacityVCores: floatPtr(sku.CapacityVCores)}
}

func configurationForCandidate(candidate CandidateConfiguration) Configuration {
	return Configuration{
		SKUName: candidate.SKUName, ComputeModel: derefString(candidate.ComputeModel),
		CapacityVCores: candidate.CapacityVCores,
	}
}

func candidateFromConfiguration(configuration Configuration) CandidateConfiguration {
	return CandidateConfiguration{
		SKUName: configuration.SKUName, ComputeModel: stringPtr(configuration.ComputeModel),
		CapacityVCores: configuration.CapacityVCores, GeneratedBy: "rule_v1",
	}
}

func costCurrencyOrRequested(currency, requested string) string {
	if currency != "" {
		return currency
	}
	return requested
}

func policyBool(values map[string]any, key string) bool {
	value, ok := values[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func stringValue(value any) string {
	if v, ok := value.(string); ok {
		return v
	}
	return ""
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtr(value string) *string     { return &value }
func floatPtr(value float64) *float64    { return &value }
func timePtr(value time.Time) *time.Time { return &value }

func selectedCandidateIndex(index int, candidates []CandidateConfiguration) *int {
	if index < 0 || index >= len(candidates) {
		return nil
	}
	return &index
}

func hasPercentile(rows []PercentileStatistic, metric string, percentile float64) bool {
	for _, row := range rows {
		if row.MetricCode == metric && row.Percentile == percentile && row.Value != nil {
			return true
		}
	}
	return false
}

func skuName(candidate *CandidateConfiguration) string {
	if candidate == nil {
		return ""
	}
	return candidate.SKUName
}

func candidateAuditResults(results []CandidateEvaluation) []map[string]any {
	out := make([]map[string]any, 0, len(results))
	for _, result := range results {
		out = append(out, map[string]any{
			"sku_name": result.Candidate.SKUName, "selected": result.Selected,
			"discard_reason": result.DiscardReason,
		})
	}
	return out
}

func maxRisk(policy map[string]any) float64 {
	if value, ok := numeric(policy["max_risk_score"]); ok {
		return value
	}
	return 1
}

func minConfidence(policy map[string]any) float64 {
	if value, ok := numeric(policy["min_confidence"]); ok {
		return value
	}
	return 0
}

func numeric(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}
