package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/platform/database"
	"github.com/JOJA-TECH/cloud-cost-optimazer/backend/internal/recommendation"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	_ recommendation.RecommendationRepository     = (*RecommendationRepository)(nil)
	_ recommendation.OptimizationPolicyRepository = (*RecommendationRepository)(nil)
)

type RecommendationRepository struct {
	db *gorm.DB
}

func NewRecommendationRepository(db *gorm.DB) *RecommendationRepository {
	return &RecommendationRepository{db: db}
}

func MigrateRecommendationTables(db *gorm.DB) error {
	models := []struct {
		schema string
		model  any
	}{
		{"opt", &optimizationPolicyRow{}},
		{"opt", &scenarioRow{}},
		{"opt", &candidateRow{}},
		{"opt", &scenarioSimulationRow{}},
		{"opt", &recommendationRow{}},
		{"opt", &recommendationEvidenceRow{}},
		{"opt", &riskAssessmentRow{}},
	}
	for _, entry := range models {
		if err := db.Table(database.TableName(db, entry.schema, tableName(entry.model))).AutoMigrate(entry.model); err != nil {
			return fmt.Errorf("migrate %s.%s: %w", entry.schema, tableName(entry.model), err)
		}
	}
	return nil
}

func tableName(model any) string {
	switch model.(type) {
	case *optimizationPolicyRow:
		return "optimization_policy"
	case *scenarioRow:
		return "scenario"
	case *candidateRow:
		return "candidate_configuration"
	case *scenarioSimulationRow:
		return "scenario_simulation"
	case *recommendationRow:
		return "recommendation"
	case *recommendationEvidenceRow:
		return "recommendation_evidence"
	case *riskAssessmentRow:
		return "risk_assessment"
	default:
		panic("unknown recommendation persistence model")
	}
}

func (r *RecommendationRepository) CreatePolicy(ctx context.Context, policy recommendation.OptimizationPolicy) (recommendation.OptimizationPolicy, error) {
	if policy.Name == "" || policy.Version < 1 || policy.CreatedBy == "" {
		return recommendation.OptimizationPolicy{}, recommendation.ErrInvalidRequest
	}
	if policy.UID == uuid.Nil {
		policy.UID = uuid.New()
	}
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = time.Now().UTC()
	}
	row, err := policyToRow(policy)
	if err != nil {
		return recommendation.OptimizationPolicy{}, err
	}
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "opt", "optimization_policy")).Create(&row).Error; err != nil {
		return recommendation.OptimizationPolicy{}, fmt.Errorf("create optimization policy: %w", err)
	}
	return policy, nil
}

func (r *RecommendationRepository) GetPolicy(ctx context.Context, policyUID uuid.UUID) (recommendation.OptimizationPolicy, error) {
	var row optimizationPolicyRow
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "opt", "optimization_policy")).
		Where("policy_uid = ?", policyUID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return recommendation.OptimizationPolicy{}, recommendation.ErrPolicyNotFound
	}
	if err != nil {
		return recommendation.OptimizationPolicy{}, fmt.Errorf("get optimization policy: %w", err)
	}
	return policyFromRow(row)
}

func (r *RecommendationRepository) GetLatestEnabledPolicy(ctx context.Context, name string) (recommendation.OptimizationPolicy, error) {
	var row optimizationPolicyRow
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "opt", "optimization_policy")).
		Where("name = ? AND enabled = ?", name, true).
		Order("version DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return recommendation.OptimizationPolicy{}, recommendation.ErrPolicyNotFound
	}
	if err != nil {
		return recommendation.OptimizationPolicy{}, fmt.Errorf("get latest enabled optimization policy: %w", err)
	}
	return policyFromRow(row)
}

func (r *RecommendationRepository) ListPolicies(ctx context.Context, name string, enabledOnly bool) ([]recommendation.OptimizationPolicy, error) {
	query := r.db.WithContext(ctx).Table(database.TableName(r.db, "opt", "optimization_policy"))
	if name != "" {
		query = query.Where("name = ?", name)
	}
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	var rows []optimizationPolicyRow
	if err := query.Order("name ASC, version DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list optimization policies: %w", err)
	}
	result := make([]recommendation.OptimizationPolicy, 0, len(rows))
	for _, row := range rows {
		policy, err := policyFromRow(row)
		if err != nil {
			return nil, err
		}
		result = append(result, policy)
	}
	return result, nil
}

func (r *RecommendationRepository) PersistDecision(ctx context.Context, decision recommendation.DecisionPersistence) (recommendation.Recommendation, error) {
	var persisted recommendation.Recommendation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resourceID, err := resolveResourceID(tx, decision.Scenario.ResourceUID)
		if err != nil {
			return err
		}
		policyID, err := resolvePolicyID(tx, decision.Scenario.PolicyUID)
		if err != nil {
			return err
		}

		scenario := decision.Scenario
		if scenario.UID == uuid.Nil {
			scenario.UID = uuid.New()
		}
		if scenario.CreatedAt.IsZero() {
			scenario.CreatedAt = time.Now().UTC()
		}
		scenarioJSON, err := jsonBytes(scenario.ObjectiveWeights)
		if err != nil {
			return err
		}
		constraintsJSON, err := jsonBytes(scenario.Constraints)
		if err != nil {
			return err
		}
		scenarioRecord := scenarioRow{
			ScenarioUID: scenario.UID, ResourceID: resourceID, PolicyID: &policyID,
			CreatedBy: scenario.CreatedBy, HorizonDays: scenario.HorizonDays,
			ObjectiveWeights: scenarioJSON, Constraints: constraintsJSON,
			Status: scenario.Status, CreatedAt: scenario.CreatedAt, CompletedAt: scenario.CompletedAt,
		}
		if err := tx.Table(database.TableName(tx, "opt", "scenario")).Create(&scenarioRecord).Error; err != nil {
			return fmt.Errorf("create scenario: %w", err)
		}
		scenario.ID = scenarioRecord.ScenarioID

		candidateIDs := make([]int64, len(decision.Candidates))
		for i, candidate := range decision.Candidates {
			if candidate.CreatedAt.IsZero() {
				candidate.CreatedAt = time.Now().UTC()
			}
			record := candidateRow{
				ScenarioID: scenario.ID, SKUID: candidate.SKUID,
				ComputeModel: candidate.ComputeModel, CapacityVCores: candidate.CapacityVCores,
				GeneratedBy: candidate.GeneratedBy, EstimatedMonthlyCost: candidate.EstimatedMonthlyCost,
				PriceBasis: candidate.PriceBasis, CreatedAt: candidate.CreatedAt,
			}
			if err := tx.Table(database.TableName(tx, "opt", "candidate_configuration")).Create(&record).Error; err != nil {
				return fmt.Errorf("create candidate %q: %w", candidate.SKUName, err)
			}
			candidateIDs[i] = record.CandidateID
		}
		for i, simulation := range decision.Simulations {
			if i >= len(candidateIDs) {
				return fmt.Errorf("simulation %d does not reference a candidate", i)
			}
			violations, err := jsonBytes(simulation.ViolatedConstraints)
			if err != nil {
				return err
			}
			record := scenarioSimulationRow{
				CandidateID: candidateIDs[i], ModelVersionID: simulation.ModelVersionID,
				ForecastPeriodStartUTC: simulation.ForecastPeriodStartUTC,
				ForecastPeriodEndUTC:   simulation.ForecastPeriodEndUTC,
				PredictedCPUP95:        simulation.PredictedCPUP95, PredictedCPUP99: simulation.PredictedCPUP99,
				PredictedWorkloadIntensity: simulation.PredictedWorkloadIntensity,
				PredictedLatencyP95MS:      simulation.PredictedLatencyP95MS,
				PredictedSLAViolations:     simulation.PredictedSLAViolations,
				PredictedMonthlyCost:       simulation.PredictedMonthlyCost,
				EstimatedSavingsPct:        simulation.EstimatedSavingsPct,
				Feasible:                   simulation.Feasible, ViolatedConstraints: violations,
				ForecastUID: simulation.ForecastUID, ForecastModelVersion: simulation.ForecastModelVersion,
				SimulationMethod: simulation.SimulationMethod, SimulatedAt: simulation.SimulatedAt,
			}
			if err := tx.Table(database.TableName(tx, "opt", "scenario_simulation")).Create(&record).Error; err != nil {
				return fmt.Errorf("create simulation for candidate %d: %w", candidateIDs[i], err)
			}
		}

		recommendationValue := decision.Recommendation
		recommendationValue.ScenarioUID, recommendationValue.ScenarioID = scenario.UID, scenario.ID
		recommendationValue.PolicyUID = scenario.PolicyUID
		if recommendationValue.UID == uuid.Nil {
			recommendationValue.UID = uuid.New()
		}
		if recommendationValue.CreatedAt.IsZero() {
			recommendationValue.CreatedAt = time.Now().UTC()
		}
		if decision.SelectedCandidateIndex != nil {
			index := *decision.SelectedCandidateIndex
			if index < 0 || index >= len(candidateIDs) {
				return fmt.Errorf("selected candidate index %d is out of range", index)
			}
			candidateID := candidateIDs[index]
			recommendationValue.ChosenCandidateID = &candidateID
		}
		var supersedesID *int64
		if recommendationValue.SupersedesUID != nil {
			var previous recommendationRow
			findErr := tx.Table(database.TableName(tx, "opt", "recommendation")).
				Select("recommendation_id").
				Where("recommendation_uid = ?", *recommendationValue.SupersedesUID).
				Take(&previous).Error
			if errors.Is(findErr, gorm.ErrRecordNotFound) {
				return fmt.Errorf("superseded recommendation not found: %w", recommendation.ErrRecommendationNotFound)
			}
			if findErr != nil {
				return fmt.Errorf("resolve superseded recommendation: %w", findErr)
			}
			supersedesID = &previous.RecommendationID
		}
		row, err := recommendationToRow(recommendationValue, policyID, supersedesID)
		if err != nil {
			return err
		}
		if err := tx.Table(database.TableName(tx, "opt", "recommendation")).Create(&row).Error; err != nil {
			return fmt.Errorf("create recommendation: %w", err)
		}
		recommendationValue.ScenarioUID, recommendationValue.ScenarioID = scenario.UID, scenario.ID
		recommendationValue.PolicyVersion = decision.Recommendation.PolicyVersion
		recommendationValue.ChosenCandidateID = row.ChosenCandidateID
		if err := persistEvidence(tx, row.RecommendationID, decision.Evidence); err != nil {
			return err
		}
		if err := persistRisk(tx, row.RecommendationID, decision.RiskAssessment); err != nil {
			return err
		}
		persisted = recommendationValue
		return nil
	})
	if err != nil {
		return recommendation.Recommendation{}, err
	}
	return persisted, nil
}

func (r *RecommendationRepository) CreateScenario(ctx context.Context, scenario recommendation.Scenario) (recommendation.Scenario, error) {
	var out recommendation.Scenario
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resourceID, err := resolveResourceID(tx, scenario.ResourceUID)
		if err != nil {
			return err
		}
		var policyID *int64
		if scenario.PolicyUID != uuid.Nil {
			id, err := resolvePolicyID(tx, scenario.PolicyUID)
			if err != nil {
				return err
			}
			policyID = &id
		}
		weights, err := jsonBytes(scenario.ObjectiveWeights)
		if err != nil {
			return err
		}
		constraints, err := jsonBytes(scenario.Constraints)
		if err != nil {
			return err
		}
		row := scenarioRow{
			ScenarioUID: scenario.UID, ResourceID: resourceID, PolicyID: policyID,
			CreatedBy: scenario.CreatedBy, HorizonDays: scenario.HorizonDays,
			ObjectiveWeights: weights, Constraints: constraints, Status: scenario.Status,
			CreatedAt: scenario.CreatedAt, CompletedAt: scenario.CompletedAt,
		}
		if row.ScenarioUID == uuid.Nil {
			row.ScenarioUID = uuid.New()
		}
		if row.CreatedAt.IsZero() {
			row.CreatedAt = time.Now().UTC()
		}
		if err := tx.Table(database.TableName(tx, "opt", "scenario")).Create(&row).Error; err != nil {
			return err
		}
		scenario.ID, scenario.UID = row.ScenarioID, row.ScenarioUID
		out = scenario
		return nil
	})
	return out, err
}

func (r *RecommendationRepository) CreateCandidate(ctx context.Context, candidate recommendation.CandidateConfiguration) (recommendation.CandidateConfiguration, error) {
	row := candidateRow{
		ScenarioID: candidate.ScenarioID, SKUID: candidate.SKUID,
		ComputeModel: candidate.ComputeModel, CapacityVCores: candidate.CapacityVCores,
		GeneratedBy: candidate.GeneratedBy, EstimatedMonthlyCost: candidate.EstimatedMonthlyCost,
		PriceBasis: candidate.PriceBasis, CreatedAt: candidate.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "opt", "candidate_configuration")).Create(&row).Error; err != nil {
		return recommendation.CandidateConfiguration{}, err
	}
	candidate.ID = row.CandidateID
	return candidate, nil
}

func (r *RecommendationRepository) CreateSimulation(ctx context.Context, simulation recommendation.ScenarioSimulation) (recommendation.ScenarioSimulation, error) {
	violations, err := jsonBytes(simulation.ViolatedConstraints)
	if err != nil {
		return recommendation.ScenarioSimulation{}, err
	}
	row := scenarioSimulationRow{
		CandidateID: simulation.CandidateID, ModelVersionID: simulation.ModelVersionID,
		ForecastPeriodStartUTC: simulation.ForecastPeriodStartUTC, ForecastPeriodEndUTC: simulation.ForecastPeriodEndUTC,
		PredictedCPUP95: simulation.PredictedCPUP95, PredictedCPUP99: simulation.PredictedCPUP99,
		PredictedWorkloadIntensity: simulation.PredictedWorkloadIntensity,
		PredictedLatencyP95MS:      simulation.PredictedLatencyP95MS, PredictedSLAViolations: simulation.PredictedSLAViolations,
		PredictedMonthlyCost: simulation.PredictedMonthlyCost, EstimatedSavingsPct: simulation.EstimatedSavingsPct,
		Feasible: simulation.Feasible, ViolatedConstraints: violations, ForecastUID: simulation.ForecastUID,
		ForecastModelVersion: simulation.ForecastModelVersion, SimulationMethod: simulation.SimulationMethod,
		SimulatedAt: simulation.SimulatedAt,
	}
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "opt", "scenario_simulation")).Create(&row).Error; err != nil {
		return recommendation.ScenarioSimulation{}, err
	}
	simulation.ID = row.SimulationID
	return simulation, nil
}

func (r *RecommendationRepository) CreateRecommendation(ctx context.Context, value recommendation.Recommendation) (recommendation.Recommendation, error) {
	var scenario scenarioRow
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "opt", "scenario")).
		Where("scenario_uid = ?", value.ScenarioUID).Take(&scenario).Error; err != nil {
		return recommendation.Recommendation{}, err
	}
	policyID := int64(0)
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "opt", "optimization_policy")).
		Select("policy_id").Where("policy_uid = ?", value.PolicyUID).Take(&policyID).Error; err != nil {
		return recommendation.Recommendation{}, err
	}
	row, err := recommendationToRow(value, policyID, nil)
	if err != nil {
		return recommendation.Recommendation{}, err
	}
	row.ScenarioID = scenario.ScenarioID
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "opt", "recommendation")).Create(&row).Error; err != nil {
		return recommendation.Recommendation{}, err
	}
	return value, nil
}

func (r *RecommendationRepository) CreateEvidence(ctx context.Context, recommendationUID uuid.UUID, evidence []recommendation.Evidence) error {
	var row recommendationRow
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "opt", "recommendation")).
		Select("recommendation_id").Where("recommendation_uid = ?", recommendationUID).Take(&row).Error; err != nil {
		return err
	}
	return persistEvidence(r.db.WithContext(ctx), row.RecommendationID, evidence)
}

func (r *RecommendationRepository) CreateRiskAssessment(ctx context.Context, recommendationUID uuid.UUID, assessment recommendation.RiskAssessment) (recommendation.RiskAssessment, error) {
	var row recommendationRow
	if err := r.db.WithContext(ctx).Table(database.TableName(r.db, "opt", "recommendation")).
		Select("recommendation_id").Where("recommendation_uid = ?", recommendationUID).Take(&row).Error; err != nil {
		return recommendation.RiskAssessment{}, err
	}
	if err := persistRisk(r.db.WithContext(ctx), row.RecommendationID, assessment); err != nil {
		return recommendation.RiskAssessment{}, err
	}
	return assessment, nil
}

func (r *RecommendationRepository) GetRecommendation(ctx context.Context, recommendationUID uuid.UUID) (recommendation.Recommendation, error) {
	var row recommendationRow
	err := r.db.WithContext(ctx).Table(database.TableName(r.db, "opt", "recommendation")).
		Where("recommendation_uid = ?", recommendationUID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return recommendation.Recommendation{}, recommendation.ErrRecommendationNotFound
	}
	if err != nil {
		return recommendation.Recommendation{}, fmt.Errorf("get recommendation: %w", err)
	}
	return decodeRecommendation(row)
}

func (r *RecommendationRepository) UpdateRecommendationStatus(
	ctx context.Context,
	recommendationUID uuid.UUID,
	expectedStatus recommendation.RecommendationStatus,
	status recommendation.RecommendationStatus,
	actorID string,
	changedAt time.Time,
) (recommendation.Recommendation, error) {
	if recommendationUID == uuid.Nil || expectedStatus == "" || status == "" || actorID == "" {
		return recommendation.Recommendation{}, recommendation.ErrInvalidRequest
	}
	allowed := false
	switch expectedStatus {
	case recommendation.RecommendationStatusProposed, recommendation.RecommendationStatusUnderReview:
		allowed = status == recommendation.RecommendationStatusAccepted ||
			status == recommendation.RecommendationStatusRejected ||
			status == recommendation.RecommendationStatusDismissed
	case recommendation.RecommendationStatusAccepted:
		allowed = status == recommendation.RecommendationStatusApplied
	}
	if !allowed {
		return recommendation.Recommendation{}, recommendation.ErrInvalidStatusTransition
	}
	if changedAt.IsZero() {
		changedAt = time.Now().UTC()
	} else {
		changedAt = changedAt.UTC()
	}

	table := database.TableName(r.db, "opt", "recommendation")
	query := r.db.WithContext(ctx).Table(table)
	var row recommendationRow
	if err := query.Where("recommendation_uid = ?", recommendationUID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return recommendation.Recommendation{}, recommendation.ErrRecommendationNotFound
		}
		return recommendation.Recommendation{}, fmt.Errorf("get recommendation for status update: %w", err)
	}
	value, err := decodeRecommendation(row)
	if err != nil {
		return recommendation.Recommendation{}, err
	}
	if value.Status != expectedStatus {
		return recommendation.Recommendation{}, recommendation.ErrInvalidStatusTransition
	}

	value.Status = status
	updates := map[string]any{
		"status": string(status),
	}
	switch status {
	case recommendation.RecommendationStatusAccepted,
		recommendation.RecommendationStatusRejected,
		recommendation.RecommendationStatusDismissed:
		value.DecidedAt = &changedAt
		value.DecidedBy = &actorID
		updates["decided_at"] = changedAt
		updates["decided_by"] = actorID
	case recommendation.RecommendationStatusApplied:
		value.AppliedAt = &changedAt
		updates["applied_at"] = changedAt
	default:
		return recommendation.Recommendation{}, recommendation.ErrInvalidStatusTransition
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return recommendation.Recommendation{}, fmt.Errorf("encode updated recommendation: %w", err)
	}
	updates["domain_payload"] = datatypes.JSON(payload)

	result := r.db.WithContext(ctx).Table(table).
		Where("recommendation_uid = ? AND status = ?", recommendationUID, string(expectedStatus)).
		Updates(updates)
	if result.Error != nil {
		return recommendation.Recommendation{}, fmt.Errorf("update recommendation status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return recommendation.Recommendation{}, recommendation.ErrInvalidStatusTransition
	}
	return value, nil
}

func (r *RecommendationRepository) ListRecommendations(ctx context.Context, filter recommendation.RecommendationFilter) ([]recommendation.Recommendation, error) {
	query := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "opt", "recommendation") + " AS rec").
		Joins("JOIN " + database.TableName(r.db, "opt", "scenario") + " AS sc ON sc.scenario_id = rec.scenario_id")
	if filter.ResourceUID != nil {
		query = query.Joins("JOIN "+database.TableName(r.db, "core", "resource")+" AS res ON res.resource_id = sc.resource_id").
			Where("res.resource_uid = ?", *filter.ResourceUID)
	}
	if filter.Status != "" {
		query = query.Where("rec.status = ?", string(filter.Status))
	}
	var rows []recommendationRow
	if err := query.Order("rec.created_at DESC").
		Limit(normalizeLimit(filter.Limit)).Offset(filter.Offset).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list recommendations: %w", err)
	}
	result := make([]recommendation.Recommendation, 0, len(rows))
	for _, row := range rows {
		value, err := decodeRecommendation(row)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func (r *RecommendationRepository) GetLatestRecommendation(ctx context.Context, resourceUID uuid.UUID) (recommendation.Recommendation, error) {
	var row recommendationRow
	err := r.db.WithContext(ctx).
		Table(database.TableName(r.db, "opt", "recommendation")+" AS rec").
		Select("rec.*").
		Joins("JOIN "+database.TableName(r.db, "opt", "scenario")+" AS sc ON sc.scenario_id = rec.scenario_id").
		Joins("JOIN "+database.TableName(r.db, "core", "resource")+" AS res ON res.resource_id = sc.resource_id").
		Where("res.resource_uid = ?", resourceUID).
		Where("rec.status NOT IN ?", []string{"dismissed", "rejected"}).
		Order("rec.created_at DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return recommendation.Recommendation{}, recommendation.ErrRecommendationNotFound
	}
	if err != nil {
		return recommendation.Recommendation{}, fmt.Errorf("get latest recommendation: %w", err)
	}
	return decodeRecommendation(row)
}

func resolveResourceID(tx *gorm.DB, uid uuid.UUID) (int64, error) {
	var row resourceLookupRow
	err := tx.Table(database.TableName(tx, "core", "resource")).
		Select("resource_id").Where("resource_uid = ?", uid).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, recommendation.ErrResourceNotFound
	}
	return row.ResourceID, err
}

func resolvePolicyID(tx *gorm.DB, uid uuid.UUID) (int64, error) {
	var row struct {
		PolicyID int64 `gorm:"column:policy_id"`
	}
	err := tx.Table(database.TableName(tx, "opt", "optimization_policy")).
		Select("policy_id").Where("policy_uid = ?", uid).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, recommendation.ErrPolicyNotFound
	}
	return row.PolicyID, err
}

func persistEvidence(tx *gorm.DB, recommendationID int64, evidence []recommendation.Evidence) error {
	for _, item := range evidence {
		contextJSON, err := jsonBytes(map[string]any{"details": item.Context, "method": item.Method, "quality": item.Quality})
		if err != nil {
			return err
		}
		row := recommendationEvidenceRow{
			RecommendationID: recommendationID, EvidenceType: item.Type,
			MetricCode: item.MetricCode, Value: item.Value, Unit: item.Unit,
			WindowStartUTC: item.WindowStartUTC, WindowEndUTC: item.WindowEndUTC,
			Context: contextJSON, CreatedAt: time.Now().UTC(),
		}
		if err := tx.Table(database.TableName(tx, "opt", "recommendation_evidence")).Create(&row).Error; err != nil {
			return fmt.Errorf("create recommendation evidence %q: %w", item.Type, err)
		}
	}
	return nil
}

func persistRisk(tx *gorm.DB, recommendationID int64, assessment recommendation.RiskAssessment) error {
	factors, err := jsonBytes(assessment.Factors)
	if err != nil {
		return err
	}
	if assessment.AssessedAt.IsZero() {
		assessment.AssessedAt = time.Now().UTC()
	}
	row := riskAssessmentRow{
		RecommendationID: recommendationID, ModelVersion: assessment.ModelVersion,
		PeakUsage: assessment.PeakUsage, Variability: assessment.Variability,
		Trend: assessment.Trend, Headroom: assessment.Headroom,
		RiskScore: assessment.Score, RiskLevel: assessment.Level,
		Factors: factors, AssessedAt: assessment.AssessedAt,
	}
	if err := tx.Table(database.TableName(tx, "opt", "risk_assessment")).Create(&row).Error; err != nil {
		return fmt.Errorf("create recommendation risk assessment: %w", err)
	}
	return nil
}

func recommendationToRow(value recommendation.Recommendation, policyID int64, supersedesID *int64) (recommendationRow, error) {
	payload, err := jsonBytes(value)
	if err != nil {
		return recommendationRow{}, err
	}
	action, err := jsonBytes(value.Action.Parameters)
	if err != nil {
		return recommendationRow{}, err
	}
	components, err := jsonBytes(value.Confidence)
	if err != nil {
		return recommendationRow{}, err
	}
	estimatedSavingsPct := value.EstimatedSaving.MonthlyPercentage * 100
	currentCostBasis := string(value.EstimatedSaving.CurrentCostBasis)
	proposedCostBasis := string(value.EstimatedSaving.ProposedCostBasis)
	currency := value.EstimatedSaving.Currency
	savingMethod := value.EstimatedSaving.Method
	return recommendationRow{
		RecommendationUID: value.UID, ScenarioID: value.ScenarioID, PolicyID: policyID,
		ChosenCandidateID: value.ChosenCandidateID, ActionType: string(value.Action.Type),
		ActionDescription: value.Action.Description, ActionParameters: action,
		Version: value.Version, SupersedesID: supersedesID,
		CurrentSKUName:         value.CurrentConfiguration.SKUName,
		CurrentComputeModel:    stringPointer(value.CurrentConfiguration.ComputeModel),
		CurrentCapacityVCores:  value.CurrentConfiguration.CapacityVCores,
		ProposedSKUName:        value.ProposedConfiguration.SKUName,
		ProposedComputeModel:   stringPointer(value.ProposedConfiguration.ComputeModel),
		ProposedCapacityVCores: value.ProposedConfiguration.CapacityVCores,
		CurrentMonthlyCost:     floatPointer(value.EstimatedSaving.CurrentMonthlyCost),
		ProposedMonthlyCost:    floatPointer(value.EstimatedSaving.ProposedMonthlyCost),
		EstimatedSavingsPct:    estimatedSavingsPct,
		EstimatedMonthlySaving: floatPointer(value.EstimatedSaving.MonthlyAmount),
		SavingCurrency:         &currency, SavingPeriodStart: timePointer(value.EstimatedSaving.PeriodStart),
		SavingPeriodEnd: timePointer(value.EstimatedSaving.PeriodEnd), SavingMethod: &savingMethod,
		CurrentCostBasis: &currentCostBasis, ProposedCostBasis: &proposedCostBasis,
		Confidence: floatPointer(value.Confidence.Score), ConfidenceComponents: components,
		DomainPayload: payload, Status: string(value.Status), Rationale: value.Rationale.Summary,
		GeneratedBy: value.GeneratedBy, CreatedAt: value.CreatedAt,
		DecidedAt: value.DecidedAt, DecidedBy: value.DecidedBy, AppliedAt: value.AppliedAt,
	}, nil
}

func decodeRecommendation(row recommendationRow) (recommendation.Recommendation, error) {
	var value recommendation.Recommendation
	if len(row.DomainPayload) > 0 {
		if err := json.Unmarshal(row.DomainPayload, &value); err != nil {
			return recommendation.Recommendation{}, fmt.Errorf("decode recommendation domain payload: %w", err)
		}
	}
	if value.UID == uuid.Nil {
		return recommendation.Recommendation{}, fmt.Errorf("recommendation %s has no domain payload", row.RecommendationUID)
	}
	value.ScenarioID = row.ScenarioID
	return value, nil
}

func policyToRow(policy recommendation.OptimizationPolicy) (optimizationPolicyRow, error) {
	weights, err := jsonBytes(policy.ObjectiveWeights)
	if err != nil {
		return optimizationPolicyRow{}, err
	}
	constraints, err := jsonBytes(policy.Constraints)
	if err != nil {
		return optimizationPolicyRow{}, err
	}
	return optimizationPolicyRow{
		PolicyUID: policy.UID, Name: policy.Name, Version: policy.Version,
		ObjectiveWeights: weights, Constraints: constraints, Enabled: policy.Enabled,
		CreatedBy: policy.CreatedBy, CreatedAt: policy.CreatedAt,
	}, nil
}

func policyFromRow(row optimizationPolicyRow) (recommendation.OptimizationPolicy, error) {
	var weights map[string]float64
	if err := json.Unmarshal(row.ObjectiveWeights, &weights); err != nil {
		return recommendation.OptimizationPolicy{}, fmt.Errorf("decode policy objective weights: %w", err)
	}
	var constraints map[string]any
	if err := json.Unmarshal(row.Constraints, &constraints); err != nil {
		return recommendation.OptimizationPolicy{}, fmt.Errorf("decode policy constraints: %w", err)
	}
	return recommendation.OptimizationPolicy{
		UID: row.PolicyUID, Name: row.Name, Version: row.Version,
		ObjectiveWeights: weights, Constraints: constraints,
		Enabled: row.Enabled, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
	}, nil
}

func jsonBytes(value any) (datatypes.JSON, error) {
	if value == nil {
		value = map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode JSON value: %w", err)
	}
	return datatypes.JSON(raw), nil
}

func stringPointer(value string) *string  { return &value }
func floatPointer(value float64) *float64 { return &value }
func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}
