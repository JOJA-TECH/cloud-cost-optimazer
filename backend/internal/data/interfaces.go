// Package data implements MODULES.md module 4 (Gestión de Datos): the single
// gateway to cco_data. It stores and serves historical metrics, cost records
// and configuration snapshots, and checks data availability/coverage (HU-05).
// It never talks to cloud SDKs and never computes features or percentiles
// (that is Analítica, module 5, on feat.*).
package data

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ResourceRef identifies a monitored resource for the cco_data.core.resource
// upsert: the contract UUID shared with cco_app.cloud.resource_bindings
// (data_resource_uid) plus the ARM identity used as natural key.
type ResourceRef struct {
	ResourceUID     uuid.UUID
	SubscriptionID  string
	ResourceGroup   string
	ServerName      string
	DatabaseName    string
	AzureResourceID string
	RegionCode      string
	Kind            *string
}

// Metric is one normalized reading ready for raw.metric_reading. MetricCode
// must be declared in cco_data.catalog.metric_catalog.
type Metric struct {
	MetricCode      string
	TimestampUTC    time.Time
	IntervalSeconds int
	AvgValue        *float64
	MinValue        *float64
	MaxValue        *float64
	TotalValue      *float64
	CountValue      *int
}

// ResourceStat mirrors cco_data.raw.db_resource_stats
// (sys.dm_db_resource_stats via Database watcher).
type ResourceStat struct {
	EndTimeUTC            time.Time
	AvgCPUPercent         *float64
	AvgDataIOPercent      *float64
	AvgLogWritePercent    *float64
	AvgMemoryUsagePercent *float64
	AvgInstanceCPUPercent *float64
	DTULimit              *float64
	CPULimit              *float64
}

// QueryStoreQueryRecord mirrors cco_data.raw.query_store_query.
type QueryStoreQueryRecord struct {
	QueryID         int64
	QueryTextID     *int64
	QueryHash       *string
	QuerySQLText    *string
	IsInternalQuery *bool
	SnapshotAtUTC   time.Time
}

// QueryStoreRuntimeRecord mirrors cco_data.raw.query_store_runtime_stats.
// Durations stay in microseconds (native Query Store units).
type QueryStoreRuntimeRecord struct {
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

// CostRecord maps core.cost_daily (canonical FOCUS-lite daily cost fact).
// SourceChargeID is the idempotency key of the unique partial index
// ux_cost_daily_source_charge; a nil value is rejected by the writer.
type CostRecord struct {
	ResourceUID    *uuid.UUID // nil: cost without a mapped database (Storage, VNet)
	ProviderCode   string
	ScopeType      string // 'subscription' | 'resource_group' | 'resource'
	ScopeName      string
	ServiceName    string
	RegionCode     *string
	UsageDate      time.Time // day (UTC)
	Currency       string
	SourceChargeID *string
	MeterID        *string
	PreTaxCost     float64
	UsageQuantity  *float64
	UsageUnit      *string
	PricingModel   *string // 'consumption'|'reservation'|'spot'|'unknown'
	ChargeCategory *string // 'usage'|'purchase'|'tax'|'adjustment'|'credit'
	Source         string  // defaults to 'azure_cost_management'
}

// ConfigurationSnapshot maps core.resource_config_snapshot (append-only ARM
// history). SKUName resolves to catalog.sql_sku.sku_id on write when known;
// unknown SKUs are stored with a NULL sku_id and the textual name.
type ConfigurationSnapshot struct {
	ResourceUID       uuid.UUID
	CollectorRunID    *int64
	SKUName           *string
	Tier              *string
	Family            *string
	CapacityVCores    *float64
	MinCapacityVCores *float64
	ComputeModel      *string // 'provisioned' | 'serverless'
	MaxSizeBytes      *int64
	AutoPauseDelayMin *int
	Status            *string
	FreeLimit         *bool
	SourcePayload     *string // ARM body verbatim (jsonb in PostgreSQL)
	ObservedAtUTC     time.Time
}

// HistoricalMetric is one point of a stored metric series.
type HistoricalMetric struct {
	TimestampUTC    time.Time
	IntervalSeconds int
	AvgValue        *float64
	MinValue        *float64
	MaxValue        *float64
	TotalValue      *float64
	CountValue      *int
}

// DataQuery selects a historical series: resource + metric + [FromUTC, ToUTC).
type DataQuery struct {
	ResourceUID uuid.UUID
	MetricCode  string
	FromUTC     time.Time
	ToUTC       time.Time
}

// CoverageReport is the HU-05 availability answer for one metric window.
type CoverageReport struct {
	Bucket          string  `json:"bucket"`            // '1h' | '1d'
	WindowsTotal    int     `json:"windows_total"`     // buckets in [from, to)
	WindowsWithData int     `json:"windows_with_data"` // buckets with >= 1 reading
	Gaps            int     `json:"gaps"`              // empty buckets
	TotalReadings   int64   `json:"total_readings"`
	CoverageRatio   float64 `json:"coverage_ratio"` // windows_with_data / windows_total
	State           string  `json:"state"`          // 'sufficient' | 'insufficient'
}

// RunFilter selects collector runs for API queries.
type RunFilter struct {
	ResourceUID *uuid.UUID
	Source      string
	Status      string
	Limit       int // default 50 when <= 0, capped at 500
}

// RunRecord is the storage view of cco_data.raw.collector_run. Presentation
// mapping (unix ms, DTOs) belongs to the consuming module (acquisition).
type RunRecord struct {
	CollectorRunID int64
	ResourceUID    *uuid.UUID
	Source         string
	Scope          *string
	StartedAtUTC   time.Time
	FinishedAtUTC  *time.Time
	Status         string
	RowsIngested   *int
	ErrorMessage   *string
}

// DataWriter persists normalized data into cco_data. It is the contract
// MODULES.md assigns to Gestión de Datos; Adquisición (module 3) consumes it
// to persist what it collects. All writes are idempotent (ON CONFLICT DO
// NOTHING on natural/unique keys).
type DataWriter interface {
	// EnsureResource upserts core.resource by azure_resource_id preserving the
	// contract resource_uid and returns the internal resource_id.
	EnsureResource(ctx context.Context, r ResourceRef) (int64, error)
	// StartRun opens a raw.collector_run row (status partial).
	StartRun(ctx context.Context, source string, resourceID int64, scope string) (int64, error)
	// FinishRun closes the run with ok/partial/error.
	FinishRun(ctx context.Context, runID int64, status string, rowsIngested int, errMsg string) error
	// WriteMetricReadings upserts raw.metric_reading; returns rows inserted.
	WriteMetricReadings(ctx context.Context, resourceID int64, metrics []Metric) (int, error)
	// WriteResourceStats upserts raw.db_resource_stats; returns rows inserted.
	WriteResourceStats(ctx context.Context, resourceID int64, stats []ResourceStat) (int, error)
	// WriteQueryStore upserts raw.query_store_query and
	// raw.query_store_runtime_stats; returns rows inserted.
	WriteQueryStore(ctx context.Context, resourceID int64, queries []QueryStoreQueryRecord, stats []QueryStoreRuntimeRecord) (int, error)
	// UpsertCostDaily upserts core.cost_daily keyed by (provider_code,
	// source_charge_id); rejects records without SourceChargeID.
	UpsertCostDaily(ctx context.Context, records []CostRecord) (int, error)
	// UpsertConfigSnapshot appends core.resource_config_snapshot
	// (unique per resource_id + observed_at_utc); returns rows affected.
	UpsertConfigSnapshot(ctx context.Context, snap ConfigurationSnapshot) (int64, error)
}

// HistoricalDataReader reads stored metric series.
type HistoricalDataReader interface {
	// MetricSeries returns readings for q in [FromUTC, ToUTC) ascending,
	// bounded by limit (default 1000, max 10000).
	MetricSeries(ctx context.Context, q DataQuery, limit int) ([]HistoricalMetric, error)
}

// RunReader exposes raw.collector_run to the acquisition module (its API
// serves run state but storage lives here, in Gestión de Datos).
type RunReader interface {
	// ListRuns returns runs, newest first, for the given contract uids only
	// (empty uids list = empty result; never an unscoped query).
	ListRuns(ctx context.Context, uids []uuid.UUID, f RunFilter) ([]RunRecord, error)
	// GetRun returns one run; ErrNotFound when missing.
	GetRun(ctx context.Context, id int64) (RunRecord, error)
}

// CostRepository reads daily cost facts.
type CostRepository interface {
	// CostDailyByResource returns costs mapped to the resource, newest first,
	// in [from, to).
	CostDailyByResource(ctx context.Context, resourceUID uuid.UUID, from, to time.Time, limit int) ([]CostRecord, error)
	// CostDailyByScope returns costs for a scope (subscription /
	// resource_group / resource name), newest first, including rows without a
	// mapped resource (ResourceUID nil).
	CostDailyByScope(ctx context.Context, scopeType, scopeName string, from, to time.Time, limit int) ([]CostRecord, error)
}

// ConfigurationRepository reads the ARM configuration history.
type ConfigurationRepository interface {
	// LatestConfigSnapshot returns the newest snapshot or (nil, nil) when the
	// resource has none yet.
	LatestConfigSnapshot(ctx context.Context, resourceUID uuid.UUID) (*ConfigurationSnapshot, error)
	// ConfigSnapshotHistory returns snapshots newest first, optionally bounded
	// by [from, to) when the times are non-zero.
	ConfigSnapshotHistory(ctx context.Context, resourceUID uuid.UUID, from, to time.Time, limit int) ([]ConfigurationSnapshot, error)
}

// DataQueryService answers availability questions over stored data.
type DataQueryService interface {
	// Coverage (HU-05): buckets readings of one metric over [from, to) and
	// reports gaps plus a sufficient/insufficient state against minCoverage.
	Coverage(ctx context.Context, resourceUID uuid.UUID, metricCode string, from, to time.Time, bucket string, minCoverage float64) (CoverageReport, error)
}
