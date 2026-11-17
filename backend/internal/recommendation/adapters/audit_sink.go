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
	selectedCandidate := ""
	if event.SelectedCandidate != nil {
		selectedCandidate = *event.SelectedCandidate
	}
	s.logger.InfoContext(ctx, "recommendation decision",
		"request_id", event.RequestID,
		"correlation_id", event.CorrelationID,
		"operation_id", event.OperationID,
		"resource_uid", event.ResourceUID,
		"actor", event.Actor,
		"policy_uid", event.PolicyUID,
		"policy_version", event.PolicyVersion,
		"algorithm_version", event.AlgorithmVersion,
		"scenario_uid", event.ScenarioUID,
		"recommendation_uid", event.RecommendationUID,
		"decision", event.Decision,
		"selected_candidate", selectedCandidate,
		"risk_score", event.RiskScore,
		"risk_level", event.RiskLevel,
		"confidence_score", event.ConfidenceScore,
		"estimated_saving", event.EstimatedSaving,
		"evidence_count", event.EvidenceCount,
		"candidate_results", event.CandidateResults,
		"details", event.Details,
		"duration", event.Duration,
		"errors", event.Errors,
	)
	return nil
}

var _ recommendation.AuditSink = (*SlogAuditSink)(nil)
