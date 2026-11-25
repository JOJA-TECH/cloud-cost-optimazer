package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFeatureMetricFloatPreservesAvailableZeroAndNullsMissingMetric(t *testing.T) {
	value := featureMetricFloat(analytics.FeatureSnapshotRow{
		AvailableMetrics: []string{"cpu_avg"},
	}, "cpu_avg", 0)
	if value == nil || *value != 0 {
		t.Fatalf("available zero metric = %v, want non-nil zero", value)
	}

	if value := featureMetricFloat(analytics.FeatureSnapshotRow{}, "cpu_avg", 0); value != nil {
		t.Fatalf("unavailable metric = %v, want nil", *value)
	}
}

func TestFeatureSnapshotAvailableMetricsUsesNullabilityNotNumericValue(t *testing.T) {
	zero := float64(0)
	available := featureSnapshotAvailableMetrics(featureSnapshotRow{
		CPUAvg: &zero,
		CPUMax: nil,
	})
	if len(available) != 1 || available[0] != "cpu_avg" {
		t.Fatalf("available metrics = %v, want only cpu_avg", available)
	}
}

func TestCapacityAnalysisUsesFeatureSnapshotInsteadOfStoredDES(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE resource (resource_id integer PRIMARY KEY, resource_uid text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&featureSnapshotRow{}); err != nil {
		t.Fatal(err)
	}
	uid := uuid.New()
	db.Exec("INSERT INTO resource (resource_id, resource_uid) VALUES (1, ?)", uid.String())
	peak := 42.0
	db.Create(&featureSnapshotRow{
		ResourceID: 1, PeriodType: "1h", PeriodStartUTC: time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC),
		CapacityVCores: 2, CPUMax: &peak,
	})

	got, err := NewAnalyticsRepository(db).CapacityAnalysis(context.Background(), uid, "1h", time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.PeakUtilization != 42 || got.CapacityE < 0.579 || got.CapacityE > 0.581 || got.CurrentVCores != 2 {
		t.Fatalf("capacity = %+v", got)
	}

	peak2 := 55.0
	db.Create(&featureSnapshotRow{
		ResourceID: 1, PeriodType: "1h", PeriodStartUTC: time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC),
		CapacityVCores: 2, CPUMax: &peak2,
	})
	daily, err := NewAnalyticsRepository(db).CapacityAnalysis(context.Background(), uid, "1d", time.Date(2026, 11, 24, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if daily.PeakUtilization != 55 || daily.CurrentVCores != 2 {
		t.Fatalf("daily capacity = %+v", daily)
	}
}
