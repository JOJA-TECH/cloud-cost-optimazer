package reporting

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ReportGenerator interface {
	Generate(ctx context.Context, input GenerateReportInput) (Report, error)
	GenerateOptimizationReport(ctx context.Context, input OptimizationReportInput) (Report, error)
	GetReport(ctx context.Context, reportUID uuid.UUID) (Report, error)
}

// ReportDataProvider is Reporting's only entry point to producer modules.
type ReportDataProvider interface {
	GetAnalytics(ctx context.Context, input AnalyticsReportQuery) (AnalyticsReportData, error)
	GetRecommendations(ctx context.Context, input RecommendationReportQuery) ([]RecommendationReportData, error)
	GetRecommendationTrace(ctx context.Context, recommendationUID uuid.UUID) (RecommendationTraceData, error)
}

// AnalyticsReportReader aggregates existing analytical results without
// introducing new business calculations.
type AnalyticsReportReader interface {
	BuildReportData(
		ctx context.Context,
		resourceUID uuid.UUID,
		from, to time.Time,
	) (AnalyticsReportData, error)
}

type RecommendationReader interface {
	GetRecommendation(ctx context.Context, recommendationUID uuid.UUID) (RecommendationReportData, error)
	ListRecommendations(ctx context.Context, filter RecommendationFilter) ([]RecommendationReportData, error)
}

type AuditTraceReader interface {
	GetRecommendationTrace(ctx context.Context, recommendationUID uuid.UUID) (RecommendationTraceData, error)
}

type ReportFormatter interface {
	Format(ctx context.Context, report Report) (FormattedReport, error)
	Supports(format ReportFormat) bool
}

type ReportExportAdapter interface {
	Export(ctx context.Context, report FormattedReport) (ReportArtifact, error)
}

// ReportObjectStorage keeps export adapters independent of storage vendors and SDKs.
type ReportObjectStorage interface {
	Store(ctx context.Context, object ReportStorageObject) (StoredReportObject, error)
}

type ReportRepository interface {
	CreateJob(ctx context.Context, job ReportGenerationJob) (ReportGenerationJob, error)
	UpdateJob(ctx context.Context, job ReportGenerationJob) error
	GetJob(ctx context.Context, jobUID uuid.UUID) (ReportGenerationJob, error)
	SaveArtifact(
		ctx context.Context,
		reportRequestUID uuid.UUID,
		artifact ReportArtifact,
	) (ReportArtifact, error)
	GetArtifact(ctx context.Context, artifactUID uuid.UUID) (ReportArtifact, error)
}

type GenerateReportInput struct {
	ProjectID   uuid.UUID
	ResourceUID *uuid.UUID
	ReportType  ReportType
	Period      ReportPeriod
	Format      ReportFormat
	RequestedBy uuid.UUID
	Parameters  map[string]any
}

type OptimizationReportInput struct {
	ProjectID   uuid.UUID
	ResourceUID uuid.UUID
	Period      ReportPeriod
	Format      ReportFormat
	RequestedBy uuid.UUID
	Parameters  map[string]any
}

type AnalyticsReportQuery struct {
	ResourceUID uuid.UUID
	From        time.Time
	To          time.Time
}

type RecommendationReportQuery struct {
	ResourceUID *uuid.UUID
	Status      string
	Limit       int
	Offset      int
}

type RecommendationFilter struct {
	ResourceUID *uuid.UUID
	Status      string
	Limit       int
	Offset      int
}

type AnalyticsReportData struct {
	ResourceUID      uuid.UUID
	Period           ReportPeriod
	Metrics          []ReportMetric
	ObservedCost     []ReportMetric
	Trends           []ReportMetric
	Utilization      []ReportMetric
	Forecasts        []ReportMetric
	Evidence         []ReportEvidence
	SourceReferences []ReportSourceReference
}

type RecommendationReportData struct {
	ReportRecommendation
	ResourceUID uuid.UUID
	Period      ReportPeriod
}

type RecommendationTraceData struct {
	RecommendationUID uuid.UUID
	DecisionTraces    []RecommendationDecisionTrace
	Events            []ReportTraceEvent
}

type RecommendationDecisionTrace struct {
	TraceUID           uuid.UUID
	Action             string
	Status             string
	RiskScore          *float64
	RiskLevel          *string
	ConfidenceScore    *float64
	EstimatedSaving    *float64
	EvidenceCount      int
	EvidenceReferences []string
	Decision           string
	Rationale          string
	CreatedAt          time.Time
}

type ReportTraceEvent struct {
	EventID       int64
	Action        string
	OccurredAt    time.Time
	RequestID     *uuid.UUID
	CorrelationID *uuid.UUID
	OperationID   *uuid.UUID
}

type FormattedReport struct {
	Format      ReportFormat
	ContentType string
	Filename    string
	Body        []byte
	SizeBytes   int64
	Checksum    string
}

type ReportStorageObject struct {
	Filename    string
	ContentType string
	Body        []byte
}

type StoredReportObject struct {
	StorageURI     string
	MIMEType       string
	FileSizeBytes  int64
	ChecksumSHA256 string
}
