package recommendation

import (
	"math"
	"time"
)

func savingEstimate(current, proposed float64, currency string, currentBasis, proposedBasis CostBasis, from, to time.Time, method string) EstimatedSaving {
	amount := current - proposed
	percentage := 0.0
	if current > 0 {
		percentage = amount / current
	}
	return EstimatedSaving{
		CurrentMonthlyCost: current, ProposedMonthlyCost: proposed,
		MonthlyAmount: amount, MonthlyPercentage: percentage,
		CurrentCostBasis: currentBasis, ProposedCostBasis: proposedBasis,
		Currency: currency, PeriodStart: from, PeriodEnd: to, Method: method,
	}
}

func calculateRisk(evidence generationEvidence, simulation ScenarioSimulation, candidate CandidateConfiguration, current Configuration) RecommendationRisk {
	peak := 0.0
	if evidence.CapacityAnalysis != nil {
		peak = clamp(evidence.CapacityAnalysis.PeakUtilization/100, 0, 1)
	}
	if p99, ok := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 99); ok {
		peak = math.Max(peak, clamp(p99/100, 0, 1))
	}
	variability := 0.0
	if p50, ok50 := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 50); ok50 {
		if p99, ok99 := evidencePercentile(evidence.PercentileStatistics, "cpu_percent", 99); ok99 {
			variability = clamp((p99-p50)/100, 0, 1)
		}
	}
	trend := 0.0
	for _, row := range evidence.Trends {
		if row.MetricCode == "cpu_percent" {
			trend = math.Max(trend, clamp(row.Slope/10, 0, 1))
		}
	}
	headroom := 0.0
	if evidence.CapacityAnalysis != nil {
		headroom = clamp(1-evidence.CapacityAnalysis.HeadroomPct/100, 0, 1)
	}
	forecastUncertainty := 0.5
	if len(evidence.Forecasts) > 0 {
		total := 0.0
		count := 0
		for _, forecast := range evidence.Forecasts {
			if forecast.YLower != nil && forecast.YUpper != nil {
				width := math.Abs(*forecast.YUpper - *forecast.YLower)
				scale := math.Max(math.Abs(forecast.YPred), 1)
				total += clamp(width/scale, 0, 1)
				count++
			}
		}
		if count > 0 {
			forecastUncertainty = total / float64(count)
		} else {
			forecastUncertainty = 0.5
		}
	}
	// A larger downgrade adds risk; upgrades and model changes do not.
	distance := 0.0
	currentCapacity := derefFloat(current.CapacityVCores)
	candidateCapacity := derefFloat(candidate.CapacityVCores)
	if currentCapacity > 0 && candidateCapacity > 0 && candidateCapacity < currentCapacity {
		distance = clamp((currentCapacity-candidateCapacity)/currentCapacity, 0, 1)
	}
	score := clamp(
		0.27*peak+0.18*variability+0.18*trend+0.20*headroom+
			0.10*forecastUncertainty+0.07*distance,
		0, 1,
	)
	level := RiskLevelLow
	switch {
	case score >= 0.8:
		level = RiskLevelCritical
	case score >= 0.6:
		level = RiskLevelHigh
	case score >= 0.35:
		level = RiskLevelMedium
	}
	return RecommendationRisk{
		Score: score, Level: level, PeakUsage: peak, Variability: variability,
		Trend: trend, Headroom: headroom, ModelVersion: "rule_v1",
		Factors: map[string]any{
			"peak_usage_risk": peak, "variability_risk": variability,
			"trend_risk": trend, "headroom_risk": headroom,
			"forecast_uncertainty_risk":   forecastUncertainty,
			"configuration_distance_risk": distance,
			"weights": map[string]float64{
				"peak_usage": 0.27, "variability": 0.18, "trend": 0.18,
				"headroom": 0.20, "forecast_uncertainty": 0.10, "configuration_distance": 0.07,
			},
		},
	}
}

func calculateConfidence(evidence generationEvidence, simulation ScenarioSimulation, policyMatch, dataComplete bool) RecommendationConfidence {
	dataQuality := 0.0
	if len(evidence.FeatureSnapshots) > 0 {
		dataQuality += 0.4
	}
	if len(evidence.Costs) > 0 || evidence.CostAnalysis != nil {
		dataQuality += 0.3
	}
	if evidence.CapacityAnalysis != nil {
		dataQuality += 0.3
	}
	if evidence.Degraded {
		dataQuality *= 0.75
	}

	requiredEvidence := 8.0
	present := 0.0
	if len(evidence.FeatureSnapshots) > 0 {
		present++
	}
	if hasPercentile(evidence.PercentileStatistics, "cpu_percent", 95) {
		present++
	}
	if hasPercentile(evidence.PercentileStatistics, "cpu_percent", 99) {
		present++
	}
	if hasPercentile(evidence.PercentileStatistics, "data_io_percent", 95) {
		present++
	}
	if hasPercentile(evidence.PercentileStatistics, "sessions_percent", 95) {
		present++
	}
	if len(evidence.Trends) > 0 {
		present++
	}
	if evidence.WorkloadPattern != nil {
		present++
	}
	if evidence.CapacityAnalysis != nil && evidence.CostAnalysis != nil && evidence.DESComponents != nil {
		present++
	}
	completeness := clamp(present/requiredEvidence, 0, 1)

	forecastQuality := 0.0
	if len(evidence.Forecasts) > 0 {
		forecastQuality = 0.6
		for _, forecast := range evidence.Forecasts {
			if forecast.YLower != nil && forecast.YUpper != nil {
				forecastQuality = 1
				break
			}
		}
	}
	feasibility := 0.0
	if simulation.Feasible != nil && *simulation.Feasible {
		feasibility = 1
	}
	policyScore := 0.0
	if policyMatch {
		policyScore = 1
	}
	if !dataComplete {
		dataQuality = math.Min(dataQuality, 0.4)
	}
	score := 0.30*dataQuality + 0.25*completeness + 0.20*forecastQuality +
		0.15*feasibility + 0.10*policyScore
	return RecommendationConfidence{
		Score:            clamp(score, 0, 1),
		DataQualityScore: dataQuality, EvidenceCompletenessScore: completeness,
		ForecastQualityScore: forecastQuality, SimulationFeasibilityScore: feasibility,
		PolicyMatchScore: policyScore, Method: RecommendationConfidenceMethodWeightedV1,
	}
}

func clamp(value, lower, upper float64) float64 {
	return math.Max(lower, math.Min(upper, value))
}
