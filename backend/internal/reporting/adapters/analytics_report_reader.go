package adapters

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

const reportSnapshotLimit = 10000

type AnalyticsReportReader struct {
	reader analytics.AnalyticsResultRepository
}

func NewAnalyticsReportReader(reader analytics.AnalyticsResultRepository) *AnalyticsReportReader {
	return &AnalyticsReportReader{reader: reader}
}

var _ reporting.AnalyticsReportReader = (*AnalyticsReportReader)(nil)

func (r *AnalyticsReportReader) BuildReportData(
	ctx context.Context,
	resourceUID uuid.UUID,
	from, to time.Time,
) (reporting.AnalyticsReportData, error) {
	if r == nil || r.reader == nil {
		return reporting.AnalyticsReportData{}, reporting.ErrReportDependencyUnavailable
	}
	data := reporting.AnalyticsReportData{
		ResourceUID: resourceUID,
		Period:      reporting.ReportPeriod{Start: from, End: to},
	}
	missing := make([]string, 0)
	missingSet := make(map[string]struct{})
	markMissing := func(component string) {
		if _, exists := missingSet[component]; exists {
			return
		}
		missingSet[component] = struct{}{}
		missing = append(missing, component)
	}
	snapshots, err := r.reader.FeatureSnapshot(ctx, resourceUID, "1h", from, to, reportSnapshotLimit)
	if err != nil {
		return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics feature snapshots: %w", err)
	}
	if len(snapshots) == reportSnapshotLimit {
		return reporting.AnalyticsReportData{}, fmt.Errorf(
			"%w: analytics snapshot result reached the %d-row limit; narrow the report period",
			reporting.ErrReportGenerationFailed, reportSnapshotLimit,
		)
	}
	if len(snapshots) == 0 {
		markMissing("feature_snapshot")
	}
	snapshotMetricCount := len(data.Metrics)
	for _, row := range snapshots {
		period := reporting.ReportPeriod{Start: row.PeriodStartUTC, End: row.PeriodEndUTC}
		reference := fmt.Sprintf("analytics.feature_snapshot/%s/%s", resourceUID, row.PeriodStartUTC.UTC().Format(time.RFC3339Nano))
		appendMetric := func(code, label string, value any, unit string) {
			data.Metrics = append(data.Metrics, reporting.ReportMetric{
				Code: code, Label: label, Value: value, Unit: unit,
				Source: reporting.ReportSourceAnalytics, Period: period,
			})
			data.Evidence = append(data.Evidence, reporting.ReportEvidence{
				Type: "metric", MetricCode: stringPointer(code), Source: reporting.ReportSourceAnalytics,
				Reference: reference, WindowStart: timePointer(period.Start), WindowEnd: timePointer(period.End),
			})
		}
		appendAvailableMetric := func(code, label string, value any, unit string) {
			if row.HasMetric(code) {
				appendMetric(code, label, value, unit)
			}
		}
		appendAvailableMetric("cpu_avg", "CPU promedio", row.CPUAvg, "percent")
		appendAvailableMetric("cpu_max", "CPU máximo", row.CPUMax, "percent")
		appendAvailableMetric("data_io_avg", "Data IO promedio", row.DataIOAvg, "percent")
		appendAvailableMetric("log_write_avg", "Log write promedio", row.LogWriteAvg, "percent")
		appendAvailableMetric("sessions_avg", "Sesiones promedio", row.SessionsAvg, "percent")
		appendAvailableMetric("workers_avg", "Workers promedio", row.WorkersAvg, "percent")
		appendAvailableMetric("storage_used_pct", "Almacenamiento utilizado", row.StorageUsedPct, "percent")
		appendAvailableMetric("storage_used_bytes", "Almacenamiento utilizado", row.StorageUsedBytes, "bytes")
		appendAvailableMetric("storage_growth_bytes", "Crecimiento de almacenamiento", row.StorageGrowthBytes, "bytes")
		appendAvailableMetric("deadlock_count", "Deadlocks", row.DeadlockCount, "count")
		if row.HasMetric("cpu_p50") && row.CPUP50 != nil {
			appendMetric("cpu_p50", "CPU percentil 50", *row.CPUP50, "percent")
		}
		if row.HasMetric("cpu_p95") && row.CPUP95 != nil {
			appendMetric("cpu_p95", "CPU percentil 95", *row.CPUP95, "percent")
		}
		if row.HasMetric("cpu_p99") && row.CPUP99 != nil {
			appendMetric("cpu_p99", "CPU percentil 99", *row.CPUP99, "percent")
		}
		if row.HasMetric("data_io_p95") && row.DataIOP95 != nil {
			appendMetric("data_io_p95", "Data IO percentil 95", *row.DataIOP95, "percent")
		}
		if row.HasMetric("log_write_p95") && row.LogWriteP95 != nil {
			appendMetric("log_write_p95", "Log write percentil 95", *row.LogWriteP95, "percent")
		}
		if row.HasMetric("latency_p95_ms") && row.LatencyP95Ms != nil {
			appendMetric("latency_p95_ms", "Latencia percentil 95", *row.LatencyP95Ms, "milliseconds")
		}
		data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
			Source: reporting.ReportSourceAnalytics, Reference: reference,
		})
	}
	if len(data.Metrics) == snapshotMetricCount {
		markMissing("feature_metrics")
	}

	cost, err := r.reader.CostAnalysis(ctx, resourceUID, from, to)
	if err != nil {
		return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics cost analysis: %w", err)
	}
	if !cost.Available {
		markMissing("cost_analysis")
	} else {
		costPeriod := reporting.ReportPeriod{Start: cost.PeriodStartUTC, End: cost.PeriodEndUTC}
		costReference := fmt.Sprintf("analytics.cost_analysis/%s/%s/%s",
			resourceUID, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
		data.ObservedCost = []reporting.ReportMetric{
			reportMetric("actual_monthly_cost", "Costo mensual observado", cost.ActualMonthlyCost, "monthly_cost", costPeriod),
			reportMetric("projected_monthly_cost", "Costo mensual proyectado", cost.ProjectedMonthly, "monthly_cost", costPeriod),
			reportMetric("cost_per_vcore", "Costo por vCore", cost.CostPerVCore, "cost_per_vcore", costPeriod),
			reportMetric("savings_vs_current", "Ahorro frente al costo actual", cost.SavingsVsCurrent, "monthly_cost", costPeriod),
		}
		for _, metric := range data.ObservedCost {
			value, ok := metric.Value.(float64)
			if !ok {
				continue
			}
			data.Evidence = append(data.Evidence, reporting.ReportEvidence{
				Type: "cost_metric", MetricCode: stringPointer(metric.Code), Value: floatPointer(value),
				Unit: stringPointer(metric.Unit), Source: reporting.ReportSourceAnalytics,
				WindowStart: timePointer(costPeriod.Start), WindowEnd: timePointer(costPeriod.End),
				Reference: costReference,
			})
		}
		data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
			Source: reporting.ReportSourceAnalytics, Reference: costReference,
		})
	}

	percentileCount := 0
	for _, metricCode := range []string{"cpu_percent", "data_io_percent", "log_write_percent"} {
		percentiles, err := r.reader.PercentileStatistic(ctx, resourceUID, metricCode, "1h", from, to)
		if err != nil {
			return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics percentiles for %s: %w", metricCode, err)
		}
		for _, percentile := range percentiles {
			if percentile.Value == nil {
				continue
			}
			period := reporting.ReportPeriod{Start: percentile.PeriodStartUTC, End: percentile.PeriodStartUTC.Add(time.Hour)}
			code := fmt.Sprintf("%s_p%s", metricCode, formatPercentile(percentile.Percentile))
			reference := fmt.Sprintf("analytics.percentile_statistic/%s/%s/%s/%g",
				resourceUID, metricCode, percentile.PeriodStartUTC.UTC().Format(time.RFC3339Nano), percentile.Percentile)
			data.Metrics = append(data.Metrics, reporting.ReportMetric{
				Code: code, Label: metricCode + " percentile", Value: *percentile.Value,
				Unit: "percent", Source: reporting.ReportSourceAnalytics, Period: period,
			})
			data.Evidence = append(data.Evidence, reporting.ReportEvidence{
				Type: "percentile", MetricCode: stringPointer(metricCode), Value: floatPointer(*percentile.Value),
				Unit: stringPointer("percent"), Source: reporting.ReportSourceAnalytics,
				WindowStart: timePointer(period.Start), WindowEnd: timePointer(period.End), Reference: reference,
				Context: map[string]any{"percentile": percentile.Percentile, "method": percentile.Method},
			})
			data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
				Source: reporting.ReportSourceAnalytics, Reference: reference,
			})
			percentileCount++
		}
	}
	if percentileCount == 0 {
		markMissing("percentiles")
	}

	trends, err := r.reader.Trend(ctx, resourceUID, from, to)
	if err != nil {
		return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics trends: %w", err)
	}
	for _, trend := range trends {
		period := reporting.ReportPeriod{Start: trend.WindowStart, End: trend.WindowEnd}
		code := trend.MetricCode + "_slope"
		reference := fmt.Sprintf("analytics.trend/%s/%s", resourceUID, trend.WindowStart.UTC().Format(time.RFC3339Nano))
		metric := reporting.ReportMetric{
			Code: code, Label: trend.MetricCode + " trend", Value: trend.Slope, Unit: "per_hour",
			Source: reporting.ReportSourceAnalytics, Period: period,
		}
		if trend.RSquaredAvailable {
			metric.Quality = floatPointer(trend.RSquared)
		}
		data.Trends = append(data.Trends, metric)
		data.Evidence = append(data.Evidence, reporting.ReportEvidence{
			Type: "trend", MetricCode: stringPointer(code), Value: floatPointer(trend.Slope),
			Unit: stringPointer("per_hour"), Source: reporting.ReportSourceAnalytics,
			WindowStart: timePointer(period.Start), WindowEnd: timePointer(period.End), Reference: reference,
		})
		data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
			Source: reporting.ReportSourceAnalytics, Reference: reference,
		})
	}
	if len(trends) == 0 {
		markMissing("trend")
	}

	if len(snapshots) > 0 {
		pattern, err := r.reader.WorkloadPattern(ctx, resourceUID, from, to)
		if err != nil {
			return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics workload pattern: %w", err)
		}
		patternReference := fmt.Sprintf("analytics.workload_pattern/%s/%s/%s",
			resourceUID, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
		patternPeriod := reporting.ReportPeriod{Start: pattern.WindowStart, End: pattern.WindowEnd}
		if pattern.HasMetric("peak_hours") {
			data.Utilization = append(data.Utilization,
				reportMetric("peak_hours", "Horas pico", pattern.PeakHours, "hour_of_day", patternPeriod),
				reportMetric("peak_frequency", "Frecuencia de picos", pattern.PeakFrequency, "peaks_per_day", patternPeriod))
		}
		if pattern.HasMetric("intensity_band") {
			data.Utilization = append(data.Utilization,
				reportMetric("intensity_band", "Banda de intensidad", pattern.IntensityBand, "band", patternPeriod))
		}
		if !pattern.HasMetric("peak_hours") {
			markMissing("peak_pattern")
		}
		if !pattern.HasMetric("intensity_band") {
			markMissing("workload_intensity")
		}
		markMissing("average_peak_duration")
		data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
			Source: reporting.ReportSourceAnalytics, Reference: patternReference,
		})
	} else {
		markMissing("workload_pattern")
	}

	capacity, err := r.reader.CapacityAnalysis(ctx, resourceUID, "1h", from)
	if err != nil && !errors.Is(err, analytics.ErrNotFound) {
		return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics capacity analysis: %w", err)
	}
	if err == nil {
		reference := fmt.Sprintf("analytics.capacity_analysis/%s/%s", resourceUID, from.UTC().Format(time.RFC3339Nano))
		data.Utilization = append(data.Utilization, reportMetric(
			"capacity_e", "Componente de capacidad", capacity.CapacityE, "score",
			reporting.ReportPeriod{Start: from, End: to},
		))
		data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
			Source: reporting.ReportSourceAnalytics, Reference: reference,
		})
	} else {
		markMissing("capacity_analysis")
	}

	des, err := r.reader.DESComponents(ctx, resourceUID, "1h", from)
	if err != nil && !errors.Is(err, analytics.ErrNotFound) {
		return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics efficiency components: %w", err)
	}
	if err == nil {
		reference := fmt.Sprintf("analytics.des_components/%s/%s", resourceUID, from.UTC().Format(time.RFC3339Nano))
		data.Metrics = append(data.Metrics,
			reportMetric("cost_e", "Componente de costo", des.CostE, "score", reporting.ReportPeriod{Start: from, End: to}),
			reportMetric("performance_e", "Componente de rendimiento", des.PerformanceE, "score", reporting.ReportPeriod{Start: from, End: to}),
		)
		data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
			Source: reporting.ReportSourceAnalytics, Reference: reference,
		})
	} else {
		markMissing("des_components")
	}

	forecasts, err := r.reader.Forecasts(ctx, resourceUID, "", from, to)
	if err != nil {
		return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics forecasts: %w", err)
	}
	for _, forecast := range forecasts {
		period := reporting.ReportPeriod{Start: forecast.PeriodStartUTC, End: forecast.PeriodStartUTC}
		reference := "analytics.forecast/" + forecast.ForecastUID.String()
		data.Forecasts = append(data.Forecasts, reporting.ReportMetric{
			Code: forecast.TargetMetric, Label: forecast.TargetMetric + " forecast",
			Value: forecast.YPred, Unit: forecast.StepUnit,
			Source: reporting.ReportSourceAnalytics, Period: period,
		})
		data.Evidence = append(data.Evidence, reporting.ReportEvidence{
			Type: "forecast", MetricCode: stringPointer(forecast.TargetMetric), Value: floatPointer(forecast.YPred),
			Source: reporting.ReportSourceAnalytics, WindowStart: timePointer(period.Start),
			WindowEnd: timePointer(period.End), Reference: reference,
			Context: map[string]any{"lower": forecast.YLower, "upper": forecast.YUpper, "alpha": forecast.Alpha},
		})
		data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
			Source: reporting.ReportSourceAnalytics, Reference: reference,
		})
	}
	if len(forecasts) == 0 {
		markMissing("forecast")
	}
	data.DataQuality.Missing = missing
	data.DataQuality.Status = "complete"
	if len(missing) > 0 {
		data.DataQuality.Status = "degraded"
	}
	return data, nil
}

func reportMetric(code, label string, value any, unit string, period reporting.ReportPeriod) reporting.ReportMetric {
	return reporting.ReportMetric{
		Code: code, Label: label, Value: value, Unit: unit,
		Source: reporting.ReportSourceAnalytics, Period: period,
	}
}

func stringPointer(value string) *string {
	return &value
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func floatPointer(value float64) *float64 {
	return &value
}

func formatPercentile(percentile float64) string {
	return strconv.FormatFloat(percentile*100, 'f', -1, 64)
}
