package recommendation

import (
	"time"

	"github.com/google/uuid"
)

type GenerateRecommendationInput struct {
	ResourceUID      uuid.UUID
	PolicyUID        uuid.UUID
	From             time.Time
	To               time.Time
	HorizonDays      int
	CreatedBy        string
	ObjectiveWeights map[string]float64
	Constraints      map[string]any
}

type OpportunityInput struct {
	ResourceUID uuid.UUID
	Candidate   CandidateConfiguration
	Policy      OptimizationPolicy
	Evidence    EvidenceBundle
}

type SavingInput struct {
	CurrentMonthlyCost  float64
	ProposedMonthlyCost float64
	CurrentCostBasis    CostBasis
	ProposedCostBasis   CostBasis
	Currency            string
	PeriodStart         time.Time
	PeriodEnd           time.Time
	Method              string
}

type RiskInput struct {
	Candidate  CandidateConfiguration
	Evidence   EvidenceBundle
	Simulation ScenarioSimulation
}

type RecommendationFilter struct {
	ResourceUID *uuid.UUID
	Status      RecommendationStatus
	Limit       int
	Offset      int
}

type Recommendation struct {
	UID                   uuid.UUID
	ScenarioUID           uuid.UUID
	PolicyUID             uuid.UUID
	PolicyVersion         int
	Version               int
	ResourceUID           uuid.UUID
	ChosenCandidateID     *int64
	SupersedesUID         *uuid.UUID
	Action                RecommendationAction
	CurrentConfiguration  Configuration
	ProposedConfiguration Configuration
	EstimatedSaving       EstimatedSaving
	Risk                  RecommendationRisk
	Confidence            RecommendationConfidence
	Rationale             RecommendationRationale
	Evidence              []RecommendationEvidence
	Status                RecommendationStatus
	GeneratedBy           string
	CreatedAt             time.Time
	DecidedAt             *time.Time
	DecidedBy             *string
	AppliedAt             *time.Time
}

type OpportunityEvaluation struct {
	Candidate CandidateConfiguration
	Feasible  bool
	Policy    PolicyEvaluation
	Saving    EstimatedSaving
	Risk      RecommendationRisk
	Rationale RecommendationRationale
}

// EstimatedSaving keeps observed current cost separate from the proposed
// price or simulation basis. MonthlyPercentage is a ratio, not a 0-100 value.
// MonthlyAmount = CurrentMonthlyCost - ProposedMonthlyCost, and
// MonthlyPercentage = MonthlyAmount / CurrentMonthlyCost.
type EstimatedSaving struct {
	CurrentMonthlyCost  float64
	ProposedMonthlyCost float64
	MonthlyAmount       float64
	MonthlyPercentage   float64 // fraction of CurrentMonthlyCost, e.g. 0.15 means 15%.
	CurrentCostBasis    CostBasis
	ProposedCostBasis   CostBasis
	Currency            string
	PeriodStart         time.Time
	PeriodEnd           time.Time
	Method              string
}

type CostBasis string

const (
	CostBasisActualCost CostBasis = "actual_cost_daily"
	CostBasisListPrice  CostBasis = "price_catalog"
	CostBasisSimulation CostBasis = "scenario_simulation"
)

type RecommendationRisk struct {
	Score        float64
	Level        RiskLevel
	PeakUsage    float64
	Variability  float64
	Trend        float64
	Headroom     float64
	Factors      map[string]any
	ModelVersion string
}

// RecommendationConfidence records both the reproducible score and the
// components used to derive it. For weighted_components_v1, Score is:
// 0.30*DataQualityScore + 0.25*EvidenceCompletenessScore +
// 0.20*ForecastQualityScore + 0.15*SimulationFeasibilityScore +
// 0.10*PolicyMatchScore.
type RecommendationConfidence struct {
	Score                      float64 `json:"score"`
	DataQualityScore           float64 `json:"data_quality_score"`
	EvidenceCompletenessScore  float64 `json:"evidence_completeness_score"`
	ForecastQualityScore       float64 `json:"forecast_quality_score"`
	SimulationFeasibilityScore float64 `json:"simulation_feasibility_score"`
	PolicyMatchScore           float64 `json:"policy_match_score"`
	Method                     string  `json:"method"`
}

const RecommendationConfidenceMethodWeightedV1 = "weighted_components_v1"

// RecommendationAction describes a proposed action; it is never executed by
// the recommendation engine.
type RecommendationAction struct {
	Type        ActionType
	Description string
	Parameters  map[string]any
}

type ActionType string

const (
	ActionResizeUp           ActionType = "resize_up"
	ActionResizeDown         ActionType = "resize_down"
	ActionChangeComputeModel ActionType = "change_compute_model"
	ActionEnableAutoscaling  ActionType = "enable_autoscaling"
	ActionDisableAutoscaling ActionType = "disable_autoscaling"
	ActionNoAction           ActionType = "no_action"
)

type RecommendationStatus string

const (
	RecommendationStatusProposed    RecommendationStatus = "proposed"
	RecommendationStatusUnderReview RecommendationStatus = "under_review"
	RecommendationStatusAccepted    RecommendationStatus = "accepted"
	RecommendationStatusRejected    RecommendationStatus = "rejected"
	RecommendationStatusApplied     RecommendationStatus = "applied"
	RecommendationStatusDismissed   RecommendationStatus = "dismissed"
)

type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

type Configuration struct {
	SKUName        string
	ComputeModel   string
	CapacityVCores *float64
	Parameters     map[string]any
}

type RecommendationEvidence struct {
	Type           string
	MetricCode     *string
	Value          *float64
	Unit           *string
	WindowStartUTC *time.Time
	WindowEndUTC   *time.Time
	Context        map[string]any
}

type RecommendationRationale struct {
	Summary  string
	Evidence []Evidence
}

type Scenario struct {
	ID               int64
	UID              uuid.UUID
	ResourceUID      uuid.UUID
	PolicyUID        uuid.UUID
	CreatedBy        string
	HorizonDays      int
	ObjectiveWeights map[string]float64
	Constraints      map[string]any
	Status           string
	CreatedAt        time.Time
	CompletedAt      *time.Time
}

type CandidateConfiguration struct {
	ID                   int64
	ScenarioID           int64
	SKUName              string
	ComputeModel         *string
	CapacityVCores       *float64
	GeneratedBy          string
	EstimatedMonthlyCost *float64
	PriceBasis           *string
	CreatedAt            time.Time
}

type ScenarioSimulation struct {
	ID                         int64
	CandidateID                int64
	ModelVersionID             *int64
	ForecastPeriodStartUTC     *time.Time
	ForecastPeriodEndUTC       *time.Time
	PredictedCPUP95            *float64
	PredictedCPUP99            *float64
	PredictedWorkloadIntensity *float64
	PredictedLatencyP95MS      *float64
	PredictedSLAViolations     *int
	PredictedMonthlyCost       *float64
	EstimatedSavingsPct        *float64
	Feasible                   *bool
	ViolatedConstraints        []string
	SimulatedAt                time.Time
}

type Evidence struct {
	Type           string
	MetricCode     *string
	Value          *float64
	Unit           *string
	WindowStartUTC *time.Time
	WindowEndUTC   *time.Time
	Context        map[string]any
}

type RiskAssessment struct {
	ModelVersion string
	PeakUsage    *float64
	Variability  *float64
	Trend        *float64
	Headroom     *float64
	Score        float64
	Level        string
	Factors      map[string]any
	AssessedAt   time.Time
}

type EvidenceBundle struct {
	FeatureSnapshots     []FeatureSnapshot
	PercentileStatistics []PercentileStatistic
	Trends               []Trend
	WorkloadPattern      *WorkloadPattern
	CapacityAnalysis     *CapacityAnalysis
	CostAnalysis         *CostAnalysis
	DESComponents        *DESComponents
	Forecasts            []Forecast
	Costs                []CostRecord
	Configuration        *ConfigurationSnapshot
}

type PolicyEvaluation struct {
	PolicyUID           uuid.UUID
	PolicyVersion       int
	Allowed             bool
	ViolatedConstraints []string
	Warnings            []string
}

type OptimizationPolicy struct {
	UID              uuid.UUID
	Name             string
	Version          int
	ObjectiveWeights map[string]float64
	Constraints      map[string]any
	Enabled          bool
	CreatedBy        string
	CreatedAt        time.Time
}

type PolicyEvaluationInput struct {
	Candidate            CandidateConfiguration
	CurrentConfiguration Configuration
	Simulation           ScenarioSimulation
	Evidence             EvidenceBundle
	Confidence           RecommendationConfidence
	Action               RecommendationAction
}

type PublishRecommendationInput struct {
	Policy               OptimizationPolicy
	Candidate            CandidateConfiguration
	CurrentConfiguration Configuration
	Simulation           ScenarioSimulation
	Evidence             EvidenceBundle
	Recommendation       Recommendation
}

type FeatureSnapshot struct {
	ResourceUID        uuid.UUID
	PeriodType         string
	PeriodStartUTC     time.Time
	PeriodEndUTC       time.Time
	SKUID              *int64
	ConfigSnapshotID   *int64
	ComputeModel       string
	CapacityVCores     float64
	CPUAvg             float64
	CPUP50             *float64
	CPUP95             *float64
	CPUP99             *float64
	CPUMax             float64
	CPUSlope           float64
	DataIOAvg          float64
	DataIOP95          *float64
	LogWriteAvg        float64
	LogWriteP95        *float64
	SessionsAvg        float64
	SessionsP95        *float64
	WorkersAvg         float64
	StorageUsedPct     float64
	StorageUsedBytes   int64
	StorageGrowthBytes float64
	IsPeakHour         bool
	DayOfWeek          int
	HourOfDay          int
	WorkloadIntensity  float64
	WorkloadRunID      *int64
	DeadlockCount      int
	LatencyP95MS       *float64
	ComputedAt         time.Time
}

type PercentileStatistic struct {
	ResourceUID       uuid.UUID
	PeriodType        string
	PeriodStartUTC    time.Time
	MetricCode        string
	Percentile        float64
	Value             *float64
	Method            string
	ObservationCount  int
	SourceGranularity string
	ComputedAt        time.Time
}

type Trend struct {
	ResourceUID uuid.UUID
	MetricCode  string
	WindowStart time.Time
	WindowEnd   time.Time
	Slope       float64
	RSquared    float64
	ComputedAt  time.Time
}

type WorkloadPattern struct {
	ResourceUID     uuid.UUID
	WindowStart     time.Time
	WindowEnd       time.Time
	PeakHours       []int
	PeakFrequency   float64
	AvgPeakDuration float64
	IntensityBand   string
	ComputedAt      time.Time
}

type CapacityAnalysis struct {
	ResourceUID       uuid.UUID
	PeriodType        string
	PeriodStartUTC    time.Time
	CurrentVCores     float64
	HeadroomPct       float64
	PeakUtilization   float64
	GrowthRateVCores  float64
	RecommendedVCores float64
	CapacityE         float64
	ComputedAt        time.Time
}

type CostAnalysis struct {
	ResourceUID       uuid.UUID
	PeriodStartUTC    time.Time
	PeriodEndUTC      time.Time
	ActualMonthlyCost float64
	CostPerVCore      float64
	ProjectedMonthly  float64
	SavingsVsCurrent  float64
	CostE             float64
	ComputedAt        time.Time
}

type DESComponents struct {
	ResourceUID    uuid.UUID
	PeriodType     string
	PeriodStartUTC time.Time
	CapacityE      float64
	CostE          float64
	PerformanceE   float64
	WeightsVersion string
	ComputedAt     time.Time
}

type Forecast struct {
	ForecastUID    uuid.UUID
	ResourceUID    uuid.UUID
	TargetMetric   string
	HorizonSteps   int
	StepUnit       string
	PeriodStartUTC time.Time
	YPred          float64
	YLower         *float64
	YUpper         *float64
	Alpha          float64
	GeneratedAt    time.Time
}

type CostRecord struct {
	ResourceUID    *uuid.UUID
	ProviderCode   string
	ScopeType      string
	ScopeName      string
	ServiceName    string
	RegionCode     *string
	UsageDate      time.Time
	Currency       string
	MeterID        *string
	PreTaxCost     float64
	UsageQuantity  *float64
	UsageUnit      *string
	PricingModel   *string
	ChargeCategory *string
}

type ConfigurationSnapshot struct {
	ResourceUID       uuid.UUID
	SKUName           *string
	Tier              *string
	Family            *string
	CapacityVCores    *float64
	MinCapacityVCores *float64
	ComputeModel      *string
	MaxSizeBytes      *int64
	AutoPauseDelayMin *int
	Status            *string
	ObservedAtUTC     time.Time
}

type PriceQuery struct {
	ProviderCode string
	RegionCode   string
	MeterID      string
	SKUName      string
	PriceType    string
	Currency     string
	EffectiveAt  time.Time
}

type PriceRecord struct {
	ProviderCode   string
	RegionCode     string
	MeterID        string
	MeterName      string
	ProductName    string
	SKUName        *string
	ARMSKUName     *string
	ServiceName    string
	PriceType      string
	UnitOfMeasure  string
	UnitPrice      float64
	Currency       string
	TierMinimum    float64
	EffectiveStart time.Time
	EffectiveEnd   *time.Time
}

type DecisionAuditEvent struct {
	RecommendationUID uuid.UUID
	ResourceUID       uuid.UUID
	Decision          string
	Actor             string
	OccurredAt        time.Time
	Details           map[string]any
}
