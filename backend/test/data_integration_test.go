package test

import (
	"context"
	"errors"
	"testing"
	"time"

	acqrepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/acquisition/repositories"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data"
	datarepos "github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/data/repositories"
	"github.com/google/uuid"
)

// TestDataStackEndToEnd exercises the Gestión de Datos service against a
// migrated cco_data SQLite: escrituras del writer (contrato de acquisition),
// lecturas del service y cobertura HU-05.
func TestDataStackEndToEnd(t *testing.T) {
	db := openTestDB(t)
	dataDB := openAcqDataDB(t)
	ctx := context.Background()

	orgID, userID, projectID := uuid.New(), uuid.New(), uuid.New()
	seedOrgAndProject(t, db, orgID, userID, projectID)
	resourceUID := seedMonitoredBinding(t, db, orgID, userID, projectID)

	// The acquisition TargetRepository reads cco_app; the data repository is
	// the single gateway to cco_data for both modules.
	targets, err := acqrepos.NewTargetRepository(db).ListMonitoredTargets(ctx, nil)
	if err != nil {
		t.Fatalf("ListMonitoredTargets: %v", err)
	}
	var target *struct {
		uid, armID, subscription, region string
		kind                             *string
	}
	for _, tt := range targets {
		if tt.ResourceUID == resourceUID {
			target = &struct {
				uid, armID, subscription, region string
				kind                             *string
			}{tt.ResourceUID.String(), tt.AzureResourceID, tt.SubscriptionID, tt.RegionCode, tt.Kind}
			break
		}
	}
	if target == nil {
		t.Fatalf("monitored target %s not found", resourceUID)
	}

	repo := datarepos.NewDataRepository(dataDB)
	ref := data.ResourceRef{
		ResourceUID:     resourceUID,
		SubscriptionID:  target.subscription,
		AzureResourceID: target.armID,
		RegionCode:      target.region,
		Kind:            target.kind,
	}
	resourceID, err := repo.EnsureResource(ctx, ref)
	if err != nil {
		t.Fatalf("EnsureResource: %v", err)
	}

	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	f64Ptr := func(v float64) *float64 { return &v }

	// Seed 24 hourly cpu readings.
	metrics := make([]data.Metric, 0, 24)
	for h := 0; h < 24; h++ {
		avg := float64(10 + h)
		metrics = append(metrics, data.Metric{
			MetricCode: "cpu_percent",
			TimestampUTC: day.Add(time.Duration(h) * time.Hour).Add(5 * time.Minute),
			IntervalSeconds: 60, AvgValue: &avg,
		})
	}
	if n, err := repo.WriteMetricReadings(ctx, resourceID, metrics); err != nil || n != 24 {
		t.Fatalf("WriteMetricReadings = (%d, %v), want (24, nil)", n, err)
	}

	// Seed two mapped costs and one unmapped (Storage-like).
	uidCopy := resourceUID
	charge := func(id string, cost float64) *string { return &id }
	costs := []data.CostRecord{
		{ResourceUID: &uidCopy, ProviderCode: "azure", ScopeType: "subscription", ScopeName: target.subscription,
			ServiceName: "SQL Database", UsageDate: day, Currency: "USD",
			SourceChargeID: charge("cost:sub:2026-11-01:sql", 1), PreTaxCost: 8.6, ChargeCategory: strPtr("usage")},
		{ResourceUID: &uidCopy, ProviderCode: "azure", ScopeType: "subscription", ScopeName: target.subscription,
			ServiceName: "SQL Database", UsageDate: day.Add(24 * time.Hour), Currency: "USD",
			SourceChargeID: charge("cost:sub:2026-11-02:sql", 1), PreTaxCost: 8.6, ChargeCategory: strPtr("usage")},
		{ProviderCode: "azure", ScopeType: "subscription", ScopeName: target.subscription,
			ServiceName: "Storage", UsageDate: day, Currency: "USD",
			SourceChargeID: charge("cost:sub:2026-11-01:storage", 1), PreTaxCost: 0.14, ChargeCategory: strPtr("usage")},
	}
	if n, err := repo.UpsertCostDaily(ctx, costs); err != nil || n != 3 {
		t.Fatalf("UpsertCostDaily = (%d, %v), want (3, nil)", n, err)
	}

	// Seed two configuration snapshots.
	sku := "GP_S_Gen5_2"
	if _, err := repo.UpsertConfigSnapshot(ctx, data.ConfigurationSnapshot{
		ResourceUID: resourceUID, SKUName: &sku, CapacityVCores: f64Ptr(2), ComputeModel: strPtr("serverless"),
		ObservedAtUTC: day,
	}); err != nil {
		t.Fatalf("UpsertConfigSnapshot 1: %v", err)
	}
	sku4 := "GP_Gen5_4"
	if _, err := repo.UpsertConfigSnapshot(ctx, data.ConfigurationSnapshot{
		ResourceUID: resourceUID, SKUName: &sku4, CapacityVCores: f64Ptr(4), ComputeModel: strPtr("provisioned"),
		ObservedAtUTC: day.Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertConfigSnapshot 2: %v", err)
	}

	svc := data.NewService(repo)
	windowEnd := day.Add(48 * time.Hour)

	t.Run("metric series returns stored points ascending", func(t *testing.T) {
		series, err := svc.MetricSeries(ctx, resourceUID, "cpu_percent", day, windowEnd, 0)
		if err != nil {
			t.Fatalf("MetricSeries: %v", err)
		}
		if len(series) != 24 || series[0].TimestampUTC.After(series[23].TimestampUTC) {
			t.Fatalf("series = %d points, want 24 ascending", len(series))
		}
	})

	t.Run("costs by resource and by scope (unmapped rows included)", func(t *testing.T) {
		byResource, err := svc.Costs(ctx, &resourceUID, "", "", day, windowEnd, 0)
		if err != nil || len(byResource) != 2 {
			t.Fatalf("Costs by resource = (%d, %v), want 2", len(byResource), err)
		}
		byScope, err := svc.Costs(ctx, nil, "subscription", target.subscription, day, windowEnd, 0)
		if err != nil || len(byScope) != 3 {
			t.Fatalf("Costs by scope = (%d, %v), want 3", len(byScope), err)
		}
		unmapped := 0
		for _, c := range byScope {
			if c.ResourceUID == nil {
				unmapped++
			}
		}
		if unmapped != 1 {
			t.Errorf("unmapped rows = %d, want 1", unmapped)
		}
	})

	t.Run("config snapshots newest first", func(t *testing.T) {
		snaps, err := svc.ConfigSnapshots(ctx, resourceUID, time.Time{}, time.Time{}, 0)
		if err != nil || len(snaps) != 2 {
			t.Fatalf("ConfigSnapshots = (%d, %v), want 2", len(snaps), err)
		}
		if *snaps[0].SKUName != "GP_Gen5_4" || *snaps[1].SKUName != "GP_S_Gen5_2" {
			t.Errorf("order = [%s, %s], want newest first", *snaps[0].SKUName, *snaps[1].SKUName)
		}
	})

	t.Run("coverage sufficient with full data and insufficient with gaps", func(t *testing.T) {
		rep, err := svc.Coverage(ctx, resourceUID, "cpu_percent", day, day.Add(24*time.Hour), data.BucketHour, 0.8)
		if err != nil {
			t.Fatalf("Coverage: %v", err)
		}
		if rep.WindowsTotal != 24 || rep.WindowsWithData != 24 || rep.State != data.CoverageSufficient {
			t.Fatalf("full report = %+v", rep)
		}
		sparse, err := svc.Coverage(ctx, resourceUID, "cpu_percent", day, day.Add(72*time.Hour), data.BucketDay, 0.8)
		if err != nil {
			t.Fatalf("Coverage sparse: %v", err)
		}
		if sparse.WindowsTotal != 3 || sparse.WindowsWithData != 1 || sparse.State != data.CoverageInsufficient {
			t.Fatalf("sparse report = %+v", sparse)
		}
	})

	t.Run("unknown resource is a domain 404", func(t *testing.T) {
		if _, err := svc.MetricSeries(ctx, uuid.New(), "cpu_percent", day, windowEnd, 0); !errors.Is(err, data.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}
