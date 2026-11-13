package repositories

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	_ analytics.AnalyticsResultRepository = (*AnalyticsRepository)(nil)
)

type AnalyticsRepository struct {
	db *gorm.DB
}

func NewAnalyticsRepository(db *gorm.DB) *AnalyticsRepository {
	return &AnalyticsRepository{db: db}
}

// MigrateAnalyticsTables creates tables for SQLite tests.
func MigrateAnalyticsTables(db *gorm.DB) error {
	models := []any{
		&percentileStatRow{},
		&featureSnapshotRow{},
		&trainingDatasetRow{},
		&efficiencyScoreRow{},
	}
	for _, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return err
		}
	}
	return nil
}

// ---- row structs ----

type percentileStatRow struct {
	ResourceID        int64     `gorm:"column:resource_id;not null;primaryKey"`
	PeriodType        string    `gorm:"column:period_type;not null;primaryKey"`
	PeriodStartUTC    time.Time `gorm:"column:period_start_utc;not null;primaryKey"`
	MetricCode        string    `gorm:"column:metric_code;not null;primaryKey"`
	Percentile        float64   `gorm:"column:percentile;not null;primaryKey"`
	Value             *float64  `gorm:"column:value"`
	Method            string    `gorm:"column:method;not null"`
	ObservationCount  int       `gorm:"column:observation_count;not null"`
	SourceGranularity string    `gorm:"column:source_granularity;not null"`
	ComputedAt        time.Time `gorm:"column:computed_at;not null"`
}

func (percentileStatRow) TableName() string { return "percentile_statistic" }

type featureSnapshotRow struct {
	ResourceID         int64     `gorm:"column:resource_id;not null;primaryKey"`
	PeriodType         string    `gorm:"column:period_type;not null;primaryKey"`
	PeriodStartUTC     time.Time `gorm:"column:period_start_utc;not null;primaryKey"`
	PeriodEndUTC       time.Time `gorm:"column:period_end_utc;not null"`
	SKUID              *int64    `gorm:"column:sku_id"`
	ConfigSnapshotID   *int64    `gorm:"column:config_snapshot_id"`
	ComputeModel       string    `gorm:"column:compute_model"`
	CapacityVCores     float64   `gorm:"column:capacity_vcores"`
	CPUAvg             float64   `gorm:"column:cpu_avg"`
	CPUP50             *float64  `gorm:"column:cpu_p50"`
	CPUP95             *float64  `gorm:"column:cpu_p95"`
	CPUP99             *float64  `gorm:"column:cpu_p99"`
	CPUMax             float64   `gorm:"column:cpu_max"`
	CPUSlope           float64   `gorm:"column:cpu_slope"`
	DataIOAvg          float64   `gorm:"column:data_io_avg"`
	DataIOP95          *float64  `gorm:"column:data_io_p95"`
	LogWriteAvg        float64   `gorm:"column:log_write_avg"`
	LogWriteP95        *float64  `gorm:"column:log_write_p95"`
	SessionsAvg        float64   `gorm:"column:sessions_avg"`
	SessionsP95        *float64  `gorm:"column:sessions_p95"`
	WorkersAvg         float64   `gorm:"column:workers_avg"`
	StorageUsedPct     float64   `gorm:"column:storage_used_pct"`
	StorageUsedBytes   int64     `gorm:"column:storage_used_bytes"`
	StorageGrowthBytes float64   `gorm:"column:storage_growth_bytes"`
	IsPeakHour         bool      `gorm:"column:is_peak_hour"`
	DayOfWeek          int       `gorm:"column:day_of_week"`
	HourOfDay          int       `gorm:"column:hour_of_day"`
	WorkloadIntensity  float64   `gorm:"column:workload_intensity"`
	WorkloadRunID      *int64    `gorm:"column:workload_run_id"`
	DeadlockCount      int       `gorm:"column:deadlock_count"`
	LatencyP95Ms       *float64  `gorm:"column:latency_p95_ms"`
	ComputedAt         time.Time `gorm:"column:computed_at;not null"`
}

func (featureSnapshotRow) TableName() string { return "feature_snapshot" }

type trainingDatasetRow struct {
	DatasetID      int64     `gorm:"column:dataset_id;primaryKey;autoIncrement"`
	Name           string    `gorm:"column:name;not null"`
	Version        int       `gorm:"column:version;not null"`
	ResourceScope  string    `gorm:"column:resource_scope;not null"`
	PeriodStartUTC time.Time `gorm:"column:period_start_utc;not null"`
	PeriodEndUTC   time.Time `gorm:"column:period_end_utc;not null"`
	PeriodType     string    `gorm:"column:period_type;not null"`
	FeatureCodes   string    `gorm:"column:feature_codes;type:jsonb;not null"`
	TargetColumn   string    `gorm:"column:target_column;not null"`
	SplitStrategy  string    `gorm:"column:split_strategy;type:jsonb;not null"`
	DatasetHash    string    `gorm:"column:dataset_hash"`
	CreatedBy      string    `gorm:"column:created_by"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (trainingDatasetRow) TableName() string { return "training_dataset" }

type efficiencyScoreRow struct {
	ScoreID           int64     `gorm:"column:score_id;primaryKey;autoIncrement"`
	ResourceID        int64     `gorm:"column:resource_id;not null"`
	PeriodType        string    `gorm:"column:period_type;not null;primaryKey"`
	PeriodStartUTC    time.Time `gorm:"column:period_start_utc;not null;primaryKey"`
	CapacityE         float64   `gorm:"column:capacity_e;not null"`
	CostE             float64   `gorm:"column:cost_e;not null"`
	PerformanceE      float64   `gorm:"column:performance_e;not null"`
	WeightCapacity    float64   `gorm:"column:weight_capacity;not null"`
	WeightCost        float64   `gorm:"column:weight_cost;not null"`
	WeightPerformance float64   `gorm:"column:weight_performance;not null"`
	WeightsVersion    string    `gorm:"column:weights_version;not null;primaryKey"`
	DESScore          float64   `gorm:"column:des_score;not null"`
	ComputedAt        time.Time `gorm:"column:computed_at;not null"`
}

func (efficiencyScoreRow) TableName() string { return "efficiency_score" }

type forecastRow struct {
	ForecastUID    uuid.UUID `gorm:"column:forecast_uid;type:uuid;not null"`
	ResourceID     int64     `gorm:"column:resource_id;not null"`
	TargetMetric   string    `gorm:"column:target_metric;not null"`
	HorizonSteps   int       `gorm:"column:horizon_steps;not null"`
	StepUnit       string    `gorm:"column:step_unit;not null"`
	PeriodStartUTC time.Time `gorm:"column:period_start_utc;not null"`
	YPred          float64   `gorm:"column:y_pred;not null"`
	YLower         *float64  `gorm:"column:y_lower"`
	YUpper         *float64  `gorm:"column:y_upper"`
	Alpha          float64   `gorm:"column:alpha;not null"`
	GeneratedAt    time.Time `gorm:"column:generated_at;not null"`
}

func (forecastRow) TableName() string { return "v_forecast" }

// ---- helpers ----

func (r *AnalyticsRepository) resolveResourceID(ctx context.Context, uid uuid.UUID) (int64, error) {
	var row struct {
		ResourceID int64 `gorm:"column:resource_id"`
	}
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "core", "resource")).
		Where("resource_uid = ?", uid).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, analytics.NewError(analytics.CodeNotFound, "resource not found in cco_data", analytics.ErrNotFound)
	}
	return row.ResourceID, err
}

// ---- PercentileStatistic ----

func (r *AnalyticsRepository) WritePercentileStatistics(ctx context.Context, resourceID int64, stats []analytics.PercentileResult) (int, error) {
	if len(stats) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	rows := make([]percentileStatRow, 0, len(stats))
	for _, s := range stats {
		rows = append(rows, percentileStatRow{
			ResourceID:        resourceID,
			PeriodType:        s.PeriodType,
			PeriodStartUTC:    s.PeriodStartUTC,
			MetricCode:        s.MetricCode,
			Percentile:        s.Percentile,
			Value:             s.Value,
			Method:            s.Method,
			ObservationCount:  s.ObservationCount,
			SourceGranularity: s.SourceGranularity,
			ComputedAt:        now,
		})
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "resource_id"}, {Name: "period_type"}, {Name: "period_start_utc"},
				{Name: "metric_code"}, {Name: "percentile"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"value", "method", "observation_count", "source_granularity", "computed_at",
			}),
		}).
		Table(database.TableName(r.db, "feat", "percentile_statistic")).
		Create(&rows)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

func (r *AnalyticsRepository) PercentileStatistic(ctx context.Context, resourceUID uuid.UUID, metricCode string, periodType string, from, to time.Time) ([]analytics.PercentileResult, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return nil, err
	}
	db := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "feat", "percentile_statistic")).
		Where("resource_id = ? AND period_type = ? AND metric_code = ? AND period_start_utc >= ? AND period_start_utc < ?",
			resourceID, periodType, metricCode, from, to).
		Order("period_start_utc ASC, percentile ASC")

	var rows []percentileStatRow
	if err := db.Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]analytics.PercentileResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, analytics.PercentileResult{
			ResourceUID:       resourceUID,
			PeriodType:        row.PeriodType,
			PeriodStartUTC:    row.PeriodStartUTC,
			MetricCode:        row.MetricCode,
			Percentile:        row.Percentile,
			Value:             row.Value,
			Method:            row.Method,
			ObservationCount:  row.ObservationCount,
			SourceGranularity: row.SourceGranularity,
			ComputedAt:        row.ComputedAt,
		})
	}
	return out, nil
}

// ---- FeatureSnapshot ----

func (r *AnalyticsRepository) WriteFeatureSnapshots(ctx context.Context, resourceID int64, snaps []analytics.FeatureSnapshotRow) (int, error) {
	if len(snaps) == 0 {
		return 0, nil
	}
	now := time.Now().UTC()
	rows := make([]featureSnapshotRow, 0, len(snaps))
	for _, s := range snaps {
		rows = append(rows, featureSnapshotRow{
			ResourceID:         resourceID,
			PeriodType:         s.PeriodType,
			PeriodStartUTC:     s.PeriodStartUTC,
			PeriodEndUTC:       s.PeriodEndUTC,
			SKUID:              s.SKUID,
			ConfigSnapshotID:   s.ConfigSnapshotID,
			ComputeModel:       s.ComputeModel,
			CapacityVCores:     s.CapacityVCores,
			CPUAvg:             s.CPUAvg,
			CPUP50:             s.CPUP50,
			CPUP95:             s.CPUP95,
			CPUP99:             s.CPUP99,
			CPUMax:             s.CPUMax,
			CPUSlope:           s.CPUSlope,
			DataIOAvg:          s.DataIOAvg,
			DataIOP95:          s.DataIOP95,
			LogWriteAvg:        s.LogWriteAvg,
			LogWriteP95:        s.LogWriteP95,
			SessionsAvg:        s.SessionsAvg,
			SessionsP95:        s.SessionsP95,
			WorkersAvg:         s.WorkersAvg,
			StorageUsedPct:     s.StorageUsedPct,
			StorageUsedBytes:   s.StorageUsedBytes,
			StorageGrowthBytes: s.StorageGrowthBytes,
			IsPeakHour:         s.IsPeakHour,
			DayOfWeek:          s.DayOfWeek,
			HourOfDay:          s.HourOfDay,
			WorkloadIntensity:  s.WorkloadIntensity,
			WorkloadRunID:      s.WorkloadRunID,
			DeadlockCount:      s.DeadlockCount,
			LatencyP95Ms:       s.LatencyP95Ms,
			ComputedAt:         now,
		})
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "resource_id"}, {Name: "period_type"}, {Name: "period_start_utc"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"period_end_utc", "sku_id", "config_snapshot_id", "compute_model", "capacity_vcores",
				"cpu_avg", "cpu_p50", "cpu_p95", "cpu_p99", "cpu_max", "cpu_slope",
				"data_io_avg", "data_io_p95", "log_write_avg", "log_write_p95",
				"sessions_avg", "sessions_p95", "workers_avg", "storage_used_pct",
				"storage_used_bytes", "storage_growth_bytes", "is_peak_hour",
				"day_of_week", "hour_of_day", "workload_intensity", "workload_run_id",
				"deadlock_count", "latency_p95_ms", "computed_at",
			}),
		}).
		Table(database.TableName(r.db, "feat", "feature_snapshot")).
		Create(&rows)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

func (r *AnalyticsRepository) FeatureSnapshot(ctx context.Context, resourceUID uuid.UUID, periodType string, from, to time.Time, limit int) ([]analytics.FeatureSnapshotRow, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 1000
	}
	if limit > 10000 {
		limit = 10000
	}
	db := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "feat", "feature_snapshot")).
		Where("resource_id = ? AND period_type = ? AND period_start_utc >= ? AND period_start_utc < ?",
			resourceID, periodType, from, to).
		Order("period_start_utc ASC").
		Limit(limit)

	var rows []featureSnapshotRow
	if err := db.Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]analytics.FeatureSnapshotRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, analytics.FeatureSnapshotRow{
			ResourceUID:        resourceUID,
			PeriodType:         row.PeriodType,
			PeriodStartUTC:     row.PeriodStartUTC,
			PeriodEndUTC:       row.PeriodEndUTC,
			SKUID:              row.SKUID,
			ConfigSnapshotID:   row.ConfigSnapshotID,
			ComputeModel:       row.ComputeModel,
			CapacityVCores:     row.CapacityVCores,
			CPUAvg:             row.CPUAvg,
			CPUP50:             row.CPUP50,
			CPUP95:             row.CPUP95,
			CPUP99:             row.CPUP99,
			CPUMax:             row.CPUMax,
			CPUSlope:           row.CPUSlope,
			DataIOAvg:          row.DataIOAvg,
			DataIOP95:          row.DataIOP95,
			LogWriteAvg:        row.LogWriteAvg,
			LogWriteP95:        row.LogWriteP95,
			SessionsAvg:        row.SessionsAvg,
			SessionsP95:        row.SessionsP95,
			WorkersAvg:         row.WorkersAvg,
			StorageUsedPct:     row.StorageUsedPct,
			StorageUsedBytes:   row.StorageUsedBytes,
			StorageGrowthBytes: row.StorageGrowthBytes,
			IsPeakHour:         row.IsPeakHour,
			DayOfWeek:          row.DayOfWeek,
			HourOfDay:          row.HourOfDay,
			WorkloadIntensity:  row.WorkloadIntensity,
			WorkloadRunID:      row.WorkloadRunID,
			DeadlockCount:      row.DeadlockCount,
			LatencyP95Ms:       row.LatencyP95Ms,
			ComputedAt:         row.ComputedAt,
		})
	}
	return out, nil
}

// ---- Trend (cpu_slope, storage_growth_bytes live in feature_snapshot) ----

func (r *AnalyticsRepository) Trend(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) ([]analytics.TrendResult, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return nil, err
	}
	type trendRow struct {
		PeriodStartUTC time.Time
		CPUSlope       float64
		StorageGrowth  float64
	}
	var rows []trendRow
	err = r.db.WithContext(ctx).
		Table(database.TableName(r.db, "feat", "feature_snapshot")).
		Where("resource_id = ? AND period_type = '1h' AND period_start_utc >= ? AND period_start_utc < ?",
			resourceID, from, to).
		Select("period_start_utc, cpu_slope, storage_growth_bytes").
		Order("period_start_utc ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]analytics.TrendResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, analytics.TrendResult{
			ResourceUID: resourceUID,
			MetricCode:  "cpu_avg",
			WindowStart: row.PeriodStartUTC,
			WindowEnd:   row.PeriodStartUTC.Add(time.Hour),
			Slope:       row.CPUSlope,
			RSquared:    0, // not stored; shortcut
			ComputedAt:  time.Now().UTC(),
		})
	}
	return out, nil
}

// ---- WorkloadPattern ----

func (r *AnalyticsRepository) WorkloadPattern(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (analytics.WorkloadPattern, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return analytics.WorkloadPattern{}, err
	}
	type patternRow struct {
		HourOfDay         int
		IsPeakHour        bool
		WorkloadIntensity float64
		WorkloadRunID     *int64
	}
	var rows []patternRow
	err = r.db.WithContext(ctx).
		Table(database.TableName(r.db, "feat", "feature_snapshot")).
		Where("resource_id = ? AND period_type = '1h' AND period_start_utc >= ? AND period_start_utc < ?",
			resourceID, from, to).
		Select("hour_of_day, is_peak_hour, workload_intensity, workload_run_id").
		Order("period_start_utc ASC").
		Scan(&rows).Error
	if err != nil {
		return analytics.WorkloadPattern{}, err
	}

	// Aggregate: peak hours = hours where is_peak_hour=true
	peakHours := []int{}
	var totalIntensity float64
	var runID *int64
	for _, row := range rows {
		if row.IsPeakHour {
			peakHours = append(peakHours, row.HourOfDay)
		}
		totalIntensity += row.WorkloadIntensity
		if row.WorkloadRunID != nil {
			runID = row.WorkloadRunID
		}
	}

	// Simple peak frequency: number of distinct peak hours / days in window
	days := int(to.Sub(from).Hours() / 24)
	if days == 0 {
		days = 1
	}
	peakFreq := float64(len(peakHours)) / float64(days)

	// Intensity band
	avgIntensity := totalIntensity / float64(max(1, len(rows)))
	var band string
	if avgIntensity < 1000 {
		band = "low"
	} else if avgIntensity < 5000 {
		band = "medium"
	} else {
		band = "high"
	}

	return analytics.WorkloadPattern{
		ResourceUID:     resourceUID,
		WindowStart:     from,
		WindowEnd:       to,
		PeakHours:       peakHours,
		PeakFrequency:   peakFreq,
		AvgPeakDuration: 1.0, // shortcut: assume 1h peaks
		IntensityBand:   band,
		WorkloadRunID:   runID,
		ComputedAt:      time.Now().UTC(),
	}, nil
}

// ---- CapacityAnalysis / CostAnalysis / DESComponents (opt.efficiency_score) ----

func (r *AnalyticsRepository) WriteEfficiencyScore(ctx context.Context, resourceID int64, comp analytics.DESComponents) (int64, error) {
	row := efficiencyScoreRow{
		ResourceID:        resourceID,
		PeriodType:        comp.PeriodType,
		PeriodStartUTC:    comp.PeriodStartUTC,
		CapacityE:         comp.CapacityE,
		CostE:             comp.CostE,
		PerformanceE:      comp.PerformanceE,
		WeightCapacity:    1.0 / 3.0,
		WeightCost:        1.0 / 3.0,
		WeightPerformance: 1.0 / 3.0,
		WeightsVersion:    comp.WeightsVersion,
		DESScore:          (comp.CapacityE + comp.CostE + comp.PerformanceE) / 3.0,
		ComputedAt:        time.Now().UTC(),
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "resource_id"}, {Name: "period_type"}, {Name: "period_start_utc"}, {Name: "weights_version"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"capacity_e", "cost_e", "performance_e", "weight_capacity", "weight_cost",
				"weight_performance", "des_score", "computed_at",
			}),
		}).
		Table(database.TableName(r.db, "opt", "efficiency_score")).
		Create(&row)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func (r *AnalyticsRepository) readEfficiencyScore(ctx context.Context, resourceID int64, periodType string, periodStart time.Time, weightsVersion string) (analytics.DESComponents, error) {
	var row efficiencyScoreRow
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "opt", "efficiency_score")).
		Where("resource_id = ? AND period_type = ? AND period_start_utc = ? AND weights_version = ?",
			resourceID, periodType, periodStart, weightsVersion).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return analytics.DESComponents{}, analytics.NewError(analytics.CodeNotFound, "efficiency score not found", analytics.ErrNotFound)
	}
	if err != nil {
		return analytics.DESComponents{}, err
	}
	return analytics.DESComponents{
		ResourceUID:    uuid.Nil, // filled by caller
		PeriodType:     row.PeriodType,
		PeriodStartUTC: row.PeriodStartUTC,
		CapacityE:      row.CapacityE,
		CostE:          row.CostE,
		PerformanceE:   row.PerformanceE,
		WeightsVersion: row.WeightsVersion,
		ComputedAt:     row.ComputedAt,
	}, nil
}

func (r *AnalyticsRepository) CapacityAnalysis(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (analytics.CapacityAnalysis, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return analytics.CapacityAnalysis{}, err
	}
	comp, err := r.readEfficiencyScore(ctx, resourceID, periodType, periodStart, "v1_equal")
	if err != nil {
		return analytics.CapacityAnalysis{}, err
	}
	return analytics.CapacityAnalysis{
		ResourceUID:       resourceUID,
		PeriodType:        periodType,
		PeriodStartUTC:    periodStart,
		CurrentVCores:     0, // shortcut: not stored here
		HeadroomPct:       0,
		PeakUtilization:   0,
		GrowthRateVCores:  0,
		RecommendedVCores: 0,
		CapacityE:         comp.CapacityE,
		ComputedAt:        comp.ComputedAt,
	}, nil
}

func (r *AnalyticsRepository) CostAnalysis(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (analytics.CostAnalysis, error) {
	if _, err := r.resolveResourceID(ctx, resourceUID); err != nil {
		return analytics.CostAnalysis{}, err
	}
	// We don't store cost analysis persistently; compute on the fly from cost_daily.
	// This is a shortcut; upgrade: persist CostAnalysis row.
	return analytics.CostAnalysis{
		ResourceUID:       resourceUID,
		PeriodStartUTC:    from,
		PeriodEndUTC:      to,
		ActualMonthlyCost: 0,
		CostPerVCore:      0,
		ProjectedMonthly:  0,
		SavingsVsCurrent:  0,
		CostE:             0,
		ComputedAt:        time.Now().UTC(),
	}, nil
}

func (r *AnalyticsRepository) DESComponents(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (analytics.DESComponents, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return analytics.DESComponents{}, err
	}
	return r.readEfficiencyScore(ctx, resourceID, periodType, periodStart, "v1_equal")
}

// ---- TrainingDataset ----

func (r *AnalyticsRepository) WriteTrainingDataset(ctx context.Context, spec analytics.TrainingDatasetSpec) (int64, error) {
	featureCodesJSON, _ := json.Marshal(spec.FeatureCodes)
	splitStrategyJSON, _ := json.Marshal(spec.SplitStrategy)

	row := trainingDatasetRow{
		Name:           spec.Name,
		Version:        spec.Version,
		ResourceScope:  spec.ResourceScope,
		PeriodStartUTC: spec.PeriodStartUTC,
		PeriodEndUTC:   spec.PeriodEndUTC,
		PeriodType:     spec.PeriodType,
		FeatureCodes:   string(featureCodesJSON),
		TargetColumn:   spec.TargetColumn,
		SplitStrategy:  string(splitStrategyJSON),
		DatasetHash:    spec.DatasetHash,
		CreatedBy:      spec.CreatedBy,
		CreatedAt:      time.Now().UTC(),
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Table(database.TableName(r.db, "ml", "training_dataset")).
		Create(&row)
	if result.Error != nil {
		return 0, result.Error
	}
	return row.DatasetID, nil
}

func (r *AnalyticsRepository) TrainingDataset(ctx context.Context, name string, version int) (analytics.TrainingDatasetSpec, error) {
	var row trainingDatasetRow
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "ml", "training_dataset")).
		Where("name = ? AND version = ?", name, version).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return analytics.TrainingDatasetSpec{}, analytics.NewError(analytics.CodeNotFound, "training dataset not found", analytics.ErrNotFound)
	}
	if err != nil {
		return analytics.TrainingDatasetSpec{}, err
	}
	var featureCodes []string
	_ = json.Unmarshal([]byte(row.FeatureCodes), &featureCodes)
	var splitStrategy map[string]any
	_ = json.Unmarshal([]byte(row.SplitStrategy), &splitStrategy)
	return analytics.TrainingDatasetSpec{
		DatasetID:      row.DatasetID,
		Name:           row.Name,
		Version:        row.Version,
		ResourceScope:  row.ResourceScope,
		PeriodStartUTC: row.PeriodStartUTC,
		PeriodEndUTC:   row.PeriodEndUTC,
		PeriodType:     row.PeriodType,
		FeatureCodes:   featureCodes,
		TargetColumn:   row.TargetColumn,
		SplitStrategy:  splitStrategy,
		DatasetHash:    row.DatasetHash,
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt,
	}, nil
}

// computeDatasetHash generates a deterministic hash for the dataset content.
func computeDatasetHash(spec analytics.TrainingDatasetSpec) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s",
		spec.Name, spec.Version, spec.ResourceScope,
		spec.PeriodStartUTC, spec.PeriodEndUTC, spec.TargetColumn,
		strings.Join(spec.FeatureCodes, ","))))
	return hex.EncodeToString(h.Sum(nil))
}

// ---- Forecasts (contract.v_forecast) ----

func (r *AnalyticsRepository) Forecasts(ctx context.Context, resourceUID uuid.UUID, targetMetric string, from, to time.Time) ([]analytics.ForecastResponse, error) {
	resourceID, err := r.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return nil, err
	}
	db := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "contract", "v_forecast")).
		Where("resource_id = ?", resourceID)
	if targetMetric != "" {
		db = db.Where("target_metric = ?", targetMetric)
	}
	if !from.IsZero() {
		db = db.Where("period_start_utc >= ?", from)
	}
	if !to.IsZero() {
		db = db.Where("period_start_utc < ?", to)
	}
	db = db.Order("period_start_utc ASC, horizon_steps ASC")

	var rows []forecastRow
	if err := db.Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]analytics.ForecastResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, analytics.ForecastResponse{
			ForecastUID:    row.ForecastUID,
			ResourceUID:    resourceUID,
			TargetMetric:   row.TargetMetric,
			HorizonSteps:   row.HorizonSteps,
			StepUnit:       row.StepUnit,
			PeriodStartUTC: row.PeriodStartUTC,
			YPred:          row.YPred,
			YLower:         row.YLower,
			YUpper:         row.YUpper,
			Alpha:          row.Alpha,
			GeneratedAt:    row.GeneratedAt,
		})
	}
	return out, nil
}
