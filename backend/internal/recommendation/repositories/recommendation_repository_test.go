package repositories

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type recommendationTestResource struct {
	ResourceID  int64     `gorm:"column:resource_id;primaryKey;autoIncrement"`
	ResourceUID uuid.UUID `gorm:"column:resource_uid;type:uuid;not null;uniqueIndex"`
}

func (recommendationTestResource) TableName() string { return "resource" }

func openRecommendationDB(t *testing.T) (*gorm.DB, *RecommendationRepository, uuid.UUID) {
	t.Helper()
	name := fmt.Sprintf("recommendation-%s", uuid.NewString())
	db, err := database.Open(database.Config{
		Driver: database.DriverSQLite,
		DSN:    "file:" + name + "?mode=memory&cache=shared",
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Table("resource").AutoMigrate(&recommendationTestResource{}); err != nil {
		t.Fatalf("migrate resource lookup table: %v", err)
	}
	if err := MigrateRecommendationTables(db); err != nil {
		t.Fatalf("migrate recommendation tables: %v", err)
	}
	resourceUID := uuid.New()
	if err := db.Table("resource").Create(&recommendationTestResource{ResourceUID: resourceUID}).Error; err != nil {
		t.Fatalf("seed resource: %v", err)
	}
	return db, NewRecommendationRepository(db), resourceUID
}

func createTestPolicy(t *testing.T, repo *RecommendationRepository) recommendation.OptimizationPolicy {
	t.Helper()
	policy, err := repo.CreatePolicy(context.Background(), recommendation.OptimizationPolicy{
		UID: uuid.New(), Name: "safe-cost-optimization", Version: 1,
		ObjectiveWeights: map[string]float64{"cost": 0.6, "risk": 0.4},
		Constraints:      map[string]any{"min_confidence": 0.7},
		Enabled:          true, CreatedBy: "test",
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}
	return policy
}

func testDecision(resourceUID uuid.UUID, policy recommendation.OptimizationPolicy, createdAt time.Time, version int, supersedes *uuid.UUID) recommendation.DecisionPersistence {
	capacity := 4.0
	computeModel := "provisioned"
	priceBasis := string(recommendation.CostBasisListPrice)
	candidateIndex := 0
	feasible := true
	predictedCost := 220.0
	confidence := recommendation.RecommendationConfidence{
		Score: 0.86, DataQualityScore: 0.9, EvidenceCompletenessScore: 0.85,
		ForecastQualityScore: 0.8, SimulationFeasibilityScore: 1,
		PolicyMatchScore: 0.8, Method: recommendation.RecommendationConfidenceMethodWeightedV1,
	}
	return recommendation.DecisionPersistence{
		Scenario: recommendation.Scenario{
			UID: uuid.New(), ResourceUID: resourceUID, PolicyUID: policy.UID,
			CreatedBy: "test", HorizonDays: 30, ObjectiveWeights: policy.ObjectiveWeights,
			Constraints: policy.Constraints, Status: "completed", CreatedAt: createdAt,
		},
		Candidates: []recommendation.CandidateConfiguration{{
			SKUName: "GP_Gen5_4", Tier: "GeneralPurpose", ComputeModel: &computeModel,
			CapacityVCores: &capacity, GeneratedBy: "rule_v1",
			EstimatedMonthlyCost: &predictedCost, PriceBasis: &priceBasis, CreatedAt: createdAt,
		}},
		Simulations: []recommendation.ScenarioSimulation{{
			CandidateID: 1, PredictedMonthlyCost: &predictedCost, Feasible: &feasible,
			SimulationMethod: "rule_v1_test", SimulatedAt: createdAt,
		}},
		Recommendation: recommendation.Recommendation{
			UID: uuid.New(), PolicyUID: policy.UID, PolicyVersion: policy.Version,
			Version: version, ResourceUID: resourceUID, SupersedesUID: supersedes,
			CurrentConfiguration: recommendation.Configuration{
				SKUName: "GP_Gen5_8", ComputeModel: "provisioned",
			},
			ProposedConfiguration: recommendation.Configuration{
				SKUName: "GP_Gen5_4", ComputeModel: "provisioned", CapacityVCores: &capacity,
			},
			Action: recommendation.RecommendationAction{
				Type: recommendation.ActionResizeDown, Description: "Reduce capacity",
				Parameters: map[string]any{"automatic_execution": false},
			},
			EstimatedSaving: recommendation.EstimatedSaving{
				CurrentMonthlyCost: 420, ProposedMonthlyCost: 220,
				MonthlyAmount: 200, MonthlyPercentage: 200.0 / 420.0,
				CurrentCostBasis:  recommendation.CostBasisActualCost,
				ProposedCostBasis: recommendation.CostBasisListPrice,
				Currency:          "USD", PeriodStart: createdAt.Add(-30 * 24 * time.Hour),
				PeriodEnd: createdAt, Method: "test",
			},
			Confidence: confidence,
			Risk: recommendation.RecommendationRisk{
				Score: 0.21, Level: recommendation.RiskLevelLow, ModelVersion: "rule_v1",
			},
			Rationale: recommendation.RecommendationRationale{Summary: "Evidence-backed test decision"},
			Evidence: []recommendation.RecommendationEvidence{{
				Type: "cpu_p95", Value: floatPointer(30), Unit: stringPointer("percent"),
			}},
			Status: recommendation.RecommendationStatusProposed, GeneratedBy: "rule_v1",
			CreatedAt: createdAt, GenerationState: "complete",
		},
		Evidence: []recommendation.Evidence{{
			Type: "cpu_p95", Value: floatPointer(30), Unit: stringPointer("percent"),
			Method: "test", Quality: floatPointer(0.9),
		}},
		RiskAssessment: recommendation.RiskAssessment{
			ModelVersion: "rule_v1", PeakUsage: floatPointer(0.5),
			Score: 0.21, Level: string(recommendation.RiskLevelLow),
			Factors: map[string]any{"peak_usage_risk": 0.5}, AssessedAt: createdAt,
		},
		SelectedCandidateIndex: &candidateIndex,
	}
}

func TestPersistDecisionReadsListsAndVersionsRecommendation(t *testing.T) {
	db, repo, resourceUID := openRecommendationDB(t)
	ctx := context.Background()
	policy := createTestPolicy(t, repo)
	firstTime := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	first, err := repo.PersistDecision(ctx, testDecision(resourceUID, policy, firstTime, 1, nil))
	if err != nil {
		t.Fatalf("persist first decision: %v", err)
	}
	if first.UID == uuid.Nil || first.ScenarioUID == uuid.Nil || first.ScenarioID == 0 {
		t.Fatalf("persisted recommendation lacks identifiers: %+v", first)
	}
	if first.ChosenCandidateID == nil {
		t.Fatal("persisted recommendation has no selected candidate id")
	}

	for table, want := range map[string]int64{
		"scenario": 1, "candidate_configuration": 1, "scenario_simulation": 1,
		"recommendation": 1, "recommendation_evidence": 1, "risk_assessment": 1,
	} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != want {
			t.Errorf("%s count = %d, want %d", table, count, want)
		}
	}

	loaded, err := repo.GetRecommendation(ctx, first.UID)
	if err != nil {
		t.Fatalf("get recommendation: %v", err)
	}
	if len(loaded.Evidence) == 0 {
		t.Fatalf("loaded recommendation has no evidence: %+v", loaded)
	}
	if loaded.Action.Type != recommendation.ActionResizeDown ||
		loaded.Confidence.Score != 0.86 ||
		loaded.EstimatedSaving.MonthlyAmount != 200 ||
		loaded.Evidence[0].Type != "cpu_p95" {
		t.Fatalf("loaded recommendation did not round-trip: %+v", loaded)
	}
	latest, err := repo.GetLatestRecommendation(ctx, resourceUID)
	if err != nil || latest.UID != first.UID {
		t.Fatalf("GetLatestRecommendation = (%+v, %v), want first recommendation", latest, err)
	}
	filtered, err := repo.ListRecommendations(ctx, recommendation.RecommendationFilter{
		ResourceUID: &resourceUID, Status: recommendation.RecommendationStatusProposed,
	})
	if err != nil || len(filtered) != 1 || filtered[0].UID != first.UID {
		t.Fatalf("ListRecommendations = (%+v, %v), want the first recommendation", filtered, err)
	}

	secondTime := firstTime.Add(time.Hour)
	secondDecision := testDecision(resourceUID, policy, secondTime, 2, &first.UID)
	second, err := repo.PersistDecision(ctx, secondDecision)
	if err != nil {
		t.Fatalf("persist superseding decision: %v", err)
	}
	if second.Version != 2 || second.SupersedesUID == nil || *second.SupersedesUID != first.UID {
		t.Fatalf("versioned recommendation did not preserve supersession: %+v", second)
	}
	latest, err = repo.GetLatestRecommendation(ctx, resourceUID)
	if err != nil || latest.UID != second.UID {
		t.Fatalf("latest recommendation = (%+v, %v), want second recommendation", latest, err)
	}
}

func TestUpdateRecommendationStatusPersistsDecisionAndApplicationTimestamps(t *testing.T) {
	ctx := context.Background()
	db, repo, resourceUID := openRecommendationDB(t)
	policy := createTestPolicy(t, repo)
	persisted, err := repo.PersistDecision(ctx, testDecision(
		resourceUID, policy, time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC), 1, nil,
	))
	if err != nil {
		t.Fatalf("persist recommendation: %v", err)
	}

	actorID := uuid.NewString()
	decidedAt := time.Date(2026, 11, 15, 21, 0, 0, 0, time.UTC)
	accepted, err := repo.UpdateRecommendationStatus(
		ctx, persisted.UID, recommendation.RecommendationStatusProposed,
		recommendation.RecommendationStatusAccepted, actorID, decidedAt,
	)
	if err != nil {
		t.Fatalf("accept recommendation: %v", err)
	}
	if accepted.Status != recommendation.RecommendationStatusAccepted ||
		accepted.DecidedAt == nil || !accepted.DecidedAt.Equal(decidedAt) ||
		accepted.DecidedBy == nil || *accepted.DecidedBy != actorID {
		t.Fatalf("accepted state was not returned: %#v", accepted)
	}

	appliedAt := decidedAt.Add(time.Hour)
	applied, err := repo.UpdateRecommendationStatus(
		ctx, persisted.UID, recommendation.RecommendationStatusAccepted,
		recommendation.RecommendationStatusApplied, actorID, appliedAt,
	)
	if err != nil {
		t.Fatalf("mark recommendation applied: %v", err)
	}
	if applied.Status != recommendation.RecommendationStatusApplied ||
		applied.DecidedAt == nil || !applied.DecidedAt.Equal(decidedAt) ||
		applied.DecidedBy == nil || *applied.DecidedBy != actorID ||
		applied.AppliedAt == nil || !applied.AppliedAt.Equal(appliedAt) {
		t.Fatalf("applied state did not preserve decision metadata: %#v", applied)
	}

	loaded, err := repo.GetRecommendation(ctx, persisted.UID)
	if err != nil {
		t.Fatalf("reload updated recommendation: %v", err)
	}
	if loaded.Status != applied.Status || loaded.DecidedAt == nil ||
		!loaded.DecidedAt.Equal(decidedAt) || loaded.AppliedAt == nil ||
		!loaded.AppliedAt.Equal(appliedAt) {
		t.Fatalf("updated status was not persisted in the domain payload: %#v", loaded)
	}

	_, err = repo.UpdateRecommendationStatus(
		ctx, persisted.UID, recommendation.RecommendationStatusAccepted,
		recommendation.RecommendationStatusRejected, actorID, appliedAt,
	)
	if !errors.Is(err, recommendation.ErrInvalidStatusTransition) {
		t.Fatalf("got %v, want invalid status transition", err)
	}

	var row recommendationRow
	if err := db.Table("recommendation").Where("recommendation_uid = ?", persisted.UID).Take(&row).Error; err != nil {
		t.Fatalf("read persisted recommendation row: %v", err)
	}
	if row.Status != string(recommendation.RecommendationStatusApplied) ||
		row.DecidedBy == nil || *row.DecidedBy != actorID ||
		row.AppliedAt == nil || !row.AppliedAt.Equal(appliedAt) {
		t.Fatalf("status columns are inconsistent with the payload: %#v", row)
	}
}

func TestPersistDecisionRollsBackWhenSimulationsOutnumberCandidates(t *testing.T) {
	db, repo, resourceUID := openRecommendationDB(t)
	policy := createTestPolicy(t, repo)
	decision := testDecision(resourceUID, policy, time.Now().UTC(), 1, nil)
	decision.Simulations = append(decision.Simulations, decision.Simulations[0])

	if _, err := repo.PersistDecision(context.Background(), decision); err == nil {
		t.Fatal("PersistDecision succeeded with a simulation that has no candidate")
	}
	for _, table := range []string{"scenario", "candidate_configuration", "scenario_simulation", "recommendation"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatalf("count %s after rollback: %v", table, err)
		}
		if count != 0 {
			t.Errorf("%s has %d rows after failed transaction, want 0", table, count)
		}
	}
}

func TestRepositoryMapsMissingRecommendationAndResource(t *testing.T) {
	_, repo, _ := openRecommendationDB(t)
	ctx := context.Background()

	if _, err := repo.GetRecommendation(ctx, uuid.New()); !errors.Is(err, recommendation.ErrRecommendationNotFound) {
		t.Errorf("GetRecommendation missing error = %v, want ErrRecommendationNotFound", err)
	}
	_, err := repo.PersistDecision(ctx, testDecision(uuid.New(), recommendation.OptimizationPolicy{
		UID: uuid.New(), Version: 1,
	}, time.Now().UTC(), 1, nil))
	if !errors.Is(err, recommendation.ErrResourceNotFound) {
		t.Errorf("PersistDecision unknown resource error = %v, want ErrResourceNotFound", err)
	}
}
