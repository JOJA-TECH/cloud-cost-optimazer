package reporting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	reportRecommendationPageSize = 500
	maxReportRecommendations     = 5000
)

type Dependencies struct {
	DataProvider  ReportDataProvider
	Repository    ReportRepository
	ResourceScope ReportResourceScope
	Formatters    []ReportFormatter
	Exporter      ReportExportAdapter
	Audit         ReportAuditSink
}

type Service struct {
	dataProvider  ReportDataProvider
	repository    ReportRepository
	resourceScope ReportResourceScope
	formatters    []ReportFormatter
	exporter      ReportExportAdapter
	audit         ReportAuditSink

	mu      sync.RWMutex
	reports map[uuid.UUID]Report
}

func NewService(deps Dependencies) (*Service, error) {
	if deps.DataProvider == nil || deps.Repository == nil || deps.ResourceScope == nil ||
		deps.Exporter == nil || deps.Audit == nil || len(deps.Formatters) == 0 {
		return nil, ErrReportDependencyUnavailable
	}
	formatters := append([]ReportFormatter(nil), deps.Formatters...)
	for _, formatter := range formatters {
		if formatter == nil {
			return nil, ErrReportDependencyUnavailable
		}
	}
	return &Service{
		dataProvider:  deps.DataProvider,
		repository:    deps.Repository,
		resourceScope: deps.ResourceScope,
		formatters:    formatters,
		exporter:      deps.Exporter,
		audit:         deps.Audit,
		reports:       make(map[uuid.UUID]Report),
	}, nil
}

var _ ReportGenerator = (*Service)(nil)

func (s *Service) Generate(ctx context.Context, input GenerateReportInput) (Report, error) {
	if input.ReportType != ReportTypeOptimization || input.ResourceUID == nil {
		return Report{}, fmt.Errorf("%w: only optimization reports are available", ErrInvalidReportRequest)
	}
	return s.GenerateOptimizationReport(ctx, OptimizationReportInput{
		ProjectID:   input.ProjectID,
		ResourceUID: *input.ResourceUID,
		Period:      input.Period,
		Format:      input.Format,
		RequestedBy: input.RequestedBy,
		Parameters:  input.Parameters,
	})
}

func (s *Service) GenerateOptimizationReport(
	ctx context.Context,
	input OptimizationReportInput,
) (Report, error) {
	if s == nil || s.repository == nil || s.dataProvider == nil || s.resourceScope == nil ||
		s.exporter == nil || s.audit == nil {
		return Report{}, ErrReportDependencyUnavailable
	}
	if err := ValidateOptimizationReportInput(input); err != nil {
		return Report{}, err
	}
	input.Format = ReportFormat(strings.ToLower(strings.TrimSpace(string(input.Format))))
	formatter := s.formatterFor(input.Format)
	if formatter == nil {
		return Report{}, fmt.Errorf("%w: %q", ErrReportFormatUnsupported, input.Format)
	}

	inProject, err := s.resourceScope.IsResourceInProject(ctx, input.ProjectID, input.ResourceUID)
	if err != nil {
		return Report{}, fmt.Errorf("check report resource scope: %w", err)
	}
	if !inProject {
		return Report{}, ErrReportResourceNotFound
	}

	now := time.Now().UTC()
	reportUID := uuid.New()
	resourceUID := input.ResourceUID
	period := ReportPeriod{Start: input.Period.Start.UTC(), End: input.Period.End.UTC()}
	parameters := copyReportParameters(input.Parameters)
	parameters["resource_uid"] = resourceUID.String()
	parameters["period"] = map[string]any{"start": period.Start, "end": period.End}
	parameters["format"] = input.Format
	job := ReportGenerationJob{
		UID:         reportUID,
		ProjectID:   input.ProjectID,
		RequestedBy: input.RequestedBy,
		ReportType:  ReportTypeOptimization,
		Parameters:  parameters,
		Status:      ReportJobStatusRunning,
		ReportUID:   &reportUID,
		CreatedAt:   now,
		StartedAt:   &now,
	}
	job, err = s.repository.CreateJob(ctx, job)
	if err != nil {
		return Report{}, fmt.Errorf("%w: create report job: %w", ErrReportRepository, err)
	}
	fail := func(cause error) error {
		completedAt := time.Now().UTC()
		message := cause.Error()
		job.Status = ReportJobStatusFailed
		job.CompletedAt = &completedAt
		job.ErrorMessage = &message
		if updateErr := s.repository.UpdateJob(ctx, job); updateErr != nil {
			return errors.Join(cause, fmt.Errorf("%w: mark report job failed: %w", ErrReportRepository, updateErr))
		}
		return cause
	}

	analyticsData, err := s.dataProvider.GetAnalytics(ctx, AnalyticsReportQuery{
		ResourceUID: resourceUID,
		From:        period.Start,
		To:          period.End,
	})
	if err != nil {
		return Report{}, fail(fmt.Errorf("read analytics report data: %w", err))
	}
	recommendations, err := s.listReportRecommendations(ctx, resourceUID, period)
	if err != nil {
		return Report{}, fail(err)
	}

	traces := make(map[uuid.UUID]RecommendationTraceData, len(recommendations))
	for i := range recommendations {
		trace, err := s.dataProvider.GetRecommendationTrace(ctx, recommendations[i].RecommendationUID)
		if err != nil {
			return Report{}, fail(fmt.Errorf("read recommendation trace %s: %w", recommendations[i].RecommendationUID, err))
		}
		recommendations[i].TraceAvailable = len(trace.DecisionTraces) > 0 || len(trace.Events) > 0
		traces[recommendations[i].RecommendationUID] = trace
	}

	report := buildOptimizationReport(reportUID, input, period, analyticsData, recommendations, traces, now)
	formatted, err := formatter.Format(ctx, report)
	if err != nil {
		return Report{}, fail(fmt.Errorf("format report: %w", err))
	}
	artifact, err := s.exporter.Export(ctx, formatted)
	if err != nil {
		return Report{}, fail(fmt.Errorf("export report: %w", err))
	}
	artifact, err = s.repository.SaveArtifact(ctx, job.UID, artifact)
	if err != nil {
		return Report{}, fail(fmt.Errorf("%w: save report artifact: %w", ErrReportRepository, err))
	}
	report.Artifact = &artifact
	completedAt := time.Now().UTC()
	report.CompletedAt = &completedAt
	job.Status = ReportJobStatusCompleted
	job.ArtifactUID = &artifact.UID
	job.CompletedAt = &completedAt
	if err := s.repository.UpdateJob(ctx, job); err != nil {
		return Report{}, fail(fmt.Errorf("%w: complete report job: %w", ErrReportRepository, err))
	}
	if err := s.audit.RecordReportGenerated(ctx, job, report); err != nil {
		return Report{}, fail(fmt.Errorf("record report audit event: %w", err))
	}

	s.mu.Lock()
	s.reports[report.UID] = report
	s.mu.Unlock()
	return report, nil
}

func (s *Service) GetReport(ctx context.Context, reportUID uuid.UUID) (Report, error) {
	if s == nil || s.repository == nil {
		return Report{}, ErrReportDependencyUnavailable
	}
	if reportUID == uuid.Nil {
		return Report{}, ErrReportNotFound
	}
	s.mu.RLock()
	report, ok := s.reports[reportUID]
	s.mu.RUnlock()
	if ok {
		return report, nil
	}
	return Report{}, ErrReportNotFound
}

func (s *Service) formatterFor(format ReportFormat) ReportFormatter {
	for _, formatter := range s.formatters {
		if formatter.Supports(format) {
			return formatter
		}
	}
	return nil
}

func (s *Service) listReportRecommendations(
	ctx context.Context,
	resourceUID uuid.UUID,
	period ReportPeriod,
) ([]RecommendationReportData, error) {
	resourceFilter := resourceUID
	var result []RecommendationReportData
	for offset := 0; ; {
		page, err := s.dataProvider.GetRecommendations(ctx, RecommendationReportQuery{
			ResourceUID: &resourceFilter,
			Limit:       reportRecommendationPageSize,
			Offset:      offset,
		})
		if err != nil {
			return nil, fmt.Errorf("read recommendations: %w", err)
		}
		if len(page) == 0 {
			break
		}
		if offset+len(page) > maxReportRecommendations {
			return nil, fmt.Errorf("%w: recommendation count exceeds %d", ErrReportGenerationFailed, maxReportRecommendations)
		}
		for _, recommendation := range page {
			if recommendationOverlapsPeriod(recommendation.Period, period) {
				result = append(result, recommendation)
			}
		}
		offset += len(page)
		if len(page) < reportRecommendationPageSize {
			break
		}
	}
	return result, nil
}

func recommendationOverlapsPeriod(recommendationPeriod, reportPeriod ReportPeriod) bool {
	if recommendationPeriod.Start.IsZero() || recommendationPeriod.End.IsZero() {
		return true
	}
	return recommendationPeriod.Start.Before(reportPeriod.End) &&
		recommendationPeriod.End.After(reportPeriod.Start)
}

func copyReportParameters(parameters map[string]any) map[string]any {
	result := make(map[string]any, len(parameters)+3)
	for key, value := range parameters {
		result[key] = value
	}
	return result
}

func buildOptimizationReport(
	uid uuid.UUID,
	input OptimizationReportInput,
	period ReportPeriod,
	analytics AnalyticsReportData,
	recommendations []RecommendationReportData,
	traces map[uuid.UUID]RecommendationTraceData,
	createdAt time.Time,
) Report {
	evidence := append([]ReportEvidence(nil), analytics.Evidence...)
	sourceReferences := append([]ReportSourceReference(nil), analytics.SourceReferences...)
	reportRecommendations := make([]ReportRecommendation, 0, len(recommendations))
	savingViews := make([]map[string]any, 0, len(recommendations))
	riskViews := make([]map[string]any, 0, len(recommendations))
	confidenceViews := make([]map[string]any, 0, len(recommendations))
	for _, recommendation := range recommendations {
		reportRecommendations = append(reportRecommendations, recommendation.ReportRecommendation)
		for _, item := range recommendation.Evidence {
			evidence = append(evidence, item)
			sourceReferences = append(sourceReferences, ReportSourceReference{
				Source: item.Source, Reference: item.Reference, Context: item.Context,
			})
		}
		savingViews = append(savingViews, map[string]any{
			"recommendation_uid": recommendation.RecommendationUID,
			"estimated_saving":   recommendation.EstimatedSaving,
		})
		riskViews = append(riskViews, map[string]any{
			"recommendation_uid": recommendation.RecommendationUID,
			"risk":               recommendation.Risk,
		})
		confidenceViews = append(confidenceViews, map[string]any{
			"recommendation_uid": recommendation.RecommendationUID,
			"confidence":         recommendation.Confidence,
		})
	}

	traceReferences := make([]ReportSourceReference, 0)
	traceData := make(map[string]RecommendationTraceData, len(traces))
	for _, recommendation := range recommendations {
		recommendationUID := recommendation.RecommendationUID
		trace := traces[recommendationUID]
		traceData[recommendationUID.String()] = trace
		for _, decision := range trace.DecisionTraces {
			traceReferences = append(traceReferences, ReportSourceReference{
				Source: ReportSourceAudit, Reference: "audit.decision_trace/" + decision.TraceUID.String(),
			})
		}
		for _, event := range trace.Events {
			traceReferences = append(traceReferences, ReportSourceReference{
				Source: ReportSourceAudit, Reference: fmt.Sprintf("audit.events/%d", event.EventID),
			})
		}
	}
	sourceReferences = append(sourceReferences, traceReferences...)
	createdAt = createdAt.UTC()
	return Report{
		UID:         uid,
		ProjectID:   input.ProjectID,
		ResourceUID: &input.ResourceUID,
		ReportType:  ReportTypeOptimization,
		Title:       "Reporte de optimización",
		Description: "Consolidación de métricas, recomendaciones, evidencias y trazabilidad existentes.",
		Period:      period,
		Format:      input.Format,
		Status:      ReportStatusCompleted,
		Sections: []ReportSection{
			{Key: ReportSectionExecutiveSummary, Title: "Resumen ejecutivo", Order: 1,
				Data: map[string]any{
					"resource_uid": input.ResourceUID, "period": period,
					"metrics_count": len(analytics.Metrics) + len(analytics.ObservedCost) +
						len(analytics.Trends) + len(analytics.Utilization) + len(analytics.Forecasts),
					"recommendations_count": len(reportRecommendations),
					"observed_cost":         analytics.ObservedCost,
				}},
			{Key: ReportSectionAnalysisPeriod, Title: "Periodo analizado", Order: 2,
				Data: map[string]any{"start": period.Start, "end": period.End}},
			{Key: ReportSectionKeyMetrics, Title: "Métricas principales", Order: 3, Metrics: analytics.Metrics},
			{Key: ReportSectionCostOverview, Title: "Costo observado", Order: 4, Metrics: analytics.ObservedCost},
			{Key: ReportSectionUtilizationAnalysis, Title: "Tendencias y utilización", Order: 5,
				Data: map[string]any{"trends": analytics.Trends, "utilization": analytics.Utilization, "forecasts": analytics.Forecasts}},
			{Key: ReportSectionRecommendations, Title: "Recomendaciones", Order: 6, Recommendations: reportRecommendations},
			{Key: ReportSectionEstimatedSavings, Title: "Ahorro estimado", Order: 7, Data: map[string]any{"items": savingViews}},
			{Key: ReportSectionRiskAssessment, Title: "Riesgo", Order: 8, Data: map[string]any{"items": riskViews}},
			{Key: ReportSectionConfidence, Title: "Confianza", Order: 9, Data: map[string]any{"items": confidenceViews}},
			{Key: ReportSectionEvidence, Title: "Evidencias", Order: 10, Evidence: evidence},
			{Key: ReportSectionAuditTrace, Title: "Trazabilidad de decisiones", Order: 11,
				Data: map[string]any{"recommendations": traceData}},
			{Key: ReportSectionGenerationMetadata, Title: "Metadata de generación", Order: 12,
				Data: map[string]any{"generated_at": createdAt, "generated_by": input.RequestedBy}},
		},
		Metadata: ReportMetadata{
			"generation_mode": "synchronous",
			"generated_at":    createdAt,
			"generated_by":    input.RequestedBy,
			"parameters":      input.Parameters,
		},
		SourceReferences: sourceReferences,
		GeneratedBy:      input.RequestedBy,
		CreatedAt:        createdAt,
	}
}
