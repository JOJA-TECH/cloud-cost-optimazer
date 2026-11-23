package adapters

import (
	"context"
	"strings"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/ai"
	"github.com/google/uuid"
)

func TestDeterministicLLM_ExplainRecommendation(t *testing.T) {
	ctx := context.Background()
	llm := NewDeterministicLLM()

	resourceUID := uuid.New()
	pack := ai.EvidencePack{
		RecommendationUID: uuid.New(),
		ResourceUID:       resourceUID,
		CurrentConfig:     "4 vCores, 100 GB",
		CandidateConfig:   "2 vCores, 50 GB",
		MetricsUsed:       "CPU p95 72%, storage p90 45 GB",
		Forecast:          "CPU p95 next hour 68%",
		EstimatedCost:     "$120/month current -> $70/month candidate",
		Counterfactual:    "$50/month savings (42%)",
		RiskLevel:         "low",
		Constraints:       "peak CPU must stay below 80%",
		Assumptions:       "workload pattern stable over next 30 days",
		Limitations:       "based on last 30 days of observed telemetry",
	}

	explanation, err := llm.ExplainRecommendation(ctx, pack)
	if err != nil {
		t.Fatalf("ExplainRecommendation() returned error: %v", err)
	}
	if explanation == "" {
		t.Fatal("ExplainRecommendation() returned empty string")
	}

	// The deterministic explanation must reference the key evidence fields,
	// proving it narrates the pack and does not invent values.
	for _, field := range []string{
		resourceUID.String(),
		pack.CurrentConfig,
		pack.CandidateConfig,
		pack.Forecast,
		pack.RiskLevel,
		pack.Counterfactual,
	} {
		if !strings.Contains(explanation, field) {
			t.Errorf("ExplainRecommendation() missing expected field %q in %q", field, explanation)
		}
	}
}

func TestDeterministicLLM_AlwaysAvailable(t *testing.T) {
	ctx := context.Background()
	llm := NewDeterministicLLM()

	// Must work with a minimal pack (empty fields) — no external provider needed.
	_, err := llm.ExplainRecommendation(ctx, ai.EvidencePack{})
	if err != nil {
		t.Fatalf("ExplainRecommendation() with empty pack failed: %v", err)
	}
}
