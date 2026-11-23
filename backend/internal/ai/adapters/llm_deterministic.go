package adapters

import (
	"context"
	"fmt"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/ai"
)

// DeterministicLLM implements ai.LLMPort using static templates.
// It never generates data, prices, configurations or metrics; it only narrates
// evidence already calculated by the platform. Always available (no external
// provider required), so the platform keeps working when no LLM is configured.
//
// shortcut: static templates cannot adapt wording to user locale or preference;
// ceiling: add ai/adapters/llm_openai.go behind CCO_LLM_PROVIDER=openai when a
// real LLM provider is required.
type DeterministicLLM struct{}

func NewDeterministicLLM() *DeterministicLLM {
	return &DeterministicLLM{}
}

func (d *DeterministicLLM) ExplainRecommendation(ctx context.Context, pack ai.EvidencePack) (string, error) {
	return fmt.Sprintf(
		"Recomendación para el recurso %s: configuración actual %s vs candidata %s. "+
			"Métricas utilizadas: %s. Pronóstico: %s. Costo estimado: %s. "+
			"Ahorro contrafactual: %s. Nivel de riesgo: %s. "+
			"Restricciones aplicadas: %s. Supuestos: %s. Limitaciones: %s.",
		pack.ResourceUID, pack.CurrentConfig, pack.CandidateConfig,
		pack.MetricsUsed, pack.Forecast, pack.EstimatedCost,
		pack.Counterfactual, pack.RiskLevel, pack.Constraints,
		pack.Assumptions, pack.Limitations,
	), nil
}
