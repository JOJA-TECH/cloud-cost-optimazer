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
}

func (s *reportRepositoryStub) CreateJob(_ context.Context, job ReportGenerationJob) (ReportGenerationJob, error) {
	s.calls = append(s.calls, "create_job")
	s.job = job
	return job, nil
}

func (s *reportRepositoryStub) UpdateJob(_ context.Context, job ReportGenerationJob) error {
	s.calls = append(s.calls, "update_job:"+string(job.Status))
	s.job = job
	return nil
}

func (s *reportRepositoryStub) GetJob(context.Context, uuid.UUID) (ReportGenerationJob, error) {
	return ReportGenerationJob{}, ErrReportNotFound
}

func (s *reportRepositoryStub) SaveArtifact(_ context.Context, _ uuid.UUID, artifact ReportArtifact) (ReportArtifact, error) {
	s.calls = append(s.calls, "save_artifact")
	return artifact, nil
}

func (s *reportRepositoryStub) GetArtifact(context.Context, uuid.UUID) (ReportArtifact, error) {
	return ReportArtifact{}, ErrReportNotFound
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
		UID: uuid.New(), StorageURI: "memory://report", MIMEType: "application/json",
		FileSizeBytes: &size, CreatedAt: time.Now().UTC(),
	}, nil
}

type reportAuditSinkStub struct {
	calls  *[]string
	job    ReportGenerationJob
	report Report
	err    error
}

func (s *reportAuditSinkStub) RecordReportGenerated(_ context.Context, job ReportGenerationJob, report Report) error {
	*s.calls = append(*s.calls, "audit")
	s.job, s.report = job, report
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
		IncludeTrace: true, IncludeEvidence: true, IncludeRisks: true, IncludeMetrics: true,
	})
	if err != nil {
		t.Fatalf("GenerateOptimizationReport() error = %v", err)
	}
	wantCalls := []string{"format", "export", "audit"}
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
	if !report.Sections[5].Recommendations[0].TraceAvailable {
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
	if len(calls) != 0 {
		t.Fatalf("formatter/export/audit called after analytics failure: %v", calls)
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
	if got := report.Sections[2].Recommendations; len(got) != 1 || got[0].RecommendationUID != selectedUID {
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
	if report.Sections[2].Recommendations[0].Risk != nil {
		t.Fatal("recommendation risk should be omitted when risks are disabled")
	}
	if report.Sections[2].Recommendations[0].EvidenceCount != 0 {
		t.Fatal("recommendation evidence count should be omitted when evidence is disabled")
	}
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
