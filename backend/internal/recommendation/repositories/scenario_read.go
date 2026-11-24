package repositories

import (
	"context"
	"errors"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListScenarioComparisons reads persisted candidates; it never evaluates or
// reconstructs missing simulation results from unrelated recommendations.
func (r *RecommendationRepository) ListScenarioComparisons(ctx context.Context, uid uuid.UUID, limit int) ([]recommendation.ScenarioComparison, error) {
	db := r.db.WithContext(ctx)
	resourceID, err := resolveResourceID(db, uid)
	if err != nil {
		return nil, err
	}
	var scenarios []scenarioRow
	err = db.Table(database.TableName(db, "opt", "scenario")).
		Where("resource_id = ?", resourceID).Order("created_at DESC, scenario_id DESC").
		Limit(limit).Find(&scenarios).Error
	if err != nil {
		return nil, err
	}
	out := make([]recommendation.ScenarioComparison, 0, len(scenarios))
	for _, scenario := range scenarios {
		item := recommendation.ScenarioComparison{
			UID: scenario.ScenarioUID, Status: scenario.Status, CreatedAt: scenario.CreatedAt,
			HorizonDays: scenario.HorizonDays, Candidates: []recommendation.ScenarioCandidate{},
		}
		var chosen recommendationRow
		err := db.Table(database.TableName(db, "opt", "recommendation")).
			Select("chosen_candidate_id").Where("scenario_id = ?", scenario.ScenarioID).
			Order("created_at DESC, recommendation_id DESC").Take(&chosen).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		var candidates []candidateRow
		err = db.Table(database.TableName(db, "opt", "candidate_configuration")).
			Where("scenario_id = ?", scenario.ScenarioID).Order("candidate_id ASC").Find(&candidates).Error
		if err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			result := recommendation.ScenarioCandidate{
				ID: candidate.CandidateID, CapacityVCores: candidate.CapacityVCores,
				ComputeModel: candidate.ComputeModel, EstimatedMonthlyCost: candidate.EstimatedMonthlyCost,
				Recommended: chosen.ChosenCandidateID != nil && *chosen.ChosenCandidateID == candidate.CandidateID,
			}
			var simulation scenarioSimulationRow
			err := db.Table(database.TableName(db, "opt", "scenario_simulation")).
				Where("candidate_id = ?", candidate.CandidateID).
				Order("simulated_at DESC, simulation_id DESC").Take(&simulation).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			if err == nil {
				result.PredictedMonthlyCost = simulation.PredictedMonthlyCost
				result.PredictedCPUP99 = simulation.PredictedCPUP99
				result.PredictedLatencyMS = simulation.PredictedLatencyP95MS
				result.EstimatedSavingsPct = simulation.EstimatedSavingsPct
				result.PredictedSLAViolations = simulation.PredictedSLAViolations
				result.Feasible = simulation.Feasible
			}
			item.Candidates = append(item.Candidates, result)
		}
		out = append(out, item)
	}
	return out, nil
}
