package repositories

import (
	"context"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	_ acquisition.AcquisitionRepository = (*dataRepository)(nil)
)

// dataRepository writes/reads acquisition data in cco_data.
type dataRepository struct{ db *gorm.DB }

// NewDataRepository builds an AcquisitionRepository backed by cco_data.
func NewDataRepository(db *gorm.DB) acquisition.AcquisitionRepository {
	return &dataRepository{db: db}
}

// MigrateDataTables creates the cco_data tables this package writes to using
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
	SnapshotAtUTC      time.Time `gorm:"column:snapshot_at_utc;not null"`
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

// ---- acquisition methods ----

func (r *dataRepository) EnsureResource(ctx context.Context, t acquisition.Target) (int64, error) {
	db := r.db.WithContext(ctx)

	// (1) ensure catalog.region row (azure, region_code).
	region := catalogRegionRow{ProviderCode: "azure", RegionCode: t.RegionCode}
	if err := db.Table(database.TableName(db, "catalog", "region")).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&region).Error; err != nil {
		return 0, err
	}

	// (2) upsert core.resource by azure_resource_id (never resource_uid).
	now := time.Now().UTC()
	row := coreResourceRow{
		ResourceUID:     t.ResourceUID,
		ProviderCode:    "azure",
		SubscriptionID:  t.SubscriptionID,
		ResourceGroup:   t.ResourceGroup,
		ServerName:      t.ServerName,
		DatabaseName:    t.DatabaseName,
		AzureResourceID: t.AzureResourceID,
		RegionCode:      t.RegionCode,
		Kind:            t.Kind,
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
		Where("azure_resource_id = ?", t.AzureResourceID).
		First(&got).Error; err != nil {
		return 0, err
	}
	return got.ResourceID, nil
}

func (r *dataRepository) StartRun(ctx context.Context, source string, resourceID int64, scope string) (int64, error) {
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

func (r *dataRepository) FinishRun(ctx context.Context, runID int64, status string, rowsIngested int, errMsg string) error {
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

func (r *dataRepository) InsertMetricReadings(ctx context.Context, resourceID int64, readings []acquisition.RawReading) (int, error) {
	if len(readings) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	rows := make([]metricReadingRow, 0, len(readings))
	for _, rd := range readings {
		rows = append(rows, metricReadingRow{
			ResourceID:      resourceID,
			MetricCode:      rd.MetricCode,
			TimestampUTC:    rd.Timestamp,
			IntervalSeconds: rd.IntervalSeconds,
			AvgValue:        rd.Average,
			MinValue:        rd.Minimum,
			MaxValue:        rd.Maximum,
			TotalValue:      rd.Total,
			CountValue:      ptrIntFromFloat(rd.Count),
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

func (r *dataRepository) InsertResourceStats(ctx context.Context, resourceID int64, stats []acquisition.DBResourceStat) (int, error) {
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

func (r *dataRepository) InsertQueryStore(ctx context.Context, resourceID int64, queries []acquisition.QueryStoreQuery, stats []acquisition.QueryStoreRuntimeStat) (int, error) {
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

func (r *dataRepository) ListRuns(ctx context.Context, uids []uuid.UUID, filter acquisition.RunFilter) ([]acquisition.AcquisitionRunDTO, error) {
	if len(uids) == 0 {
		return []acquisition.AcquisitionRunDTO{}, nil
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

	limit := filter.Limit
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

	if filter.Source != "" {
		db = db.Where("collector_run.source = ?", filter.Source)
	}
	if filter.Status != "" {
		db = db.Where("collector_run.status = ?", filter.Status)
	}
	if filter.ResourceUID != nil {
		db = db.Where("resource.resource_uid = ?", *filter.ResourceUID)
	}

	var scanRows []runScanRow
	if err := db.Scan(&scanRows).Error; err != nil {
		return nil, err
	}

	out := make([]acquisition.AcquisitionRunDTO, 0, len(scanRows))
	for _, row := range scanRows {
		dto := acquisition.AcquisitionRunDTO{
			CollectorRunID: row.CollectorRunID,
			ResourceUID:    &row.ResourceUID,
			Source:         row.Source,
			Scope:          strValPtr(row.Scope),
			StartedAtUTC:   row.StartedAtUTC.UnixMilli(),
			Status:         row.Status,
			ErrorMessage:   strValPtr(row.ErrorMessage),
		}
		if row.FinishedAtUTC != nil {
			ms := row.FinishedAtUTC.UnixMilli()
			dto.FinishedAtUTC = &ms
		}
		if row.RowsIngested != nil {
			dto.RowsIngested = row.RowsIngested
		}
		out = append(out, dto)
	}
	return out, nil
}

func (r *dataRepository) GetRun(ctx context.Context, id int64) (acquisition.AcquisitionRunDTO, error) {
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
		return acquisition.AcquisitionRunDTO{}, err
	}
	if scanRow.CollectorRunID == 0 {
		return acquisition.AcquisitionRunDTO{}, acquisition.ErrRunNotFound
	}

	dto := acquisition.AcquisitionRunDTO{
		CollectorRunID: scanRow.CollectorRunID,
		ResourceUID:    &scanRow.ResourceUID,
		Source:         scanRow.Source,
		Scope:          strValPtr(scanRow.Scope),
		StartedAtUTC:   scanRow.StartedAtUTC.UnixMilli(),
		Status:         scanRow.Status,
		ErrorMessage:   strValPtr(scanRow.ErrorMessage),
	}
	if scanRow.FinishedAtUTC != nil {
		ms := scanRow.FinishedAtUTC.UnixMilli()
		dto.FinishedAtUTC = &ms
	}
	if scanRow.RowsIngested != nil {
		dto.RowsIngested = scanRow.RowsIngested
	}
	return dto, nil
}

func ptrIntFromFloat(f *float64) *int {
	if f == nil {
		return nil
	}
	v := int(*f)
	return &v
}

func strValPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
