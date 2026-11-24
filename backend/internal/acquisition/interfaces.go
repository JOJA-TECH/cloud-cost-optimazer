package acquisition

import (
	"context"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
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

// ---- Gestión de Datos contract mappings ----
//
// Per MODULES.md, Adquisición persists through Gestión de Datos "mediante
// contratos": internal/data owns every cco_data access. These helpers convert
// acquisition types to the data contracts and back; no storage type leaks
// beyond the service layer.

// resourceRef converts the monitored target into the storage contract type
// used by data.DataWriter.EnsureResource.
func (t Target) resourceRef() data.ResourceRef {
	return data.ResourceRef{
		ResourceUID:     t.ResourceUID,
		SubscriptionID:  t.SubscriptionID,
		ResourceGroup:   t.ResourceGroup,
		ServerName:      t.ServerName,
		DatabaseName:    t.DatabaseName,
		AzureResourceID: t.AzureResourceID,
		RegionCode:      t.RegionCode,
		Kind:            t.Kind,
	}
}

// metricsFromReadings maps validated readings into the data.Metric write
// contract (Count loses sub-1 precision, as raw.metric_reading stores int).
func metricsFromReadings(readings []RawReading) []data.Metric {
	out := make([]data.Metric, 0, len(readings))
	for _, rd := range readings {
		var count *int
		if rd.Count != nil {
			c := int(*rd.Count)
			count = &c
		}
		out = append(out, data.Metric{
			MetricCode:      rd.MetricCode,
			TimestampUTC:    rd.Timestamp,
			IntervalSeconds: rd.IntervalSeconds,
			AvgValue:        rd.Average,
			MinValue:        rd.Minimum,
			MaxValue:        rd.Maximum,
			TotalValue:      rd.Total,
			CountValue:      count,
		})
	}
	return out
}

// resourceStatsFrom maps DMV rows into the data.ResourceStat write contract.
func resourceStatsFrom(stats []DBResourceStat) []data.ResourceStat {
	out := make([]data.ResourceStat, 0, len(stats))
	for _, st := range stats {
		out = append(out, data.ResourceStat{
			EndTimeUTC:            st.EndTimeUTC,
			AvgCPUPercent:         st.AvgCPUPercent,
			AvgDataIOPercent:      st.AvgDataIOPercent,
			AvgLogWritePercent:    st.AvgLogWritePercent,
			AvgMemoryUsagePercent: st.AvgMemoryUsagePercent,
			AvgInstanceCPUPercent: st.AvgInstanceCPUPercent,
			DTULimit:              st.DTULimit,
			CPULimit:              st.CPULimit,
		})
	}
	return out
}

// queryStoreRecordsFrom maps Query Store rows into the data write contracts.
func queryStoreRecordsFrom(queries []QueryStoreQuery, stats []QueryStoreRuntimeStat) ([]data.QueryStoreQueryRecord, []data.QueryStoreRuntimeRecord) {
	qs := make([]data.QueryStoreQueryRecord, 0, len(queries))
	for _, q := range queries {
		qs = append(qs, data.QueryStoreQueryRecord{
			QueryID:         q.QueryID,
			QueryTextID:     q.QueryTextID,
			QueryHash:       q.QueryHash,
			QuerySQLText:    q.QuerySQLText,
			IsInternalQuery: q.IsInternalQuery,
			SnapshotAtUTC:   q.SnapshotAtUTC,
		})
	}
	rs := make([]data.QueryStoreRuntimeRecord, 0, len(stats))
	for _, s := range stats {
		rs = append(rs, data.QueryStoreRuntimeRecord{
			QueryID:            s.QueryID,
			IntervalStartUTC:   s.IntervalStartUTC,
			IntervalEndUTC:     s.IntervalEndUTC,
			CountExecutions:    s.CountExecutions,
			AvgDurationUS:      s.AvgDurationUS,
			AvgCPUTimeUS:       s.AvgCPUTimeUS,
			AvgLogicalIOReads:  s.AvgLogicalIOReads,
			AvgLogicalIOWrites: s.AvgLogicalIOWrites,
			SnapshotAtUTC:      s.SnapshotAtUTC,
		})
	}
	return qs, rs
}

// runDTOFrom maps a storage run record into the API DTO (unix ms timestamps).
func runDTOFrom(r data.RunRecord) AcquisitionRunDTO {
	dto := AcquisitionRunDTO{
		CollectorRunID: r.CollectorRunID,
		ResourceUID:    r.ResourceUID,
		Source:         r.Source,
		Scope:          strValPtr(r.Scope),
		StartedAtUTC:   r.StartedAtUTC.UnixMilli(),
		Status:         r.Status,
		ErrorMessage:   strValPtr(r.ErrorMessage),
	}
	if r.FinishedAtUTC != nil {
		ms := r.FinishedAtUTC.UnixMilli()
		dto.FinishedAtUTC = &ms
	}
	if r.RowsIngested != nil {
		rows := *r.RowsIngested
		dto.RowsIngested = &rows
	}
	return dto
}

// runDTOsFrom maps a page of run records into API DTOs.
func runDTOsFrom(records []data.RunRecord) []AcquisitionRunDTO {
	out := make([]AcquisitionRunDTO, 0, len(records))
	for _, r := range records {
		out = append(out, runDTOFrom(r))
	}
	return out
}

func strValPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// samplesFromSamples converts acquisition.RawSample to data.RawSample for persistence.
func samplesFromSamples(samples []RawSample) []data.RawSample {
	out := make([]data.RawSample, 0, len(samples))
	for _, s := range samples {
		out = append(out, data.RawSample{
			MetricCode:     s.MetricCode,
			ObservedAtUTC:  s.ObservedAtUTC,
			Value:          s.Value,
			Source:         s.Source,
			SourceSampleID: s.SourceSampleID,
		})
	}
	return out
}

// ---- T4: ConfigProvider (ARM database GET) ----

// ConfigReading is the normalized snapshot from ARM.
type ConfigReading struct {
	SKUName           *string
	Tier              *string
	Family            *string
	CapacityVCores    *float64
	MinCapacityVCores *float64
	ComputeModel      *string
	MaxSizeBytes      *int64
	AutoPauseDelayMin *int
	Status            *string
	FreeLimit         *bool
	SourcePayload     *string
	ObservedAtUTC     time.Time
}

// ConfigProvider reads the current ARM configuration of an Azure SQL Database.
type ConfigProvider interface {
	FetchConfigSnapshot(ctx context.Context, creds cloud.ConnectionCredentials, azureResourceID string) (ConfigReading, error)
}

// ---- T5: CostProvider (Cost Management query) ----

// CostProvider reads daily actual costs from Azure Cost Management.
type CostProvider interface {
	// FetchDailyCosts returns costs for a subscription over [from, to).
	// The returned records must include SourceChargeID for idempotent upsert.
	FetchDailyCosts(ctx context.Context, creds cloud.ConnectionCredentials, subscriptionID string, from, to time.Time) ([]data.CostRecord, error)
}

// ---- T1b: SampleProvider (individual observations for exact percentiles) ----

// RawSample is one individual observation for raw.metric_sample.
// Providers return these when the source exposes individual measurements
// (Query Store runtime stats, workload observations).
// Azure Monitor PT1M aggregates do NOT provide individual samples; they are omitted.
type RawSample struct {
	MetricCode     string
	Unit           string
	ObservedAtUTC  time.Time
	Value          float64
	Source         string
	SourceSampleID *string // id in source for idempotency; nil if source has no ID
}

// SampleProvider reads individual observations from sources that expose them.
type SampleProvider interface {
	FetchSamples(ctx context.Context, creds cloud.ConnectionCredentials, azureResourceID string, start, end time.Time) ([]RawSample, error)
}
