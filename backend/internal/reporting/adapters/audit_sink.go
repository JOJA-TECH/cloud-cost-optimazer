package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/audit"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

type AuditSink struct {
	logger audit.AuditLogger
}

func NewAuditSink(logger audit.AuditLogger) *AuditSink {
	return &AuditSink{logger: logger}
}

var _ reporting.ReportAuditSink = (*AuditSink)(nil)

func (s *AuditSink) RecordReportGenerated(
	ctx context.Context,
	job reporting.ReportGenerationJob,
	report reporting.Report,
) error {
	if s == nil || s.logger == nil {
		return errors.New("report audit logger is unavailable")
	}
	if report.UID == uuid.Nil || job.UID == uuid.Nil {
		return reporting.ErrInvalidReportRequest
	}
	metadata := map[string]any{
		"report_request_id": job.UID.String(),
		"report_type":       report.ReportType,
		"format":            report.Format,
		"resource_uid":      report.ResourceUID,
		"period_start":      report.Period.Start,
		"period_end":        report.Period.End,
	}
	if report.Artifact != nil {
		metadata["report_artifact_id"] = report.Artifact.UID.String()
	}
	occurredAt := report.CreatedAt
	if report.CompletedAt != nil {
		occurredAt = *report.CompletedAt
	}
	reportUID := report.UID
	if err := s.logger.Record(ctx, audit.AuditEvent{
		Action:     "report_generated",
		Resource:   audit.ResourceReference{Type: "report", ID: &reportUID},
		OccurredAt: occurredAt,
		Metadata:   metadata,
	}); err != nil {
		return fmt.Errorf("record report generated audit event: %w", err)
	}
	return nil
}
