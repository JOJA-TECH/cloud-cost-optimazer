package adapters

import (
	"context"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
)

// This file is the dev/tests implementation of the acquisition providers
// (pattern: internal/cloud/adapters/fake.go): deterministic, hard-coded data,
// no logs, no randomness. Selected when the real Azure/SQL credentials are
// absent.

var (
	_ acquisition.MetricsProvider     = (*FakeMonitor)(nil)
	_ acquisition.DiagnosticsProvider = (*FakeDiagnostics)(nil)
	_ acquisition.QueryStoreProvider  = (*FakeQueryStore)(nil)
)

// FakeMonitor implements acquisition.MetricsProvider with one reading per
// minute in [start, end) for every catalog metric name.
type FakeMonitor struct{}

// NewFakeMonitor returns a FakeMonitor.
func NewFakeMonitor() *FakeMonitor {
	return &FakeMonitor{}
}

// FetchPlatformMetrics returns deterministic PT1M readings: a base value per
// metric index plus a minute offset, clamped to 0..100.
func (f *FakeMonitor) FetchPlatformMetrics(_ context.Context, _ cloud.ConnectionCredentials, _ string, start, end time.Time) ([]acquisition.RawReading, error) {
	var out []acquisition.RawReading
	for i, name := range acquisition.AzureMonitorMetricNames {
		code, ok := acquisition.MapMetricName(name)
		if !ok {
			continue
		}
		base := float64(i * 7)
		for ts := start.UTC().Truncate(time.Minute); ts.Before(end); ts = ts.Add(time.Minute) {
			avg := base + float64(ts.Minute()%30)
			min := avg - 2
			if min < 0 {
				min = 0
			}
			max := avg + 2
			if max > 100 {
				max = 100
			}
			out = append(out, acquisition.RawReading{
				MetricCode:      code,
				Unit:            acquisition.UnitForCode(code),
				Timestamp:       ts,
				IntervalSeconds: 60,
				Average:         &avg,
				Minimum:         &min,
				Maximum:         &max,
				Source:          acquisition.SourceAzureMonitor,
			})
		}
	}
	return out, nil
}

// FakeDiagnostics implements acquisition.DiagnosticsProvider with four
// db_resource_stats rows every 15s ending at now, plus a session snapshot.
type FakeDiagnostics struct{}

// NewFakeDiagnostics returns a FakeDiagnostics.
func NewFakeDiagnostics() *FakeDiagnostics {
	return &FakeDiagnostics{}
}

// FetchResourceStats returns the last four 15s resource stats newer than
// lastSeen and a fixed user-session count reading.
func (f *FakeDiagnostics) FetchResourceStats(_ context.Context, _ acquisition.SQLConfig, _ acquisition.Target, lastSeen time.Time) ([]acquisition.DBResourceStat, []acquisition.RawReading, error) {
	f64 := func(v float64) *float64 { return &v }
	now := time.Now().UTC().Truncate(time.Second)
	var stats []acquisition.DBResourceStat
	for i := 0; i < 4; i++ { // oldest..newest, 15s cadence ending at now
		end := now.Add(-time.Duration((3-i)*15) * time.Second)
		if !end.After(lastSeen) {
			continue
		}
		stats = append(stats, acquisition.DBResourceStat{
			EndTimeUTC:            end,
			AvgCPUPercent:         f64(float64(20 + i*5)),
			AvgDataIOPercent:      f64(float64(10 + i*2)),
			AvgLogWritePercent:    f64(float64(5 + i*3)),
			AvgMemoryUsagePercent: f64(float64(40 + i*5)),
			CPULimit:              f64(2),
		})
	}

	count := float64(7)
	readings := []acquisition.RawReading{{
		MetricCode:      acquisition.CodeSessionsCount,
		Unit:            acquisition.UnitForCode(acquisition.CodeSessionsCount),
		Timestamp:       now,
		IntervalSeconds: 60,
		Average:         &count,
		Source:          acquisition.SourceSQLDiag,
	}}
	return stats, readings, nil
}

// FakeQueryStore implements acquisition.QueryStoreProvider with two fixed
// queries and their runtime stats.
type FakeQueryStore struct{}

// NewFakeQueryStore returns a FakeQueryStore.
func NewFakeQueryStore() *FakeQueryStore {
	return &FakeQueryStore{}
}

// FetchRuntimeStats returns two deterministic Query Store queries and their
// interval stats (avg_duration in microseconds: 1500 and 42000).
func (f *FakeQueryStore) FetchRuntimeStats(_ context.Context, _ acquisition.SQLConfig, _ acquisition.Target) ([]acquisition.QueryStoreQuery, []acquisition.QueryStoreRuntimeStat, error) {
	f64 := func(v float64) *float64 { return &v }
	i64 := func(v int64) *int64 { return &v }
	snapshot := time.Now().UTC().Truncate(time.Second)
	intervalStart := snapshot.Add(-time.Minute)

	textID1, textID2 := int64(101), int64(202)
	hash1, hash2 := "a1b2c3d4e5f60718", "0f1e2d3c4b5a6978"
	sql1 := "SELECT TOP (10) * FROM dbo.orders WHERE customer_id = @customer_id"
	sql2 := "UPDATE dbo.inventory SET qty = qty - @qty WHERE sku = @sku"
	internal := false
	queries := []acquisition.QueryStoreQuery{
		{QueryID: 1, QueryTextID: &textID1, QueryHash: &hash1, QuerySQLText: &sql1, IsInternalQuery: &internal, SnapshotAtUTC: snapshot},
		{QueryID: 2, QueryTextID: &textID2, QueryHash: &hash2, QuerySQLText: &sql2, IsInternalQuery: &internal, SnapshotAtUTC: snapshot},
	}
	stats := []acquisition.QueryStoreRuntimeStat{
		{
			QueryID: 1, IntervalStartUTC: intervalStart, IntervalEndUTC: snapshot,
			CountExecutions: i64(120), AvgDurationUS: f64(1500), AvgCPUTimeUS: f64(600),
			AvgLogicalIOReads: f64(40), AvgLogicalIOWrites: f64(2), SnapshotAtUTC: snapshot,
		},
		{
			QueryID: 2, IntervalStartUTC: intervalStart, IntervalEndUTC: snapshot,
			CountExecutions: i64(15), AvgDurationUS: f64(42000), AvgCPUTimeUS: f64(18000),
			AvgLogicalIOReads: f64(900), AvgLogicalIOWrites: f64(120), SnapshotAtUTC: snapshot,
		},
	}
	return queries, stats, nil
}
