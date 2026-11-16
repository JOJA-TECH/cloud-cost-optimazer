package analytics

import (
	"context"
	"errors"
	"testing"
	"time"

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
