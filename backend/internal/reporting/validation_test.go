package reporting

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateOptimizationReportInputBoundsPeriodAndRecommendationSelection(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	valid := OptimizationReportInput{
		ProjectID: uuid.New(), ResourceUID: uuid.New(),
		From: from, To: from.Add(maxReportPeriod), Format: ReportFormatJSON,
		RequestedBy: uuid.New(),
	}
	if err := ValidateOptimizationReportInput(valid); err != nil {
		t.Fatalf("ValidateOptimizationReportInput() rejected maximum valid period: %v", err)
	}
	valid.To = from.Add(maxReportPeriod + time.Second)
	if err := ValidateOptimizationReportInput(valid); err == nil {
		t.Fatal("ValidateOptimizationReportInput() accepted a period longer than 365 days")
	}

	valid.To = from.Add(time.Hour)
	valid.RecommendationUIDs = make([]uuid.UUID, maxExplicitReportRecommendations+1)
	for i := range valid.RecommendationUIDs {
		valid.RecommendationUIDs[i] = uuid.New()
	}
	if err := ValidateOptimizationReportInput(valid); err == nil {
		t.Fatal("ValidateOptimizationReportInput() accepted too many selected recommendations")
	}

	valid.RecommendationUIDs = []uuid.UUID{uuid.New(), uuid.Nil}
	if err := ValidateOptimizationReportInput(valid); err == nil {
		t.Fatal("ValidateOptimizationReportInput() accepted a zero recommendation UID")
	}
}

func TestValidateGenerateReportInputRequiresResourceAndSupportedType(t *testing.T) {
	from := time.Now().UTC()
	resourceUID := uuid.New()
	valid := GenerateReportInput{
		ProjectID: uuid.New(), ResourceUID: &resourceUID, ReportType: ReportTypeOptimization,
		From: from, To: from.Add(time.Hour), Format: ReportFormatJSON, RequestedBy: uuid.New(),
	}
	if err := ValidateGenerateReportInput(valid); err != nil {
		t.Fatalf("ValidateGenerateReportInput() error = %v", err)
	}

	valid.ResourceUID = nil
	if err := ValidateGenerateReportInput(valid); err == nil {
		t.Fatal("ValidateGenerateReportInput() accepted a missing resource")
	}
	valid.ResourceUID = &resourceUID
	valid.ReportType = ReportTypeAnalytics
	if err := ValidateGenerateReportInput(valid); err == nil {
		t.Fatal("ValidateGenerateReportInput() accepted an unsupported report type")
	}
}

func TestGenerateMapsGeneralInputContractToOptimizationReport(t *testing.T) {
	service, err := newHandlerTestService()
	if err != nil {
		t.Fatalf("newHandlerTestService() error = %v", err)
	}
	now := time.Now().UTC()
	projectID, resourceUID, userID := uuid.New(), uuid.New(), uuid.New()
	report, err := service.Generate(t.Context(), GenerateReportInput{
		ProjectID: projectID, ResourceUID: &resourceUID, ReportType: ReportTypeOptimization,
		From: now, To: now.Add(time.Hour), Format: ReportFormatJSON,
		IncludeMetrics: false, IncludeEvidence: false, IncludeAudit: false, IncludeRisks: false,
		Title: "Custom title", RequestedBy: userID,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if report.ProjectID != projectID || report.GeneratedBy != userID || report.Title != "Custom title" {
		t.Fatalf("Generate() report identity/title = %#v", report)
	}
	for _, section := range report.Sections {
		if section.Key == ReportSectionKeyMetrics || section.Key == ReportSectionAuditTrace ||
			section.Key == ReportSectionEvidence || section.Key == ReportSectionRiskAssessment {
			t.Errorf("Generate() included disabled section %q", section.Key)
		}
	}
}
