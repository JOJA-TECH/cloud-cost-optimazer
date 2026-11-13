package adapters

import (
	"context"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/analytics"
	"github.com/google/uuid"
)

// FakeMLClient implements analytics.MLPort for dev/tests.
// Returns a deterministic forecast_uid without calling a real ML service.
type FakeMLClient struct{}

func NewFakeMLClient() *FakeMLClient {
	return &FakeMLClient{}
}

func (f *FakeMLClient) Forecast(ctx context.Context, req analytics.ForecastRequest) (uuid.UUID, error) {
	// Deterministic UUID based on request params
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(req.ResourceUID.String()+req.TargetMetric)), nil
}

func (f *FakeMLClient) ValidateContract(ctx context.Context) error {
	// Always valid in fake mode
	return nil
}
