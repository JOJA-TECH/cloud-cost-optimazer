package adapters

import (
	"context"
	"testing"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/ai"
	"github.com/google/uuid"
)

func TestFakeMLClient_Forecast(t *testing.T) {
	ctx := context.Background()
	client := NewFakeMLClient()

	// Same params must yield the same (deterministic) UID.
	req1 := ai.ForecastRequest{ResourceUID: uuid.New(), TargetMetric: "cpu_avg"}
	uid1, err := client.Forecast(ctx, req1)
	if err != nil {
		t.Fatalf("Forecast() returned error: %v", err)
	}
	if uid1 == uuid.Nil {
		t.Fatal("Forecast() returned nil UUID")
	}

	req2 := ai.ForecastRequest{ResourceUID: req1.ResourceUID, TargetMetric: req1.TargetMetric}
	uid2, err := client.Forecast(ctx, req2)
	if err != nil {
		t.Fatalf("Forecast() returned error: %v", err)
	}
	if uid1 != uid2 {
		t.Errorf("deterministic Forecast() changed: %v != %v", uid1, uid2)
	}

	// Different target must yield a different UID.
	req3 := ai.ForecastRequest{ResourceUID: req1.ResourceUID, TargetMetric: "workload_intensity"}
	uid3, _ := client.Forecast(ctx, req3)
	if uid1 == uid3 {
		t.Errorf("different targets produced the same UID: %v", uid1)
	}
}

func TestFakeMLClient_ValidateContract(t *testing.T) {
	ctx := context.Background()
	client := NewFakeMLClient()
	if err := client.ValidateContract(ctx); err != nil {
		t.Fatalf("ValidateContract() returned error: %v", err)
	}
}

func TestNotAvailableMLClient(t *testing.T) {
	ctx := context.Background()
	client := NewNotAvailableMLClient()

	_, err := client.Forecast(ctx, ai.ForecastRequest{})
	if err == nil {
		t.Fatal("expected error from not-available client")
	}

	if err := client.ValidateContract(ctx); err == nil {
		t.Fatal("expected error from not-available client ValidateContract")
	}
}
