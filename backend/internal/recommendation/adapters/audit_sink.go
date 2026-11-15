package adapters

import (
	"context"
	"log/slog"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
)

type SlogAuditSink struct {
	logger *slog.Logger
}

func NewSlogAuditSink(logger *slog.Logger) *SlogAuditSink {
	if logger == nil {
		logger = slog.Default()
	}
	return &SlogAuditSink{logger: logger}
}

func (s *SlogAuditSink) RecordDecision(ctx context.Context, event recommendation.DecisionAuditEvent) error {
	s.logger.InfoContext(ctx, "recommendation decision",
		"request_id", event.RequestID,
		"correlation_id", event.CorrelationID,
		"resource_uid", event.ResourceUID,
		"actor", event.Actor,
		"policy_uid", event.PolicyUID,
		"policy_version", event.PolicyVersion,
		"algorithm_version", event.AlgorithmVersion,
		"scenario_uid", event.ScenarioUID,
		"recommendation_uid", event.RecommendationUID,
		"decision", event.Decision,
		"selected_candidate", event.SelectedCandidate,
		"candidate_results", event.CandidateResults,
		"duration", event.Duration,
		"errors", event.Errors,
	)
	return nil
}

var _ recommendation.AuditSink = (*SlogAuditSink)(nil)
