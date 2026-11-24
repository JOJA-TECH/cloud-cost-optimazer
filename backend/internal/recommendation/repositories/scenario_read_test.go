package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
)

func TestListScenarioComparisonsReadsPersistedValuesOnly(t *testing.T) {
	_, repo, resourceUID := openRecommendationDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	scenario, err := repo.CreateScenario(ctx, recommendation.Scenario{
		UID: uuid.New(), ResourceUID: resourceUID, CreatedBy: "test",
		HorizonDays: 30, ObjectiveWeights: map[string]float64{"cost": 1},
		Constraints: map[string]any{}, Status: "completed", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	capacity, cost, cpu, feasible := 4.0, 120.0, 65.0, true
	candidate, err := repo.CreateCandidate(ctx, recommendation.CandidateConfiguration{
		ScenarioID: scenario.ID, CapacityVCores: &capacity, GeneratedBy: "test",
		EstimatedMonthlyCost: &cost, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateSimulation(ctx, recommendation.ScenarioSimulation{
		CandidateID: candidate.ID, PredictedCPUP99: &cpu, Feasible: &feasible,
		SimulationMethod: "test", SimulatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := repo.ListScenarioComparisons(ctx, resourceUID, 10)
	if err != nil || len(rows) != 1 || len(rows[0].Candidates) != 1 {
		t.Fatalf("scenario listing: %+v, err=%v", rows, err)
	}
	got := rows[0].Candidates[0]
	if got.ID != candidate.ID || got.PredictedCPUP99 == nil || *got.PredictedCPUP99 != cpu || got.PredictedMonthlyCost != nil || got.Recommended {
		t.Fatalf("candidate must preserve absent simulation values: %+v", got)
	}
	other, err := repo.ListScenarioComparisons(ctx, uuid.New(), 10)
	if err == nil || len(other) != 0 {
		t.Fatalf("unknown resource: %+v, err=%v", other, err)
	}
}
