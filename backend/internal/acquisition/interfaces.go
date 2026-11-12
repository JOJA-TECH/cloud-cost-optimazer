package acquisition

import (
	"context"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/google/uuid"
)

// Quality statuses assigned by the normalizer after bounds validation.
const (
	QualityUsable       = "usable"
	QualityDegraded     = "degraded"
	QualityInvalid      = "invalid"
	QualityNotAvailable = "not_available"
)

// Acquisition sources. They mirror the values allowed in
// cco_data.raw.collector_run.source.
const (
	SourceAzureMonitor = "azure_monitor_metrics"
	SourceSQLDiag      = "sql_diagnostics"
	SourceQueryStore   = "query_store"
)

// RawReading is one normalized observation ready for validation and
// persistence into cco_data.raw.metric_reading. MetricCode must be a code
// declared in cco_data.catalog.metric_catalog; providers drop unknown raw
// names (they are logged) instead of inventing codes.
type RawReading struct {
	MetricCode      string
	Unit            string
	Timestamp       time.Time // UTC observation timestamp
	IntervalSeconds int
	Average         *float64
	Minimum         *float64
	Maximum         *float64
	Total           *float64
	Count           *float64
	Source          string

	// Set by ValidateReading.
	Quality  string
	Warnings []string
}

// DBResourceStat mirrors cco_data.raw.db_resource_stats (sys.dm_db_resource_stats).
type DBResourceStat struct {
	EndTimeUTC            time.Time
	AvgCPUPercent         *float64
	AvgDataIOPercent      *float64
	AvgLogWritePercent    *float64
	AvgMemoryUsagePercent *float64
	AvgInstanceCPUPercent *float64
	DTULimit              *float64
	CPULimit              *float64
}

// QueryStoreQuery mirrors cco_data.raw.query_store_query.
type QueryStoreQuery struct {
	QueryID         int64
	QueryTextID     *int64
	QueryHash       *string
	QuerySQLText    *string
	IsInternalQuery *bool
	SnapshotAtUTC   time.Time
}

// QueryStoreRuntimeStat mirrors cco_data.raw.query_store_runtime_stats.
// Durations are in microseconds (native Query Store units).
type QueryStoreRuntimeStat struct {
	QueryID            int64
	IntervalStartUTC   time.Time
	IntervalEndUTC     time.Time
	CountExecutions    *int64
	AvgDurationUS      *float64
	AvgCPUTimeUS       *float64
	AvgLogicalIOReads  *float64
	AvgLogicalIOWrites *float64
	SnapshotAtUTC      time.Time
}

// Target is a monitored resource: a cco_app.cloud.resource_bindings row with
// is_monitored=true joined with its cloud.resources row.
type Target struct {
	ResourceUID     uuid.UUID // contract UUID shared with cco_data.core.resource
	CloudAccountID  uuid.UUID
	AzureResourceID string
	SubscriptionID  string
	ResourceGroup   string
	ServerName      string
	DatabaseName    string
	RegionCode      string
	Kind            *string
}

// SQLConfig holds the workload login for direct SQL diagnostics (DMVs and
// Query Store) against the monitored database. It requires a login with
// VIEW DATABASE STATE.
// shortcut: one credential per environment; upgrade: per-resource credentials.
type SQLConfig struct {
	Server   string // host or host:port of the Azure SQL server
	Database string
	User     string
	Password string
}

// MetricsProvider reads platform metrics from the provider control plane
// (Azure Monitor REST, PT1M granularity, read-only).
type MetricsProvider interface {
	FetchPlatformMetrics(ctx context.Context, creds cloud.ConnectionCredentials, azureResourceID string, start, end time.Time) ([]RawReading, error)
}

// DiagnosticsProvider reads SQL DMVs over a direct connection.
// lastSeen is the incremental cursor: only rows with end_time > lastSeen are
// returned (the cursor is owned and advanced by the service).
type DiagnosticsProvider interface {
	FetchResourceStats(ctx context.Context, sql SQLConfig, t Target, lastSeen time.Time) ([]DBResourceStat, []RawReading, error)
}

// QueryStoreProvider reads Query Store runtime stats over a direct connection.
type QueryStoreProvider interface {
	FetchRuntimeStats(ctx context.Context, sql SQLConfig, t Target) ([]QueryStoreQuery, []QueryStoreRuntimeStat, error)
}

// TargetRepository reads monitored targets and account credentials from
// cco_app (cloud schema). orgID == nil selects every monitored target (system
// scheduler); non-nil restricts to the organization (manual trigger / API).
type TargetRepository interface {
	ListMonitoredTargets(ctx context.Context, orgID *uuid.UUID) ([]Target, error)
	ResolveCredentials(ctx context.Context, cloudAccountID uuid.UUID) (cloud.ConnectionCredentials, error)
}

// RunFilter selects collector runs for API queries.
type RunFilter struct {
	ResourceUID *uuid.UUID
	Source      string
	Status      string
	Limit       int // default 50 when <= 0
}

// AcquisitionRunDTO is the API-facing view of cco_data.raw.collector_run.
type AcquisitionRunDTO struct {
	CollectorRunID int64      `json:"collector_run_id"`
	ResourceUID    *uuid.UUID `json:"resource_uid,omitempty"`
	Source         string     `json:"source"`
	Scope          string     `json:"scope,omitempty"`
	StartedAtUTC   int64      `json:"started_at_utc"` // unix ms
	FinishedAtUTC  *int64     `json:"finished_at_utc,omitempty"`
	Status         string     `json:"status"`
	RowsIngested   *int       `json:"rows_ingested,omitempty"`
	ErrorMessage   string     `json:"error_message,omitempty"`
}

// AcquisitionRepository persists acquisition data in cco_data (raw schema)
// and resolves the cco_app contract UUID against the internal resource_id.
type AcquisitionRepository interface {
	// EnsureResource upserts cco_data.core.resource by azure_resource_id
	// preserving the contract resource_uid and returns the internal id.
	EnsureResource(ctx context.Context, t Target) (int64, error)
	// StartRun opens a raw.collector_run row (status partial).
	StartRun(ctx context.Context, source string, resourceID int64, scope string) (int64, error)
	// FinishRun closes the run with ok/partial/error.
	FinishRun(ctx context.Context, runID int64, status string, rowsIngested int, errMsg string) error
	// InsertMetricReadings upserts raw.metric_reading idempotently
	// (ON CONFLICT DO NOTHING on the PK); returns rows inserted.
	InsertMetricReadings(ctx context.Context, resourceID int64, readings []RawReading) (int, error)
	// InsertResourceStats upserts raw.db_resource_stats idempotently.
	InsertResourceStats(ctx context.Context, resourceID int64, stats []DBResourceStat) (int, error)
	// InsertQueryStore upserts raw.query_store_query and
	// raw.query_store_runtime_stats idempotently.
	InsertQueryStore(ctx context.Context, resourceID int64, queries []QueryStoreQuery, stats []QueryStoreRuntimeStat) (int, error)
	// ListRuns returns collector runs, newest first, optionally filtered.
	// When filter.ResourceUID is nil it returns runs for the given uids only
	// (empty uids list = empty result; never an unscoped query).
	ListRuns(ctx context.Context, uids []uuid.UUID, filter RunFilter) ([]AcquisitionRunDTO, error)
	// GetRun returns one run; ErrRunNotFound when missing.
	GetRun(ctx context.Context, id int64) (AcquisitionRunDTO, error)
}
