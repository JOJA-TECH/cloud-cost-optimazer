import { apiJson } from './api'

export interface AnalyticsSnapshot {
  ResourceUID?: string
  PeriodType?: string
  PeriodStartUTC?: string
  PeriodEndUTC?: string
  CPUAvg?: number
  CPUP50?: number | null
  CPUP95?: number | null
  CPUP99?: number | null
  CPUMax?: number
  DataIOAvg?: number
  DataIOP95?: number | null
  LogWriteAvg?: number
  LogWriteP95?: number | null
  StorageUsedPct?: number
  StorageUsedBytes?: number
  StorageGrowthBytes?: number
  IsPeakHour?: boolean
  WorkloadIntensity?: number
  AvailableMetrics?: string[]
  ComputedAt?: string
}

export interface PercentileEvidence {
  ResourceUID?: string
  PeriodType?: string
  PeriodStartUTC?: string
  MetricCode?: string
  Percentile?: number
  Value?: number | null
  Method?: string
  ObservationCount?: number
  SourceGranularity?: string
  ComputedAt?: string
}

export interface TrendEvidence {
  ResourceUID?: string
  MetricCode?: string
  WindowStart?: string
  WindowEnd?: string
  Slope?: number
  RSquared?: number
  RSquaredAvailable?: boolean
  ComputedAt?: string
}

export interface WorkloadEvidence {
  ResourceUID?: string
  WindowStart?: string
  WindowEnd?: string
  PeakHours?: number[]
  PeakFrequency?: number
  AvgPeakDuration?: number
  IntensityBand?: string
  AvailableMetrics?: string[]
  ComputedAt?: string
}

export interface CapacityEvidence {
  ResourceUID?: string
  PeriodType?: string
  PeriodStartUTC?: string
  CurrentVCores?: number
  HeadroomPct?: number
  PeakUtilization?: number
  GrowthRateVCores?: number
  RecommendedVCores?: number
  CapacityE?: number
  ComputedAt?: string
}

export interface CostEvidence {
  ResourceUID?: string
  PeriodStartUTC?: string
  PeriodEndUTC?: string
  Currency?: string
  ActualMonthlyCost?: number
  CostPerVCore?: number
  ProjectedMonthly?: number
  SavingsVsCurrent?: number
  CostE?: number
  Available?: boolean
  ComputedAt?: string
}

export interface DESEvidence {
  ResourceUID?: string
  PeriodType?: string
  PeriodStartUTC?: string
  CapacityE?: number
  CostE?: number
  PerformanceE?: number
  WeightsVersion?: string
  ComputedAt?: string
}

export interface ForecastEvidence {
  ForecastUID?: string
  TargetMetric?: string
  HorizonSteps?: number
  StepUnit?: string
  PeriodStartUTC?: string
  YPred?: number
  YLower?: number | null
  YUpper?: number | null
  Alpha?: number
  GeneratedAt?: string
}

export interface BackendRecommendation {
  recommendation_uid: string
  scenario_uid: string
  policy_uid: string
  policy_version: number
  resource_uid: string
  version: number
  status: string
  generation_state: 'complete' | 'no_action' | 'insufficient_information' | string
  action: { type: string; description: string; parameters?: Record<string, unknown> }
  current_configuration: Record<string, unknown>
  proposed_configuration: Record<string, unknown>
  estimated_saving: {
    current_monthly_cost: number
    proposed_monthly_cost: number
    monthly_amount: number
    monthly_percentage: number
    currency: string
    current_cost_basis: string
    proposed_cost_basis: string
    interpretation: string
    comparison_assumptions: string[]
    period_start: string
    period_end: string
    method: string
  }
  risk: { score: number; level: string }
  confidence: {
    score: number
    data_quality_score: number
    evidence_completeness_score: number
    forecast_quality_score: number
    simulation_feasibility_score: number
    policy_match_score: number
    method: string
  }
  rationale: { summary: string; factors: string[] }
  evidence_count: number
  created_at: string
  audit_status?: string
}

export interface BackendJobStatus {
  id: string
  type: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | string
  error?: string
  created_at: string
  finished_at?: string
}

export interface OptimizationPolicy {
  policy_uid: string
  name: string
  version: number
  objective_weights: Record<string, number>
  constraints: Record<string, unknown>
  enabled: boolean
  created_at: string
}

export interface AnalysisWindow {
  from: string
  to: string
}

function utcDayStart(value: string): string {
  return `${value.slice(0, 10)}T00:00:00Z`
}

function addUtcDays(value: string, days: number): string {
  const date = new Date(value)
  date.setUTCDate(date.getUTCDate() + days)
  return date.toISOString()
}

export interface AnalyticsWorkspace {
  snapshots: AnalyticsSnapshot[]
  cpuPercentiles: PercentileEvidence[]
  ioPercentiles: PercentileEvidence[]
  trends: TrendEvidence[]
  workload: WorkloadEvidence | null
  capacity: CapacityEvidence | null
  costs: CostEvidence | null
  des: DESEvidence | null
  forecasts: ForecastEvidence[]
  recommendations: BackendRecommendation[]
  policies: OptimizationPolicy[]
  errors: string[]
}

function params(values: Record<string, string | number | undefined>) {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(values)) {
    if (value !== undefined && value !== '') query.set(key, String(value))
  }
  return query.toString()
}

async function settled<T>(label: string, promise: Promise<T>, errors: string[]): Promise<T | null> {
  try {
    return await promise
  } catch (error) {
    errors.push(`${label}: ${error instanceof Error ? error.message : 'falló la solicitud'}`)
    return null
  }
}

export async function loadAnalyticsWorkspace(resourceUid: string, window: AnalysisWindow): Promise<AnalyticsWorkspace> {
  const errors: string[] = []
  const common = { resource_uid: resourceUid, from: window.from, to: window.to }
  const periodStart = utcDayStart(window.to)
  const forecastEnd = addUtcDays(window.to, 7)
  const [snapshots, cpuPercentiles, ioPercentiles, trends, workload, capacity, costs, des, forecasts, recommendationList, policyList] = await Promise.all([
    settled('Snapshots', apiJson<AnalyticsSnapshot[]>(`/api/v1/analytics/snapshots?${params({ ...common, period_type: '1h', limit: 5000 })}`), errors),
    settled('Percentiles CPU', apiJson<PercentileEvidence[]>(`/api/v1/analytics/percentiles?${params({ ...common, metric_code: 'cpu_percent', period_type: '1h' })}`), errors),
    settled('Percentiles I/O', apiJson<PercentileEvidence[]>(`/api/v1/analytics/percentiles?${params({ ...common, metric_code: 'data_io_percent', period_type: '1h' })}`), errors),
    settled('Tendencias', apiJson<TrendEvidence[]>(`/api/v1/analytics/trends?${params(common)}`), errors),
    settled('Patrón de carga', apiJson<WorkloadEvidence>(`/api/v1/analytics/patterns?${params(common)}`), errors),
    settled('Capacidad', apiJson<CapacityEvidence>(`/api/v1/analytics/capacity?${params({ resource_uid: resourceUid, period_type: '1d', period_start: periodStart })}`), errors),
    settled('Costos analíticos', apiJson<CostEvidence>(`/api/v1/analytics/costs?${params(common)}`), errors),
    settled('DES', apiJson<DESEvidence>(`/api/v1/analytics/des?${params({ resource_uid: resourceUid, period_type: '1d', period_start: periodStart })}`), errors),
    settled('Forecast', apiJson<ForecastEvidence[]>(`/api/v1/analytics/forecasts?${params({ resource_uid: resourceUid, target_metric: 'cpu_avg', from: window.to, to: forecastEnd })}`), errors),
    settled('Recomendaciones', apiJson<{ items?: BackendRecommendation[] }>(`/api/v1/recommendations?${params({ resource_uid: resourceUid, limit: 20 })}`), errors),
    settled('Políticas', apiJson<{ items?: OptimizationPolicy[] }>('/api/v1/recommendations/policies?enabled_only=true'), errors),
  ])

  return {
    snapshots: snapshots ?? [],
    cpuPercentiles: cpuPercentiles ?? [],
    ioPercentiles: ioPercentiles ?? [],
    trends: trends ?? [],
    workload,
    capacity,
    costs,
    des,
    forecasts: forecasts ?? [],
    recommendations: recommendationList?.items ?? [],
    policies: policyList?.items ?? [],
    errors,
  }
}

export async function computeAnalytics(resourceUid: string, window: AnalysisWindow): Promise<string[]> {
  const failures: string[] = []
  const body = { resource_uid: resourceUid, from: window.from, to: window.to }
  const operations: Array<[string, string, Record<string, unknown>]> = [
    ['Snapshots horarios', '/api/v1/analytics/snapshots', body],
    ['Percentiles CPU', '/api/v1/analytics/percentiles', { ...body, metric_code: 'cpu_percent', period_type: '1h' }],
    ['Percentiles I/O', '/api/v1/analytics/percentiles', { ...body, metric_code: 'data_io_percent', period_type: '1h' }],
    ['Tendencias', '/api/v1/analytics/trends', body],
    ['Patrón de carga', '/api/v1/analytics/patterns', body],
    ['Capacidad', '/api/v1/analytics/capacity', { resource_uid: resourceUid, period_type: '1d', period_start: utcDayStart(window.to) }],
    ['Costos', '/api/v1/analytics/costs', body],
    ['Componentes DES', '/api/v1/analytics/des', { resource_uid: resourceUid, period_type: '1d', period_start: utcDayStart(window.to) }],
  ]

  for (const [label, endpoint, payload] of operations) {
    try {
      await apiJson(endpoint, { method: 'POST', body: JSON.stringify(payload) })
    } catch (error) {
      failures.push(`${label}: ${error instanceof Error ? error.message : 'falló el cómputo'}`)
    }
  }
  return failures
}

export async function enqueueBackendRecommendation(input: {
  resource_uid: string
  policy_uid: string
  from: string
  to: string
  horizon_days: number
  currency: string
}): Promise<BackendJobStatus> {
  return apiJson<BackendJobStatus>('/api/v1/jobs/recommendation', {
    method: 'POST',
    body: JSON.stringify({ ...input, force_refresh: true }),
  })
}

export async function getBackendJobStatus(jobId: string): Promise<BackendJobStatus> {
  return apiJson<BackendJobStatus>(`/api/v1/jobs/${encodeURIComponent(jobId)}`)
}


export async function listBackendRecommendations(): Promise<BackendRecommendation[]> {
  const response = await apiJson<{ items?: BackendRecommendation[] }>('/api/v1/recommendations?limit=100')
  return response.items ?? []
}

export async function decideBackendRecommendation(
  recommendationUid: string,
  decision: 'accept' | 'reject',
): Promise<BackendRecommendation> {
  return apiJson<BackendRecommendation>(`/api/v1/recommendations/${encodeURIComponent(recommendationUid)}/${decision}`, {
    method: 'POST',
  })
}
