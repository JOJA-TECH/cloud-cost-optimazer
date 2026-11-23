package ai

import (
	"context"

	"github.com/google/uuid"
)

// AIService is the single entry point for ML/LLM/IA capabilities.
// It combines MLPort (forecasting) and LLMPort (explanation) and can enqueue
// background jobs through the existing jobs.Service (no new worker is created).
type AIService struct {
	ml      MLPort
	llm     LLMPort
	jobs    JobEnqueuer
}

// JobEnqueuer is the subset of jobs.Service AIService needs to enqueue work.
type JobEnqueuer interface {
	Enqueue(ctx context.Context, in EnqueueInput) (*Job, error)
}

// EnqueueInput mirrors jobs.EnqueueInput so AIService does not import jobs.
type EnqueueInput struct {
	Type           string
	Payload        []byte
	OrganizationID *uuid.UUID
	ProjectID      *uuid.UUID
	IdempotencyKey *string
	MaxAttempts    int
}

// Job is a minimal view of a persisted job returned by Enqueue.
type Job struct {
	ID        uuid.UUID
	Type      string
	Status    string
	CreatedAt string
}

// NewAIService builds the AI service.
func NewAIService(ml MLPort, llm LLMPort, jobs JobEnqueuer) *AIService {
	return &AIService{ml: ml, llm: llm, jobs: jobs}
}

// Forecast delegates to MLPort.Forecast.
func (s *AIService) Forecast(ctx context.Context, req ForecastRequest) (uuid.UUID, error) {
	if s.ml == nil {
		return uuid.Nil, NewError(CodeMLUnavailable, "ML service not configured", ErrMLUnavailable)
	}
	return s.ml.Forecast(ctx, req)
}

// ValidateContract delegates to MLPort.ValidateContract.
func (s *AIService) ValidateContract(ctx context.Context) error {
	if s.ml == nil {
		return NewError(CodeMLUnavailable, "ML service not configured", ErrMLUnavailable)
	}
	return s.ml.ValidateContract(ctx)
}

// ExplainRecommendation delegates to LLMPort.ExplainRecommendation.
func (s *AIService) ExplainRecommendation(ctx context.Context, pack EvidencePack) (string, error) {
	if s.llm == nil {
		return "", NewError(CodeLLMUnavailable, "LLM service not configured", ErrLLMUnavailable)
	}
	return s.llm.ExplainRecommendation(ctx, pack)
}

var _ MLPort = (*AIService)(nil)
var _ LLMPort = (*AIService)(nil)
