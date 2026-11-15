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

	currentVcores := derefFloat(current.CapacityVCores)
	candidateVcores := derefFloat(candidate.CapacityVCores)
	cpu95, cpu95OK := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 95)
	cpu99, cpu99OK := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 99)
	if !cpu95OK {
		cpu95 = latestSnapshotCPU(evidence.FeatureSnapshots, true)
		cpu95OK = len(evidence.FeatureSnapshots) > 0
	}
	if !cpu99OK {
		cpu99 = latestSnapshotCPU(evidence.FeatureSnapshots, false)
		cpu99OK = len(evidence.FeatureSnapshots) > 0
	}
	if len(evidence.Forecasts) > 0 {
		forecastPeak := 0.0
		for _, forecast := range evidence.Forecasts {
			if forecast.TargetMetric != "cpu_avg" {
				continue
			}
			value := forecast.YPred
			if forecast.YUpper != nil && *forecast.YUpper > value {
				value = *forecast.YUpper
			}
			forecastPeak = math.Max(forecastPeak, value)
		}
		if forecastPeak > 0 {
			cpu95 = math.Max(cpu95, forecastPeak)
			cpu99 = math.Max(cpu99, forecastPeak)
			cpu95OK, cpu99OK = true, true
			simulation.SimulationMethod = "rule_v1_capacity_scaled_with_cpu_avg_forecast"
		}
	}
	if currentVcores > 0 && candidateVcores > 0 && cpu95OK && cpu99OK {
		ratio := currentVcores / candidateVcores
		simulation.PredictedCPUP95 = floatPtr(cpu95 * ratio)
		simulation.PredictedCPUP99 = floatPtr(cpu99 * ratio)
	}
	if currentCost > 0 {
		simulation.EstimatedSavingsPct = floatPtr((currentCost - proposedCost) / currentCost * 100)
	}
	if simulation.PredictedCPUP95 == nil || simulation.PredictedCPUP99 == nil ||
		proposedCost <= 0 || currentCost <= 0 {
		simulation.Feasible = boolPtr(false)
		simulation.ViolatedConstraints = []string{"evidence_insufficient_for_simulation"}
		return simulation
	}

	violations := make([]string, 0)
	if *simulation.PredictedCPUP95 >= 100 {
		violations = append(violations, "predicted_cpu_p95_exceeds_capacity")
	}
	if *simulation.PredictedCPUP99 >= 100 {
		violations = append(violations, "predicted_cpu_p99_exceeds_capacity")
	}
	// Latency/SLA claims require a corresponding forecast or measured contract;
	// do not synthesize them from CPU utilization.
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
	if saving.MonthlyAmount <= 0 {
		return false, "no_positive_savings"
	}
	if candidate.ComputeModel != nil && derefString(candidate.ComputeModel) != current.ComputeModel {
		if !policyBool(policy.Constraints, "allow_compute_model_change") {
			return false, "compute_model_change_not_allowed"
		}
		if strings.EqualFold(derefString(candidate.ComputeModel), "serverless") {
			averageCPU := averageSnapshotCPU(evidence.FeatureSnapshots)
			if averageCPU >= 20 || evidence.WorkloadPattern == nil ||
				evidence.WorkloadPattern.IntensityBand != "low" ||
				evidence.WorkloadPattern.PeakFrequency > 2 {
				return false, "serverless_workload_not_low_or_intermitent"
			}
		}
		return true, ""
	}
	currentCapacity := derefFloat(current.CapacityVCores)
	candidateCapacity := derefFloat(candidate.CapacityVCores)
	if candidateCapacity < currentCapacity {
		cpu95, ok95 := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 95)
		cpu99, ok99 := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 99)
		if !ok95 || !ok99 || evidence.CapacityAnalysis == nil {
			return false, "scale_down_evidence_insufficient"
		}
		if cpu95 >= 40 || cpu99 >= 60 || evidence.CapacityAnalysis.HeadroomPct <= 40 {
			return false, "scale_down_utilization_or_headroom_threshold"
		}
		increasing := false
		trendFound := false
		for _, trend := range evidence.Trends {
			if trend.MetricCode == "cpu_percent" {
				trendFound = true
				if trend.Slope > 0.1 {
					increasing = true
				}
			}
		}
		if !trendFound || increasing {
			return false, "scale_down_trend_unavailable_or_increasing"
		}
		if simulation.PredictedCPUP95 == nil || simulation.PredictedCPUP99 == nil ||
			*simulation.PredictedCPUP95 >= 80 || *simulation.PredictedCPUP99 >= 90 {
			return false, "scale_down_forecast_exceeds_utilization_threshold"
		}
		return true, ""
	}
	if candidateCapacity > currentCapacity {
		cpu95, ok95 := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 95)
		cpu99, ok99 := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 99)
		needsScaleUp := (ok95 && cpu95 > 80) || (ok99 && cpu99 > 90)
		if evidence.CapacityAnalysis != nil && evidence.CapacityAnalysis.HeadroomPct < 15 {
			needsScaleUp = true
		}
		for _, forecast := range evidence.Forecasts {
			if forecast.TargetMetric == "cpu_avg" && forecast.YPred > 80 {
				needsScaleUp = true
			}
		}
		if !needsScaleUp {
			return false, "scale_up_threshold_not_met"
		}
		return true, ""
	}
	return false, "candidate_is_current_configuration"
}

func averageSnapshotCPU(snapshots []FeatureSnapshot) float64 {
	if len(snapshots) == 0 {
		return math.Inf(1)
	}
	total := 0.0
	for _, snapshot := range snapshots {
		total += snapshot.CPUAvg
	}
	return total / float64(len(snapshots))
}

func rationaleFromEvidence(evidence generationEvidence, simulation ScenarioSimulation, saving EstimatedSaving, evaluation PolicyEvaluation) RecommendationRationale {
	rationale := RecommendationRationale{
		Summary: "La configuración propuesta minimiza el costo entre los candidatos factibles que cumplen la política.",
	}
	if v, ok := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 95); ok {
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "cpu_p95", MetricCode: stringPtr("cpu_percent"), Value: floatPtr(v), Unit: stringPtr("percent"),
			WindowStartUTC: timePtr(evidenceWindowStart(evidence)), WindowEndUTC: timePtr(evidenceWindowEnd(evidence)),
			Method: "analytics.percentile_statistic", Quality: floatPtr(1),
		})
	}
	if v, ok := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 99); ok {
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "cpu_p99", MetricCode: stringPtr("cpu_percent"), Value: floatPtr(v), Unit: stringPtr("percent"),
			WindowStartUTC: timePtr(evidenceWindowStart(evidence)), WindowEndUTC: timePtr(evidenceWindowEnd(evidence)),
			Method: "analytics.percentile_statistic", Quality: floatPtr(1),
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
	if simulation.ForecastUID != nil {
		rationale.Evidence = append(rationale.Evidence, Evidence{
			Type: "forecast", Context: map[string]any{"forecast_uid": simulation.ForecastUID},
			Method: simulation.SimulationMethod,
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
			Context: map[string]any{"source": row.Method, "quality": row.Quality},
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

func latestSnapshotCPU(rows []FeatureSnapshot, p95 bool) float64 {
	if len(rows) == 0 {
		return 0
	}
	latest := rows[0]
	for _, row := range rows[1:] {
		if row.PeriodStartUTC.After(latest.PeriodStartUTC) {
			latest = row
		}
	}
	if p95 && latest.CPUP95 != nil {
		return *latest.CPUP95
	}
	if !p95 && latest.CPUP99 != nil {
		return *latest.CPUP99
	}
	return latest.CPUAvg
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
