package repositories

import (
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics"
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
