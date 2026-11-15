package repositories

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type optimizationPolicyRow struct {
	PolicyID         int64          `gorm:"column:policy_id;primaryKey;autoIncrement"`
	PolicyUID        uuid.UUID      `gorm:"column:policy_uid;type:uuid;not null;uniqueIndex"`
	Name             string         `gorm:"column:name;not null"`
	Version          int            `gorm:"column:version;not null"`
	ObjectiveWeights datatypes.JSON `gorm:"column:objective_weights;type:jsonb;not null"`
	Constraints      datatypes.JSON `gorm:"column:constraints;type:jsonb;not null"`
	Enabled          bool           `gorm:"column:enabled;not null"`
	CreatedBy        string         `gorm:"column:created_by;not null"`
	CreatedAt        time.Time      `gorm:"column:created_at;not null"`
}

func (optimizationPolicyRow) TableName() string { return "optimization_policy" }

type scenarioRow struct {
	ScenarioID       int64          `gorm:"column:scenario_id;primaryKey;autoIncrement"`
	ScenarioUID      uuid.UUID      `gorm:"column:scenario_uid;type:uuid;not null;uniqueIndex"`
	ResourceID       int64          `gorm:"column:resource_id;not null"`
	PolicyID         *int64         `gorm:"column:policy_id"`
	CreatedBy        string         `gorm:"column:created_by;not null"`
	HorizonDays      int            `gorm:"column:horizon_days;not null"`
	ObjectiveWeights datatypes.JSON `gorm:"column:objective_weights;type:jsonb;not null"`
	Constraints      datatypes.JSON `gorm:"column:constraints;type:jsonb;not null"`
	Status           string         `gorm:"column:status;not null"`
	CreatedAt        time.Time      `gorm:"column:created_at;not null"`
	CompletedAt      *time.Time     `gorm:"column:completed_at"`
}

func (scenarioRow) TableName() string { return "scenario" }

type candidateRow struct {
	CandidateID          int64     `gorm:"column:candidate_id;primaryKey;autoIncrement"`
	ScenarioID           int64     `gorm:"column:scenario_id;not null"`
	SKUID                *int64    `gorm:"column:sku_id"`
	ComputeModel         *string   `gorm:"column:compute_model"`
	CapacityVCores       *float64  `gorm:"column:capacity_vcores"`
	GeneratedBy          string    `gorm:"column:generated_by;not null"`
	EstimatedMonthlyCost *float64  `gorm:"column:estimated_monthly_cost"`
	PriceBasis           *string   `gorm:"column:price_basis"`
	CreatedAt            time.Time `gorm:"column:created_at;not null"`
}

func (candidateRow) TableName() string { return "candidate_configuration" }

type scenarioSimulationRow struct {
	SimulationID               int64          `gorm:"column:simulation_id;primaryKey;autoIncrement"`
	CandidateID                int64          `gorm:"column:candidate_id;not null"`
	ModelVersionID             *int64         `gorm:"column:model_version_id"`
	ForecastPeriodStartUTC     *time.Time     `gorm:"column:forecast_period_start_utc"`
	ForecastPeriodEndUTC       *time.Time     `gorm:"column:forecast_period_end_utc"`
	PredictedCPUP95            *float64       `gorm:"column:predicted_cpu_p95"`
	PredictedCPUP99            *float64       `gorm:"column:predicted_cpu_p99"`
	PredictedWorkloadIntensity *float64       `gorm:"column:predicted_workload_intensity"`
	PredictedLatencyP95MS      *float64       `gorm:"column:predicted_latency_p95_ms"`
	PredictedSLAViolations     *int           `gorm:"column:predicted_sla_violations"`
	PredictedMonthlyCost       *float64       `gorm:"column:predicted_monthly_cost"`
	EstimatedSavingsPct        *float64       `gorm:"column:estimated_savings_pct"`
	Feasible                   *bool          `gorm:"column:feasible"`
	ViolatedConstraints        datatypes.JSON `gorm:"column:violated_constraints;type:jsonb"`
	ForecastUID                *uuid.UUID     `gorm:"column:forecast_uid;type:uuid"`
	ForecastModelVersion       *string        `gorm:"column:forecast_model_version"`
	SimulationMethod           string         `gorm:"column:simulation_method;not null"`
	SimulatedAt                time.Time      `gorm:"column:simulated_at;not null"`
}

func (scenarioSimulationRow) TableName() string { return "scenario_simulation" }

type recommendationRow struct {
	RecommendationID       int64          `gorm:"column:recommendation_id;primaryKey;autoIncrement"`
	RecommendationUID      uuid.UUID      `gorm:"column:recommendation_uid;type:uuid;not null;uniqueIndex"`
	ScenarioID             int64          `gorm:"column:scenario_id;not null"`
	PolicyID               int64          `gorm:"column:policy_id;not null"`
	ChosenCandidateID      *int64         `gorm:"column:chosen_candidate_id"`
	ActionType             string         `gorm:"column:action_type;not null"`
	ActionDescription      string         `gorm:"column:action_description;not null"`
	ActionParameters       datatypes.JSON `gorm:"column:action_parameters;type:jsonb;not null"`
	Version                int            `gorm:"column:version;not null"`
	SupersedesID           *int64         `gorm:"column:supersedes_id"`
	CurrentSKUName         string         `gorm:"column:current_sku_name;not null"`
	CurrentComputeModel    *string        `gorm:"column:current_compute_model"`
	CurrentCapacityVCores  *float64       `gorm:"column:current_capacity_vcores"`
	ProposedSKUName        string         `gorm:"column:proposed_sku_name;not null"`
	ProposedComputeModel   *string        `gorm:"column:proposed_compute_model"`
	ProposedCapacityVCores *float64       `gorm:"column:proposed_capacity_vcores"`
	CurrentMonthlyCost     *float64       `gorm:"column:estimated_current_monthly_cost"`
	ProposedMonthlyCost    *float64       `gorm:"column:estimated_proposed_monthly_cost"`
	EstimatedSavingsPct    float64        `gorm:"column:estimated_savings_pct;not null"`
	EstimatedMonthlySaving *float64       `gorm:"column:estimated_monthly_savings"`
	SavingCurrency         *string        `gorm:"column:saving_currency"`
	SavingPeriodStart      *time.Time     `gorm:"column:saving_period_start_utc"`
	SavingPeriodEnd        *time.Time     `gorm:"column:saving_period_end_utc"`
	SavingMethod           *string        `gorm:"column:saving_method"`
	CurrentCostBasis       *string        `gorm:"column:current_cost_basis"`
	ProposedCostBasis      *string        `gorm:"column:proposed_cost_basis"`
	Confidence             *float64       `gorm:"column:confidence"`
	ConfidenceComponents   datatypes.JSON `gorm:"column:confidence_components;type:jsonb"`
	DomainPayload          datatypes.JSON `gorm:"column:domain_payload;type:jsonb;not null"`
	Status                 string         `gorm:"column:status;not null"`
	Rationale              string         `gorm:"column:rationale;not null"`
	GeneratedBy            string         `gorm:"column:generated_by;not null"`
	CreatedAt              time.Time      `gorm:"column:created_at;not null"`
	DecidedAt              *time.Time     `gorm:"column:decided_at"`
	DecidedBy              *string        `gorm:"column:decided_by"`
	AppliedAt              *time.Time     `gorm:"column:applied_at"`
}

func (recommendationRow) TableName() string { return "recommendation" }

type recommendationEvidenceRow struct {
	EvidenceID       int64          `gorm:"column:evidence_id;primaryKey;autoIncrement"`
	RecommendationID int64          `gorm:"column:recommendation_id;not null"`
	EvidenceType     string         `gorm:"column:evidence_type;not null"`
	MetricCode       *string        `gorm:"column:metric_code"`
	Value            *float64       `gorm:"column:value"`
	Unit             *string        `gorm:"column:unit"`
	WindowStartUTC   *time.Time     `gorm:"column:window_start_utc"`
	WindowEndUTC     *time.Time     `gorm:"column:window_end_utc"`
	Context          datatypes.JSON `gorm:"column:context;type:jsonb"`
	CreatedAt        time.Time      `gorm:"column:created_at;not null"`
}

func (recommendationEvidenceRow) TableName() string { return "recommendation_evidence" }

type riskAssessmentRow struct {
	RiskID           int64          `gorm:"column:risk_id;primaryKey;autoIncrement"`
	RecommendationID int64          `gorm:"column:recommendation_id;not null"`
	ModelVersion     string         `gorm:"column:model_version;not null"`
	PeakUsage        *float64       `gorm:"column:peak_usage"`
	Variability      *float64       `gorm:"column:variability"`
	Trend            *float64       `gorm:"column:trend"`
	Headroom         *float64       `gorm:"column:headroom"`
	RiskScore        float64        `gorm:"column:risk_score;not null"`
	RiskLevel        string         `gorm:"column:risk_level;not null"`
	Factors          datatypes.JSON `gorm:"column:factors;type:jsonb;not null"`
	AssessedAt       time.Time      `gorm:"column:assessed_at;not null"`
}

func (riskAssessmentRow) TableName() string { return "risk_assessment" }

type resourceLookupRow struct {
	ResourceID int64 `gorm:"column:resource_id"`
}
