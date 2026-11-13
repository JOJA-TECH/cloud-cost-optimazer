package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/cloud"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
)

// This file is the dev/tests implementation of the acquisition providers
// (pattern: internal/cloud/adapters/fake.go): deterministic, hard-coded data,
// no logs, no randomness. Selected when the real Azure/SQL credentials are
// absent.

var (
	_ acquisition.MetricsProvider     = (*FakeMonitor)(nil)
	_ acquisition.DiagnosticsProvider = (*FakeDiagnostics)(nil)
	_ acquisition.QueryStoreProvider  = (*FakeQueryStore)(nil)
	_ acquisition.ConfigProvider      = (*FakeConfig)(nil)
	_ acquisition.CostProvider        = (*FakeCost)(nil)
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

// FakeConfig implements acquisition.ConfigProvider with a deterministic ARM-like
// configuration snapshot for one resource.
type FakeConfig struct{}

// NewFakeConfig returns a FakeConfig.
func NewFakeConfig() *FakeConfig {
	return &FakeConfig{}
}

// FetchConfigSnapshot returns a fixed config reading (serverless 2 vCore).
func (f *FakeConfig) FetchConfigSnapshot(_ context.Context, _ cloud.ConnectionCredentials, _ string) (acquisition.ConfigReading, error) {
	sku := "GP_S_Gen5_2"
	tier := "GeneralPurpose"
	family := "Gen5"
	cap := 2.0
	model := "serverless"
	maxBytes := int64(32 * 1024 * 1024 * 1024)
	delay := 60
	status := "Online"
	free := true
	payload := `{"sku":{"name":"GP_S_Gen5_2"},"properties":{"maxSizeBytes":34359738368,"autoPauseDelay":60,"useFreeLimit":true}}`
	return acquisition.ConfigReading{
		SKUName:           &sku,
		Tier:              &tier,
		Family:            &family,
		CapacityVCores:    &cap,
		ComputeModel:      &model,
		MaxSizeBytes:      &maxBytes,
		AutoPauseDelayMin: &delay,
		Status:            &status,
		FreeLimit:         &free,
		SourcePayload:     &payload,
		ObservedAtUTC:     time.Now().UTC(),
	}, nil
}

// FakeCost implements acquisition.CostProvider with two deterministic daily
// cost rows for one subscription (SQL Database + Storage).
type FakeCost struct{}

// NewFakeCost returns a FakeCost.
func NewFakeCost() *FakeCost {
	return &FakeCost{}
}

// FetchDailyCosts returns two days of costs for the subscription.
func (f *FakeCost) FetchDailyCosts(_ context.Context, _ cloud.ConnectionCredentials, subscriptionID string, from, to time.Time) ([]data.CostRecord, error) {
	var out []data.CostRecord
	sub := subscriptionID
	currency := "USD"
	src := "azure_cost_management"
	cat := "usage"
	model := "consumption"
	unit := "1 Hour"

	for d := from.Truncate(24 * time.Hour); d.Before(to); d = d.Add(24 * time.Hour) {
		// SQL Database cost
		out = append(out, data.CostRecord{
			ProviderCode: "azure", ScopeType: "subscription", ScopeName: sub,
			ServiceName: "SQL Database", UsageDate: d, Currency: currency,
			SourceChargeID: strPtr(fmt.Sprintf("cost:%s:%s:sql", sub, d.Format("2006-01-02"))),
			PreTaxCost: 8.6, UsageQuantity: f64Ptr(24), UsageUnit: &unit,
			PricingModel: &model, ChargeCategory: &cat, Source: src,
		})
		// Storage cost (unmapped resource_uid)
		out = append(out, data.CostRecord{
			ProviderCode: "azure", ScopeType: "subscription", ScopeName: sub,
			ServiceName: "Storage", UsageDate: d, Currency: currency,
			SourceChargeID: strPtr(fmt.Sprintf("cost:%s:%s:storage", sub, d.Format("2006-01-02"))),
			PreTaxCost: 0.14, UsageQuantity: f64Ptr(10), UsageUnit: &unit,
			PricingModel: &model, ChargeCategory: &cat, Source: src,
		})
	}
	return out, nil
}

func strPtr(s string) *string { return &s }
func f64Ptr(v float64) *float64 { return &v }
