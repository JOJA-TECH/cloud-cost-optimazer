package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/reporting"
	"github.com/google/uuid"
)

type incompleteAnalyticsResultStub struct {
	analytics.AnalyticsResultRepository
	snapshots []analytics.FeatureSnapshotRow
	cost      analytics.CostAnalysis
	trends    []analytics.TrendResult
	pattern   analytics.WorkloadPattern
}

func (s incompleteAnalyticsResultStub) FeatureSnapshot(context.Context, uuid.UUID, string, time.Time, time.Time, int) ([]analytics.FeatureSnapshotRow, error) {
	return s.snapshots, nil
}

func (s incompleteAnalyticsResultStub) CostAnalysis(context.Context, uuid.UUID, time.Time, time.Time) (analytics.CostAnalysis, error) {
	return s.cost, nil
}

func (s incompleteAnalyticsResultStub) PercentileStatistic(context.Context, uuid.UUID, string, string, time.Time, time.Time) ([]analytics.PercentileResult, error) {
	return nil, nil
}

func (s incompleteAnalyticsResultStub) Trend(context.Context, uuid.UUID, time.Time, time.Time) ([]analytics.TrendResult, error) {
	return s.trends, nil
}

func (s incompleteAnalyticsResultStub) WorkloadPattern(context.Context, uuid.UUID, time.Time, time.Time) (analytics.WorkloadPattern, error) {
	return s.pattern, nil
}

func (incompleteAnalyticsResultStub) CapacityAnalysis(context.Context, uuid.UUID, string, time.Time) (analytics.CapacityAnalysis, error) {
	return analytics.CapacityAnalysis{}, analytics.ErrNotFound
}

func (incompleteAnalyticsResultStub) DESComponents(context.Context, uuid.UUID, string, time.Time) (analytics.DESComponents, error) {
	return analytics.DESComponents{}, analytics.ErrNotFound
}

func (incompleteAnalyticsResultStub) Forecasts(context.Context, uuid.UUID, string, time.Time, time.Time) ([]analytics.ForecastResponse, error) {
	return nil, nil
}

func TestAnalyticsReportReaderOmitsUnavailableValuesAndMarksDegradedData(t *testing.T) {
	resourceUID := uuid.New()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	reader := NewAnalyticsReportReader(incompleteAnalyticsResultStub{
		snapshots: []analytics.FeatureSnapshotRow{{
			ResourceUID:      resourceUID,
			PeriodStartUTC:   from,
			PeriodEndUTC:     from.Add(time.Hour),
			CPUAvg:           0,
			CPUMax:           0,
			AvailableMetrics: []string{"cpu_avg"},
		}},
		cost: analytics.CostAnalysis{
			ResourceUID:    resourceUID,
			PeriodStartUTC: from,
			PeriodEndUTC:   to,
		},
		trends: []analytics.TrendResult{{
			ResourceUID: resourceUID,
			MetricCode:  "cpu_avg",
			WindowStart: from,
			WindowEnd:   from.Add(time.Hour),
			Slope:       0,
		}},
		pattern: analytics.WorkloadPattern{
			ResourceUID:     resourceUID,
			WindowStart:     from,
			WindowEnd:       to,
			PeakHours:       []int{9},
			PeakFrequency:   1,
			AvgPeakDuration: 1,
			AvailableMetrics: []string{
				"peak_hours", "peak_frequency",
			},
		},
	})

	data, err := reader.BuildReportData(context.Background(), resourceUID, from, to)
	if err != nil {
		t.Fatalf("BuildReportData() error = %v", err)
	}

	if !hasMetric(data.Metrics, "cpu_avg", float64(0)) {
		t.Fatal("available CPU average with a real zero value was omitted")
	}
	if len(data.Evidence) == 0 || data.Evidence[0].Value == nil ||
		*data.Evidence[0].Value != 0 || data.Evidence[0].Unit == nil ||
		*data.Evidence[0].Unit != "percent" || data.Evidence[0].Reference == "" {
		t.Fatalf("Analytics evidence is missing its value, unit, or reference: %#v", data.Evidence)
	}
	if hasMetricCode(data.Metrics, "cpu_max") {
		t.Fatal("unavailable CPU maximum was emitted")
	}
	if len(data.ObservedCost) != 0 || !hasMissingComponent(data.DataQuality.Missing, "cost_analysis") {
		t.Fatalf("unavailable cost analysis was not represented as missing: %#v", data)
	}
	if len(data.Trends) != 1 || data.Trends[0].Quality != nil {
		t.Fatalf("unavailable trend quality should be omitted without dropping the trend: %#v", data.Trends)
	}
	if hasMetricCode(data.Utilization, "average_peak_duration") ||
		hasMetricCode(data.Utilization, "intensity_band") {
		t.Fatalf("placeholder workload values were emitted: %#v", data.Utilization)
	}
	if data.DataQuality.Status != "degraded" ||
		!hasMissingComponent(data.DataQuality.Missing, "forecast") {
		t.Fatalf("incomplete Analytics data was not marked degraded: %#v", data.DataQuality)
	}
}

func hasMetric(metrics []reporting.ReportMetric, code string, value float64) bool {
	for _, metric := range metrics {
		if metric.Code == code && metric.Value == value {
			return true
		}
	}
	return false
}

func hasMetricCode(metrics []reporting.ReportMetric, code string) bool {
	for _, metric := range metrics {
		if metric.Code == code {
			return true
		}
	}
	return false
}

func hasMissingComponent(missing []string, component string) bool {
	for _, item := range missing {
		if item == component {
			return true
		}
	}
	return false
}
