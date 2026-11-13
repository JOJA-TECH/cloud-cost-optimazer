package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// fakeRepo satisfies the service repository surface without a database;
// embedded nil interfaces cover the readers these tests never call.
type fakeRepo struct {
	HistoricalDataReader
	CostRepository
	ConfigurationRepository
	timestamps []time.Time
}

func (f *fakeRepo) MetricReadingTimestamps(_ context.Context, _ uuid.UUID, _ string, _, _ time.Time) ([]time.Time, error) {
	return f.timestamps, nil
}

func TestCoverageValidation(t *testing.T) {
	svc := NewService(&fakeRepo{})
	ctx := context.Background()
	uid := uuid.New()
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)

	cases := []struct {
		name        string
		from, to    time.Time
		bucket      string
		minCoverage float64
	}{
		{"from_after_to", to, from, BucketHour, 0.8},
		{"from_equal_to", from, from, BucketHour, 0.8},
		{"bad_bucket", from, to, "5m", 0.8},
		{"zero_min_coverage", from, to, BucketHour, 0},
		{"min_coverage_above_one", from, to, BucketHour, 1.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Coverage(ctx, uid, "cpu_percent", tc.from, tc.to, tc.bucket, tc.minCoverage)
			if !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("Coverage err = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

func TestCoverageHourlyBucketing(t *testing.T) {
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ts := []time.Time{
		day.Add(15 * time.Minute),             // slot 00
		day.Add(1*time.Hour + 45*time.Minute), // slot 01
		day.Add(1*time.Hour + 50*time.Minute), // slot 01 (same bucket)
	}
	svc := NewService(&fakeRepo{timestamps: ts})
	ctx := context.Background()

	rep, err := svc.Coverage(ctx, uuid.New(), "cpu_percent", day, day.Add(24*time.Hour), BucketHour, 0.8)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if rep.Bucket != BucketHour || rep.WindowsTotal != 24 || rep.WindowsWithData != 2 ||
		rep.Gaps != 22 || rep.TotalReadings != 3 {
		t.Fatalf("report = %+v, want total=24 withData=2 gaps=22 readings=3", rep)
	}
	if rep.State != CoverageInsufficient {
		t.Errorf("state = %q, want insufficient (ratio %v < 0.8)", rep.State, rep.CoverageRatio)
	}

	rep, err = svc.Coverage(ctx, uuid.New(), "cpu_percent", day, day.Add(24*time.Hour), BucketHour, 0.05)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if rep.State != CoverageSufficient {
		t.Errorf("state = %q, want sufficient (ratio %v >= 0.05)", rep.State, rep.CoverageRatio)
	}
}

func TestCoverageDayBucketingAndBoundary(t *testing.T) {
	d1 := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	ts := []time.Time{
		d1.Add(5 * time.Hour),
		d1.Add(24*time.Hour + 6*time.Hour), // day 2
	}
	svc := NewService(&fakeRepo{timestamps: ts})
	ctx := context.Background()

	// Three full days: [Nov 1, Nov 4).
	rep, err := svc.Coverage(ctx, uuid.New(), "cpu_percent", d1, d1.Add(72*time.Hour), BucketDay, 0.8)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if rep.WindowsTotal != 3 || rep.WindowsWithData != 2 || rep.Gaps != 1 || rep.TotalReadings != 2 {
		t.Fatalf("report = %+v, want total=3 withData=2 gaps=1 readings=2", rep)
	}

	// to is exclusive: a reading in the last instant bucket of [from,to) counts,
	// but the bucket starting exactly at to does not exist in the grid.
	oneReading := []time.Time{d1.Add(23*time.Hour + 30*time.Minute)}
	svc2 := NewService(&fakeRepo{timestamps: oneReading})
	rep, err = svc2.Coverage(ctx, uuid.New(), "cpu_percent", d1, d1.Add(24*time.Hour), BucketHour, 0.8)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if rep.WindowsTotal != 24 || rep.WindowsWithData != 1 || rep.TotalReadings != 1 {
		t.Fatalf("report = %+v, want total=24 withData=1 readings=1", rep)
	}
}

func TestCoverageEmptyWindow(t *testing.T) {
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	svc := NewService(&fakeRepo{})
	rep, err := svc.Coverage(context.Background(), uuid.New(), "cpu_percent", day, day.Add(24*time.Hour), BucketHour, 0.8)
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if rep.WindowsTotal != 24 || rep.WindowsWithData != 0 || rep.TotalReadings != 0 ||
		rep.State != CoverageInsufficient || rep.CoverageRatio != 0 {
		t.Fatalf("report = %+v, want empty window insufficient", rep)
	}
}
