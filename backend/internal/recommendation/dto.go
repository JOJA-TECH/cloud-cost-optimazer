package recommendation

import (
	"time"

	"github.com/google/uuid"
)

type GenerateRecommendationInput struct {
	ResourceUID      uuid.UUID
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
	Currency            string
	PriceBasis          string
}

type RiskInput struct {
	Candidate  CandidateConfiguration
	Evidence   EvidenceBundle
	Simulation ScenarioSimulation
}

type RecommendationFilter struct {
	ResourceUID *uuid.UUID
	Status      string
	Limit       int
	Offset      int
}

type Recommendation struct {
	UID                    uuid.UUID
	ScenarioUID            uuid.UUID
	ResourceUID            uuid.UUID
	ChosenCandidateID      *int64
	Version                int
	SupersedesUID          *uuid.UUID
	CurrentSKUName         string
	CurrentComputeModel    *string
	CurrentCapacityVCores  *float64
	ProposedSKUName        string
	ProposedComputeModel   *string
	ProposedCapacityVCores *float64
	EstimatedSavingsPct    float64
	EstimatedMonthlySaving *float64
	Confidence             *float64
	Status                 string
	Rationale              string
	GeneratedBy            string
	CreatedAt              time.Time
	DecidedAt              *time.Time
	DecidedBy              *string
	AppliedAt              *time.Time
}

type OpportunityEvaluation struct {
	Candidate CandidateConfiguration
	Feasible  bool
	Policy    PolicyEvaluation
	Saving    EstimatedSaving
	Risk      RecommendationRisk
	Rationale RecommendationRationale
}

type EstimatedSaving struct {
	CurrentMonthlyCost  float64
	ProposedMonthlyCost float64
	MonthlyAmount       float64
	AnnualAmount        float64
	Percent             float64
	Currency            string
	PriceBasis          string
}

type RecommendationRisk struct {
	Score      float64
	Level      string
	Factors    []string
	AssessedAt time.Time
}

type RecommendationRationale struct {
	Summary  string
	Evidence []Evidence
}

type Scenario struct {
	ID               int64
	UID              uuid.UUID
	ResourceUID      uuid.UUID
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
	Allowed             bool
	ViolatedConstraints []string
	Warnings            []string
}

type OptimizationPolicy struct {
	Name        string
	Version     string
	Constraints map[string]any
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
