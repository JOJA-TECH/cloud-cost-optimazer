package analytics

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
	"github.com/google/uuid"
)

type unavailableCostRepo struct {
	repo
	wroteEfficiencyScore bool
}

func (unavailableCostRepo) DESComponents(context.Context, uuid.UUID, string, time.Time) (DESComponents, error) {
	return DESComponents{}, ErrNotFound
}

func (unavailableCostRepo) CapacityAnalysis(context.Context, uuid.UUID, string, time.Time) (CapacityAnalysis, error) {
	return CapacityAnalysis{PeakUtilization: 80, CapacityE: 0.7}, nil
}

func (unavailableCostRepo) CostAnalysis(context.Context, uuid.UUID, time.Time, time.Time) (CostAnalysis, error) {
	return CostAnalysis{CostE: 0}, nil
}

func (r *unavailableCostRepo) WriteEfficiencyScore(context.Context, int64, DESComponents) (int64, error) {
	r.wroteEfficiencyScore = true
	return 1, nil
}

type percentileRepo struct {
	repo
	written []PercentileResult
}

func (*percentileRepo) ResolveResourceID(context.Context, uuid.UUID) (int64, error) { return 1, nil }

func (r *percentileRepo) WritePercentileStatistics(_ context.Context, _ int64, stats []PercentileResult) (int, error) {
	r.written = stats
	return len(stats), nil
}

func (r *percentileRepo) PercentileStatistic(context.Context, uuid.UUID, string, string, time.Time, time.Time) ([]PercentileResult, error) {
	return r.written, nil
}

type metricReader struct{ values []float64 }

func (r metricReader) MetricSeries(_ context.Context, _ data.DataQuery, _ int) ([]data.HistoricalMetric, error) {
	pts := make([]data.HistoricalMetric, len(r.values))
	for i, v := range r.values {
		value := v
		pts[i] = data.HistoricalMetric{AvgValue: &value}
	}
	return pts, nil
}

func TestComputePercentilesWritesExactSamplesFromRawSeries(t *testing.T) {
	r := &percentileRepo{}
	service := NewService(r, metricReader{values: []float64{10, 20, 30, 40}}, nil, nil, nil, 1)
	from := time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC)
	_, err := service.ComputePercentiles(context.Background(), uuid.New(), "cpu_percent", from, from.Add(time.Hour), "1h")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.written) != 3 || r.written[1].Value == nil || *r.written[1].Value < 38.49 || *r.written[1].Value > 38.51 {
		t.Fatalf("p95 = %+v", r.written[1])
	}
}

func TestLinearSlopeUsesHourlySnapshots(t *testing.T) {
	base := time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC)
	snaps := []FeatureSnapshotRow{
		{PeriodStartUTC: base, CPUAvg: 10},
		{PeriodStartUTC: base.Add(time.Hour), CPUAvg: 20},
		{PeriodStartUTC: base.Add(2 * time.Hour), CPUAvg: 30},
	}
	if got := linearSlope(snaps); math.Abs(got-10) > 1e-9 {
		t.Fatalf("slope = %v, want 10", got)
	}
}

func TestComputeDESComponentsDoesNotPersistWhenCostDataIsUnavailable(t *testing.T) {
	r := &unavailableCostRepo{}
	service := NewService(r, nil, nil, nil, nil, 1)

	_, err := service.ComputeDESComponents(
		context.Background(),
		uuid.New(),
		"1h",
		time.Now().UTC(),
	)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("ComputeDESComponents() error = %v, want ErrNotFound", err)
	}
	if r.wroteEfficiencyScore {
		t.Fatal("persisted DES components despite unavailable cost data")
	}
}
