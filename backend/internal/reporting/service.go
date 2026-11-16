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
	if err := ValidateGenerateReportInput(input); err != nil {
		return Report{}, err
	}
	return s.GenerateOptimizationReport(ctx, OptimizationReportInput{
		ProjectID:          input.ProjectID,
		ResourceUID:        *input.ResourceUID,
		From:               input.From,
		To:                 input.To,
		Format:             input.Format,
		IncludeAudit:       input.IncludeAudit,
		RecommendationUIDs: input.RecommendationUIDs,
		IncludeEvidence:    input.IncludeEvidence,
		IncludeRisks:       input.IncludeRisks,
		IncludeMetrics:     input.IncludeMetrics,
		Title:              input.Title,
		RequestedBy:        input.RequestedBy,
		Parameters:         input.Parameters,
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
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		input.Title = "Reporte de optimización"
	}

	inProject, err := s.resourceScope.IsResourceInProject(ctx, input.ProjectID, input.ResourceUID)
	if err != nil {
		return Report{}, fmt.Errorf("check report resource scope: %w", err)
	}
	if !inProject {
		return Report{}, ErrReportResourceNotFound
	}

	period := ReportPeriod{Start: input.From.UTC(), End: input.To.UTC()}
	var recommendations []RecommendationReportData
	if len(input.RecommendationUIDs) > 0 {
		recommendations, err = s.getSelectedRecommendations(ctx, input.RecommendationUIDs, input.ResourceUID, period)
		if err != nil {
			return Report{}, err
		}
	}

	now := time.Now().UTC()
	reportUID := uuid.New()
	resourceUID := input.ResourceUID
	parameters := copyReportParameters(input.Parameters)
	parameters["resource_uid"] = resourceUID.String()
	parameters["period"] = map[string]any{"start": period.Start, "end": period.End}
	parameters["format"] = input.Format
	parameters["recommendation_uids"] = input.RecommendationUIDs
	parameters["include_audit"] = input.IncludeAudit
	parameters["include_trace"] = input.IncludeAudit
	parameters["include_evidence"] = input.IncludeEvidence
	parameters["include_risks"] = input.IncludeRisks
	parameters["include_metrics"] = input.IncludeMetrics
	parameters["title"] = input.Title
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

	analyticsData := AnalyticsReportData{ResourceUID: resourceUID, Period: period}
	if input.IncludeMetrics {
		analyticsData, err = s.dataProvider.GetAnalytics(ctx, AnalyticsReportQuery{
			ResourceUID: resourceUID,
			From:        period.Start,
			To:          period.End,
		})
		if err != nil {
			return Report{}, fail(fmt.Errorf("read analytics report data: %w", err))
		}
	}
	if len(input.RecommendationUIDs) == 0 {
		recommendations, err = s.listReportRecommendations(ctx, resourceUID, period)
		if err != nil {
			return Report{}, fail(err)
		}
	}

	traces := make(map[uuid.UUID]RecommendationTraceData, len(recommendations))
	if input.IncludeAudit {
		for i := range recommendations {
			trace, err := s.dataProvider.GetRecommendationTrace(ctx, recommendations[i].RecommendationUID)
			if err != nil {
				return Report{}, fail(fmt.Errorf("read recommendation trace %s: %w", recommendations[i].RecommendationUID, err))
			}
			recommendations[i].TraceAvailable = len(trace.DecisionTraces) > 0 || len(trace.Events) > 0
			traces[recommendations[i].RecommendationUID] = trace
		}
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
	// Caché de generación síncrona (mismo proceso).
	s.mu.RLock()
	report, ok := s.reports[reportUID]
	s.mu.RUnlock()
	if ok {
		return report, nil
	}
	// Persistencia: app.report_requests / app.report_artifacts.
	job, err := s.repository.GetJob(ctx, reportUID)
	if err != nil {
		return Report{}, err
	}
	var artifact *ReportArtifact
	if job.ArtifactUID != nil {
		a, err := s.repository.GetArtifact(ctx, *job.ArtifactUID)
		if err != nil {
			return Report{}, err
		}
		artifact = &a
	}
	return Report{
		UID:         job.UID,
		ProjectID:   job.ProjectID,
		ReportType:  job.ReportType,
		Status:      jobStatusToReportStatus(job.Status),
		GeneratedBy: job.RequestedBy,
		CreatedAt:   job.CreatedAt,
		CompletedAt: job.CompletedAt,
		Artifact:    artifact,
	}, nil
}

// DownloadArtifact returns the artifact content bytes through the export
// adapter's underlying object storage, without exposing cloud SDKs to the
// service layer.
func (s *Service) DownloadArtifact(ctx context.Context, artifact ReportArtifact) ([]byte, error) {
	if s == nil || s.exporter == nil {
		return nil, ErrReportDependencyUnavailable
	}
	return s.exporter.Download(ctx, artifact)
}

func jobStatusToReportStatus(status ReportJobStatus) ReportStatus {
	switch status {
	case ReportJobStatusQueued:
		return ReportStatusPending
	case ReportJobStatusRunning:
		return ReportStatusGenerating
	case ReportJobStatusCompleted:
		return ReportStatusCompleted
	case ReportJobStatusFailed:
		return ReportStatusFailed
	case ReportJobStatusCancelled:
		return ReportStatusCancelled
	default:
		return ReportStatusPending
	}
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
			if recommendation.RecommendationUID == uuid.Nil || recommendation.ResourceUID != resourceUID {
				return nil, fmt.Errorf("%w: recommendation reader returned invalid resource data", ErrReportGenerationFailed)
			}
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

func (s *Service) getSelectedRecommendations(
	ctx context.Context,
	recommendationUIDs []uuid.UUID,
	resourceUID uuid.UUID,
	period ReportPeriod,
) ([]RecommendationReportData, error) {
	result := make([]RecommendationReportData, 0, len(recommendationUIDs))
	for _, recommendationUID := range recommendationUIDs {
		recommendation, err := s.dataProvider.GetRecommendation(ctx, recommendationUID)
		if errors.Is(err, ErrReportRecommendationNotFound) {
			return nil, fmt.Errorf("%w: selected recommendations must belong to the requested resource and period", ErrInvalidReportRequest)
		}
		if err != nil {
			return nil, fmt.Errorf("read selected recommendation: %w", err)
		}
		if recommendation.RecommendationUID != recommendationUID ||
			recommendation.ResourceUID != resourceUID ||
			!recommendationOverlapsPeriod(recommendation.Period, period) {
			return nil, fmt.Errorf("%w: selected recommendations must belong to the requested resource and period", ErrInvalidReportRequest)
		}
		result = append(result, recommendation)
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

func buildExecutiveSummaryData(
	input OptimizationReportInput,
	period ReportPeriod,
	analytics AnalyticsReportData,
	recommendations []RecommendationReportData,
) map[string]any {
	currentMonthlyCost, hasCurrentCost := reportMetricValue(analytics.ObservedCost, "actual_monthly_cost")
	currentCostSource := ReportSourceAnalytics
	if !hasCurrentCost {
		if value, ok := commonRecommendationCost(recommendations, false); ok {
			currentMonthlyCost = value
			hasCurrentCost = true
			currentCostSource = ReportSourceRecommendation
		}
	}

	currency := analytics.CostCurrency
	hasCurrency := currency != ""
	if !hasCurrency {
		currency, hasCurrency = commonRecommendationCurrency(recommendations)
	}
	var currencyValue any
	if !hasCurrency {
		currencyValue = nil
	} else {
		currencyValue = currency
	}
	estimatedSaving, savingCurrency := selectedRecommendationSaving(recommendations)
	confidence, confidenceAggregation := recommendationConfidenceSummary(recommendations)
	var highestRisk any
	if input.IncludeRisks {
		highestRisk = highestRecommendationRisk(recommendations)
	}

	summary := map[string]any{
		"resource_uid":                      input.ResourceUID,
		"period":                            period,
		"current_monthly_cost":              currentMonthlyCost,
		"estimated_monthly_saving":          estimatedSaving,
		"estimated_monthly_saving_currency": savingCurrency,
		"recommendation_count":              len(recommendations),
		"highest_risk_level":                highestRisk,
		"average_or_selected_confidence":    confidence,
		"confidence_aggregation":            confidenceAggregation,
	}
	if hasCurrentCost {
		summary["current_monthly_cost_source"] = currentCostSource
	}
	if hasCurrency {
		summary["currency"] = currencyValue
	}
	if len(recommendations) > 1 {
		summary["estimated_monthly_saving_by_recommendation"] = recommendationSavingEstimates(recommendations)
	}
	return summary
}

func buildCostOverviewData(
	period ReportPeriod,
	analytics AnalyticsReportData,
	recommendations []RecommendationReportData,
) map[string]any {
	observedCost, hasObservedCost := reportMetricValue(analytics.ObservedCost, "actual_monthly_cost")
	estimatedCost, hasEstimatedCost := reportMetricValue(analytics.ObservedCost, "projected_monthly_cost")
	currency := analytics.CostCurrency
	hasCurrency := currency != ""
	if !hasCurrency {
		currency, hasCurrency = commonRecommendationCurrency(recommendations)
	}
	var currencyValue any
	if hasCurrency {
		currencyValue = currency
	}

	var baseCost any
	var baseCostSource any
	if hasObservedCost {
		baseCost = map[string]any{
			"amount": observedCost, "currency": currencyValue,
			"basis": "observed_monthly_cost", "source": ReportSourceAnalytics,
		}
		baseCostSource = ReportSourceAnalytics
	} else if value, ok := commonRecommendationCost(recommendations, false); ok {
		basis, hasBasis := commonRecommendationBasis(recommendations, false)
		baseCost = map[string]any{
			"amount": value, "currency": currencyValue,
			"basis": optionalMetricValue(basis, hasBasis), "source": ReportSourceRecommendation,
		}
		baseCostSource = ReportSourceRecommendation
	}

	estimatedCostSource := any(nil)
	if hasEstimatedCost {
		estimatedCostSource = ReportSourceAnalytics
	} else if len(recommendations) == 1 {
		estimatedCost = recommendations[0].EstimatedSaving.ProposedMonthlyCost
		hasEstimatedCost = true
		estimatedCostSource = ReportSourceRecommendation
	}

	costPeriod := period
	if metric, ok := reportMetric(analytics.ObservedCost, "actual_monthly_cost"); ok {
		costPeriod = metric.Period
	}
	return map[string]any{
		"observed_cost":                 optionalMetricValue(observedCost, hasObservedCost),
		"currency":                      currencyValue,
		"period":                        costPeriod,
		"base_cost":                     baseCost,
		"base_cost_source":              baseCostSource,
		"estimated_monthly_cost":        optionalMetricValue(estimatedCost, hasEstimatedCost),
		"estimated_monthly_cost_source": estimatedCostSource,
		"estimated_monthly_saving":      monthlySavingOverview(recommendations),
		"recommendation_cost_estimates": recommendationCostEstimates(recommendations),
		"observed_cost_source":          optionalSource(hasObservedCost, ReportSourceAnalytics),
		"source": map[string]any{
			"observed_cost":          optionalSource(hasObservedCost, ReportSourceAnalytics),
			"base_cost":              baseCostSource,
			"estimated_monthly_cost": estimatedCostSource,
			"estimated_monthly_saving": optionalSource(
				len(recommendations) > 0, ReportSourceRecommendation,
			),
		},
		"data_quality": analytics.DataQuality,
	}
}

func buildUtilizationAnalysisData(
	analytics AnalyticsReportData,
	recommendations []RecommendationReportData,
	includeRisks bool,
) map[string]any {
	cpuP95 := make([]ReportMetric, 0)
	cpuP99 := make([]ReportMetric, 0)
	workloadPattern := make([]ReportMetric, 0)
	for _, metric := range analytics.Metrics {
		code := strings.ToLower(metric.Code)
		if strings.Contains(code, "cpu") && strings.Contains(code, "p95") {
			cpuP95 = append(cpuP95, metric)
		}
		if strings.Contains(code, "cpu") && strings.Contains(code, "p99") {
			cpuP99 = append(cpuP99, metric)
		}
	}
	for _, metric := range analytics.Utilization {
		switch metric.Code {
		case "peak_hours", "peak_frequency", "intensity_band":
			workloadPattern = append(workloadPattern, metric)
		}
	}

	variability := make([]map[string]any, 0, len(recommendations))
	headroom := make([]ReportEvidence, 0)
	for _, recommendation := range recommendations {
		if includeRisks && recommendation.Risk != nil {
			variability = append(variability, map[string]any{
				"recommendation_uid": recommendation.RecommendationUID,
				"value":              recommendation.Risk.Variability,
				"source":             ReportSourceRecommendation,
			})
		}
		for _, evidence := range recommendation.Evidence {
			if evidence.Type == "headroom" || (evidence.MetricCode != nil && *evidence.MetricCode == "headroom_pct") {
				headroom = append(headroom, evidence)
			}
		}
	}

	return map[string]any{
		"cpu_p95":          cpuP95,
		"cpu_p99":          cpuP99,
		"variability":      variability,
		"trend":            analytics.Trends,
		"headroom":         headroom,
		"workload_pattern": workloadPattern,
		"data_quality":     analytics.DataQuality,
		"data_coverage": map[string]any{
			"status":  analytics.DataQuality.Status,
			"missing": analytics.DataQuality.Missing,
		},
		"forecasts": analytics.Forecasts,
	}
}

func reportMetric(metrics []ReportMetric, code string) (ReportMetric, bool) {
	for _, metric := range metrics {
		if metric.Code == code && metric.Value != nil {
			return metric, true
		}
	}
	return ReportMetric{}, false
}

func reportMetricValue(metrics []ReportMetric, code string) (any, bool) {
	metric, ok := reportMetric(metrics, code)
	if !ok {
		return nil, false
	}
	return metric.Value, true
}

func optionalMetricValue(value any, available bool) any {
	if !available {
		return nil
	}
	return value
}

func optionalSource(available bool, source ReportSource) any {
	if !available {
		return nil
	}
	return source
}

func commonRecommendationCurrency(recommendations []RecommendationReportData) (string, bool) {
	if len(recommendations) == 0 || recommendations[0].EstimatedSaving.Currency == "" {
		return "", false
	}
	currency := recommendations[0].EstimatedSaving.Currency
	for _, recommendation := range recommendations[1:] {
		if recommendation.EstimatedSaving.Currency != currency {
			return "", false
		}
	}
	return currency, true
}

func commonRecommendationCost(recommendations []RecommendationReportData, proposed bool) (float64, bool) {
	if len(recommendations) == 0 {
		return 0, false
	}
	saving := recommendations[0].EstimatedSaving
	value := saving.CurrentMonthlyCost
	if proposed {
		value = saving.ProposedMonthlyCost
	}
	currency := saving.Currency
	for _, recommendation := range recommendations[1:] {
		candidate := recommendation.EstimatedSaving
		candidateValue := candidate.CurrentMonthlyCost
		if proposed {
			candidateValue = candidate.ProposedMonthlyCost
		}
		if candidateValue != value || candidate.Currency != currency {
			return 0, false
		}
	}
	return value, true
}

func commonRecommendationBasis(recommendations []RecommendationReportData, proposed bool) (string, bool) {
	if len(recommendations) == 0 {
		return "", false
	}
	saving := recommendations[0].EstimatedSaving
	basis := saving.CurrentCostBasis
	if proposed {
		basis = saving.ProposedCostBasis
	}
	for _, recommendation := range recommendations[1:] {
		candidateBasis := recommendation.EstimatedSaving.CurrentCostBasis
		if proposed {
			candidateBasis = recommendation.EstimatedSaving.ProposedCostBasis
		}
		if candidateBasis != basis {
			return "", false
		}
	}
	return basis, true
}

func selectedRecommendationSaving(recommendations []RecommendationReportData) (any, string) {
	if len(recommendations) != 1 {
		return nil, ""
	}
	saving := recommendations[0].EstimatedSaving
	return saving.MonthlyAmount, saving.Currency
}

func monthlySavingOverview(recommendations []RecommendationReportData) any {
	if len(recommendations) == 1 {
		recommendation := recommendations[0]
		return map[string]any{
			"recommendation_uid": recommendation.RecommendationUID,
			"amount":             recommendation.EstimatedSaving.MonthlyAmount,
			"currency":           recommendation.EstimatedSaving.Currency,
			"source":             ReportSourceRecommendation,
		}
	}
	return nil
}

func recommendationSavingEstimates(recommendations []RecommendationReportData) []map[string]any {
	estimates := make([]map[string]any, 0, len(recommendations))
	for _, recommendation := range recommendations {
		estimates = append(estimates, map[string]any{
			"recommendation_uid": recommendation.RecommendationUID,
			"amount":             recommendation.EstimatedSaving.MonthlyAmount,
			"currency":           recommendation.EstimatedSaving.Currency,
			"source":             ReportSourceRecommendation,
		})
	}
	return estimates
}

func recommendationCostEstimates(recommendations []RecommendationReportData) []map[string]any {
	estimates := make([]map[string]any, 0, len(recommendations))
	for _, recommendation := range recommendations {
		estimates = append(estimates, map[string]any{
			"recommendation_uid":   recommendation.RecommendationUID,
			"base_cost":            recommendation.EstimatedSaving.CurrentMonthlyCost,
			"estimated_cost":       recommendation.EstimatedSaving.ProposedMonthlyCost,
			"estimated_saving":     recommendation.EstimatedSaving.MonthlyAmount,
			"currency":             recommendation.EstimatedSaving.Currency,
			"base_cost_basis":      recommendation.EstimatedSaving.CurrentCostBasis,
			"estimated_cost_basis": recommendation.EstimatedSaving.ProposedCostBasis,
			"source":               ReportSourceRecommendation,
		})
	}
	return estimates
}

func recommendationConfidenceSummary(recommendations []RecommendationReportData) (any, string) {
	if len(recommendations) == 0 {
		return nil, ""
	}
	total := 0.0
	for _, recommendation := range recommendations {
		total += recommendation.Confidence.Score
	}
	if len(recommendations) == 1 {
		return total, "single_recommendation"
	}
	return total / float64(len(recommendations)), "average"
}

func highestRecommendationRisk(recommendations []RecommendationReportData) any {
	var highest *RiskView
	for index := range recommendations {
		risk := recommendations[index].Risk
		if risk == nil {
			continue
		}
		if highest == nil || riskSeverity(risk.Level) > riskSeverity(highest.Level) {
			highest = risk
		}
	}
	if highest == nil {
		return nil
	}
	return highest.Level
}

func riskSeverity(level string) int {
	switch strings.ToLower(level) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
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
	evidence := make([]ReportEvidence, 0, len(analytics.Evidence))
	if input.IncludeEvidence {
		evidence = append(evidence, analytics.Evidence...)
	}
	sourceReferences := append([]ReportSourceReference(nil), analytics.SourceReferences...)
	reportRecommendations := make([]ReportRecommendation, 0, len(recommendations))
	savingViews := make([]map[string]any, 0, len(recommendations))
	riskViews := make([]map[string]any, 0, len(recommendations))
	confidenceViews := make([]map[string]any, 0, len(recommendations))
	for _, recommendation := range recommendations {
		recommendationView := recommendation.ReportRecommendation
		if !input.IncludeRisks {
			recommendationView.Risk = nil
		}
		reportRecommendations = append(reportRecommendations, recommendationView)
		if input.IncludeEvidence {
			for _, item := range recommendation.Evidence {
				evidence = append(evidence, item)
				sourceReferences = append(sourceReferences, ReportSourceReference{
					Source: item.Source, Reference: item.Reference, Context: item.Context,
				})
			}
		}
		savingViews = append(savingViews, map[string]any{
			"recommendation_uid": recommendation.RecommendationUID,
			"estimated_saving":   recommendation.EstimatedSaving,
		})
		if input.IncludeRisks {
			riskViews = append(riskViews, map[string]any{
				"recommendation_uid": recommendation.RecommendationUID,
				"risk":               recommendation.Risk,
			})
		}
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
	if input.IncludeAudit {
		sourceReferences = append(sourceReferences, traceReferences...)
	}
	createdAt = createdAt.UTC()
	dataQuality := analytics.DataQuality
	if dataQuality.Status == "" {
		dataQuality.Status = "not_requested"
	}
	sections := []ReportSection{
		{Key: ReportSectionExecutiveSummary, Title: "Resumen ejecutivo", Order: 1,
			Data: buildExecutiveSummaryData(input, period, analytics, recommendations)},
		{Key: ReportSectionAnalysisPeriod, Title: "Periodo analizado", Order: 2,
			Data: map[string]any{"start": period.Start, "end": period.End}},
	}
	if input.IncludeMetrics {
		sections = append(sections,
			ReportSection{Key: ReportSectionKeyMetrics, Title: "Métricas principales", Order: 3, Metrics: analytics.Metrics},
			ReportSection{Key: ReportSectionCostOverview, Title: "Costo observado", Order: 4,
				Metrics: analytics.ObservedCost, Data: buildCostOverviewData(period, analytics, recommendations)},
			ReportSection{Key: ReportSectionUtilizationAnalysis, Title: "Tendencias y utilización", Order: 5,
				Data: buildUtilizationAnalysisData(analytics, recommendations, input.IncludeRisks)},
		)
	}
	sections = append(sections, ReportSection{
		Key: ReportSectionOptimizationOpportunities, Title: "Oportunidades de optimización", Order: 6,
		Recommendations: reportRecommendations,
		Data:            map[string]any{"source": ReportSourceRecommendation},
	})
	sections = append(sections, ReportSection{
		Key: ReportSectionRecommendations, Title: "Recomendaciones", Order: 7, Recommendations: reportRecommendations,
	})
	sections = append(sections,
		ReportSection{Key: ReportSectionEstimatedSavings, Title: "Ahorro estimado", Order: 8, Data: map[string]any{"items": savingViews}},
	)
	if input.IncludeRisks {
		sections = append(sections, ReportSection{
			Key: ReportSectionRiskAssessment, Title: "Riesgo", Order: 9, Data: map[string]any{"items": riskViews},
		})
	}
	sections = append(sections, ReportSection{
		Key: ReportSectionConfidence, Title: "Confianza", Order: 10, Data: map[string]any{"items": confidenceViews},
	})
	if input.IncludeEvidence {
		sections = append(sections, ReportSection{
			Key: ReportSectionEvidence, Title: "Evidencias", Order: 11, Evidence: evidence,
		})
	}
	if input.IncludeAudit {
		sections = append(sections, ReportSection{
			Key: ReportSectionAuditTrace, Title: "Trazabilidad de decisiones", Order: 12,
			Data: map[string]any{"recommendations": traceData},
		})
	}
	sections = append(sections, ReportSection{
		Key: ReportSectionGenerationMetadata, Title: "Metadata de generación", Order: 13,
		Data: map[string]any{"generated_at": createdAt, "generated_by": input.RequestedBy},
	})
	return Report{
		UID:         uid,
		ProjectID:   input.ProjectID,
		ResourceUID: &input.ResourceUID,
		ReportType:  ReportTypeOptimization,
		Title:       input.Title,
		Description: "Consolidación de métricas, recomendaciones, evidencias y trazabilidad existentes.",
		Period:      period,
		Format:      input.Format,
		Status:      ReportStatusCompleted,
		Sections:    sections,
		DataQuality: dataQuality,
		Metadata: ReportMetadata{
			"generation_mode":  "synchronous",
			"generated_at":     createdAt,
			"generated_by":     input.RequestedBy,
			"parameters":       input.Parameters,
			"include_audit":    input.IncludeAudit,
			"include_evidence": input.IncludeEvidence,
			"include_risks":    input.IncludeRisks,
			"include_metrics":  input.IncludeMetrics,
		},
		SourceReferences: sourceReferences,
		GeneratedBy:      input.RequestedBy,
		CreatedAt:        createdAt,
	}
}
