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
		appendMetric("cpu_avg", "CPU promedio", row.CPUAvg, "percent")
		appendMetric("cpu_max", "CPU máximo", row.CPUMax, "percent")
		appendMetric("data_io_avg", "Data IO promedio", row.DataIOAvg, "percent")
		appendMetric("log_write_avg", "Log write promedio", row.LogWriteAvg, "percent")
		appendMetric("sessions_avg", "Sesiones promedio", row.SessionsAvg, "percent")
		appendMetric("workers_avg", "Workers promedio", row.WorkersAvg, "percent")
		appendMetric("storage_used_pct", "Almacenamiento utilizado", row.StorageUsedPct, "percent")
		appendMetric("storage_used_bytes", "Almacenamiento utilizado", row.StorageUsedBytes, "bytes")
		appendMetric("storage_growth_bytes", "Crecimiento de almacenamiento", row.StorageGrowthBytes, "bytes")
		appendMetric("deadlock_count", "Deadlocks", row.DeadlockCount, "count")
		if row.CPUP95 != nil {
			appendMetric("cpu_p95", "CPU percentil 95", *row.CPUP95, "percent")
		}
		if row.CPUP99 != nil {
			appendMetric("cpu_p99", "CPU percentil 99", *row.CPUP99, "percent")
		}
		if row.DataIOP95 != nil {
			appendMetric("data_io_p95", "Data IO percentil 95", *row.DataIOP95, "percent")
		}
		if row.LogWriteP95 != nil {
			appendMetric("log_write_p95", "Log write percentil 95", *row.LogWriteP95, "percent")
		}
		if row.LatencyP95Ms != nil {
			appendMetric("latency_p95_ms", "Latencia percentil 95", *row.LatencyP95Ms, "milliseconds")
		}
		data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
			Source: reporting.ReportSourceAnalytics, Reference: reference,
		})
	}

	cost, err := r.reader.CostAnalysis(ctx, resourceUID, from, to)
	if err != nil {
		return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics cost analysis: %w", err)
	}
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
		}
	}

	trends, err := r.reader.Trend(ctx, resourceUID, from, to)
	if err != nil {
		return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics trends: %w", err)
	}
	for _, trend := range trends {
		period := reporting.ReportPeriod{Start: trend.WindowStart, End: trend.WindowEnd}
		code := trend.MetricCode + "_slope"
		reference := fmt.Sprintf("analytics.trend/%s/%s", resourceUID, trend.WindowStart.UTC().Format(time.RFC3339Nano))
		data.Trends = append(data.Trends, reporting.ReportMetric{
			Code: code, Label: trend.MetricCode + " trend", Value: trend.Slope, Unit: "per_hour",
			Source: reporting.ReportSourceAnalytics, Period: period, Quality: floatPointer(trend.RSquared),
		})
		data.Evidence = append(data.Evidence, reporting.ReportEvidence{
			Type: "trend", MetricCode: stringPointer(code), Value: floatPointer(trend.Slope),
			Unit: stringPointer("per_hour"), Source: reporting.ReportSourceAnalytics,
			WindowStart: timePointer(period.Start), WindowEnd: timePointer(period.End), Reference: reference,
		})
		data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
			Source: reporting.ReportSourceAnalytics, Reference: reference,
		})
	}

	if len(snapshots) > 0 {
		pattern, err := r.reader.WorkloadPattern(ctx, resourceUID, from, to)
		if err != nil {
			return reporting.AnalyticsReportData{}, fmt.Errorf("read analytics workload pattern: %w", err)
		}
		patternReference := fmt.Sprintf("analytics.workload_pattern/%s/%s/%s",
			resourceUID, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
		patternPeriod := reporting.ReportPeriod{Start: pattern.WindowStart, End: pattern.WindowEnd}
		data.Utilization = []reporting.ReportMetric{
			reportMetric("peak_hours", "Horas pico", pattern.PeakHours, "hour_of_day", patternPeriod),
			reportMetric("peak_frequency", "Frecuencia de picos", pattern.PeakFrequency, "peaks_per_day", patternPeriod),
			reportMetric("average_peak_duration", "Duración promedio del pico", pattern.AvgPeakDuration, "hours", patternPeriod),
			reportMetric("intensity_band", "Banda de intensidad", pattern.IntensityBand, "band", patternPeriod),
		}
		data.SourceReferences = append(data.SourceReferences, reporting.ReportSourceReference{
			Source: reporting.ReportSourceAnalytics, Reference: patternReference,
		})
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
