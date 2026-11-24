package adapters

import (
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

func TestRecommendationViewPreservesDecisionAndSourceValues(t *testing.T) {
	recommendationUID, resourceUID := uuid.New(), uuid.New()
	quality := 0.9
	value := recommendation.Recommendation{
		UID:         recommendationUID,
		ResourceUID: resourceUID,
		WindowStart: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		Action: recommendation.RecommendationAction{
			Type:        recommendation.ActionResizeDown,
			Description: "resize to a smaller tier",
		},
		EstimatedSaving: recommendation.EstimatedSaving{
			CurrentMonthlyCost:  100,
			ProposedMonthlyCost: 75,
			MonthlyAmount:       25,
			Currency:            "USD",
			CurrentCostBasis:    recommendation.CostBasisActualCost,
			ProposedCostBasis:   recommendation.CostBasisSimulation,
		},
		Risk: recommendation.RecommendationRisk{
			Score: 0.4, Level: recommendation.RiskLevelMedium, Variability: 0.2,
		},
		Confidence: recommendation.RecommendationConfidence{Score: 0.85},
		Rationale:  recommendation.RecommendationRationale{Summary: "source rationale"},
		Status:     recommendation.RecommendationStatus("accepted"),
		Evidence: []recommendation.RecommendationEvidence{{
			Type: "cpu_p95", Context: map[string]any{"quality": &quality},
		}},
	}

	got := recommendationToReportData(value)
	if got.RecommendationUID != recommendationUID || got.ResourceUID != resourceUID ||
		got.Action != string(recommendation.ActionResizeDown) ||
		got.Status != string(recommendation.RecommendationStatus("accepted")) ||
		got.Rationale != "source rationale" || got.EstimatedSaving.MonthlyAmount != 25 ||
		got.EstimatedSaving.CurrentCostBasis != string(recommendation.CostBasisActualCost) ||
		got.Risk == nil || got.Risk.Variability != 0.2 || got.Confidence.Score != 0.85 {
		t.Fatalf("recommendation report view changed source data: %#v", got)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].Source != reporting.ReportSourceRecommendation ||
		got.Evidence[0].Quality == nil || *got.Evidence[0].Quality != quality {
		t.Fatalf("recommendation evidence did not preserve source and quality: %#v", got.Evidence)
	}
}
