package reporting

import (
	"time"

	"github.com/google/uuid"
)

type ReportType string

type ReportFormat string

const (
	ReportFormatJSON ReportFormat = "json"
	ReportFormatCSV  ReportFormat = "csv"
)

type ReportStatus string

const (
	ReportStatusPending    ReportStatus = "pending"
	ReportStatusGenerating ReportStatus = "generating"
	ReportStatusCompleted  ReportStatus = "completed"
	ReportStatusFailed     ReportStatus = "failed"
	ReportStatusCancelled  ReportStatus = "cancelled"
)

type ReportJobStatus string

const (
	ReportJobStatusQueued    ReportJobStatus = "queued"
	ReportJobStatusRunning   ReportJobStatus = "running"
	ReportJobStatusCompleted ReportJobStatus = "completed"
	ReportJobStatusFailed    ReportJobStatus = "failed"
	ReportJobStatusCancelled ReportJobStatus = "cancelled"
)

type ReportSource string

const (
	ReportSourceAnalytics      ReportSource = "analytics"
	ReportSourceRecommendation ReportSource = "recommendation"
	ReportSourceAudit          ReportSource = "audit"
)

type ReportMetadata map[string]any

// Report is the structured report document; its artifact is only an export of it.
type Report struct {
	UID              uuid.UUID               `json:"uid"`
	ProjectID        uuid.UUID               `json:"project_id"`
	ResourceUID      *uuid.UUID              `json:"resource_uid,omitempty"`
	ReportType       ReportType              `json:"report_type"`
	Title            string                  `json:"title"`
	Description      string                  `json:"description"`
	Period           ReportPeriod            `json:"period"`
	Format           ReportFormat            `json:"format"`
	Status           ReportStatus            `json:"status"`
	Sections         []ReportSection         `json:"sections"`
	Metadata         ReportMetadata          `json:"metadata"`
	SourceReferences []ReportSourceReference `json:"source_references"`
	GeneratedBy      uuid.UUID               `json:"generated_by"`
	CreatedAt        time.Time               `json:"created_at"`
	CompletedAt      *time.Time              `json:"completed_at,omitempty"`
	Artifact         *ReportArtifact         `json:"artifact,omitempty"`
}

type ReportPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type ReportSection struct {
	Key             string                 `json:"key"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	Order           int                    `json:"order"`
	Metrics         []ReportMetric         `json:"metrics"`
	Recommendations []ReportRecommendation `json:"recommendations"`
	Evidence        []ReportEvidence       `json:"evidence"`
	Data            map[string]any         `json:"data"`
}

type ReportMetric struct {
	Code    string       `json:"code"`
	Label   string       `json:"label"`
	Value   any          `json:"value"`
	Unit    string       `json:"unit"`
	Source  ReportSource `json:"source"`
	Period  ReportPeriod `json:"period"`
	Quality *float64     `json:"quality,omitempty"`
}

type ReportRecommendation struct {
	RecommendationUID uuid.UUID           `json:"recommendation_uid"`
	Action            string              `json:"action"`
	Description       string              `json:"description"`
	Status            string              `json:"status"`
	EstimatedSaving   EstimatedSavingView `json:"estimated_saving"`
	Risk              RiskView            `json:"risk"`
	Confidence        ConfidenceView      `json:"confidence"`
	Rationale         string              `json:"rationale"`
	EvidenceCount     int                 `json:"evidence_count"`
	TraceAvailable    bool                `json:"trace_available"`
}

type EstimatedSavingView struct {
	CurrentMonthlyCost  float64   `json:"current_monthly_cost"`
	ProposedMonthlyCost float64   `json:"proposed_monthly_cost"`
	MonthlyAmount       float64   `json:"monthly_amount"`
	MonthlyPercentage   float64   `json:"monthly_percentage"`
	Currency            string    `json:"currency"`
	PeriodStart         time.Time `json:"period_start"`
	PeriodEnd           time.Time `json:"period_end"`
}

type RiskView struct {
	Score float64 `json:"score"`
	Level string  `json:"level"`
}

type ConfidenceView struct {
	Score float64 `json:"score"`
}

type ReportEvidence struct {
	Type        string         `json:"type"`
	MetricCode  *string        `json:"metric_code,omitempty"`
	Value       *float64       `json:"value,omitempty"`
	Unit        *string        `json:"unit,omitempty"`
	Source      ReportSource   `json:"source"`
	WindowStart *time.Time     `json:"window_start,omitempty"`
	WindowEnd   *time.Time     `json:"window_end,omitempty"`
	Reference   string         `json:"reference"`
	Context     map[string]any `json:"context"`
}

type ReportSourceReference struct {
	Source    ReportSource   `json:"source"`
	Reference string         `json:"reference"`
	Context   map[string]any `json:"context"`
}

type ReportArtifact struct {
	UID            uuid.UUID  `json:"uid"`
	StorageURI     string     `json:"storage_uri"`
	MIMEType       string     `json:"mime_type"`
	FileSizeBytes  *int64     `json:"file_size_bytes,omitempty"`
	ChecksumSHA256 *string    `json:"checksum_sha256,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

// ReportGenerationJob is the domain view of app.report_requests. It does not
// introduce a separate persistence table.
type ReportGenerationJob struct {
	UID          uuid.UUID       `json:"uid"`
	ProjectID    uuid.UUID       `json:"project_id"`
	RequestedBy  uuid.UUID       `json:"requested_by"`
	ReportType   ReportType      `json:"report_type"`
	Parameters   map[string]any  `json:"parameters"`
	Status       ReportJobStatus `json:"status"`
	ReportUID    *uuid.UUID      `json:"report_uid,omitempty"`
	ArtifactUID  *uuid.UUID      `json:"artifact_uid,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`
}
