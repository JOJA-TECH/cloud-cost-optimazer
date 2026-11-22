package recommendation

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func simulateCandidate(candidate CandidateConfiguration, current Configuration, evidence generationEvidence, currentCost, proposedCost float64, input GenerateRecommendationInput) ScenarioSimulation {
	now := time.Now().UTC()
	simulation := ScenarioSimulation{
		SimulatedAt:            now,
		ForecastPeriodStartUTC: timePtr(input.To.UTC()),
		ForecastPeriodEndUTC:   timePtr(input.To.UTC().Add(time.Duration(input.HorizonDays) * 24 * time.Hour)),
		PredictedMonthlyCost:   floatPtr(proposedCost),
		SimulationMethod:       "rule_v1_capacity_scaled",
	}
	if len(evidence.Forecasts) > 0 {
		var selected *Forecast
		for i := range evidence.Forecasts {
			forecast := &evidence.Forecasts[i]
			if forecast.TargetMetric == "cpu_avg" && (selected == nil || forecast.YPred > selected.YPred) {
				selected = forecast
			}
		}
		if selected != nil {
			simulation.ForecastUID = &selected.ForecastUID
			simulation.ModelVersionID = selected.ModelVersionID
			if selected.ModelVersionID != nil {
				version := fmt.Sprintf("model_version_id:%d", *selected.ModelVersionID)
				simulation.ForecastModelVersion = &version
			}
		}
	}

	simulation.SimulationMethod = "rule_v1_current_configuration_baseline"
	cpu95, cpu95OK := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 95)
	cpu99, cpu99OK := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 99)
	if currentCost > 0 {
		simulation.EstimatedSavingsPct = floatPtr((currentCost - proposedCost) / currentCost * 100)
	}
	if candidate.SKUName != current.SKUName || derefString(candidate.ComputeModel) != current.ComputeModel ||
		derefFloat(candidate.CapacityVCores) != derefFloat(current.CapacityVCores) {
		// Release 1.0 requires validated performance-transfer evidence for SKU changes.
		// No such adapter/contract exists yet, so alternative configurations fail closed.
		simulation.Feasible = boolPtr(false)
		simulation.SimulationMethod = "rule_v1_unvalidated_candidate_transfer"
		simulation.ViolatedConstraints = []string{"candidate_transfer_validation_unavailable"}
		return simulation
	}
	violations := make([]string, 0)
	if !cpu95OK || !cpu99OK || proposedCost <= 0 || currentCost <= 0 {
		violations = append(violations, "evidence_insufficient_for_simulation")
	}
	if cpu95OK && cpu95 >= 100 {
		violations = append(violations, "observed_cpu_p95_exceeds_capacity")
	}
	if cpu99OK && cpu99 >= 100 {
		violations = append(violations, "observed_cpu_p99_exceeds_capacity")
	}
	simulation.Feasible = boolPtr(len(violations) == 0)
	simulation.ViolatedConstraints = violations
	return simulation
}

func actionForConfigurations(current, proposed Configuration, costNeutral bool) RecommendationAction {
	if current.SKUName == proposed.SKUName && current.ComputeModel == proposed.ComputeModel &&
		derefFloat(current.CapacityVCores) == derefFloat(proposed.CapacityVCores) {
		return RecommendationAction{Type: ActionNoAction}
	}
	action := ActionResizeDown
	switch {
	case current.ComputeModel != proposed.ComputeModel:
		action = ActionChangeComputeModel
	case derefFloat(proposed.CapacityVCores) > derefFloat(current.CapacityVCores):
		action = ActionResizeUp
	case derefFloat(proposed.CapacityVCores) < derefFloat(current.CapacityVCores):
		action = ActionResizeDown
	default:
		action = ActionNoAction
	}
	verb := map[ActionType]string{
		ActionResizeUp: "Aumentar", ActionResizeDown: "Reducir",
		ActionChangeComputeModel: "Cambiar el modelo de cómputo",
	}[action]
	description := fmt.Sprintf("%s la configuración de %s a %s", verb, current.SKUName, proposed.SKUName)
	if costNeutral {
		description += " (sin ahorro estimado)"
	}
	return RecommendationAction{
		Type: action, Description: description,
		Parameters: map[string]any{
			"current_sku": current.SKUName, "proposed_sku": proposed.SKUName,
			"current_compute_model": current.ComputeModel, "proposed_compute_model": proposed.ComputeModel,
			"current_capacity_vcores": current.CapacityVCores, "proposed_capacity_vcores": proposed.CapacityVCores,
			"automatic_execution": false,
		},
	}
}

func betterCandidate(cost, risk, confidence, bestCost, bestRisk, bestConfidence float64) bool {
	if cost < bestCost-0.005 {
		return true
	}
	if math.Abs(cost-bestCost) <= 0.005 {
		if risk < bestRisk-1e-9 {
			return true
		}
		return math.Abs(risk-bestRisk) <= 1e-9 && confidence > bestConfidence
	}
	return false
}

func candidateDiscardReason(simulation ScenarioSimulation, policy PolicyEvaluation, risk RecommendationRisk, confidence RecommendationConfidence, optimizationPolicy OptimizationPolicy) string {
	if simulation.Feasible == nil || !*simulation.Feasible {
		return "simulation_infeasible"
	}
	if len(policy.ViolatedConstraints) > 0 {
		return strings.Join(policy.ViolatedConstraints, ",")
	}
	if risk.Score > maxRisk(optimizationPolicy.Constraints) {
		return "risk_exceeds_policy_maximum"
	}
	if confidence.Score < minConfidence(optimizationPolicy.Constraints) {
		return "confidence_below_policy_minimum"
	}
	return "policy_rejected"
}

func evaluateInitialOpportunity(candidate CandidateConfiguration, current Configuration, evidence generationEvidence, simulation ScenarioSimulation, saving EstimatedSaving, policy OptimizationPolicy) (bool, string) {
	if candidate.SKUName != current.SKUName || derefString(candidate.ComputeModel) != current.ComputeModel ||
		derefFloat(candidate.CapacityVCores) != derefFloat(current.CapacityVCores) {
		return false, "candidate_transfer_validation_unavailable"
	}
	if saving.MonthlyAmount <= 0 {
		return false, "no_positive_savings"
	}
	return false, "candidate_is_current_configuration"
}

func rationaleFromEvidence(evidence generationEvidence, simulation ScenarioSimulation, saving EstimatedSaving, evaluation PolicyEvaluation) RecommendationRationale {
	rationale := RecommendationRationale{
		Summary: "La configuración propuesta minimiza el costo entre los candidatos factibles que cumplen la política.",
	}
	if v, ok := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 95); ok {
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "cpu_p95", MetricCode: stringPtr("cpu_percent"), Value: floatPtr(v), Unit: stringPtr("percent"),
			WindowStartUTC: timePtr(evidenceWindowStart(evidence)), WindowEndUTC: timePtr(evidenceWindowEnd(evidence)),
			Method: "analytics.percentile_statistic", Quality: floatPtr(evidence.CPUDataCoveragePct / 100),
		})
	}
	if evidence.CPUDataCoverageAvailable {
		context := map[string]any{"expected_buckets": evidence.CPUExpectedBuckets, "received_buckets": evidence.CPUReceivedBuckets}
		if evidence.CPUDataCoverageThresholdPct != nil {
			context["required_threshold_pct"] = *evidence.CPUDataCoverageThresholdPct
		}
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "cpu_p95_hourly_coverage", MetricCode: stringPtr("cpu_percent"),
			Value: floatPtr(evidence.CPUDataCoveragePct), Unit: stringPtr("percent"),
			WindowStartUTC: timePtr(evidence.WindowStartUTC), WindowEndUTC: timePtr(evidence.WindowEndUTC),
			Method: "distinct_hourly_cpu_p95_buckets_with_observations", Quality: floatPtr(evidence.CPUDataCoveragePct / 100),
			Context: context,
		})
	}
	if v, ok := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 99); ok {
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "cpu_p99", MetricCode: stringPtr("cpu_percent"), Value: floatPtr(v), Unit: stringPtr("percent"),
			WindowStartUTC: timePtr(evidenceWindowStart(evidence)), WindowEndUTC: timePtr(evidenceWindowEnd(evidence)),
			Method: "analytics.percentile_statistic", Quality: floatPtr(evidence.CPUDataCoveragePct / 100),
		})
	}
	if evidence.CapacityAnalysis != nil {
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "headroom", Value: floatPtr(evidence.CapacityAnalysis.HeadroomPct), Unit: stringPtr("percent"),
			WindowStartUTC: timePtr(evidence.CapacityAnalysis.PeriodStartUTC), Method: "analytics.capacity_analysis",
		})
	}
	if saving.ProposedMonthlyCost > 0 {
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "projected_monthly_cost", Value: floatPtr(saving.ProposedMonthlyCost), Unit: stringPtr(saving.Currency),
			Method: saving.Method,
		})
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "estimated_savings", Value: floatPtr(saving.MonthlyAmount), Unit: stringPtr(saving.Currency),
			Method: saving.Method,
		})
	}
	if evidence.WorkloadPattern != nil && len(evidence.WorkloadPattern.PeakHours) > 0 {
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "peak_hours", Context: map[string]any{"hours": evidence.WorkloadPattern.PeakHours},
			Method: "analytics.workload_pattern",
		})
	}
	for _, violation := range evaluation.ViolatedConstraints {
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "policy_constraint", Context: map[string]any{"constraint": violation},
			Method: "policy_evaluator",
		})
	}
	var selectedForecast *Forecast
	for i := range evidence.Forecasts {
		forecast := &evidence.Forecasts[i]
		if forecast.TargetMetric == "cpu_avg" && (selectedForecast == nil || forecast.YPred > selectedForecast.YPred) {
			selectedForecast = forecast
		}
	}
	if selectedForecast != nil {
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "forecast_cpu_avg", MetricCode: stringPtr("cpu_avg"),
			Value: floatPtr(selectedForecast.YPred), Unit: stringPtr("percent"),
			WindowStartUTC: timePtr(selectedForecast.PeriodStartUTC), Method: "analytics.forecast.cpu_avg",
			Context: map[string]any{
				"forecast_uid": selectedForecast.ForecastUID,
				"y_lower":      selectedForecast.YLower, "y_upper": selectedForecast.YUpper,
				"used_as_p95_or_p99": false,
			},
		})
	}
	return rationale
}

func recommendationEvidence(evidence generationEvidence, rationale RecommendationRationale) []RecommendationEvidence {
	result := make([]RecommendationEvidence, 0, len(rationale.Evidence))
	for _, row := range rationale.Evidence {
		result = append(result, RecommendationEvidence{
			Type: row.Type, MetricCode: row.MetricCode, Value: row.Value, Unit: row.Unit,
			WindowStartUTC: row.WindowStartUTC, WindowEndUTC: row.WindowEndUTC,
			Context: func() map[string]any {
				context := map[string]any{"source": row.Method, "quality": row.Quality}
				for key, value := range row.Context {
					context[key] = value
				}
				return context
			}(),
		})
	}
	for _, partialError := range evidence.PartialErrors {
		result = append(result, RecommendationEvidence{
			Type: "partial_error", Context: map[string]any{"error": partialError},
		})
	}
	return result
}

func evidencePercentile(rows []PercentileStatistic, metric string, percentile float64) (float64, bool) {
	var value float64
	found := false
	var latest time.Time
	for _, row := range rows {
		if row.MetricCode != metric || row.Percentile != percentile || row.Value == nil {
			continue
		}
		if !found || row.PeriodStartUTC.After(latest) {
			value, latest, found = *row.Value, row.PeriodStartUTC, true
		}
	}
	return value, found
}

func evidenceWindowStart(evidence generationEvidence) time.Time {
	if len(evidence.FeatureSnapshots) == 0 {
		return time.Time{}
	}
	start := evidence.FeatureSnapshots[0].PeriodStartUTC
	for _, row := range evidence.FeatureSnapshots[1:] {
		if row.PeriodStartUTC.Before(start) {
			start = row.PeriodStartUTC
		}
	}
	return start
}

func evidenceWindowEnd(evidence generationEvidence) time.Time {
	if len(evidence.FeatureSnapshots) == 0 {
		return time.Time{}
	}
	end := evidence.FeatureSnapshots[0].PeriodEndUTC
	for _, row := range evidence.FeatureSnapshots[1:] {
		if row.PeriodEndUTC.After(end) {
			end = row.PeriodEndUTC
		}
	}
	return end
}

func riskAssessmentFromRisk(risk RecommendationRisk) RiskAssessment {
	return RiskAssessment{
		ModelVersion: risk.ModelVersion, PeakUsage: floatPtr(risk.PeakUsage),
		Variability: floatPtr(risk.Variability), Trend: floatPtr(risk.Trend),
		Headroom: floatPtr(risk.Headroom), Score: risk.Score, Level: string(risk.Level),
		Factors: risk.Factors, AssessedAt: time.Now().UTC(),
	}
}

func boolPtr(value bool) *bool { return &value }

func derefFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}
