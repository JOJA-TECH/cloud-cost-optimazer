package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
)

type AnalyticsReader struct {
	repository analytics.AnalyticsResultRepository
}

func NewAnalyticsReader(repository analytics.AnalyticsResultRepository) *AnalyticsReader {
	return &AnalyticsReader{repository: repository}
}

func (r *AnalyticsReader) FeatureSnapshot(ctx context.Context, resourceUID uuid.UUID, periodType string, from, to time.Time, limit int) ([]recommendation.FeatureSnapshot, error) {
	rows, err := r.repository.FeatureSnapshot(ctx, resourceUID, periodType, from, to, limit)
	if err != nil {
		if errors.Is(err, analytics.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]recommendation.FeatureSnapshot, 0, len(rows))
	for _, row := range rows {
		out = append(out, recommendation.FeatureSnapshot{
			ResourceUID: row.ResourceUID, PeriodType: row.PeriodType,
			PeriodStartUTC: row.PeriodStartUTC, PeriodEndUTC: row.PeriodEndUTC,
			SKUID: row.SKUID, ConfigSnapshotID: row.ConfigSnapshotID,
			ComputeModel: row.ComputeModel, CapacityVCores: row.CapacityVCores,
			CPUAvg: row.CPUAvg, CPUP50: row.CPUP50, CPUP95: row.CPUP95,
			CPUP99: row.CPUP99, CPUMax: row.CPUMax, CPUSlope: row.CPUSlope,
			DataIOAvg: row.DataIOAvg, DataIOP95: row.DataIOP95,
			LogWriteAvg: row.LogWriteAvg, LogWriteP95: row.LogWriteP95,
			SessionsAvg: row.SessionsAvg, SessionsP95: row.SessionsP95,
			WorkersAvg: row.WorkersAvg, StorageUsedPct: row.StorageUsedPct,
			StorageUsedBytes: row.StorageUsedBytes, StorageGrowthBytes: row.StorageGrowthBytes,
			IsPeakHour: row.IsPeakHour, DayOfWeek: row.DayOfWeek, HourOfDay: row.HourOfDay,
			WorkloadIntensity: row.WorkloadIntensity, WorkloadRunID: row.WorkloadRunID,
			DeadlockCount: row.DeadlockCount, LatencyP95MS: row.LatencyP95Ms,
			ComputedAt: row.ComputedAt,
		})
	}
	return out, nil
}

func (r *AnalyticsReader) PercentileStatistic(ctx context.Context, resourceUID uuid.UUID, metricCode, periodType string, from, to time.Time) ([]recommendation.PercentileStatistic, error) {
	rows, err := r.repository.PercentileStatistic(ctx, resourceUID, metricCode, periodType, from, to)
	if err != nil {
		if errors.Is(err, analytics.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]recommendation.PercentileStatistic, 0, len(rows))
	for _, row := range rows {
		out = append(out, recommendation.PercentileStatistic{
			ResourceUID: row.ResourceUID, PeriodType: row.PeriodType,
			PeriodStartUTC: row.PeriodStartUTC, MetricCode: row.MetricCode,
			Percentile: row.Percentile, Value: row.Value, Method: row.Method,
			ObservationCount: row.ObservationCount, SourceGranularity: row.SourceGranularity,
			ComputedAt: row.ComputedAt,
		})
	}
	return out, nil
}

func (r *AnalyticsReader) Trend(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) ([]recommendation.Trend, error) {
	rows, err := r.repository.Trend(ctx, resourceUID, from, to)
	if err != nil {
		if errors.Is(err, analytics.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]recommendation.Trend, 0, len(rows))
	for _, row := range rows {
		out = append(out, recommendation.Trend{
			ResourceUID: row.ResourceUID, MetricCode: row.MetricCode,
			WindowStart: row.WindowStart, WindowEnd: row.WindowEnd,
			Slope: row.Slope, RSquared: row.RSquared, ComputedAt: row.ComputedAt,
		})
	}
	return out, nil
}

func (r *AnalyticsReader) WorkloadPattern(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (recommendation.WorkloadPattern, error) {
	row, err := r.repository.WorkloadPattern(ctx, resourceUID, from, to)
	if err != nil {
		if errors.Is(err, analytics.ErrNotFound) {
			return recommendation.WorkloadPattern{}, nil
		}
		return recommendation.WorkloadPattern{}, err
	}
	return recommendation.WorkloadPattern{
		ResourceUID: row.ResourceUID, WindowStart: row.WindowStart, WindowEnd: row.WindowEnd,
		PeakHours: row.PeakHours, PeakFrequency: row.PeakFrequency,
		AvgPeakDuration: row.AvgPeakDuration, IntensityBand: row.IntensityBand,
		ComputedAt: row.ComputedAt,
	}, nil
}

func (r *AnalyticsReader) CapacityAnalysis(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (recommendation.CapacityAnalysis, error) {
	row, err := r.repository.CapacityAnalysis(ctx, resourceUID, periodType, periodStart)
	if err != nil {
		if errors.Is(err, analytics.ErrNotFound) {
			return recommendation.CapacityAnalysis{}, nil
		}
		return recommendation.CapacityAnalysis{}, err
	}
	return recommendation.CapacityAnalysis{
		ResourceUID: row.ResourceUID, PeriodType: row.PeriodType, PeriodStartUTC: row.PeriodStartUTC,
		CurrentVCores: row.CurrentVCores, HeadroomPct: row.HeadroomPct,
		PeakUtilization: row.PeakUtilization, GrowthRateVCores: row.GrowthRateVCores,
		RecommendedVCores: row.RecommendedVCores, CapacityE: row.CapacityE, ComputedAt: row.ComputedAt,
	}, nil
}

func (r *AnalyticsReader) CostAnalysis(ctx context.Context, resourceUID uuid.UUID, from, to time.Time) (recommendation.CostAnalysis, error) {
	row, err := r.repository.CostAnalysis(ctx, resourceUID, from, to)
	if err != nil {
		if errors.Is(err, analytics.ErrNotFound) {
			return recommendation.CostAnalysis{}, nil
		}
		return recommendation.CostAnalysis{}, err
	}
	return recommendation.CostAnalysis{
		ResourceUID: row.ResourceUID, PeriodStartUTC: row.PeriodStartUTC, PeriodEndUTC: row.PeriodEndUTC,
		ActualMonthlyCost: row.ActualMonthlyCost, CostPerVCore: row.CostPerVCore,
		ProjectedMonthly: row.ProjectedMonthly, SavingsVsCurrent: row.SavingsVsCurrent,
		CostE: row.CostE, ComputedAt: row.ComputedAt,
	}, nil
}

func (r *AnalyticsReader) DESComponents(ctx context.Context, resourceUID uuid.UUID, periodType string, periodStart time.Time) (recommendation.DESComponents, error) {
	row, err := r.repository.DESComponents(ctx, resourceUID, periodType, periodStart)
	if err != nil {
		if errors.Is(err, analytics.ErrNotFound) {
			return recommendation.DESComponents{}, nil
		}
		return recommendation.DESComponents{}, err
	}
	return recommendation.DESComponents{
		ResourceUID: row.ResourceUID, PeriodType: row.PeriodType, PeriodStartUTC: row.PeriodStartUTC,
		CapacityE: row.CapacityE, CostE: row.CostE, PerformanceE: row.PerformanceE,
		WeightsVersion: row.WeightsVersion, ComputedAt: row.ComputedAt,
	}, nil
}

func (r *AnalyticsReader) Forecasts(ctx context.Context, resourceUID uuid.UUID, metric string, from, to time.Time) ([]recommendation.Forecast, error) {
	rows, err := r.repository.Forecasts(ctx, resourceUID, metric, from, to)
	if err != nil {
		if errors.Is(err, analytics.ErrMLUnavailable) {
			return nil, recommendation.ErrMLUnavailable
		}
		return nil, err
	}
	out := make([]recommendation.Forecast, 0, len(rows))
	for _, row := range rows {
		out = append(out, recommendation.Forecast{
			ForecastUID: row.ForecastUID, ModelVersionID: row.ModelVersionID, ResourceUID: row.ResourceUID,
			TargetMetric: row.TargetMetric, HorizonSteps: row.HorizonSteps,
			StepUnit: row.StepUnit, PeriodStartUTC: row.PeriodStartUTC,
			YPred: row.YPred, YLower: row.YLower, YUpper: row.YUpper,
			Alpha: row.Alpha, GeneratedAt: row.GeneratedAt,
		})
	}
	return out, nil
}

var _ recommendation.AnalyticsReader = (*AnalyticsReader)(nil)
var _ recommendation.MLPredictionReader = (*AnalyticsReader)(nil)
