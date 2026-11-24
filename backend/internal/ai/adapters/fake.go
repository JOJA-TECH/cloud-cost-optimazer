package adapters

import (
	"context"
	"errors"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/ai"
	"github.com/google/uuid"
)

// FakeMLClient implements ai.MLPort for dev/tests.
// Returns a deterministic forecast_uid without calling a real ML service.
type FakeMLClient struct{}

func NewFakeMLClient() *FakeMLClient {
	return &FakeMLClient{}
}

func (f *FakeMLClient) Forecast(ctx context.Context, req ai.ForecastRequest) (uuid.UUID, error) {
	// Deterministic UUID based on request params
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(req.ResourceUID.String()+req.TargetMetric)), nil
}

func (f *FakeMLClient) ValidateContract(ctx context.Context) error {
	// Always valid in fake mode
	return nil
}

// notAvailableMLClient implements MLPort but always returns an error,
// used when CCO_ML_MODE=not_available so the system never presents a fake
// forecast as productive in Azure mode.
type notAvailableMLClient struct{}

func NewNotAvailableMLClient() *notAvailableMLClient {
	return &notAvailableMLClient{}
}

func (n *notAvailableMLClient) Forecast(ctx context.Context, req ai.ForecastRequest) (uuid.UUID, error) {
	return uuid.Nil, errors.New("ML service not available")
}

func (n *notAvailableMLClient) ValidateContract(ctx context.Context) error {
	return errors.New("ML service not available")
}
