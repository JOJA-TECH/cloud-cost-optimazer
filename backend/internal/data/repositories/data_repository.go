// Package repositories holds the single GORM gateway to cco_data. Production
// runs against PostgreSQL applying databases/cco_data/cco_data_schema.sql;
// tests run on SQLite via MigrateDataTables (schemas are qualified only on
// PostgreSQL, see database.TableName).
package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	_ data.DataWriter              = (*DataRepository)(nil)
	_ data.HistoricalDataReader    = (*DataRepository)(nil)
	_ data.RunReader               = (*DataRepository)(nil)
	_ data.CostRepository          = (*DataRepository)(nil)
	_ data.ConfigurationRepository = (*DataRepository)(nil)
)

// DataRepository implements the Gestión de Datos contracts (DataWriter,
// HistoricalDataReader, RunReader, CostRepository, ConfigurationRepository)
// over cco_data.
type DataRepository struct{ db *gorm.DB }

// NewDataRepository builds the cco_data gateway.
func NewDataRepository(db *gorm.DB) *DataRepository { return &DataRepository{db: db} }

// MigrateDataTables creates the cco_data tables this package touches using
// GORM AutoMigrate. Production applies databases/cco_data/cco_data_schema.sql;
// this helper exists for SQLite dev/integration tests.
func MigrateDataTables(db *gorm.DB) error {
	models := []any{
		&collectorRunRow{},
		&metricReadingRow{},
		&dbResourceStatRow{},
		&queryStoreQueryRow{},
		&queryStoreRuntimeStatRow{},
		&coreResourceRow{},
		&catalogRegionRow{},
		&costDailyRow{},
		&configSnapshotRow{},
		&sqlSkuRow{},
	}
	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return err
		}
	}
	return nil
}

// ---- row structs for cco_data (TableName returns bare table name) ----

type collectorRunRow struct {
	CollectorRunID int64      `gorm:"column:collector_run_id;primaryKey;autoIncrement"`
	Source         string     `gorm:"column:source;not null"`
	ResourceID     *int64     `gorm:"column:resource_id"`
	Scope          *string    `gorm:"column:scope"`
	StartedAtUTC   time.Time  `gorm:"column:started_at_utc;not null"`
	FinishedAtUTC  *time.Time `gorm:"column:finished_at_utc"`
	Status         string     `gorm:"column:status;not null"`
	RowsIngested   *int       `gorm:"column:rows_ingested"`
	ErrorMessage   *string    `gorm:"column:error_message"`
	RequestPayload *string    `gorm:"column:request_payload"`
}

func (collectorRunRow) TableName() string { return "collector_run" }

type metricReadingRow struct {
	ResourceID      int64     `gorm:"column:resource_id;not null;primaryKey"`
	MetricCode      string    `gorm:"column:metric_code;not null;primaryKey"`
	TimestampUTC    time.Time `gorm:"column:timestamp_utc;not null;primaryKey"`
	IntervalSeconds int       `gorm:"column:interval_seconds;not null;primaryKey"`
	AvgValue        *float64  `gorm:"column:avg_value"`
	MinValue        *float64  `gorm:"column:min_value"`
	MaxValue        *float64  `gorm:"column:max_value"`
	TotalValue      *float64  `gorm:"column:total_value"`
	CountValue      *int      `gorm:"column:count_value"`
	CollectedAt     time.Time `gorm:"column:collected_at;not null"`
}

func (metricReadingRow) TableName() string { return "metric_reading" }

type dbResourceStatRow struct {
	ResourceID            int64     `gorm:"column:resource_id;not null;primaryKey"`
	EndTimeUTC            time.Time `gorm:"column:end_time_utc;not null;primaryKey"`
	AvgCPUPercent         *float64  `gorm:"column:avg_cpu_percent"`
	AvgDataIOPercent      *float64  `gorm:"column:avg_data_io_percent"`
	AvgLogWritePercent    *float64  `gorm:"column:avg_log_write_percent"`
	AvgMemoryUsagePercent *float64  `gorm:"column:avg_memory_usage_percent"`
	AvgInstanceCPUPercent *float64  `gorm:"column:avg_instance_cpu_percent"`
	DTULimit              *float64  `gorm:"column:dtu_limit"`
	CPULimit              *float64  `gorm:"column:cpu_limit"`
	CollectedAt           time.Time `gorm:"column:collected_at;not null"`
}

func (dbResourceStatRow) TableName() string { return "db_resource_stats" }

type queryStoreQueryRow struct {
	ResourceID      int64     `gorm:"column:resource_id;not null;primaryKey"`
	QueryID         int64     `gorm:"column:query_id;not null;primaryKey"`
	QueryTextID     *int64    `gorm:"column:query_text_id"`
	QueryHash       *string   `gorm:"column:query_hash"`
	QuerySQLText    *string   `gorm:"column:query_sql_text"`
	IsInternalQuery *bool     `gorm:"column:is_internal_query"`
	SnapshotAtUTC   time.Time `gorm:"column:snapshot_at_utc;not null;primaryKey"`
	CollectedAt     time.Time `gorm:"column:collected_at;not null"`
}

func (queryStoreQueryRow) TableName() string { return "query_store_query" }

type queryStoreRuntimeStatRow struct {
	ResourceID         int64     `gorm:"column:resource_id;not null;primaryKey"`
	QueryID            int64     `gorm:"column:query_id;not null;primaryKey"`
	IntervalStartUTC   time.Time `gorm:"column:interval_start_utc;not null;primaryKey"`
	IntervalEndUTC     time.Time `gorm:"column:interval_end_utc;not null"`
	CountExecutions    *int64    `gorm:"column:count_executions"`
	AvgDurationUS      *float64  `gorm:"column:avg_duration_us"`
	AvgCPUTimeUS       *float64  `gorm:"column:avg_cpu_time_us"`
	AvgLogicalIOReads  *float64  `gorm:"column:avg_logical_io_reads"`
	AvgLogicalIOWrites *float64  `gorm:"column:avg_logical_io_writes"`
	SnapshotAtUTC      time.Time `gorm:"column:snapshot_at_utc"`
	CollectedAt        time.Time `gorm:"column:collected_at;not null"`
}

func (queryStoreRuntimeStatRow) TableName() string { return "query_store_runtime_stats" }

type coreResourceRow struct {
	ResourceID      int64      `gorm:"column:resource_id;primaryKey;autoIncrement"`
	ResourceUID     uuid.UUID  `gorm:"column:resource_uid;type:uuid;not null;uniqueIndex"`
	ProviderCode    string     `gorm:"column:provider_code;not null"`
	SubscriptionID  string     `gorm:"column:subscription_id;not null"`
	ResourceGroup   string     `gorm:"column:resource_group;not null"`
	ServerName      string     `gorm:"column:server_name;not null"`
	DatabaseName    string     `gorm:"column:database_name;not null"`
	AzureResourceID string     `gorm:"column:azure_resource_id;not null;uniqueIndex"`
	RegionCode      string     `gorm:"column:region_code;not null"`
	Kind            *string    `gorm:"column:kind"`
	LifecycleStatus string     `gorm:"column:lifecycle_status;not null"`
	RegisteredAt    *time.Time `gorm:"column:registered_at"`
	DeactivatedAt   *time.Time `gorm:"column:deactivated_at"`
}

func (coreResourceRow) TableName() string { return "resource" }

type catalogRegionRow struct {
	ProviderCode string  `gorm:"column:provider_code;not null;primaryKey"`
	RegionCode   string  `gorm:"column:region_code;not null;primaryKey"`
	DisplayName  *string `gorm:"column:display_name"`
}

func (catalogRegionRow) TableName() string { return "region" }

type costDailyRow struct {
	CostDailyID    int64     `gorm:"column:cost_daily_id;primaryKey;autoIncrement"`
	ResourceID     *int64    `gorm:"column:resource_id"`
	ProviderCode   string    `gorm:"column:provider_code;not null;uniqueIndex:ux_cost_daily_source_charge"`
	ScopeType      string    `gorm:"column:scope_type;not null"`
	ScopeName      string    `gorm:"column:scope_name;not null"`
	ServiceName    string    `gorm:"column:service_name;not null"`
	RegionCode     *string   `gorm:"column:region_code"`
	UsageDate      time.Time `gorm:"column:usage_date;not null"`
	Currency       string    `gorm:"column:currency;not null"`
	SourceChargeID *string   `gorm:"column:source_charge_id;uniqueIndex:ux_cost_daily_source_charge"`
	MeterID        *string   `gorm:"column:meter_id"`
	PreTaxCost     float64   `gorm:"column:pre_tax_cost;not null"`
	UsageQuantity  *float64  `gorm:"column:usage_quantity"`
	UsageUnit      *string   `gorm:"column:usage_unit"`
	PricingModel   *string   `gorm:"column:pricing_model"`
	ChargeCategory *string   `gorm:"column:charge_category"`
	Source         string    `gorm:"column:source;not null"`
	CollectedAt    time.Time `gorm:"column:collected_at;not null"`
}

func (costDailyRow) TableName() string { return "cost_daily" }

type configSnapshotRow struct {
	ConfigSnapshotID      int64     `gorm:"column:config_snapshot_id;primaryKey;autoIncrement"`
	ResourceID            int64     `gorm:"column:resource_id;not null;uniqueIndex:ux_config_snapshot_resource_time"`
	SKUID                 *int64    `gorm:"column:sku_id"`
	ObservedAtUTC         time.Time `gorm:"column:observed_at_utc;not null;uniqueIndex:ux_config_snapshot_resource_time"`
	CollectorRunID        *int64    `gorm:"column:collector_run_id"`
	SKUName               *string   `gorm:"column:sku_name"`
	Tier                  *string   `gorm:"column:tier"`
	Family                *string   `gorm:"column:family"`
	CapacityVCores        *float64  `gorm:"column:capacity_vcores"`
	MinCapacityVCores     *float64  `gorm:"column:min_capacity_vcores"`
	ComputeModel          *string   `gorm:"column:compute_model"`
	MaxSizeBytes          *int64    `gorm:"column:max_size_bytes"`
	AutoPauseDelayMinutes *int      `gorm:"column:auto_pause_delay_minutes"`
	Status                *string   `gorm:"column:status"`
	FreeLimit             *bool     `gorm:"column:free_limit"`
	SourcePayload         *string   `gorm:"column:source_payload"`
}

func (configSnapshotRow) TableName() string { return "resource_config_snapshot" }

type sqlSkuRow struct {
	SKUID             int64     `gorm:"column:sku_id;primaryKey;autoIncrement"`
	SKUName           string    `gorm:"column:sku_name;not null;uniqueIndex"`
	ARMSKUName        *string   `gorm:"column:arm_sku_name"`
	Tier              string    `gorm:"column:tier;not null"`
	Family            *string   `gorm:"column:family"`
	ComputeModel      string    `gorm:"column:compute_model;not null"`
	CapacityVCores    float64   `gorm:"column:capacity_vcores;not null"`
	MinCapacityVCores *float64  `gorm:"column:min_capacity_vcores"`
	MaxSizeGB         *int      `gorm:"column:max_size_gb"`
	InScopeR1         bool      `gorm:"column:in_scope_r1;not null"`
	Notes             *string   `gorm:"column:notes"`
	CreatedAt         time.Time `gorm:"column:created_at;not null"`
}

func (sqlSkuRow) TableName() string { return "sql_sku" }

// ---- resource resolution ----

// resolveResourceID translates the contract resource_uid (the UUID shared
// with cco_app.cloud.resource_bindings.data_resource_uid) into the internal
// surrogate key. Unknown uids are a domain error, never a silent miss.
func (r *DataRepository) resolveResourceID(ctx context.Context, uid uuid.UUID) (int64, error) {
	var row coreResourceRow
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "core", "resource")).
		Where("resource_uid = ?", uid).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, data.NewError(data.CodeNotFound, "resource not found in cco_data", data.ErrNotFound)
	}
	if err != nil {
		return 0, err
	}
	return row.ResourceID, nil
}

// ---- DataWriter ----

func (r *DataRepository) EnsureResource(ctx context.Context, ref data.ResourceRef) (int64, error) {
	db := r.db.WithContext(ctx)

	// (1) ensure catalog.region row (azure, region_code).
	region := catalogRegionRow{ProviderCode: "azure", RegionCode: ref.RegionCode}
	if err := db.Table(database.TableName(db, "catalog", "region")).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&region).Error; err != nil {
		return 0, err
	}

	// (2) upsert core.resource by azure_resource_id (never resource_uid).
	now := time.Now().UTC()
	row := coreResourceRow{
		ResourceUID:     ref.ResourceUID,
		ProviderCode:    "azure",
		SubscriptionID:  ref.SubscriptionID,
		ResourceGroup:   ref.ResourceGroup,
		ServerName:      ref.ServerName,
		DatabaseName:    ref.DatabaseName,
		AzureResourceID: ref.AzureResourceID,
		RegionCode:      ref.RegionCode,
		Kind:            ref.Kind,
		LifecycleStatus: "active",
		RegisteredAt:    &now,
	}
	if err := db.Table(database.TableName(db, "core", "resource")).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "azure_resource_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"subscription_id", "resource_group", "server_name", "database_name",
				"region_code", "kind", "lifecycle_status",
			}),
		}).
		Create(&row).Error; err != nil {
		return 0, err
	}

	// (3) select resource_id by azure_resource_id (no RETURNING: works in SQLite).
	var got coreResourceRow
	if err := db.Table(database.TableName(db, "core", "resource")).
		Where("azure_resource_id = ?", ref.AzureResourceID).
		First(&got).Error; err != nil {
		return 0, err
	}
	return got.ResourceID, nil
}

func (r *DataRepository) StartRun(ctx context.Context, source string, resourceID int64, scope string) (int64, error) {
	now := time.Now().UTC()
	row := collectorRunRow{
		Source:       source,
		ResourceID:   &resourceID,
		Scope:        &scope,
		StartedAtUTC: now,
		Status:       "partial",
	}
	if err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "raw", "collector_run")).
		Create(&row).Error; err != nil {
		return 0, err
	}
	return row.CollectorRunID, nil
}

func (r *DataRepository) FinishRun(ctx context.Context, runID int64, status string, rowsIngested int, errMsg string) error {
	updates := map[string]interface{}{
		"finished_at_utc": time.Now().UTC(),
		"status":          status,
		"rows_ingested":   rowsIngested,
	}
	var errorMessage interface{} = nil
	if errMsg != "" {
		errorMessage = errMsg
	}
	updates["error_message"] = errorMessage
	return r.db.WithContext(ctx).
		Table(database.TableName(r.db, "raw", "collector_run")).
		Where("collector_run_id = ?", runID).
		Updates(updates).Error
}

func (r *DataRepository) WriteMetricReadings(ctx context.Context, resourceID int64, metrics []data.Metric) (int, error) {
	if len(metrics) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	rows := make([]metricReadingRow, 0, len(metrics))
	for _, m := range metrics {
		rows = append(rows, metricReadingRow{
			ResourceID:      resourceID,
			MetricCode:      m.MetricCode,
			TimestampUTC:    m.TimestampUTC,
			IntervalSeconds: m.IntervalSeconds,
			AvgValue:        m.AvgValue,
			MinValue:        m.MinValue,
			MaxValue:        m.MaxValue,
			TotalValue:      m.TotalValue,
			CountValue:      m.CountValue,
			CollectedAt:     now,
		})
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Table(database.TableName(r.db, "raw", "metric_reading")).
		Create(&rows)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

func (r *DataRepository) WriteResourceStats(ctx context.Context, resourceID int64, stats []data.ResourceStat) (int, error) {
	if len(stats) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	rows := make([]dbResourceStatRow, 0, len(stats))
	for _, st := range stats {
		rows = append(rows, dbResourceStatRow{
			ResourceID:            resourceID,
			EndTimeUTC:            st.EndTimeUTC,
			AvgCPUPercent:         st.AvgCPUPercent,
			AvgDataIOPercent:      st.AvgDataIOPercent,
			AvgLogWritePercent:    st.AvgLogWritePercent,
			AvgMemoryUsagePercent: st.AvgMemoryUsagePercent,
			AvgInstanceCPUPercent: st.AvgInstanceCPUPercent,
			DTULimit:              st.DTULimit,
			CPULimit:              st.CPULimit,
			CollectedAt:           now,
		})
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Table(database.TableName(r.db, "raw", "db_resource_stats")).
		Create(&rows)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

func (r *DataRepository) WriteQueryStore(ctx context.Context, resourceID int64, queries []data.QueryStoreQueryRecord, stats []data.QueryStoreRuntimeRecord) (int, error) {
	total := 0
	now := time.Now().UTC()

	if len(queries) > 0 {
		qsRows := make([]queryStoreQueryRow, 0, len(queries))
		for _, q := range queries {
			qsRows = append(qsRows, queryStoreQueryRow{
				ResourceID:      resourceID,
				QueryID:         q.QueryID,
				QueryTextID:     q.QueryTextID,
				QueryHash:       q.QueryHash,
				QuerySQLText:    q.QuerySQLText,
				IsInternalQuery: q.IsInternalQuery,
				SnapshotAtUTC:   q.SnapshotAtUTC,
				CollectedAt:     now,
			})
		}
		result := r.db.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Table(database.TableName(r.db, "raw", "query_store_query")).
			Create(&qsRows)
		if result.Error != nil {
			return 0, result.Error
		}
		total += int(result.RowsAffected)
	}

	if len(stats) > 0 {
		rsRows := make([]queryStoreRuntimeStatRow, 0, len(stats))
		for _, s := range stats {
			rsRows = append(rsRows, queryStoreRuntimeStatRow{
				ResourceID:         resourceID,
				QueryID:            s.QueryID,
				IntervalStartUTC:   s.IntervalStartUTC,
				IntervalEndUTC:     s.IntervalEndUTC,
				CountExecutions:    s.CountExecutions,
				AvgDurationUS:      s.AvgDurationUS,
				AvgCPUTimeUS:       s.AvgCPUTimeUS,
				AvgLogicalIOReads:  s.AvgLogicalIOReads,
				AvgLogicalIOWrites: s.AvgLogicalIOWrites,
				SnapshotAtUTC:      s.SnapshotAtUTC,
				CollectedAt:        now,
			})
		}
		result := r.db.WithContext(ctx).
			Clauses(clause.OnConflict{DoNothing: true}).
			Table(database.TableName(r.db, "raw", "query_store_runtime_stats")).
			Create(&rsRows)
		if result.Error != nil {
			return 0, result.Error
		}
		total += int(result.RowsAffected)
	}

	return total, nil
}

func (r *DataRepository) UpsertCostDaily(ctx context.Context, records []data.CostRecord) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}
	for _, rec := range records {
		// The unique index ux_cost_daily_source_charge is partial (WHERE
		// source_charge_id IS NOT NULL): a nil id would bypass idempotency.
		if rec.SourceChargeID == nil || *rec.SourceChargeID == "" {
			return 0, data.NewError(data.CodeInvalidCostRecord,
				"source_charge_id is required for idempotent cost upserts", data.ErrInvalidCostRecord)
		}
		switch rec.ScopeType {
		case "subscription", "resource_group", "resource":
		default:
			return 0, data.NewError(data.CodeInvalidCostRecord,
				"scope_type must be subscription, resource_group or resource", data.ErrInvalidCostRecord)
		}
	}

	db := r.db.WithContext(ctx)
	now := time.Now().UTC()
	resolved := make(map[uuid.UUID]*int64)
	rows := make([]costDailyRow, 0, len(records))
	for _, rec := range records {
		var resourceID *int64
		if rec.ResourceUID != nil {
			if id, ok := resolved[*rec.ResourceUID]; ok {
				resourceID = id
			} else {
				// Unknown uid: keep the cost with NULL resource_id (costs of
				// Storage/VNet and unregistered resources are still recorded).
				id, err := r.resolveResourceID(ctx, *rec.ResourceUID)
				var ptr *int64
				if err == nil {
					ptr = &id
				} else if !errors.Is(err, data.ErrNotFound) {
					return 0, err
				}
				resolved[*rec.ResourceUID] = ptr
				resourceID = ptr
			}
		}
		source := rec.Source
		if source == "" {
			source = "azure_cost_management"
		}
		rows = append(rows, costDailyRow{
			ResourceID:     resourceID,
			ProviderCode:   rec.ProviderCode,
			ScopeType:      rec.ScopeType,
			ScopeName:      rec.ScopeName,
			ServiceName:    rec.ServiceName,
			RegionCode:     rec.RegionCode,
			UsageDate:      rec.UsageDate,
			Currency:       rec.Currency,
			SourceChargeID: rec.SourceChargeID,
			MeterID:        rec.MeterID,
			PreTaxCost:     rec.PreTaxCost,
			UsageQuantity:  rec.UsageQuantity,
			UsageUnit:      rec.UsageUnit,
			PricingModel:   rec.PricingModel,
			ChargeCategory: rec.ChargeCategory,
			Source:         source,
			CollectedAt:    now,
		})
	}
	result := db.
		Clauses(clause.OnConflict{DoNothing: true}).
		Table(database.TableName(r.db, "core", "cost_daily")).
		Create(&rows)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

func (r *DataRepository) UpsertConfigSnapshot(ctx context.Context, snap data.ConfigurationSnapshot) (int64, error) {
	resourceID, err := r.resolveResourceID(ctx, snap.ResourceUID)
	if err != nil {
		return 0, err
	}

	// Resolve catalog.sql_sku by service objective name; unknown SKUs are
	// stored with sku_id NULL and the textual name (the column allows it).
	var skuID *int64
	if snap.SKUName != nil && *snap.SKUName != "" {
		var id int64
		if err := r.db.WithContext(ctx).
			Table(database.TableName(r.db, "catalog", "sql_sku")).
			Where("sku_name = ?", *snap.SKUName).
			Limit(1).
			Pluck("sku_id", &id).Error; err != nil {
			return 0, err
		}
		if id > 0 {
			skuID = &id
		}
	}

	row := configSnapshotRow{
		ResourceID:            resourceID,
		SKUID:                 skuID,
		ObservedAtUTC:         snap.ObservedAtUTC,
		CollectorRunID:        snap.CollectorRunID,
		SKUName:               snap.SKUName,
		Tier:                  snap.Tier,
		Family:                snap.Family,
		CapacityVCores:        snap.CapacityVCores,
		MinCapacityVCores:     snap.MinCapacityVCores,
		ComputeModel:          snap.ComputeModel,
		MaxSizeBytes:          snap.MaxSizeBytes,
		AutoPauseDelayMinutes: snap.AutoPauseDelayMin,
		Status:                snap.Status,
		FreeLimit:             snap.FreeLimit,
		SourcePayload:         snap.SourcePayload,
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Table(database.TableName(r.db, "core", "resource_config_snapshot")).
		Create(&row)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// ---- HistoricalDataReader ----

func (r *DataRepository) MetricSeries(ctx context.Context, q data.DataQuery, limit int) ([]data.HistoricalMetric, error) {
	resourceID, err := r.resolveResourceID(ctx, q.ResourceUID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		limit = 10000
	}
	var rows []metricReadingRow
	if err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "raw", "metric_reading")).
		Where("resource_id = ? AND metric_code = ? AND timestamp_utc >= ? AND timestamp_utc < ?",
			resourceID, q.MetricCode, q.FromUTC, q.ToUTC).
		Order("timestamp_utc ASC").
		Limit(limit).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]data.HistoricalMetric, 0, len(rows))
	for _, row := range rows {
		out = append(out, data.HistoricalMetric{
			TimestampUTC:    row.TimestampUTC,
			IntervalSeconds: row.IntervalSeconds,
			AvgValue:        row.AvgValue,
			MinValue:        row.MinValue,
			MaxValue:        row.MaxValue,
			TotalValue:      row.TotalValue,
			CountValue:      row.CountValue,
		})
	}
	return out, nil
}

// MetricReadingTimestamps returns the raw timestamps of one metric window;
// input of the Go-side coverage computation (service.Coverage).
func (r *DataRepository) MetricReadingTimestamps(ctx context.Context, resourceUID uuid.UUID, metricCode string, from, to time.Time) ([]time.Time, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return nil, err
	}
	var ts []time.Time
	if err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "raw", "metric_reading")).
		Where("resource_id = ? AND metric_code = ? AND timestamp_utc >= ? AND timestamp_utc < ?",
			resourceID, metricCode, from, to).
		Order("timestamp_utc ASC").
		Pluck("timestamp_utc", &ts).Error; err != nil {
		return nil, err
	}
	return ts, nil
}

// ---- RunReader ----

func (r *DataRepository) ListRuns(ctx context.Context, uids []uuid.UUID, f data.RunFilter) ([]data.RunRecord, error) {
	if len(uids) == 0 {
		return []data.RunRecord{}, nil
	}

	type runScanRow struct {
		CollectorRunID int64
		Source         string
		ResourceID     *int64
		Scope          *string
		StartedAtUTC   time.Time
		FinishedAtUTC  *time.Time
		Status         string
		RowsIngested   *int
		ErrorMessage   *string
		RequestPayload *string
		ResourceUID    uuid.UUID `gorm:"column:resource_uid"`
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	db := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "raw", "collector_run")).
		Select("collector_run.*, resource.resource_uid").
		Joins("JOIN "+database.TableName(r.db, "core", "resource")+" resource ON resource.resource_id = collector_run.resource_id").
		Where("resource.resource_uid IN ?", uids).
		Order("collector_run.collector_run_id DESC").
		Limit(limit)

	if f.Source != "" {
		db = db.Where("collector_run.source = ?", f.Source)
	}
	if f.Status != "" {
		db = db.Where("collector_run.status = ?", f.Status)
	}
	if f.ResourceUID != nil {
		db = db.Where("resource.resource_uid = ?", *f.ResourceUID)
	}

	var scanRows []runScanRow
	if err := db.Scan(&scanRows).Error; err != nil {
		return nil, err
	}

	out := make([]data.RunRecord, 0, len(scanRows))
	for _, row := range scanRows {
		uid := row.ResourceUID
		out = append(out, data.RunRecord{
			CollectorRunID: row.CollectorRunID,
			ResourceUID:    &uid,
			Source:         row.Source,
			Scope:          row.Scope,
			StartedAtUTC:   row.StartedAtUTC,
			FinishedAtUTC:  row.FinishedAtUTC,
			Status:         row.Status,
			RowsIngested:   row.RowsIngested,
			ErrorMessage:   row.ErrorMessage,
		})
	}
	return out, nil
}

func (r *DataRepository) GetRun(ctx context.Context, id int64) (data.RunRecord, error) {
	type runScanRow struct {
		CollectorRunID int64
		Source         string
		ResourceID     *int64
		Scope          *string
		StartedAtUTC   time.Time
		FinishedAtUTC  *time.Time
		Status         string
		RowsIngested   *int
		ErrorMessage   *string
		RequestPayload *string
		ResourceUID    uuid.UUID `gorm:"column:resource_uid"`
	}

	var scanRow runScanRow
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "raw", "collector_run")).
		Select("collector_run.*, resource.resource_uid").
		Joins("JOIN "+database.TableName(r.db, "core", "resource")+" resource ON resource.resource_id = collector_run.resource_id").
		Where("collector_run.collector_run_id = ?", id).
		Scan(&scanRow).Error
	if err != nil {
		return data.RunRecord{}, err
	}
	if scanRow.CollectorRunID == 0 {
		return data.RunRecord{}, data.NewError(data.CodeNotFound, "collector run not found", data.ErrNotFound)
	}
	return data.RunRecord{
		CollectorRunID: scanRow.CollectorRunID,
		ResourceUID:    &scanRow.ResourceUID,
		Source:         scanRow.Source,
		Scope:          scanRow.Scope,
		StartedAtUTC:   scanRow.StartedAtUTC,
		FinishedAtUTC:  scanRow.FinishedAtUTC,
		Status:         scanRow.Status,
		RowsIngested:   scanRow.RowsIngested,
		ErrorMessage:   scanRow.ErrorMessage,
	}, nil
}

// ---- CostRepository ----

func (r *DataRepository) CostDailyByResource(ctx context.Context, resourceUID uuid.UUID, from, to time.Time, limit int) ([]data.CostRecord, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return nil, err
	}
	var rows []costDailyRow
	if err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "core", "cost_daily")).
		Where("resource_id = ? AND usage_date >= ? AND usage_date < ?", resourceID, from, to).
		Order("usage_date DESC, cost_daily_id DESC").
		Limit(normalizeLimit(limit)).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]data.CostRecord, 0, len(rows))
	for _, row := range rows {
		rec := costRowToRecord(row)
		uid := resourceUID
		rec.ResourceUID = &uid
		out = append(out, rec)
	}
	return out, nil
}

func (r *DataRepository) CostDailyByScope(ctx context.Context, scopeType, scopeName string, from, to time.Time, limit int) ([]data.CostRecord, error) {
	type costScopeScanRow struct {
		CostDailyID    int64      `gorm:"column:cost_daily_id"`
		ResourceUID    *uuid.UUID `gorm:"column:resource_uid"`
		ProviderCode   string     `gorm:"column:provider_code"`
		ScopeType      string     `gorm:"column:scope_type"`
		ScopeName      string     `gorm:"column:scope_name"`
		ServiceName    string     `gorm:"column:service_name"`
		RegionCode     *string    `gorm:"column:region_code"`
		UsageDate      time.Time  `gorm:"column:usage_date"`
		Currency       string     `gorm:"column:currency"`
		SourceChargeID *string    `gorm:"column:source_charge_id"`
		MeterID        *string    `gorm:"column:meter_id"`
		PreTaxCost     float64    `gorm:"column:pre_tax_cost"`
		UsageQuantity  *float64   `gorm:"column:usage_quantity"`
		UsageUnit      *string    `gorm:"column:usage_unit"`
		PricingModel   *string    `gorm:"column:pricing_model"`
		ChargeCategory *string    `gorm:"column:charge_category"`
		Source         string     `gorm:"column:source"`
		CollectedAt    time.Time  `gorm:"column:collected_at"`
	}

	var scanRows []costScopeScanRow
	if err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "core", "cost_daily")+" AS cost").
		Select("cost.*, res.resource_uid").
		Joins("LEFT JOIN "+database.TableName(r.db, "core", "resource")+" res ON res.resource_id = cost.resource_id").
		Where("cost.scope_type = ? AND cost.scope_name = ? AND cost.usage_date >= ? AND cost.usage_date < ?",
			scopeType, scopeName, from, to).
		Order("cost.usage_date DESC, cost.cost_daily_id DESC").
		Limit(normalizeLimit(limit)).
		Scan(&scanRows).Error; err != nil {
		return nil, err
	}

	out := make([]data.CostRecord, 0, len(scanRows))
	for _, row := range scanRows {
		out = append(out, data.CostRecord{
			ResourceUID:    row.ResourceUID,
			ProviderCode:   row.ProviderCode,
			ScopeType:      row.ScopeType,
			ScopeName:      row.ScopeName,
			ServiceName:    row.ServiceName,
			RegionCode:     row.RegionCode,
			UsageDate:      row.UsageDate,
			Currency:       row.Currency,
			SourceChargeID: row.SourceChargeID,
			MeterID:        row.MeterID,
			PreTaxCost:     row.PreTaxCost,
			UsageQuantity:  row.UsageQuantity,
			UsageUnit:      row.UsageUnit,
			PricingModel:   row.PricingModel,
			ChargeCategory: row.ChargeCategory,
			Source:         row.Source,
		})
	}
	return out, nil
}

func costRowToRecord(row costDailyRow) data.CostRecord {
	return data.CostRecord{
		ResourceUID:    nil,
		ProviderCode:   row.ProviderCode,
		ScopeType:      row.ScopeType,
		ScopeName:      row.ScopeName,
		ServiceName:    row.ServiceName,
		RegionCode:     row.RegionCode,
		UsageDate:      row.UsageDate,
		Currency:       row.Currency,
		SourceChargeID: row.SourceChargeID,
		MeterID:        row.MeterID,
		PreTaxCost:     row.PreTaxCost,
		UsageQuantity:  row.UsageQuantity,
		UsageUnit:      row.UsageUnit,
		PricingModel:   row.PricingModel,
		ChargeCategory: row.ChargeCategory,
		Source:         row.Source,
	}
}

// ---- ConfigurationRepository ----

func (r *DataRepository) LatestConfigSnapshot(ctx context.Context, resourceUID uuid.UUID) (*data.ConfigurationSnapshot, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return nil, err
	}
	var row configSnapshotRow
	err = r.db.WithContext(ctx).
		Table(database.TableName(r.db, "core", "resource_config_snapshot")).
		Where("resource_id = ?", resourceID).
		Order("observed_at_utc DESC").
		Limit(1).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := configRowToSnapshot(row)
	out.ResourceUID = resourceUID
	return &out, nil
}

func (r *DataRepository) ConfigSnapshotHistory(ctx context.Context, resourceUID uuid.UUID, from, to time.Time, limit int) ([]data.ConfigurationSnapshot, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return nil, err
	}
	db := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "core", "resource_config_snapshot")).
		Where("resource_id = ?", resourceID)
	if !from.IsZero() {
		db = db.Where("observed_at_utc >= ?", from)
	}
	if !to.IsZero() {
		db = db.Where("observed_at_utc < ?", to)
	}
	var rows []configSnapshotRow
	if err := db.
		Order("observed_at_utc DESC").
		Limit(normalizeLimit(limit)).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]data.ConfigurationSnapshot, 0, len(rows))
	for _, row := range rows {
		snap := configRowToSnapshot(row)
		snap.ResourceUID = resourceUID
		out = append(out, snap)
	}
	return out, nil
}

func configRowToSnapshot(row configSnapshotRow) data.ConfigurationSnapshot {
	return data.ConfigurationSnapshot{
		CollectorRunID:    row.CollectorRunID,
		SKUName:           row.SKUName,
		Tier:              row.Tier,
		Family:            row.Family,
		CapacityVCores:    row.CapacityVCores,
		MinCapacityVCores: row.MinCapacityVCores,
		ComputeModel:      row.ComputeModel,
		MaxSizeBytes:      row.MaxSizeBytes,
		AutoPauseDelayMin: row.AutoPauseDelayMinutes,
		Status:            row.Status,
		FreeLimit:         row.FreeLimit,
		SourcePayload:     row.SourcePayload,
		ObservedAtUTC:     row.ObservedAtUTC,
	}
}

// normalizeLimit defaults page sizes and caps them (read endpoints).
func normalizeLimit(limit int) int {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	return limit
}
