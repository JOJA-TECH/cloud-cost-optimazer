package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// fakeMLPort is a test double for MLPort.
type fakeMLPort struct {
	forecastUID uuid.UUID
	forecastErr error
	validateErr error
	called      bool
}

func (f *fakeMLPort) Forecast(ctx context.Context, req ForecastRequest) (uuid.UUID, error) {
	f.called = true
	return f.forecastUID, f.forecastErr
}

func (f *fakeMLPort) ValidateContract(ctx context.Context) error {
	f.called = true
	return f.validateErr
}

// fakeLLMPort is a test double for LLMPort.
type fakeLLMPort struct {
	explanation string
	err         error
	called      bool
}

func (f *fakeLLMPort) ExplainRecommendation(ctx context.Context, pack EvidencePack) (string, error) {
	f.called = true
	return f.explanation, f.err
}

func TestAIService_Forecast(t *testing.T) {
	ctx := context.Background()
	resourceUID := uuid.New()

	tests := []struct {
		name       string
		ml         MLPort
		wantErr    bool
		wantCalled bool
	}{
		{"ML not configured", nil, true, false},
		{"ML returns error", &fakeMLPort{forecastErr: errors.New("ml down")}, true, true},
		{"ML returns UID", &fakeMLPort{forecastUID: resourceUID}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewAIService(tt.ml, nil, nil)

			req := ForecastRequest{ResourceUID: resourceUID, TargetMetric: "cpu_avg"}
			got, err := svc.Forecast(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (uid=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != resourceUID {
				t.Errorf("Forecast() = %v, want %v", got, resourceUID)
			}
			if tt.wantCalled {
				fake, ok := tt.ml.(*fakeMLPort)
				if !ok || !fake.called {
					t.Error("expected MLPort.Forecast to be called")
				}
			}
		})
	}
}

func TestAIService_ValidateContract(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		ml       MLPort
		wantErr  bool
	}{
		{"ML not configured", nil, true},
		{"ValidateContract returns error", &fakeMLPort{validateErr: errors.New("contract mismatch")}, true},
		{"ValidateContract ok", &fakeMLPort{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewAIService(tt.ml, nil, nil)

			err := svc.ValidateContract(ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAIService_ExplainRecommendation(t *testing.T) {
	ctx := context.Background()
	pack := EvidencePack{
		ResourceUID:    uuid.New(),
		CurrentConfig:  "4 vCores",
		CandidateConfig: "2 vCores",
		RiskLevel:      "low",
	}

	tests := []struct {
		name     string
		llm      LLMPort
		wantErr  bool
	}{
		{"LLM not configured", nil, true},
		{"LLM returns error", &fakeLLMPort{err: errors.New("llm down")}, true},
		{"LLM returns explanation", &fakeLLMPort{explanation: "explicación determinista"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewAIService(nil, tt.llm, nil)

			got, err := svc.ExplainRecommendation(ctx, pack)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != "explicación determinista" {
				t.Errorf("ExplainRecommendation() = %q, want %q", got, "explicación determinista")
			}
			if fake, ok := tt.llm.(*fakeLLMPort); ok && !fake.called {
				t.Error("expected LLMPort.ExplainRecommendation to be called")
			}
		})
	}
}
