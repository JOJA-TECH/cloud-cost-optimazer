package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AnalyticsEngine is the core analytical contract (Module 5). It produces
// evidence: percentiles, trends, patterns, capacity/cost analysis, feature
// snapshots, training datasets, and forecast requests. It never makes final
// business recommendations (that is Module 7).
type AnalyticsEngine interface {
	// ComputeFeatureSnapshot computes an hourly (period_type='1h') feature
	// snapshot for a resource in [from, to), writing to feat.feature_snapshot
	// and feat.percentile_statistic. Idempotent by
	// (resource_id, period_type, period_start_utc).
	ComputeFeatureSnapshot(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (int, error)

	// ComputePercentiles calculates exact percentiles from raw.metric_sample
	// for a metric/window and writes feat.percentile_statistic.
	// If raw.metric_sample has no observations → method='not_available', value=NULL, obs=0.
	ComputePercentiles(ctx context.Context, resourceUID uuid.UUID, metricCode string, from, to time.Time, periodType string) ([]PercentileResult, error)

	// ComputeTrends calculates slopes (slope) and R² over hourly series and
	// updates feat.feature_snapshot (cpu_slope, storage_growth_bytes).
	ComputeTrends(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) ([]TrendResult, error)

	// DetectWorkloadPatterns identifies peak hours, frequency/duration of
	// peaks, intensity bands. Writes to feat.feature_snapshot
	// (is_peak_hour, workload_intensity, workload_run_id).
	DetectWorkloadPatterns(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (WorkloadPattern, error)

	// AnalyzeCapacity computes headroom, peak%, growth rate, recommended vcores.
	// Writes capacity_e component to opt.efficiency_score.
	AnalyzeCapacity(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (CapacityAnalysis, error)

	// AnalyzeCosts computes cost per vCore, projected monthly, vs candidates.
	// Writes cost_e component to opt.efficiency_score.
	AnalyzeCosts(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (CostAnalysis, error)

	// BuildTrainingDataset creates/updates ml.training_dataset with
	// feature_codes, target_column, split_strategy, dataset_hash. Returns dataset_id.
	BuildTrainingDataset(ctx context.Context, spec TrainingDatasetSpec) (int64, error)

	// RequestForecast triggers the ML service (Module 6) via MLPort for a
	// resource/horizon. ML service writes ml.forecast; this returns forecast_uid.
	RequestForecast(ctx context.Context, req ForecastRequest) (uuid.UUID, error)

	// ComputeDESComponents calculates capacity_e, cost_e, performance_e for a
	// resource/period and writes/updates opt.efficiency_score (without weights;
	// Module 7 applies weights). weights_version='v1_equal' in Release 1.0.
	ComputeDESComponents(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (DESComponents, error)
}

// AnalyticsResultRepository provides read access to analytical evidence for
// consumers (Module 7, dashboards, ML).
type AnalyticsResultRepository interface {
	// FeatureSnapshot returns snapshots from feat.feature_snapshot.
	FeatureSnapshot(ctx context.Context, resourceUID uuid.UUID, periodType string, from, to time.Time, limit int) ([]FeatureSnapshotRow, error)

	// PercentileStatistic returns exact percentiles from feat.percentile_statistic.
	PercentileStatistic(ctx context.Context, resourceUID uuid.UUID, metricCode string, periodType string, from, to time.Time) ([]PercentileResult, error)

	// Trend returns computed trends (cpu_slope, storage_growth_bytes).
	Trend(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) ([]TrendResult, error)

	// WorkloadPattern returns detected pattern for a resource/window.
	WorkloadPattern(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (WorkloadPattern, error)

	// CapacityAnalysis / CostAnalysis / DESComponents for Module 7.
	CapacityAnalysis(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (CapacityAnalysis, error)
	CostAnalysis(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (CostAnalysis, error)
	DESComponents(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (DESComponents, error)

	// TrainingDataset returns dataset spec for ML module.
	TrainingDataset(ctx context.Context, name string, version int) (TrainingDatasetSpec, error)

	// Forecasts returns forecast rows from contract.v_forecast, optionally
	// filtered by resource_uid, target_metric and [from, to).
	Forecasts(ctx context.Context, resourceUID uuid.UUID, targetMetric string, from, to time.Time) ([]ForecastResponse, error)
}

// MLPort is the contract to the external Python ML service (Module 6).
type MLPort interface {
	// Forecast requests a prediction from the ML service. ML service writes
	// ml.forecast and returns forecast_uid.
	Forecast(ctx context.Context, req ForecastRequest) (uuid.UUID, error)

	// ValidateContract checks the gRPC/HTTP contract version (ml.inference.v1).
	ValidateContract(ctx context.Context) error
}

// ---- DTOs ----

// PercentileResult is one row of feat.percentile_statistic.
type PercentileResult struct {
	ResourceUID       uuid.UUID
	PeriodType        string
	PeriodStartUTC    time.Time
	MetricCode        string
	Percentile        float64
	Value             *float64
	Method            string // 'exact_sample' | 'weighted_histogram' | 'not_available'
	ObservationCount  int
	SourceGranularity string
	ComputedAt        time.Time
}

// TrendResult represents a computed slope/R² over a window.
type TrendResult struct {
	ResourceUID uuid.UUID
	MetricCode  string
	WindowStart time.Time
	WindowEnd   time.Time
	Slope       float64 // per hour
	RSquared    float64
	ComputedAt  time.Time
}

// WorkloadPattern captures peak hours, frequency/duration of peaks, intensity.
type WorkloadPattern struct {
	ResourceUID     uuid.UUID
	WindowStart     time.Time
	WindowEnd       time.Time
	PeakHours       []int   // 0-23
	PeakFrequency   float64 // peaks per day
	AvgPeakDuration float64 // hours
	IntensityBand   string  // 'low' | 'medium' | 'high'
	WorkloadRunID   *int64  // if window falls in a controlled run
	ComputedAt      time.Time
}

// CapacityAnalysis is the capacity evidence for Module 7.
type CapacityAnalysis struct {
	ResourceUID       uuid.UUID
	PeriodType        string
	PeriodStartUTC    time.Time
	CurrentVCores     float64
	HeadroomPct       float64
	PeakUtilization   float64
	GrowthRateVCores  float64 // per day
	RecommendedVCores float64
	CapacityE         float64 // 0-1 component for DES
	ComputedAt        time.Time
}

// CostAnalysis is the cost evidence for Module 7.
type CostAnalysis struct {
	ResourceUID       uuid.UUID
	PeriodStartUTC    time.Time
	PeriodEndUTC      time.Time
	ActualMonthlyCost float64
	CostPerVCore      float64
	ProjectedMonthly  float64
	SavingsVsCurrent  float64
	CostE             float64 // 0-1 component for DES
	ComputedAt        time.Time
}

// DESComponents are the three sub-scores for Database Efficiency Score.
// Module 7 applies weights (v1_equal = 1/3 each).
type DESComponents struct {
	ResourceUID    uuid.UUID
	PeriodType     string
	PeriodStartUTC time.Time
	CapacityE      float64
	CostE          float64
	PerformanceE   float64
	WeightsVersion string // 'v1_equal' in Release 1.0
	ComputedAt     time.Time
}

// FeatureSnapshotRow is a row from feat.feature_snapshot (wide table).
type FeatureSnapshotRow struct {
	ResourceUID        uuid.UUID
	PeriodType         string
	PeriodStartUTC     time.Time
	PeriodEndUTC       time.Time
	SKUID              *int64
	ConfigSnapshotID   *int64
	ComputeModel       string
	CapacityVCores     float64
	CPUAvg             float64
	CPUP50             *float64
	CPUP95             *float64
	CPUP99             *float64
	CPUMax             float64
	CPUSlope           float64
	DataIOAvg          float64
	DataIOP95          *float64
	LogWriteAvg        float64
	LogWriteP95        *float64
	SessionsAvg        float64
	SessionsP95        *float64
	WorkersAvg         float64
	StorageUsedPct     float64
	StorageUsedBytes   int64
	StorageGrowthBytes float64
	IsPeakHour         bool
	DayOfWeek          int
	HourOfDay          int
	WorkloadIntensity  float64
	WorkloadRunID      *int64
	DeadlockCount      int
	LatencyP95Ms       *float64
	ComputedAt         time.Time
}

// TrainingDatasetSpec defines a versioned dataset for ML (ml.training_dataset).
type TrainingDatasetSpec struct {
	DatasetID      int64
	Name           string
	Version        int
	ResourceScope  string // 'all' or resource_uid
	PeriodStartUTC time.Time
	PeriodEndUTC   time.Time
	PeriodType     string
	FeatureCodes   []string
	TargetColumn   string
	SplitStrategy  map[string]any // jsonb
	DatasetHash    string
	CreatedBy      string
	CreatedAt      time.Time
}

// ForecastRequest is sent to MLPort to trigger forecasting.
type ForecastRequest struct {
	ResourceUID    uuid.UUID
	TargetMetric   string    // 'cpu_avg' | 'workload_intensity' | 'storage_used_bytes'
	HorizonSteps   int       // 1..H
	StepUnit       string    // '15m' | '1h' | '1d'
	PeriodStartUTC time.Time // first future period to forecast
	ModelVersionID *int64    // specific model_version_id, or nil for champion
}

// ForecastResponse mirrors ml.forecast (read via contract.v_forecast).
type ForecastResponse struct {
	ForecastUID    uuid.UUID
	ResourceUID    uuid.UUID
	TargetMetric   string
	HorizonSteps   int
	StepUnit       string
	PeriodStartUTC time.Time
	YPred          float64
	YLower         *float64
	YUpper         *float64
	Alpha          float64
	GeneratedAt    time.Time
}
