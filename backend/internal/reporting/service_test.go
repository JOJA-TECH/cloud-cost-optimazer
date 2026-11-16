package reporting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type reportDataProviderStub struct {
	calls           []string
	analytics       AnalyticsReportData
	recommendations []RecommendationReportData
	trace           RecommendationTraceData
	err             error
}

func (s *reportDataProviderStub) GetAnalytics(context.Context, AnalyticsReportQuery) (AnalyticsReportData, error) {
	s.calls = append(s.calls, "analytics")
	return s.analytics, s.err
}

func (s *reportDataProviderStub) GetRecommendation(_ context.Context, recommendationUID uuid.UUID) (RecommendationReportData, error) {
	s.calls = append(s.calls, "recommendation")
	if s.err != nil {
		return RecommendationReportData{}, s.err
	}
	for _, recommendation := range s.recommendations {
		if recommendation.RecommendationUID == recommendationUID {
			return recommendation, nil
		}
	}
	return RecommendationReportData{}, ErrReportRecommendationNotFound
}

func (s *reportDataProviderStub) GetRecommendations(context.Context, RecommendationReportQuery) ([]RecommendationReportData, error) {
	s.calls = append(s.calls, "recommendations")
	return s.recommendations, s.err
}

func (s *reportDataProviderStub) GetRecommendationTrace(context.Context, uuid.UUID) (RecommendationTraceData, error) {
	s.calls = append(s.calls, "trace")
	return s.trace, s.err
}

type reportRepositoryStub struct {
	calls []string
	job   ReportGenerationJob
	jobs  map[uuid.UUID]ReportGenerationJob
}

func (s *reportRepositoryStub) CreateJob(_ context.Context, job ReportGenerationJob) (ReportGenerationJob, error) {
	s.calls = append(s.calls, "create_job")
	s.job = job
	if s.jobs != nil {
		s.jobs[job.UID] = job
	}
	return job, nil
}

func (s *reportRepositoryStub) UpdateJob(_ context.Context, job ReportGenerationJob) error {
	s.calls = append(s.calls, "update_job:"+string(job.Status))
	s.job = job
	if s.jobs != nil {
		s.jobs[job.UID] = job
	}
	return nil
}

func (s *reportRepositoryStub) GetJob(_ context.Context, jobUID uuid.UUID) (ReportGenerationJob, error) {
	if s.jobs != nil {
		if j, ok := s.jobs[jobUID]; ok {
			return j, nil
		}
	}
	return ReportGenerationJob{}, ErrReportNotFound
}

func (s *reportRepositoryStub) SaveArtifact(_ context.Context, _ uuid.UUID, artifact ReportArtifact) (ReportArtifact, error) {
	s.calls = append(s.calls, "save_artifact")
	return artifact, nil
}

func (s *reportRepositoryStub) GetArtifact(context.Context, uuid.UUID) (ReportArtifact, error) {
	return ReportArtifact{}, ErrReportNotFound
}

func (s *reportRepositoryStub) ListPendingJobs(context.Context) ([]ReportGenerationJob, error) {
	if s.jobs == nil {
		return nil, nil
	}
	var pending []ReportGenerationJob
	for _, j := range s.jobs {
		if j.Status == ReportJobStatusQueued || j.Status == ReportJobStatusRunning {
			pending = append(pending, j)
		}
	}
	return pending, nil
}

type reportResourceScopeStub struct {
	allowed bool
}

func (s reportResourceScopeStub) IsResourceInProject(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return s.allowed, nil
}

type reportFormatterStub struct {
	calls  *[]string
	report Report
}

func (s reportFormatterStub) Supports(format ReportFormat) bool {
	return format == ReportFormatJSON
}

func (s reportFormatterStub) Format(_ context.Context, report Report) (FormattedReport, error) {
	*s.calls = append(*s.calls, "format")
	s.report = report
	body := []byte(`{"report":true}`)
	return FormattedReport{
		Format: ReportFormatJSON, ContentType: "application/json",
		Filename: "report.json", Body: body, SizeBytes: int64(len(body)),
	}, nil
}

type reportExporterStub struct {
	calls *[]string
}

func (s reportExporterStub) Export(context.Context, FormattedReport) (ReportArtifact, error) {
	*s.calls = append(*s.calls, "export")
	size := int64(15)
	return ReportArtifact{
		UID:            uuid.New(),
		StorageURI:     "memory://report",
		MIMEType:       "application/json",
		FileSizeBytes:  &size,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

func (s reportExporterStub) Download(context.Context, ReportArtifact) ([]byte, error) {
	*s.calls = append(*s.calls, "download")
	return []byte(`{"report":true}`), nil
}

type reportAuditSinkStub struct {
	calls  *[]string
	job    ReportGenerationJob
	report Report
	err    error
}

func (s *reportAuditSinkStub) RecordReportGenerationStarted(_ context.Context, job ReportGenerationJob, input OptimizationReportInput) error {
	*s.calls = append(*s.calls, "audit:started")
	s.job = job
	return s.err
}

func (s *reportAuditSinkStub) RecordReportGenerationCompleted(_ context.Context, job ReportGenerationJob, report Report) error {
	*s.calls = append(*s.calls, "audit:completed")
	s.job, s.report = job, report
	return s.err
}

func (s *reportAuditSinkStub) RecordReportGenerationFailed(_ context.Context, job ReportGenerationJob, reportUID uuid.UUID, err error) error {
	*s.calls = append(*s.calls, "audit:failed")
	s.job = job
	return s.err
}

func (s *reportAuditSinkStub) RecordReportExported(_ context.Context, job ReportGenerationJob, report Report, artifact ReportArtifact) error {
	*s.calls = append(*s.calls, "audit:exported")
	s.job, s.report = job, report
	return s.err
}

func (s *reportAuditSinkStub) RecordReportDownloaded(_ context.Context, job ReportGenerationJob, artifact ReportArtifact) error {
	*s.calls = append(*s.calls, "audit:downloaded")
	s.job = job
	return s.err
}

func TestGenerateOptimizationReportRunsSynchronousFlowInOrder(t *testing.T) {
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	resourceUID := uuid.New()
	recommendationUID := uuid.New()
	calls := []string{}
	provider := &reportDataProviderStub{
		analytics: AnalyticsReportData{
			ResourceUID: resourceUID,
			Period:      ReportPeriod{Start: now, End: now.Add(24 * time.Hour)},
			DataQuality: ReportDataQuality{Status: "degraded", Missing: []string{"forecast"}},
			Metrics: []ReportMetric{{
				Code: "cpu_avg", Value: 45.0, Source: ReportSourceAnalytics,
			}},
		},
		recommendations: []RecommendationReportData{{
			ReportRecommendation: ReportRecommendation{
				RecommendationUID: recommendationUID,
				Action:            "resize_down",
				EstimatedSaving:   EstimatedSavingView{MonthlyAmount: 12.5},
				Risk:              &RiskView{Level: "low"},
			},
			ResourceUID: resourceUID,
			Period:      ReportPeriod{Start: now, End: now.Add(24 * time.Hour)},
		}},
		trace: RecommendationTraceData{
			RecommendationUID: recommendationUID,
			DecisionTraces:    []RecommendationDecisionTrace{{TraceUID: uuid.New(), Decision: "accepted"}},
		},
	}
	repository := &reportRepositoryStub{}
	formatter := reportFormatterStub{calls: &calls}
	audit := &reportAuditSinkStub{calls: &calls}
	service, err := NewService(Dependencies{
		DataProvider: provider,
		Repository:   repository,
		ResourceScope: reportResourceScopeStub{
			allowed: true,
		},
		Formatters: []ReportFormatter{formatter},
		Exporter:   reportExporterStub{calls: &calls},
		Audit:      audit,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	report, err := service.GenerateOptimizationReport(context.Background(), OptimizationReportInput{
		ProjectID: uuid.New(), ResourceUID: resourceUID,
		From: now, To: now.Add(24 * time.Hour),
		Format: ReportFormat(" JSON "), RequestedBy: uuid.New(),
		IncludeAudit: true, IncludeEvidence: true, IncludeRisks: true, IncludeMetrics: true,
	})
	if err != nil {
		t.Fatalf("GenerateOptimizationReport() error = %v", err)
	}
	wantCalls := []string{"audit:started", "format", "export", "audit:exported", "audit:completed"}
	if !equalStrings(calls, wantCalls) {
		t.Fatalf("post-persistence call order = %v, want %v", calls, wantCalls)
	}
	if !equalStrings(provider.calls, []string{"analytics", "recommendations", "trace"}) {
		t.Fatalf("data provider calls = %v", provider.calls)
	}
	if report.Status != ReportStatusCompleted || report.Format != ReportFormatJSON ||
		report.Artifact == nil || report.Sections[0].Key != ReportSectionExecutiveSummary {
		t.Fatalf("generated report is incomplete: %#v", report)
	}
	if report.DataQuality.Status != "degraded" || !equalStrings(report.DataQuality.Missing, []string{"forecast"}) {
		t.Fatalf("report data quality = %#v, want degraded with missing forecast", report.DataQuality)
	}
	if repository.job.Status != ReportJobStatusCompleted || audit.job.Status != ReportJobStatusCompleted {
		t.Fatalf("report job was not completed before audit: repository=%s audit=%s", repository.job.Status, audit.job.Status)
	}
	if !reportSection(report, ReportSectionRecommendations).Recommendations[0].TraceAvailable {
		t.Fatal("report recommendation should indicate available decision trace")
	}
}

func TestGenerateOptimizationReportRejectsResourceOutsideProjectBeforeCreatingJob(t *testing.T) {
	now := time.Now().UTC()
	repository := &reportRepositoryStub{}
	service, err := NewService(Dependencies{
		DataProvider: &reportDataProviderStub{},
		Repository:   repository,
		ResourceScope: reportResourceScopeStub{
			allowed: false,
		},
		Formatters: []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:   reportExporterStub{calls: &[]string{}},
		Audit:      &reportAuditSinkStub{calls: &[]string{}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	_, err = service.GenerateOptimizationReport(context.Background(), OptimizationReportInput{
		ProjectID: uuid.New(), ResourceUID: uuid.New(),
		From: now, To: now.Add(time.Hour),
		Format: ReportFormatJSON, RequestedBy: uuid.New(),
	})
	if !errors.Is(err, ErrReportResourceNotFound) {
		t.Fatalf("GenerateOptimizationReport() error = %v, want ErrReportResourceNotFound", err)
	}
	if len(repository.calls) != 0 {
		t.Fatalf("created a report job before checking resource scope: %v", repository.calls)
	}
}

func TestGenerateOptimizationReportMarksJobFailedWhenAnalyticsReadFails(t *testing.T) {
	now := time.Now().UTC()
	repository := &reportRepositoryStub{}
	calls := []string{}
	service, err := NewService(Dependencies{
		DataProvider: &reportDataProviderStub{err: errors.New("analytics unavailable")},
		Repository:   repository,
		ResourceScope: reportResourceScopeStub{
			allowed: true,
		},
		Formatters: []ReportFormatter{reportFormatterStub{calls: &calls}},
		Exporter:   reportExporterStub{calls: &calls},
		Audit:      &reportAuditSinkStub{calls: &calls},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.GenerateOptimizationReport(context.Background(), OptimizationReportInput{
		ProjectID: uuid.New(), ResourceUID: uuid.New(),
		From: now, To: now.Add(time.Hour),
		Format: ReportFormatJSON, RequestedBy: uuid.New(), IncludeMetrics: true,
	})
	if err == nil {
		t.Fatal("GenerateOptimizationReport() error = nil, want analytics read error")
	}
	if repository.job.Status != ReportJobStatusFailed || repository.job.ErrorMessage == nil {
		t.Fatalf("failed report job was not persisted: %#v", repository.job)
	}
	if !equalStrings(repository.calls, []string{"create_job", "update_job:failed"}) {
		t.Fatalf("repository calls = %v, want job creation and failure update", repository.calls)
	}
	if !equalStrings(calls, []string{"audit:started", "audit:failed"}) {
		t.Fatalf("audit calls = %v, want started and failed", calls)
	}
}

func TestGenerateOptimizationReportRejectsRecommendationOutsideResource(t *testing.T) {
	now := time.Now().UTC()
	resourceUID := uuid.New()
	recommendationUID := uuid.New()
	repository := &reportRepositoryStub{}
	service, err := NewService(Dependencies{
		DataProvider: &reportDataProviderStub{
			recommendations: []RecommendationReportData{{
				ReportRecommendation: ReportRecommendation{RecommendationUID: recommendationUID},
				ResourceUID:          uuid.New(),
				Period:               ReportPeriod{Start: now, End: now.Add(time.Hour)},
			}},
		},
		Repository: repository,
		ResourceScope: reportResourceScopeStub{
			allowed: true,
		},
		Formatters: []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:   reportExporterStub{calls: &[]string{}},
		Audit:      &reportAuditSinkStub{calls: &[]string{}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.GenerateOptimizationReport(context.Background(), OptimizationReportInput{
		ProjectID: uuid.New(), ResourceUID: resourceUID,
		From: now, To: now.Add(time.Hour), Format: ReportFormatJSON,
		RequestedBy: uuid.New(), RecommendationUIDs: []uuid.UUID{recommendationUID},
	})
	if !errors.Is(err, ErrInvalidReportRequest) {
		t.Fatalf("GenerateOptimizationReport() error = %v, want ErrInvalidReportRequest", err)
	}
	if len(repository.calls) != 0 {
		t.Fatalf("created report job for a recommendation outside the resource: %v", repository.calls)
	}
}

func TestGenerateOptimizationReportIncludesOnlySelectedRecommendations(t *testing.T) {
	now := time.Now().UTC()
	resourceUID := uuid.New()
	selectedUID, otherUID := uuid.New(), uuid.New()
	provider := &reportDataProviderStub{
		recommendations: []RecommendationReportData{
			{
				ReportRecommendation: ReportRecommendation{RecommendationUID: selectedUID},
				ResourceUID:          resourceUID,
				Period:               ReportPeriod{Start: now, End: now.Add(time.Hour)},
			},
			{
				ReportRecommendation: ReportRecommendation{RecommendationUID: otherUID},
				ResourceUID:          resourceUID,
				Period:               ReportPeriod{Start: now, End: now.Add(time.Hour)},
			},
		},
	}
	service, err := NewService(Dependencies{
		DataProvider: provider,
		Repository:   &reportRepositoryStub{},
		ResourceScope: reportResourceScopeStub{
			allowed: true,
		},
		Formatters: []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:   reportExporterStub{calls: &[]string{}},
		Audit:      &reportAuditSinkStub{calls: &[]string{}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	report, err := service.GenerateOptimizationReport(context.Background(), OptimizationReportInput{
		ProjectID: uuid.New(), ResourceUID: resourceUID,
		From: now, To: now.Add(time.Hour), Format: ReportFormatJSON,
		RequestedBy: uuid.New(), RecommendationUIDs: []uuid.UUID{selectedUID},
	})
	if err != nil {
		t.Fatalf("GenerateOptimizationReport() error = %v", err)
	}
	if !equalStrings(provider.calls, []string{"recommendation"}) {
		t.Fatalf("provider calls = %v, want only the selected recommendation read", provider.calls)
	}
	if got := reportSection(report, ReportSectionRecommendations).Recommendations; len(got) != 1 || got[0].RecommendationUID != selectedUID {
		t.Fatalf("report recommendations = %#v, want only %s", got, selectedUID)
	}
}

func TestGenerateOptimizationReportHonorsIncludeOptions(t *testing.T) {
	now := time.Now().UTC()
	resourceUID := uuid.New()
	calls := []string{}
	provider := &reportDataProviderStub{
		recommendations: []RecommendationReportData{{
			ReportRecommendation: ReportRecommendation{
				RecommendationUID: uuid.New(),
				Risk:              &RiskView{Level: "high"},
				EvidenceCount:     3,
			},
			ResourceUID: resourceUID,
			Period:      ReportPeriod{Start: now, End: now.Add(time.Hour)},
		}},
	}
	service, err := NewService(Dependencies{
		DataProvider: provider,
		Repository:   &reportRepositoryStub{},
		ResourceScope: reportResourceScopeStub{
			allowed: true,
		},
		Formatters: []ReportFormatter{reportFormatterStub{calls: &calls}},
		Exporter:   reportExporterStub{calls: &calls},
		Audit:      &reportAuditSinkStub{calls: &calls},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	report, err := service.GenerateOptimizationReport(context.Background(), OptimizationReportInput{
		ProjectID: uuid.New(), ResourceUID: resourceUID,
		From: now, To: now.Add(time.Hour), Format: ReportFormatJSON, RequestedBy: uuid.New(),
	})
	if err != nil {
		t.Fatalf("GenerateOptimizationReport() error = %v", err)
	}
	if !equalStrings(provider.calls, []string{"recommendations"}) {
		t.Fatalf("provider calls = %v, want recommendation read only", provider.calls)
	}
	for _, section := range report.Sections {
		switch section.Key {
		case ReportSectionKeyMetrics, ReportSectionCostOverview, ReportSectionUtilizationAnalysis,
			ReportSectionRiskAssessment, ReportSectionEvidence, ReportSectionAuditTrace:
			t.Errorf("section %q should be omitted when its include option is false", section.Key)
		}
	}
	if reportSection(report, ReportSectionRecommendations).Recommendations[0].Risk != nil {
		t.Fatal("recommendation risk should be omitted when risks are disabled")
	}
	if reportSection(report, ReportSectionRecommendations).Recommendations[0].EvidenceCount != 3 {
		t.Fatal("recommendation evidence count should be retained when evidence details are disabled")
	}
}

func TestBuildOptimizationReportOrganizesSummaryCostUtilizationAndOpportunities(t *testing.T) {
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	resourceUID := uuid.New()
	firstUID, secondUID := uuid.New(), uuid.New()
	headroom := 42.0
	percentUnit := "percent"
	analytics := AnalyticsReportData{
		ResourceUID: resourceUID,
		Period:      ReportPeriod{Start: now, End: now.Add(24 * time.Hour)},
		DataQuality: ReportDataQuality{Status: "degraded", Missing: []string{"forecast"}},
		Metrics: []ReportMetric{
			{Code: "cpu_p95", Value: 70.0, Source: ReportSourceAnalytics},
			{Code: "cpu_p99", Value: 90.0, Source: ReportSourceAnalytics},
		},
		ObservedCost: []ReportMetric{
			{Code: "actual_monthly_cost", Value: 1000.0, Source: ReportSourceAnalytics},
			{Code: "projected_monthly_cost", Value: 850.0, Source: ReportSourceAnalytics},
		},
		Trends: []ReportMetric{{Code: "cpu_avg_slope", Value: 0.2, Source: ReportSourceAnalytics}},
		Utilization: []ReportMetric{{
			Code: "peak_hours", Value: []int{9, 10}, Source: ReportSourceAnalytics,
		}},
	}
	recommendations := []RecommendationReportData{
		{
			ReportRecommendation: ReportRecommendation{
				RecommendationUID: firstUID,
				EstimatedSaving: EstimatedSavingView{
					CurrentMonthlyCost: 1000, ProposedMonthlyCost: 900, MonthlyAmount: 100,
					Currency: "USD", CurrentCostBasis: "actual_cost_daily",
				},
				Risk:       &RiskView{Score: 0.4, Level: "medium", Variability: 0.2},
				Confidence: ConfidenceView{Score: 0.8},
			},
			Evidence: []ReportEvidence{{
				Type: "headroom", Value: &headroom, Unit: &percentUnit,
				Source: ReportSourceRecommendation, Reference: "recommendation.recommendation_evidence/1",
			}},
		},
		{
			ReportRecommendation: ReportRecommendation{
				RecommendationUID: secondUID,
				EstimatedSaving: EstimatedSavingView{
					CurrentMonthlyCost: 1000, ProposedMonthlyCost: 850, MonthlyAmount: 50,
					Currency: "USD", CurrentCostBasis: "actual_cost_daily",
				},
				Risk:       &RiskView{Score: 0.8, Level: "critical", Variability: 0.6},
				Confidence: ConfidenceView{Score: 0.6},
			},
		},
	}
	report := buildOptimizationReport(
		uuid.New(),
		OptimizationReportInput{
			ProjectID: uuid.New(), ResourceUID: resourceUID, Format: ReportFormatJSON,
			IncludeMetrics: true, IncludeEvidence: true, IncludeRisks: true,
		},
		ReportPeriod{Start: now, End: now.Add(24 * time.Hour)},
		analytics, recommendations, nil, now,
	)

	summary := reportSection(report, ReportSectionExecutiveSummary).Data
	if summary["current_monthly_cost"] != 1000.0 ||
		summary["estimated_monthly_saving"] != nil ||
		summary["recommendation_count"] != 2 ||
		summary["highest_risk_level"] != "critical" ||
		summary["average_or_selected_confidence"] != 0.7 {
		t.Fatalf("executive summary data = %#v", summary)
	}
	if len(summary["estimated_monthly_saving_by_recommendation"].([]map[string]any)) != 2 {
		t.Fatalf("alternative savings were not kept separate: %#v", summary)
	}
	singleSummary := buildExecutiveSummaryData(
		OptimizationReportInput{ResourceUID: resourceUID, IncludeRisks: true},
		ReportPeriod{Start: now, End: now.Add(24 * time.Hour)},
		analytics, recommendations[:1],
	)
	if singleSummary["estimated_monthly_saving"] != 100.0 ||
		singleSummary["estimated_monthly_saving_currency"] != "USD" {
		t.Fatalf("single selected recommendation saving = %#v", singleSummary)
	}

	cost := reportSection(report, ReportSectionCostOverview).Data
	if cost["currency"] != "USD" || cost["estimated_monthly_cost"] != 850.0 {
		t.Fatalf("cost overview data = %#v", cost)
	}
	if cost["estimated_monthly_saving"] != nil ||
		len(cost["recommendation_cost_estimates"].([]map[string]any)) != 2 {
		t.Fatalf("cost overview combined mutually exclusive recommendation savings: %#v", cost)
	}
	if baseCost, ok := cost["base_cost"].(map[string]any); !ok ||
		baseCost["source"] != ReportSourceAnalytics || baseCost["amount"] != 1000.0 {
		t.Fatalf("base cost = %#v, want observed Analytics cost", cost["base_cost"])
	}

	utilization := reportSection(report, ReportSectionUtilizationAnalysis).Data
	if len(utilization["cpu_p95"].([]ReportMetric)) != 1 ||
		len(utilization["cpu_p99"].([]ReportMetric)) != 1 ||
		len(utilization["variability"].([]map[string]any)) != 2 ||
		len(utilization["headroom"].([]ReportEvidence)) != 1 ||
		utilization["data_coverage"].(map[string]any)["status"] != "degraded" {
		t.Fatalf("utilization data = %#v", utilization)
	}
	if got := reportSection(report, ReportSectionOptimizationOpportunities).Recommendations; len(got) != 2 ||
		got[0].RecommendationUID != firstUID || got[1].RecommendationUID != secondUID {
		t.Fatalf("optimization opportunities changed recommendation order or content: %#v", got)
	}
	if hasReportSection(report, ReportSectionAuditTrace) {
		t.Fatal("audit trace section was included without an audit request")
	}
}

func reportSection(report Report, key string) ReportSection {
	for _, section := range report.Sections {
		if section.Key == key {
			return section
		}
	}
	return ReportSection{}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestQueueOptimizationReportCreatesQueuedJob(t *testing.T) {
	now := time.Now().UTC()
	projectID := uuid.New()
	userID := uuid.New()
	resourceUID := uuid.New()
	repository := &reportRepositoryStub{jobs: make(map[uuid.UUID]ReportGenerationJob)}
	service, err := NewService(Dependencies{
		DataProvider:  &reportDataProviderStub{},
		Repository:    repository,
		ResourceScope: reportResourceScopeStub{allowed: true},
		Formatters:    []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:      reportExporterStub{calls: &[]string{}},
		Audit:         &reportAuditSinkStub{calls: &[]string{}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	job, err := service.QueueOptimizationReport(context.Background(), OptimizationReportInput{
		ProjectID:   projectID, ResourceUID: resourceUID,
		From: now, To: now.Add(time.Hour), Format: ReportFormatJSON,
		RequestedBy: userID,
	})
	if err != nil {
		t.Fatalf("QueueOptimizationReport() error = %v", err)
	}
	if job.Status != ReportJobStatusQueued || job.ReportUID == nil || *job.ReportUID == uuid.Nil {
		t.Fatalf("job not queued: %#v", job)
	}
	if repository.job.Status != ReportJobStatusQueued {
		t.Fatalf("repository job status = %s, want queued", repository.job.Status)
	}
}

func TestGetJobStatusReturnsPersistedStatus(t *testing.T) {
	jobUID := uuid.New()
	repository := &reportRepositoryStub{jobs: make(map[uuid.UUID]ReportGenerationJob)}
	repository.jobs[jobUID] = ReportGenerationJob{
		UID:     jobUID, Status: ReportJobStatusRunning, ReportUID: &jobUID,
	}
	service, err := NewService(Dependencies{
		DataProvider:  &reportDataProviderStub{},
		Repository:    repository,
		ResourceScope: reportResourceScopeStub{allowed: true},
		Formatters:    []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:      reportExporterStub{calls: &[]string{}},
		Audit:         &reportAuditSinkStub{calls: &[]string{}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	status, err := service.GetJobStatus(context.Background(), jobUID)
	if err != nil {
		t.Fatalf("GetJobStatus() error = %v", err)
	}
	if status != ReportJobStatusRunning {
		t.Fatalf("status = %s, want running", status)
	}
}

func TestCancelJobCancelsQueuedJob(t *testing.T) {
	jobUID := uuid.New()
	repository := &reportRepositoryStub{jobs: make(map[uuid.UUID]ReportGenerationJob)}
	repository.jobs[jobUID] = ReportGenerationJob{
		UID:     jobUID, Status: ReportJobStatusQueued, ReportUID: &jobUID,
	}
	service, err := NewService(Dependencies{
		DataProvider:  &reportDataProviderStub{},
		Repository:    repository,
		ResourceScope: reportResourceScopeStub{allowed: true},
		Formatters:    []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:      reportExporterStub{calls: &[]string{}},
		Audit:         &reportAuditSinkStub{calls: &[]string{}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.CancelJob(context.Background(), jobUID); err != nil {
		t.Fatalf("CancelJob() error = %v", err)
	}
	if repository.job.Status != ReportJobStatusCancelled || repository.job.CompletedAt == nil {
		t.Fatalf("job not cancelled: %#v", repository.job)
	}
}

func TestCancelJobRejectsTerminalJob(t *testing.T) {
	jobUID := uuid.New()
	repository := &reportRepositoryStub{jobs: make(map[uuid.UUID]ReportGenerationJob)}
	repository.jobs[jobUID] = ReportGenerationJob{
		UID:     jobUID, Status: ReportJobStatusCompleted, ReportUID: &jobUID,
	}
	service, err := NewService(Dependencies{
		DataProvider:  &reportDataProviderStub{},
		Repository:    repository,
		ResourceScope: reportResourceScopeStub{allowed: true},
		Formatters:    []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:      reportExporterStub{calls: &[]string{}},
		Audit:         &reportAuditSinkStub{calls: &[]string{}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.CancelJob(context.Background(), jobUID); err == nil {
		t.Fatal("CancelJob() error = nil, want terminal job error")
	}
}

func TestProcessJobExecutesQueuedJob(t *testing.T) {
	now := time.Now().UTC()
	projectID := uuid.New()
	userID := uuid.New()
	resourceUID := uuid.New()
	jobUID := uuid.New()
	repository := &reportRepositoryStub{jobs: make(map[uuid.UUID]ReportGenerationJob)}
	repository.jobs[jobUID] = ReportGenerationJob{
		UID:         jobUID,
		ProjectID:   projectID,
		RequestedBy: userID,
		ReportType:  ReportTypeOptimization,
		Parameters: map[string]any{
			"resource_uid":  resourceUID.String(),
			"period_start":  now.Format(time.RFC3339),
			"period_end":    now.Add(time.Hour).Format(time.RFC3339),
			"format":        string(ReportFormatJSON),
			"include_audit": false,
			"title":         "async report",
		},
		Status: ReportJobStatusQueued, ReportUID: &jobUID,
	}
	service, err := NewService(Dependencies{
		DataProvider:  &reportDataProviderStub{},
		Repository:    repository,
		ResourceScope: reportResourceScopeStub{allowed: true},
		Formatters:    []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:      reportExporterStub{calls: &[]string{}},
		Audit:         &reportAuditSinkStub{calls: &[]string{}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.ProcessJob(context.Background(), jobUID); err != nil {
		t.Fatalf("ProcessJob() error = %v", err)
	}
	if repository.job.Status != ReportJobStatusCompleted {
		t.Fatalf("job not completed after process: %s", repository.job.Status)
	}
}

func TestProcessPendingJobsProcessesQueuedJobs(t *testing.T) {
	now := time.Now().UTC()
	projectID := uuid.New()
	userID := uuid.New()
	resourceUID := uuid.New()
	jobUID1, jobUID2 := uuid.New(), uuid.New()
	repository := &reportRepositoryStub{jobs: make(map[uuid.UUID]ReportGenerationJob)}
	repository.jobs[jobUID1] = ReportGenerationJob{
		UID:         jobUID1, ProjectID: projectID, RequestedBy: userID,
		ReportType:  ReportTypeOptimization,
		Parameters: map[string]any{
			"resource_uid": resourceUID.String(),
			"period_start": now.Format(time.RFC3339),
			"period_end":   now.Add(time.Hour).Format(time.RFC3339),
			"format":       string(ReportFormatJSON),
		},
		Status: ReportJobStatusQueued, ReportUID: &jobUID1,
	}
	repository.jobs[jobUID2] = ReportGenerationJob{
		UID:         jobUID2, ProjectID: projectID, RequestedBy: userID,
		ReportType:  ReportTypeOptimization,
		Parameters: map[string]any{
			"resource_uid": resourceUID.String(),
			"period_start": now.Format(time.RFC3339),
			"period_end":   now.Add(time.Hour).Format(time.RFC3339),
			"format":       string(ReportFormatJSON),
		},
		Status: ReportJobStatusQueued, ReportUID: &jobUID2,
	}
	service, err := NewService(Dependencies{
		DataProvider:  &reportDataProviderStub{},
		Repository:    repository,
		ResourceScope: reportResourceScopeStub{allowed: true},
		Formatters:    []ReportFormatter{reportFormatterStub{calls: &[]string{}}},
		Exporter:      reportExporterStub{calls: &[]string{}},
		Audit:         &reportAuditSinkStub{calls: &[]string{}},
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if err := service.ProcessPendingJobs(context.Background()); err != nil {
		t.Fatalf("ProcessPendingJobs() error = %v", err)
	}
	if repository.jobs[jobUID1].Status != ReportJobStatusCompleted ||
		repository.jobs[jobUID2].Status != ReportJobStatusCompleted {
		t.Fatalf("pending jobs not completed: %s / %s", repository.jobs[jobUID1].Status, repository.jobs[jobUID2].Status)
	}
}
