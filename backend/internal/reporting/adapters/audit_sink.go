package adapters

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func (s *AuditSink) RecordReportGenerationStarted(
	ctx context.Context,
	job reporting.ReportGenerationJob,
	input reporting.OptimizationReportInput,
) error {
	if s == nil || s.logger == nil {
		return errors.New("report audit logger is unavailable")
	}
	if job.UID == uuid.Nil {
		return reporting.ErrInvalidReportRequest
	}
	trace, _ := audit.CorrelationContextFromContext(ctx)
	event := audit.AuditEvent{
		Action:     "report_generation_started",
		Resource:   audit.ResourceReference{Type: "report", ID: &job.UID},
		OccurredAt: job.CreatedAt.UTC(),
		Trace:      trace,
		Metadata:   buildStartedMetadata(job, input),
	}
	if err := s.logger.Record(ctx, event); err != nil {
		return fmt.Errorf("record report generation started audit event: %w", err)
	}
	return nil
}

func (s *AuditSink) RecordReportGenerationCompleted(
	ctx context.Context,
	job reporting.ReportGenerationJob,
	report reporting.Report,
) error {
	if s == nil || s.logger == nil {
		return errors.New("report audit logger is unavailable")
	}
	if job.UID == uuid.Nil || report.UID == uuid.Nil {
		return reporting.ErrInvalidReportRequest
	}
	trace, _ := audit.CorrelationContextFromContext(ctx)
	occurredAt := job.CreatedAt.UTC()
	if report.CompletedAt != nil {
		occurredAt = report.CompletedAt.UTC()
	}
	event := audit.AuditEvent{
		Action:     "report_generation_completed",
		Resource:   audit.ResourceReference{Type: "report", ID: &report.UID},
		OccurredAt: occurredAt,
		Trace:      trace,
		Metadata:   buildCompletedMetadata(job, report),
	}
	if err := s.logger.Record(ctx, event); err != nil {
		return fmt.Errorf("record report generation completed audit event: %w", err)
	}
	return nil
}

func (s *AuditSink) RecordReportGenerationFailed(
	ctx context.Context,
	job reporting.ReportGenerationJob,
	reportUID uuid.UUID,
	err error,
) error {
	if s == nil || s.logger == nil {
		return errors.New("report audit logger is unavailable")
	}
	if job.UID == uuid.Nil {
		return reporting.ErrInvalidReportRequest
	}
	trace, _ := audit.CorrelationContextFromContext(ctx)
	event := audit.AuditEvent{
		Action:     "report_generation_failed",
		Resource:   audit.ResourceReference{Type: "report", ID: &reportUID},
		OccurredAt: time.Now().UTC(),
		Trace:      trace,
		Metadata:   buildFailedMetadata(job, reportUID, err),
	}
	if err := s.logger.Record(ctx, event); err != nil {
		return fmt.Errorf("record report generation failed audit event: %w", err)
	}
	return nil
}

func (s *AuditSink) RecordReportExported(
	ctx context.Context,
	job reporting.ReportGenerationJob,
	report reporting.Report,
	artifact reporting.ReportArtifact,
) error {
	if s == nil || s.logger == nil {
		return errors.New("report audit logger is unavailable")
	}
	if job.UID == uuid.Nil || artifact.UID == uuid.Nil {
		return reporting.ErrInvalidReportRequest
	}
	trace, _ := audit.CorrelationContextFromContext(ctx)
	event := audit.AuditEvent{
		Action:     "report_exported",
		Resource:   audit.ResourceReference{Type: "report_artifact", ID: &artifact.UID},
		OccurredAt: artifact.CreatedAt.UTC(),
		Trace:      trace,
		Metadata:   buildExportedMetadata(job, report, artifact),
	}
	if err := s.logger.Record(ctx, event); err != nil {
		return fmt.Errorf("record report exported audit event: %w", err)
	}
	return nil
}

func (s *AuditSink) RecordReportDownloaded(
	ctx context.Context,
	job reporting.ReportGenerationJob,
	artifact reporting.ReportArtifact,
) error {
	if s == nil || s.logger == nil {
		return errors.New("report audit logger is unavailable")
	}
	if job.UID == uuid.Nil || artifact.UID == uuid.Nil {
		return reporting.ErrInvalidReportRequest
	}
	trace, _ := audit.CorrelationContextFromContext(ctx)
	event := audit.AuditEvent{
		Action:     "report_downloaded",
		Resource:   audit.ResourceReference{Type: "report_artifact", ID: &artifact.UID},
		OccurredAt: time.Now().UTC(),
		Trace:      trace,
		Metadata:   buildDownloadedMetadata(job, artifact),
	}
	if err := s.logger.Record(ctx, event); err != nil {
		return fmt.Errorf("record report downloaded audit event: %w", err)
	}
	return nil
}

// countReportRecommendations cuenta todas las recomendaciones en todas las secciones.
func countReportRecommendations(report reporting.Report) int {
	total := 0
	for _, section := range report.Sections {
		total += len(section.Recommendations)
	}
	return total
}

// buildStartedMetadata construye la metadata mínima para report_generation_started.
func buildStartedMetadata(job reporting.ReportGenerationJob, input reporting.OptimizationReportInput) map[string]any {
	metadata := map[string]any{
		"report_uid":        job.ReportUID,
		"report_request_id": job.UID,
		"report_type":       job.ReportType,
		"format":            input.Format,
		"project_id":        job.ProjectID,
		"from":              input.From.UTC().Format(time.RFC3339),
		"to":                input.To.UTC().Format(time.RFC3339),
		"recommendation_count": 0,
		"section_count":     0,
		"status":            job.Status,
	}
	if input.ResourceUID != uuid.Nil {
		metadata["resource_uid"] = input.ResourceUID
	}
	if job.ReportUID != nil {
		metadata["artifact_uid"] = job.ReportUID
	}
	return metadata
}

// buildCompletedMetadata construye la metadata mínima para report_generation_completed.
func buildCompletedMetadata(job reporting.ReportGenerationJob, report reporting.Report) map[string]any {
	metadata := map[string]any{
		"report_uid":         report.UID,
		"report_request_id":  job.UID,
		"report_type":        report.ReportType,
		"format":             report.Format,
		"project_id":         report.ProjectID,
		"from":               report.Period.Start.UTC().Format(time.RFC3339),
		"to":                 report.Period.End.UTC().Format(time.RFC3339),
		"recommendation_count": countReportRecommendations(report),
		"section_count":      len(report.Sections),
		"status":             report.Status,
	}
	if report.ResourceUID != nil {
		metadata["resource_uid"] = *report.ResourceUID
	}
	if report.Artifact != nil {
		metadata["artifact_uid"] = report.Artifact.UID
	}
	return metadata
}

// buildFailedMetadata construye la metadata mínima para report_generation_failed.
func buildFailedMetadata(job reporting.ReportGenerationJob, reportUID uuid.UUID, err error) map[string]any {
	metadata := map[string]any{
		"report_uid":         reportUID,
		"report_request_id":  job.UID,
		"report_type":        job.ReportType,
		"project_id":         job.ProjectID,
		"status":             reporting.ReportJobStatusFailed,
		"generation_error":   err.Error(),
	}
	if job.ReportUID != nil {
		metadata["artifact_uid"] = job.ReportUID
	}
	return metadata
}

// buildExportedMetadata construye la metadata mínima para report_exported.
func buildExportedMetadata(job reporting.ReportGenerationJob, report reporting.Report, artifact reporting.ReportArtifact) map[string]any {
	metadata := map[string]any{
		"report_uid":         report.UID,
		"report_request_id":  job.UID,
		"report_type":        report.ReportType,
		"format":             report.Format,
		"project_id":         report.ProjectID,
		"artifact_uid":       artifact.UID,
		"mime_type":          artifact.MIMEType,
		"status":             reporting.ReportJobStatusCompleted,
	}
	if report.ResourceUID != nil {
		metadata["resource_uid"] = *report.ResourceUID
	}
	if artifact.FileSizeBytes != nil {
		metadata["file_size_bytes"] = *artifact.FileSizeBytes
	}
	if artifact.ChecksumSHA256 != nil {
		metadata["checksum_sha256"] = *artifact.ChecksumSHA256
	}
	return metadata
}

// buildDownloadedMetadata construye la metadata mínima para report_downloaded.
func buildDownloadedMetadata(job reporting.ReportGenerationJob, artifact reporting.ReportArtifact) map[string]any {
	metadata := map[string]any{
		"report_uid":         job.ReportUID,
		"report_request_id":  job.UID,
		"report_type":        job.ReportType,
		"artifact_uid":       artifact.UID,
		"mime_type":          artifact.MIMEType,
		"status":             reporting.ReportJobStatusCompleted,
	}
	if artifact.FileSizeBytes != nil {
		metadata["file_size_bytes"] = *artifact.FileSizeBytes
	}
	return metadata
}
