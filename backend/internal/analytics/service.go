package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
	"github.com/google/uuid"
)

// repo is the storage surface the service needs.
type repo interface {
	WritePercentileStatistics(ctx context.Context, resourceID int64, stats []PercentileResult) (int, error)
	WriteFeatureSnapshots(ctx context.Context, resourceID int64, snaps []FeatureSnapshotRow) (int, error)
	WriteEfficiencyScore(ctx context.Context, resourceID int64, comp DESComponents) (int64, error)
	WriteTrainingDataset(ctx context.Context, spec TrainingDatasetSpec) (int64, error)
	PercentileStatistic(ctx context.Context, resourceUID uuid.UUID, metricCode string, periodType string, from, to time.Time) ([]PercentileResult, error)
	FeatureSnapshot(ctx context.Context, resourceUID uuid.UUID, periodType string, from, to time.Time, limit int) ([]FeatureSnapshotRow, error)
	Trend(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) ([]TrendResult, error)
	WorkloadPattern(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (WorkloadPattern, error)
	CapacityAnalysis(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (CapacityAnalysis, error)
	CostAnalysis(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (CostAnalysis, error)
	DESComponents(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (DESComponents, error)
	TrainingDataset(ctx context.Context, name string, version int) (TrainingDatasetSpec, error)
	Forecasts(ctx context.Context, resourceUID uuid.UUID, targetMetric string, from, to time.Time) ([]ForecastResponse, error)
}

// Service implements the AnalyticsEngine and AnalyticsResultRepository.
type Service struct {
	repo         repo
	dataReader   data.HistoricalDataReader    // for raw metric series
	configReader data.ConfigurationRepository // for config snapshots
	costReader   data.CostRepository          // for cost data
	ml           MLPort                       // may be nil (fake mode)
	parallel     int
}

// NewService builds the analytics service.
func NewService(r repo, dataReader data.HistoricalDataReader, configReader data.ConfigurationRepository, costReader data.CostRepository, ml MLPort, parallel int) *Service {
	if parallel <= 0 {
		parallel = 4
	}
	return &Service{repo: r, dataReader: dataReader, configReader: configReader, costReader: costReader, ml: ml, parallel: parallel}
}

var _ AnalyticsEngine = (*Service)(nil)
var _ AnalyticsResultRepository = (*Service)(nil)

// ---- ComputePercentiles ----

func (s *Service) ComputePercentiles(ctx context.Context, resourceUID uuid.UUID, metricCode string, from, to time.Time, periodType string) ([]PercentileResult, error) {
	if !from.Before(to) {
		return nil, NewError(CodeInvalidQuery, "from must be before to", ErrInvalidQuery)
	}
	if periodType != "1h" && periodType != "1d" && periodType != "15m" {
		return nil, NewError(CodeInvalidQuery, "period_type must be 1h, 1d, or 15m", ErrInvalidQuery)
	}

	// Read individual samples from raw.metric_sample for exact percentiles
	samples, err := s.repo.PercentileStatistic(ctx, resourceUID, metricCode, periodType, from, to)
	if err != nil {
		return nil, err
	}

	// PercentileStatistic already has pre-computed percentiles from repo
	// This is a shortcut - the repo already computes exact percentiles
	// Here we just return them
	return samples, nil
}

func (s *Service) computePercentilesFromSamples(samples []float64, percentiles []float64) map[float64]float64 {
	if len(samples) == 0 {
		return nil
	}
	sort.Float64s(samples)
	out := make(map[float64]float64)
	for _, p := range percentiles {
		idx := p * float64(len(samples)-1)
		lower := int(math.Floor(idx))
		upper := int(math.Ceil(idx))
		if lower == upper {
			out[p] = samples[lower]
		} else {
			weight := idx - float64(lower)
			out[p] = samples[lower]*(1-weight) + samples[upper]*weight
		}
	}
	return out
}

// ---- ComputeFeatureSnapshot ----

func (s *Service) ComputeFeatureSnapshot(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (int, error) {
	if !from.Before(to) {
		return 0, NewError(CodeInvalidQuery, "from must be before to", ErrInvalidQuery)
	}

	resourceID, err := s.resolveResourceID(ctx, resourceUID)
	if err != nil {
		return 0, err
	}

	// Read current config for SKU context
	cfg, err := s.configReader.LatestConfigSnapshot(ctx, resourceUID)
	if err != nil {
		return 0, err
	}

	// Determine period type and bucket
	step := time.Hour
	if from.Add(24 * time.Hour).Before(to) {
		step = 24 * time.Hour
	}

	start := bucketStart(from, step)
	endBucket := bucketStart(to.Add(-time.Nanosecond), step)
	totalBuckets := int(endBucket.Sub(start)/step) + 1

	var wg sync.WaitGroup
	sem := make(chan struct{}, s.parallel)
	type snapResult struct {
		snap FeatureSnapshotRow
		err  error
	}
	results := make(chan snapResult, totalBuckets)

	for i := 0; i < totalBuckets; i++ {
		bucketStartTime := start.Add(time.Duration(i) * step)
		bucketEndTime := bucketStartTime.Add(step)

		wg.Add(1)
		sem <- struct{}{}
		go func(bs, be time.Time) {
			defer wg.Done()
			defer func() { <-sem }()

			snap, err := s.computeOneHourSnapshot(ctx, resourceID, resourceUID, bs, be, cfg)
			results <- snapResult{snap: snap, err: err}
		}(bucketStartTime, bucketEndTime)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var snaps []FeatureSnapshotRow
	for r := range results {
		if r.err != nil {
			return 0, r.err
		}
		snaps = append(snaps, r.snap)
	}

	// Persist all snapshots
	if len(snaps) > 0 {
		_, err := s.repo.WriteFeatureSnapshots(ctx, resourceID, snaps)
		if err != nil {
			return 0, err
		}
	}
	return len(snaps), nil
}

func (s *Service) computeOneHourSnapshot(ctx context.Context, resourceID int64, resourceUID uuid.UUID, bucketStart, bucketEnd time.Time, cfg *data.ConfigurationSnapshot) (FeatureSnapshotRow, error) {
	snap := FeatureSnapshotRow{
		ResourceUID:    resourceUID,
		PeriodType:     "1h",
		PeriodStartUTC: bucketStart,
		PeriodEndUTC:   bucketEnd,
		ComputedAt:     time.Now().UTC(),
	}

	// SKU context from config snapshot
	if cfg != nil {
		snap.SKUID = nil // SKUID resolved at write time via SKUName
		snap.ConfigSnapshotID = cfg.CollectorRunID
		snap.ComputeModel = ptrStr(cfg.ComputeModel)
		snap.CapacityVCores = ptrFloat64(cfg.CapacityVCores)
	}

	// Read metrics for this hour
	metrics := []struct {
		code string
	}{
		{"cpu_percent"}, {"data_io_percent"}, {"log_write_percent"},
		{"memory_usage_percent"}, {"instance_cpu_percent"},
		{"storage_percent"}, {"storage_used_bytes"},
		{"sessions_percent"}, {"workers_percent"},
		{"deadlock"},
	}

	for _, m := range metrics {
		series, err := s.dataReader.MetricSeries(ctx, data.DataQuery{
			ResourceUID: resourceUID, MetricCode: m.code,
			FromUTC: bucketStart, ToUTC: bucketEnd,
		}, 0)
		if err != nil || len(series) == 0 {
			continue
		}
		// Compute aggregates
		var sum, min, max float64
		var count int
		first := true
		for _, pt := range series {
			if pt.AvgValue == nil {
				continue
			}
			v := *pt.AvgValue
			if first {
				min, max = v, v
				first = false
			} else {
				if v < min {
					min = v
				}
				if v > max {
					max = v
				}
			}
			sum += v
			count++
		}
		if count == 0 {
			continue
		}
		avg := sum / float64(count)

		// Assign to snapshot fields
		switch m.code {
		case "cpu_percent":
			snap.CPUAvg = avg
			snap.CPUMax = max
		case "data_io_percent":
			snap.DataIOAvg = avg
		case "log_write_percent":
			snap.LogWriteAvg = avg
		case "memory_usage_percent":
			// no field for memory avg in snapshot row; shortcut
		case "storage_percent":
			snap.StorageUsedPct = avg
		case "storage_used_bytes":
			snap.StorageUsedBytes = int64(avg)
		case "sessions_percent":
			snap.SessionsAvg = avg
		case "workers_percent":
			snap.WorkersAvg = avg
		case "deadlock":
			snap.DeadlockCount = int(sum) // total deadlocks in hour
		}
	}

	// Percentiles from exact samples (if available)
	type pctTarget struct {
		code   string
		setter func(*float64)
	}
	targets := []pctTarget{
		{"cpu_percent", func(v *float64) { snap.CPUP50 = v }},
		{"cpu_percent", func(v *float64) { snap.CPUP95 = v }},
		{"cpu_percent", func(v *float64) { snap.CPUP99 = v }},
		{"data_io_percent", func(v *float64) { snap.DataIOP95 = v }},
		{"log_write_percent", func(v *float64) { snap.LogWriteP95 = v }},
		{"sessions_percent", func(v *float64) { snap.SessionsP95 = v }},
	}
	for _, t := range targets {
		ps, err := s.repo.PercentileStatistic(ctx, resourceUID, t.code, "1h", bucketStart, bucketEnd)
		if err != nil {
			continue
		}
		for _, pr := range ps {
			if pr.Method == "exact_sample" && pr.Value != nil {
				if math.Abs(pr.Percentile-0.50) < 0.001 {
					t.setter(pr.Value)
				}
				if math.Abs(pr.Percentile-0.95) < 0.001 {
					t.setter(pr.Value)
				}
				if math.Abs(pr.Percentile-0.99) < 0.001 {
					t.setter(pr.Value)
				}
			}
		}
	}

	// Temporal features
	snap.IsPeakHour = isPeakHour(bucketStart)
	snap.DayOfWeek = int(bucketStart.Weekday())
	snap.HourOfDay = bucketStart.Hour()

	// Workload intensity from workload_run (if any)
	// shortcut: not linked; upgrade: join core.workload_cycle

	// Storage growth: diff vs previous bucket
	if bucketStart.Hour() > 0 {
		prevBucket := bucketStart.Add(-time.Hour)
		prevSnaps, _ := s.repo.FeatureSnapshot(ctx, resourceUID, "1h", prevBucket, bucketStart, 1)
		if len(prevSnaps) > 0 {
			snap.StorageGrowthBytes = float64(snap.StorageUsedBytes - prevSnaps[0].StorageUsedBytes)
		}
	}

	return snap, nil
}

// ---- ComputeTrends ----

func (s *Service) ComputeTrends(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) ([]TrendResult, error) {
	// Trends are computed as part of feature_snapshot (cpu_slope, storage_growth_bytes)
	// This method delegates to repo.Trend which reads from feature_snapshot.
	return s.repo.Trend(ctx, resourceUID, from, to)
}

// ---- DetectWorkloadPatterns ----

func (s *Service) DetectWorkloadPatterns(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (WorkloadPattern, error) {
	return s.repo.WorkloadPattern(ctx, resourceUID, from, to)
}

// ---- AnalyzeCapacity ----

func (s *Service) AnalyzeCapacity(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (CapacityAnalysis, error) {
	return s.repo.CapacityAnalysis(ctx, resourceUID, periodType, periodStart)
}

// ---- AnalyzeCosts ----

func (s *Service) AnalyzeCosts(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (CostAnalysis, error) {
	return s.repo.CostAnalysis(ctx, resourceUID, from, to)
}

// ---- BuildTrainingDataset ----

func (s *Service) BuildTrainingDataset(ctx context.Context, spec TrainingDatasetSpec) (int64, error) {
	if spec.Name == "" || spec.TargetColumn == "" || len(spec.FeatureCodes) == 0 {
		return 0, NewError(CodeInvalidQuery, "name, target_column, and feature_codes required", ErrInvalidQuery)
	}
	spec.DatasetHash = computeDatasetHash(spec)
	spec.CreatedAt = time.Now().UTC()
	return s.repo.WriteTrainingDataset(ctx, spec)
}

// ---- RequestForecast ----

func (s *Service) RequestForecast(ctx context.Context, req ForecastRequest) (uuid.UUID, error) {
	if s.ml == nil {
		return uuid.Nil, NewError(CodeMLUnavailable, "ML service not configured", ErrMLUnavailable)
	}
	if req.ResourceUID == uuid.Nil || req.TargetMetric == "" || req.HorizonSteps <= 0 {
		return uuid.Nil, NewError(CodeInvalidQuery, "resource_uid, target_metric, horizon_steps required", ErrInvalidQuery)
	}
	// Validate contract version
	if err := s.ml.ValidateContract(ctx); err != nil {
		return uuid.Nil, NewError(CodeContractMismatch, "ML contract mismatch: "+err.Error(), ErrContractMismatch)
	}
	return s.ml.Forecast(ctx, req)
}

// ---- ComputeDESComponents ----

func (s *Service) ComputeDESComponents(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (DESComponents, error) {
	// Read components from repo (which reads opt.efficiency_score)
	comp, err := s.repo.DESComponents(ctx, resourceUID, periodType, periodStart)
	if err != nil {
		// If not computed yet, compute on the fly (shortcut)
		slog.Warn("DES components not found, computing on the fly", "resource_uid", resourceUID, "period_type", periodType, "period_start", periodStart)

		cap, err := s.AnalyzeCapacity(ctx, resourceUID, periodType, periodStart)
		if err != nil {
			slog.Warn("DES capacity analysis failed, skipping DES", "resource_uid", resourceUID, "error", err)
			return DESComponents{}, err
		}
		cost, err := s.AnalyzeCosts(ctx, resourceUID, periodStart, periodStart.Add(24*time.Hour))
		if err != nil {
			slog.Warn("DES cost analysis failed, skipping DES", "resource_uid", resourceUID, "error", err)
			return DESComponents{}, err
		}
		if cap.PeakUtilization == 0 {
			slog.Warn("DES capacity has no utilization data, skipping DES", "resource_uid", resourceUID)
			return DESComponents{}, NewError(CodeNotFound, "no capacity data available for DES", ErrNotFound)
		}
		// Performance component: based on headroom, peak%, latency
		perf := 1.0 - math.Min(1.0, cap.PeakUtilization/100.0) // shortcut
		comp = DESComponents{
			ResourceUID:    resourceUID,
			PeriodType:     periodType,
			PeriodStartUTC: periodStart,
			CapacityE:      cap.CapacityE,
			CostE:          cost.CostE,
			PerformanceE:   perf,
			WeightsVersion: "v1_equal",
			ComputedAt:     time.Now().UTC(),
		}
		_, _ = s.repo.WriteEfficiencyScore(ctx, 0, comp) // resourceID resolved inside; ignore rows affected
	}
	comp.ResourceUID = resourceUID
	return comp, nil
}

// ---- AnalyticsResultRepository read methods ----

func (s *Service) FeatureSnapshot(ctx context.Context, resourceUID uuid.UUID, periodType string, from, to time.Time, limit int) ([]FeatureSnapshotRow, error) {
	return s.repo.FeatureSnapshot(ctx, resourceUID, periodType, from, to, limit)
}

func (s *Service) PercentileStatistic(ctx context.Context, resourceUID uuid.UUID, metricCode string, periodType string, from, to time.Time) ([]PercentileResult, error) {
	return s.repo.PercentileStatistic(ctx, resourceUID, metricCode, periodType, from, to)
}

func (s *Service) Trend(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) ([]TrendResult, error) {
	return s.repo.Trend(ctx, resourceUID, from, to)
}

func (s *Service) WorkloadPattern(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (WorkloadPattern, error) {
	return s.repo.WorkloadPattern(ctx, resourceUID, from, to)
}

func (s *Service) CapacityAnalysis(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (CapacityAnalysis, error) {
	return s.repo.CapacityAnalysis(ctx, resourceUID, periodType, periodStart)
}

func (s *Service) CostAnalysis(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (CostAnalysis, error) {
	return s.repo.CostAnalysis(ctx, resourceUID, from, to)
}

func (s *Service) DESComponents(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (DESComponents, error) {
	return s.repo.DESComponents(ctx, resourceUID, periodType, periodStart)
}

func (s *Service) TrainingDataset(ctx context.Context, name string, version int) (TrainingDatasetSpec, error) {
	return s.repo.TrainingDataset(ctx, name, version)
}

func (s *Service) Forecasts(ctx context.Context, resourceUID uuid.UUID, targetMetric string, from, to time.Time) ([]ForecastResponse, error) {
	return s.repo.Forecasts(ctx, resourceUID, targetMetric, from, to)
}

// ---- helpers ----

func (s *Service) resolveResourceID(ctx context.Context, uid uuid.UUID) (int64, error) {
	// This would need access to data repository; shortcut: use repo method
	type resolver interface {
		resolveResourceID(ctx context.Context, uid uuid.UUID) (int64, error)
	}
	if r, ok := s.repo.(resolver); ok {
		return r.resolveResourceID(ctx, uid)
	}
	return 0, NewError(CodeInvalidQuery, "repository does not support resource resolution", ErrInvalidQuery)
}

func bucketStart(t time.Time, step time.Duration) time.Time {
	t = t.UTC()
	if step == 24*time.Hour {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	return t.Truncate(step)
}

func isPeakHour(t time.Time) bool {
	h := t.Hour()
	// Peak hours: 9-12 and 14-18 (business hours)
	return (h >= 9 && h <= 12) || (h >= 14 && h <= 18)
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrFloat64(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func computeDatasetHash(spec TrainingDatasetSpec) string {
	// Simplified hash
	data := fmt.Sprintf("%s|%d|%s|%s|%s|%s",
		spec.Name, spec.Version, spec.ResourceScope,
		spec.PeriodStartUTC, spec.PeriodEndUTC, spec.TargetColumn)
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}
