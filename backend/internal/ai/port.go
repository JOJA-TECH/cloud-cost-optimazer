package ai

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MLPort is the contract to the external Python ML service (Module 6).
// It never makes final business recommendations (that is Module 7).
type MLPort interface {
	// Forecast requests a prediction from the ML service. ML service writes
	// ml.forecast and returns forecast_uid.
	Forecast(ctx context.Context, req ForecastRequest) (uuid.UUID, error)

	// ValidateContract checks the gRPC/HTTP contract version (ml.inference.v1).
	ValidateContract(ctx context.Context) error
}

// LLMPort is the contract to a generative-LLM explanation service.
// The LLM only narrates evidence already calculated by the platform; it never
// generates data, prices, configurations or metrics, and never decides capacity.
type LLMPort interface {
	// ExplainRecommendation turns a structured evidence pack into a natural
	// language explanation. Returns a deterministic explanation when no LLM
	// provider is configured.
	ExplainRecommendation(ctx context.Context, pack EvidencePack) (string, error)
}

// ForecastRequest is sent to MLPort to trigger forecasting.
type ForecastRequest struct {
	ResourceUID    uuid.UUID `json:"resource_uid"`
	TargetMetric   string    `json:"target_metric"`              // 'cpu_avg' | 'workload_intensity' | 'storage_used_bytes'
	HorizonSteps   int       `json:"horizon_steps"`              // 1..H
	StepUnit       string    `json:"step_unit"`                  // '15m' | '1h' | '1d'
	PeriodStartUTC time.Time `json:"period_start_utc"`           // first future period to forecast
	ModelVersionID *int64    `json:"model_version_id,omitempty"` // specific model_version_id, or nil for champion
	// Correlation/tracing ids propagated to the ML service, Jobs and audit logs.
	RequestID     string `json:"request_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// ForecastResponse mirrors ml.forecast (read via contract.v_forecast).
type ForecastResponse struct {
	ForecastUID    uuid.UUID
	ModelVersionID *int64
	ResourceUID    uuid.UUID
	TargetMetric   string
	HorizonSteps   int
	StepUnit       string
	PeriodStartUTC time.Time
	YPred          float64
	YLower         *float64
	YUpper         *float64
	Alpha          float64
	GeneratedAt    time.Time
}

// EvidencePack is the structured input to LLMPort.ExplainRecommendation.
// Every field must come from data already calculated or retrieved by the
// platform; the LLM must not invent values.
type EvidencePack struct {
	RecommendationUID uuid.UUID `json:"recommendation_uid"`
	ResourceUID       uuid.UUID `json:"resource_uid"`
	CurrentConfig     string    `json:"current_config"`
	CandidateConfig   string    `json:"candidate_config"`
	MetricsUsed       string    `json:"metrics_used"`
	Forecast          string    `json:"forecast"`
	EstimatedCost     string    `json:"estimated_cost"`
	Counterfactual    string    `json:"counterfactual_savings"`
	RiskLevel         string    `json:"risk_level"`
	Constraints       string    `json:"constraints"`
	Assumptions       string    `json:"assumptions"`
	Limitations       string    `json:"limitations"`
	RequestID         string    `json:"request_id,omitempty"`
}
