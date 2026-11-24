import { apiFetch, apiJson } from './api'
import { isDemoMode } from './demo-mode'
import {
  kpis,
  resource as demoResource,
  costDaily,
  costComponents,
  recommendation,
  resources,
  perfSeries,
  perfSummary,
  reliabilityEvents,
  scenarios,
  forecastSeries,
  forecastMeta,
  workloadHourly,
  workloadClassification,
  workloadPatterns,
  topQueries,
  waitCategories,
  history,
  dataQuality,
  type RiskLevel,
  type EvidenceTone,
} from './mock-data'
import type { HistoryStatus } from './mock-data'
export type { HistoryStatus } from './mock-data'

// ─────────────────────────────────────────────────────────────────────────────
// Tipos del backend (subconjunto usado por el frontend)
// ─────────────────────────────────────────────────────────────────────────────

export interface CloudResource {
  resource_uid: string
  azure_resource_id: string
  server_name: string
  database_name: string
  resource_group: string
  region_code: string
  kind: string
  lifecycle_status: string
  last_seen_at: number
}

export interface CloudAccount {
  cloud_account_id: string
  organization_id: string
  provider_code: string
  display_name: string
  subscription_id: string
  tenant_id?: string
  status: string
  created_at: number
}

export interface MetricPoint {
  timestamp_utc: string
  interval_seconds: number
  avg: number
  min: number
  max: number
  total: number
  count: number
}

export interface MetricsResponse {
  resource_uid: string
  metric_code: string
  points: MetricPoint[]
}

export interface CostRecord {
  resource_uid: string | null
  provider_code: string
  scope_type: string
  scope_name: string
  service_name: string
  region_code: string
  usage_date: string
  currency: string
  source_charge_id: string
  meter_id: string
  pre_tax_cost: number
  usage_quantity: number
  usage_unit: string
  pricing_model: string
  charge_category: string
  source: string
}

export interface ConfigSnapshot {
  observed_at_utc: string
  sku_name: string
  tier: string
  family: string
  capacity_vcores: number
  compute_model: string
  max_size_bytes: number
  auto_pause_delay_minutes: number | null
  status: string
  free_limit: boolean
}

export interface RecommendationItem {
  id: string
  resourceUid?: string
  status?: string
  title: string
  from: string
  to: string
  savingsPct: number
  savingsAbs: number
  risk: RiskLevel
  confidence: number
  latencyImpact: number | null
  analysisPeriod: string
  generated: string
  modelVersion: string
  evidence: Array<{ label: string; value: string; tone: EvidenceTone }>
  constraints: Array<{ label: string; pass: boolean }>
  limitations: string[]
  rationale: string
}

export interface ScenarioItem {
  id: string
  label: string
  capacity: string
  vCores: number
  cost: number
  savings: number | null
  cpuP99: number
  latency: string
  latencyPct: number
  risk: RiskLevel
  sla: 'Pass' | 'Warning' | 'Fail'
  confidence: number | null
  feasible: boolean
  recommended: boolean
}

export interface PersistedScenario {
  scenario_uid: string
  status: string
  created_at: string
  horizon_days: number
  candidates: Array<{
    candidate_id: number
    capacity_vcores: number | null
    compute_model: string | null
    estimated_monthly_cost: number | null
    predicted_monthly_cost: number | null
    predicted_cpu_p99: number | null
    predicted_latency_p95_ms: number | null
    estimated_savings_pct: number | null
    predicted_sla_violations: number | null
    feasible: boolean | null
    recommended: boolean
  }>
}

export interface WorkloadClassificationItem {
  pattern: string
  confidence: number | null
  evidence: string[]
  metrics: Array<{ label: string; value: string }>
}

export interface HistoryItem {
  id: string
  date: string
  resource: string
  current: string
  proposed: string
  savings: string
  risk: RiskLevel
  confidence: number
  status: HistoryStatus
  version: string
  analysisPeriod?: string
}

export interface DataQualityItem {
  label: string
  value: string
  info: string
  tone: EvidenceTone
}

export interface DataQuality {
  globalConfidence: string
  items: DataQualityItem[]
}

// ─────────────────────────────────────────────────────────────────────────────
// Tipos de estado para consultas al backend
// ─────────────────────────────────────────────────────────────────────────────

export type DataSource = 'backend' | 'mock'

export type DataStatus = 'loading' | 'ready' | 'empty' | 'error'

export interface ServiceResult<T> {
  data: T | null
  source: DataSource | null
  status: DataStatus
  error: string | null
}

// Helpers para construir resultados
export function backendResult<T>(data: T): ServiceResult<T> {
  if (Array.isArray(data) && data.length === 0) {
    return { data: null, source: 'backend', status: 'empty', error: 'no_data' }
  }
  return { data, source: 'backend', status: 'ready', error: null }
}

export function emptyResult<T>(reason: 'no_resource' | 'no_data'): ServiceResult<T> {
  return { data: null, source: null, status: 'empty', error: reason }
}

export function mockResult<T>(data: T, reason: 'demo_mode' = 'demo_mode'): ServiceResult<T> {
  return { data, source: 'mock', status: 'ready', error: reason }
}

export function errorResult<T>(message: string): ServiceResult<T> {
  return { data: null, source: null, status: 'error', error: message }
}

function missingRange<T>(): ServiceResult<T> {
  return { data: null, source: null, status: 'empty', error: 'time_range missing' }
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper: llama al backend y devuelve ServiceResult
// ─────────────────────────────────────────────────────────────────────────────

async function tryBackend<T>(fn: () => Promise<T>): Promise<ServiceResult<T>> {
  try {
    const data = await fn()
    return backendResult(data)
  } catch (err: any) {
    return { data: null, source: null, status: 'error', error: err?.message ?? 'Unknown error' }
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Cloud accounts / resources (funcionan con solo SQLite)
// ─────────────────────────────────────────────────────────────────────────────

export async function getCloudAccounts(): Promise<ServiceResult<CloudAccount[]>> {
  return tryBackend(() => apiJson<CloudAccount[]>('/api/v1/cloud-accounts'))
}

export async function getCloudResources(
  cloudAccountId?: string | null
): Promise<ServiceResult<CloudResource[]>> {
  if (!cloudAccountId) {
    const accounts = await getCloudAccounts()
    if (accounts.status !== 'ready' || !accounts.data?.length) {
      return accounts.status === 'error' ? errorResult(accounts.error ?? 'Failed to load accounts') : emptyResult('no_resource')
    }
    cloudAccountId = accounts.data[0].cloud_account_id
  }
  return tryBackend(() => apiJson<CloudResource[]>(`/api/v1/cloud-accounts/${cloudAccountId}/resources`))
}

export interface DashboardResource {
  resource_uid: string
  name: string
  server: string
  service: string
  tier: string | null
  computeModel: string | null
  vCores: number | null
  memoryGb: number | null
  region: string
  storageUsedGb: number | null
  storageLimitGb: number | null
  redundancy: string | null
  state: string
  lastSyncMinutes: number | null
  collation: string | null
  maxWorkers: number | null
  maxSessions: number | null
}

export async function getDashboardResource(cloudAccountId?: string): Promise<ServiceResult<DashboardResource>> {
  if (isDemoMode()) return mockResult({ ...demoResource, resource_uid: 'demo-resource' })
  const cloudResourcesRes = await getCloudResources(cloudAccountId)
  if (cloudResourcesRes.status !== 'ready' || !cloudResourcesRes.data?.length) {
    return cloudResourcesRes.status === 'error'
      ? errorResult(cloudResourcesRes.error ?? 'Failed to load resources')
      : emptyResult('no_resource')
  }

  const r = cloudResourcesRes.data.find(item => item.resource_uid === getSelectedResourceUid()) ?? cloudResourcesRes.data[0]
  const resourceUID = r.resource_uid
  setSelectedResourceUid(resourceUID)

  const configRes = await getConfigSnapshots(resourceUID)
  const latestSnapshot = configRes.source === 'backend' && configRes.data ? configRes.data[0] : null

  const resource: DashboardResource = {
    resource_uid: resourceUID,
    name: r.database_name,
    server: r.server_name,
    service: 'Azure SQL Database',
    tier: latestSnapshot?.tier ?? null,
    computeModel: latestSnapshot?.compute_model ?? null,
    vCores: latestSnapshot?.capacity_vcores ?? null,
    memoryGb: null,
    region: r.region_code,
    storageUsedGb: null,
    storageLimitGb: latestSnapshot?.max_size_bytes ? latestSnapshot.max_size_bytes / (1024 * 1024 * 1024) : null,
    redundancy: null,
    state: r.lifecycle_status,
    lastSyncMinutes: null,
    collation: null,
    maxWorkers: null,
    maxSessions: null,
  }

  return backendResult(resource)
}

// ─────────────────────────────────────────────────────────────────────────────
// Data / metrics (requieren CCO_DATA_DSN)
// ─────────────────────────────────────────────────────────────────────────────

export async function getMetrics(
  resourceUid?: string | null,
  metricCode?: string | null,
  from?: string | null,
  to?: string | null
): Promise<ServiceResult<MetricsResponse | null>> {
  const uid = resourceUid ?? getSelectedResourceUid()
  const range = from && to ? { from, to } : getTimeRange()
  if (!uid || !metricCode) return { data: null, source: null, status: 'empty', error: 'resource_uid or metric_code missing' }
  if (!range) return missingRange()
  return tryBackend(() =>
    apiJson<MetricsResponse>(
      `/api/v1/data/metrics?resource_uid=${uid}&metric_code=${metricCode}&from=${range.from}&to=${range.to}`
    )
  )
}

export async function getCosts(
  resourceUid?: string | null,
  from?: string | null,
  to?: string | null
): Promise<ServiceResult<CostRecord[]>> {
  const uid = resourceUid ?? getSelectedResourceUid()
  const range = from && to ? { from, to } : getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()
  return tryBackend(() =>
    apiJson<CostRecord[]>(`/api/v1/data/costs?resource_uid=${uid}&from=${range.from}&to=${range.to}`)
  )
}

export async function getConfigSnapshots(
  resourceUid?: string | null
): Promise<ServiceResult<ConfigSnapshot[]>> {
  const uid = resourceUid ?? getSelectedResourceUid()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  return tryBackend(() => apiJson<ConfigSnapshot[]>(`/api/v1/data/config-snapshots?resource_uid=${uid}`))
}

// ─────────────────────────────────────────────────────────────────────────────
// Analytics (requieren CCO_DATA_DSN)
// ─────────────────────────────────────────────────────────────────────────────

export interface PercentileResult {
  PeriodStartUTC: string
  Percentile: number
  Value: number | null
  Method: string
}

export interface TrendResult {
  timestamp_utc: string
  cpu_slope: number
  storage_growth: number
}

export interface WorkloadPattern {
  PeakHours: number[]
  PeakFrequency: number
  AvgPeakDuration: number
  IntensityBand: string
  AvailableMetrics: string[]
}

export interface CapacityAnalysis {
  resource_uid: string
  period_type: string
  period_start: string
  headroom_pct: number
  peak_utilization_pct: number
  growth_rate_pct: number
  recommended_vcores: number
  feasible: boolean
}

export interface CostAnalysis {
  ActualMonthlyCost: number
  ProjectedMonthly: number
  SavingsVsCurrent: number
  Available: boolean
}

export interface DESComponents {
  resource_uid: string
  period_type: string
  period_start: string
  capacity_e: number
  cost_e: number
  performance_e: number
}

export interface ForecastResponse {
  ForecastUID: string
  ModelVersionID: number | null
  ResourceUID: string
  TargetMetric: string
  HorizonSteps: number
  StepUnit: string
  PeriodStartUTC: string
  YPred: number
  YLower: number | null
  YUpper: number | null
  Alpha: number
  GeneratedAt: string
}

export interface AnalyticsJob {
  job_uid: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  created_at: string
  completed_at: string | null
  error: string | null
}

// ── Helper para llamadas POST de analytics (computar) ──
async function analyticsPost<T>(path: string, body: any): Promise<ServiceResult<T>> {
  if (isDemoMode()) return errorResult('Backend actions are disabled in demo mode.')
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()
  return tryBackend(() =>
    apiJson<T>(path, {
      method: 'POST',
      body: JSON.stringify({ resource_uid: uid, from: range.from, to: range.to, ...body }),
    })
  )
}

// ── Helper para llamadas GET de analytics (leer) ──
async function analyticsGet<T>(path: string): Promise<ServiceResult<T>> {
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()
  return tryBackend(() =>
    apiJson<T>(`${path}?resource_uid=${uid}&from=${range.from}&to=${range.to}`)
  )
}

export async function getPercentiles(
  metricCode: string = 'cpu_percent',
  periodType: string = '1h',
  resourceUid?: string | null,
  from?: string | null,
  to?: string | null
): Promise<ServiceResult<PercentileResult[]>> {
  const uid = resourceUid ?? getSelectedResourceUid()
  const range = from && to ? { from, to } : getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()
  return tryBackend(() =>
    apiJson<PercentileResult[]>(
      `/api/v1/analytics/percentiles?resource_uid=${uid}&metric_code=${metricCode}&period_type=${periodType}&from=${range.from}&to=${range.to}`
    )
  )
}

export async function getTrends(): Promise<ServiceResult<TrendResult[]>> {
  return analyticsGet<TrendResult[]>('/api/v1/analytics/trends')
}

export async function getWorkloadPattern(): Promise<ServiceResult<WorkloadPattern | null>> {
  return analyticsGet<WorkloadPattern>('/api/v1/analytics/patterns')
}

export async function getCapacityAnalysis(
  periodType: string = '1d'
): Promise<ServiceResult<CapacityAnalysis | null>> {
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()
  return tryBackend(() =>
    apiJson<CapacityAnalysis>(`/api/v1/analytics/capacity?resource_uid=${uid}&period_type=${periodType}&period_start=${range.from}`)
  )
}

export async function getAnalyticsCosts(): Promise<ServiceResult<CostAnalysis | null>> {
  return analyticsGet<CostAnalysis>('/api/v1/analytics/costs')
}

export async function getDESComponents(
  periodType: string = '1d'
): Promise<ServiceResult<DESComponents | null>> {
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()
  return tryBackend(() =>
    apiJson<DESComponents>(`/api/v1/analytics/des?resource_uid=${uid}&period_type=${periodType}&period_start=${range.from}`)
  )
}

export async function getAnalyticsForecasts(
  targetMetric: string = 'cpu_avg'
): Promise<ServiceResult<ForecastResponse[]>> {
  if (isDemoMode()) {
    return mockResult(forecastSeries.filter(point => point.forecast !== null).map((point, index) => ({
      ForecastUID: `demo-${index}`, ModelVersionID: null, ResourceUID: 'demo-resource',
      TargetMetric: targetMetric, HorizonSteps: 14, StepUnit: '1d',
      PeriodStartUTC: new Date(Date.now() + (index + 1) * 86400000).toISOString(),
      YPred: point.forecast!, YLower: point.lower, YUpper: point.upper,
      Alpha: 0.05, GeneratedAt: new Date().toISOString(),
    })))
  }
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()
  return tryBackend(() =>
    apiJson<ForecastResponse[]>(`/api/v1/analytics/forecasts?resource_uid=${uid}&target_metric=${targetMetric}&from=${range.from}&to=${range.to}`)
  )
}

export async function computeSnapshots(): Promise<ServiceResult<AnalyticsJob | null>> {
  return analyticsPost<AnalyticsJob>('/api/v1/analytics/snapshots', {})
}

export async function computePercentiles(
  metricCode: string = 'cpu_percent',
  periodType: string = '1h'
): Promise<ServiceResult<AnalyticsJob | null>> {
  return analyticsPost<AnalyticsJob>('/api/v1/analytics/percentiles', { metric_code: metricCode, period_type: periodType })
}

export async function computeTrends(): Promise<ServiceResult<AnalyticsJob | null>> {
  return analyticsPost<AnalyticsJob>('/api/v1/analytics/trends', {})
}

export async function detectPatterns(): Promise<ServiceResult<AnalyticsJob | null>> {
  return analyticsPost<AnalyticsJob>('/api/v1/analytics/patterns', {})
}

export async function analyzeCapacity(
  periodType: string = '1d'
): Promise<ServiceResult<AnalyticsJob | null>> {
  return analyticsPost<AnalyticsJob>('/api/v1/analytics/capacity', { period_type: periodType })
}

export async function analyzeAnalyticsCosts(): Promise<ServiceResult<AnalyticsJob | null>> {
  return analyticsPost<AnalyticsJob>('/api/v1/analytics/costs', {})
}

export async function computeDES(
  periodType: string = '1d'
): Promise<ServiceResult<AnalyticsJob | null>> {
  return analyticsPost<AnalyticsJob>('/api/v1/analytics/des', { period_type: periodType })
}

export async function requestForecast(
  targetMetric: string = 'cpu_percent',
  horizonSteps: number = 24
): Promise<ServiceResult<AnalyticsJob | null>> {
  return analyticsPost<AnalyticsJob>('/api/v1/analytics/forecasts', {
    target_metric: targetMetric,
    horizon_steps: horizonSteps,
    step_unit: '1h',
  })
}

// ─────────────────────────────────────────────────────────────────────────────
// Recommendations (requieren CCO_DATA_DSN → 503 sin él)
// ─────────────────────────────────────────────────────────────────────────────

interface BackendRecommendation {
  recommendation_uid: string
  resource_uid: string
  status: string
  version: number
  current_configuration: { SKUName?: string; CapacityVCores?: number | null }
  proposed_configuration: { SKUName?: string; CapacityVCores?: number | null }
  estimated_saving: {
    monthly_amount: number
    monthly_percentage: number
    period_start?: string
    period_end?: string
  }
  risk: { level: string }
  confidence: { score: number }
  rationale: { summary: string }
  created_at: string
}

function mapRecommendation(value: BackendRecommendation): RecommendationItem {
  if (!value.recommendation_uid || !value.resource_uid) {
    throw new Error('Invalid recommendation response')
  }
  const level = value.risk?.level?.toLowerCase()
  const risk: RiskLevel = level === 'low' ? 'Low' : level === 'medium' ? 'Medium' : level === 'high' || level === 'critical' ? 'High' : 'None'
  const configuration = (config: BackendRecommendation['current_configuration']) =>
    config?.SKUName || (config?.CapacityVCores != null ? `${config.CapacityVCores} vCores` : 'Not available')
  return {
    id: value.recommendation_uid,
    resourceUid: value.resource_uid,
    status: value.status,
    title: 'Capacity recommendation',
    from: configuration(value.current_configuration),
    to: configuration(value.proposed_configuration),
    savingsPct: value.estimated_saving?.monthly_percentage ?? 0,
    savingsAbs: value.estimated_saving?.monthly_amount ?? 0,
    risk,
    confidence: Math.round((value.confidence?.score ?? 0) * 100),
    latencyImpact: null,
    analysisPeriod: value.estimated_saving?.period_start && value.estimated_saving?.period_end
      ? `${value.estimated_saving.period_start.slice(0, 10)} – ${value.estimated_saving.period_end.slice(0, 10)}`
      : 'Not available',
    generated: value.created_at,
    modelVersion: `v${value.version}`,
    evidence: [],
    constraints: [],
    limitations: [],
    rationale: value.rationale?.summary ?? '',
  }
}

export async function getRecommendations(resourceUid?: string): Promise<ServiceResult<RecommendationItem[]>> {
  return tryBackend(async () => {
    const query = resourceUid ? `?resource_uid=${encodeURIComponent(resourceUid)}` : ''
    const response = await apiJson<{ items: BackendRecommendation[]; count: number }>(`/api/v1/recommendations${query}`)
    if (!Array.isArray(response?.items)) {
      throw new Error('Invalid recommendations response: missing items')
    }
    return response.items.map(mapRecommendation)
  })
}

export async function getRecommendation(uid: string): Promise<ServiceResult<RecommendationItem | null>> {
  return tryBackend(async () => mapRecommendation(await apiJson<BackendRecommendation>(`/api/v1/recommendations/${encodeURIComponent(uid)}`)))
}

export interface RecommendationEvidence {
  recommendation_uid: string
  evidence_items: Array<{
    label: string
    value: string
    tone?: 'good' | 'warn' | 'bad'
  }>
  constraints: Array<{ label: string; pass: boolean }>
  limitations: string[]
  rationale: string
}

export interface AuditEvent {
  event_id: number
  organization_id?: string
  actor: { user_id?: string; actor_type: string; auth_method?: string; principal_id?: string }
  action: string
  resource: { type?: string; id?: string; external_id?: string }
  occurred_at: string
  trace: { request_id?: string; correlation_id?: string; operation_id?: string }
  metadata: Record<string, unknown>
  ip_address?: string
  user_agent?: string
}

export interface AuditEventsResponse {
  items: AuditEvent[]
  count: number
}

export interface RecommendationTrace {
  recommendation_uid: string
  resource_uid: string
  scenario_uid: string
  policy_uid: string
  decision: string
  status: string
  risk: { score: number; level: string }
  confidence: { score: number }
  estimated_saving: { monthly_amount: number; currency: string }
  events: Array<{ event_id: number; action: string; occurred_at: string }>
}

export interface RecommendationDecision {
  recommendation_uid: string
  action: 'accept' | 'reject' | 'dismiss' | 'apply'
  decided_at: string
  decided_by: string
  decided_at_utc: string
  decision_trace_id: string | null
}

export async function getRecommendationEvidence(
  uid: string
): Promise<ServiceResult<RecommendationEvidence | null>> {
  return tryBackend(async () => {
    const response = await apiJson<{
      recommendation_uid: string
      evidence: Array<{ Type: string; Value: number | null; Unit: string | null }>
    }>(`/api/v1/recommendations/${encodeURIComponent(uid)}/evidence`)
    return {
      recommendation_uid: response.recommendation_uid,
      evidence_items: (response.evidence ?? []).map((item) => ({
        label: item.Type,
         value: item.Value === null ? 'Not available' : `${item.Value}${item.Unit ? ` ${item.Unit}` : ''}`,
      })),
      constraints: [],
      limitations: [],
      rationale: '',
    }
  })
}

export async function evaluateRecommendations(
  resourceUid: string,
  policyUid: string | null,
  horizonDays: number,
  from: string,
  to: string,
  currency: string = 'USD'
): Promise<ServiceResult<any>> {
  if (isDemoMode()) return errorResult('Evaluation is not available in demo mode.')
  const uid = resourceUid || getSelectedResourceUid()
  const range = from && to ? { from, to } : getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()
  return tryBackend(() =>
    apiJson('/api/v1/recommendations/evaluate', {
      method: 'POST',
      body: JSON.stringify({
        resource_uid: uid,
        policy_uid: policyUid,
        horizon_days: horizonDays,
        from: range.from,
        to: range.to,
        currency,
      }),
    })
  )
}

export async function acceptRecommendation(uid: string): Promise<ServiceResult<any>> {
  return tryBackend(() => apiJson(`/api/v1/recommendations/${uid}/accept`, { method: 'POST', body: JSON.stringify({}) }))
}

export async function rejectRecommendation(uid: string): Promise<ServiceResult<any>> {
  return tryBackend(() => apiJson(`/api/v1/recommendations/${uid}/reject`, { method: 'POST', body: JSON.stringify({}) }))
}

export async function dismissRecommendation(uid: string): Promise<ServiceResult<any>> {
  return tryBackend(() => apiJson(`/api/v1/recommendations/${uid}/dismiss`, { method: 'POST', body: JSON.stringify({}) }))
}

export async function applyRecommendation(uid: string): Promise<ServiceResult<any>> {
  return tryBackend(() => apiJson(`/api/v1/recommendations/${uid}/apply`, { method: 'POST', body: JSON.stringify({}) }))
}

export async function getRecommendationTrace(uid: string): Promise<ServiceResult<RecommendationTrace | null>> {
  return tryBackend(() => apiJson<RecommendationTrace>(`/api/v1/recommendations/${uid}/trace`))
}

// ─────────────────────────────────────────────────────────────────────────────
// Audit & Traceability (requieren CCO_DATA_DSN → 503 sin él)
// ─────────────────────────────────────────────────────────────────────────────

export async function getAuditEvents(
  params?: {
    action?: string
    entity_type?: string
    entity_id?: string
    correlation_id?: string
    operation_id?: string
    user_id?: string
    from?: string
    to?: string
    limit?: number
    offset?: number
  }
): Promise<ServiceResult<AuditEventsResponse | null>> {
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()

  const query = new URLSearchParams()
  query.set('resource_uid', uid)
  query.set('from', range.from)
  query.set('to', range.to)
  if (params?.action) query.set('action', params.action)
  if (params?.entity_type) query.set('entity_type', params.entity_type)
  if (params?.entity_id) query.set('entity_id', params.entity_id)
  if (params?.correlation_id) query.set('correlation_id', params.correlation_id)
  if (params?.operation_id) query.set('operation_id', params.operation_id)
  if (params?.user_id) query.set('user_id', params.user_id)
  if (params?.limit) query.set('limit', String(params.limit))
  if (params?.offset) query.set('offset', String(params.offset))

  return tryBackend(() => apiJson<AuditEventsResponse>(`/api/v1/audit/events?${query.toString()}`))
}

export async function getAuditEvent(eventId: number): Promise<ServiceResult<AuditEvent | null>> {
  return tryBackend(() => apiJson<AuditEvent>(`/api/v1/audit/events/${eventId}`))
}

export async function getAuditCorrelationHistory(
  correlationId: string
): Promise<ServiceResult<AuditEventsResponse | null>> {
  return tryBackend(() => apiJson<AuditEventsResponse>(`/api/v1/audit/correlations/${correlationId}`))
}

export async function getAuditResourceHistory(
  entityType: string,
  entityId: string
): Promise<ServiceResult<AuditEventsResponse | null>> {
  return tryBackend(() =>
    apiJson<AuditEventsResponse>(`/api/v1/audit/resources/${entityType}/${entityId}/history`)
  )
}

// ─────────────────────────────────────────────────────────────────────────────
// Presentación: backend en modo real, fixtures únicamente en modo demo explícito
// ─────────────────────────────────────────────────────────────────────────────

export async function getKpis(): Promise<ServiceResult<any>> {
  if (isDemoMode()) return mockResult(kpis)
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid || !range) return emptyResult('no_resource')
  const costs = await getCosts(uid, range.from, range.to)
  if (costs.status !== 'ready' || !costs.data) return { ...costs, data: null }
  const [recommendations, analysis] = await Promise.all([getRecommendations(uid), getAnalyticsCosts()])
  const rec = recommendations.status === 'ready' ? recommendations.data?.[0] : null
  const projected = analysis.status === 'ready' && analysis.data?.Available ? analysis.data.ProjectedMonthly : null
  if (projected !== null && !Number.isFinite(projected)) return errorResult('Invalid estimated costs response')
  return backendResult({
    status: null,
    currentCost: costs.data.reduce((sum, item) => sum + item.pre_tax_cost, 0),
    currentCostDelta: null,
    estimatedCost: projected,
    estimatedSavings: rec?.savingsAbs ?? null,
    savingsPct: rec?.savingsPct ?? null,
    risk: rec?.risk ?? null,
    confidence: rec?.confidence ?? null,
  })
}

export async function getCostDaily(
  resourceUid?: string | null,
  from?: string | null,
  to?: string | null
): Promise<ServiceResult<any[]>> {
  if (isDemoMode()) return mockResult(costDaily)
  const costsRes = await getCosts(resourceUid, from, to)
  if (costsRes.status !== 'ready' || !costsRes.data) {
    return { ...costsRes, data: null }
  }
  const daily = new Map<string, number>()
  for (const c of costsRes.data) daily.set(c.usage_date, (daily.get(c.usage_date) ?? 0) + c.pre_tax_cost)
  let cumulative = 0
  return backendResult([...daily.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([day, billed]) => {
    cumulative += billed
    return { day, billed, estimated: null, reference: null, cumulative: +cumulative.toFixed(2) }
  }))
}

export async function getCostComponents(
  resourceUid?: string | null,
  from?: string | null,
  to?: string | null
): Promise<ServiceResult<any[]>> {
  if (isDemoMode()) return mockResult(costComponents)
  const costsRes = await getCosts(resourceUid, from, to)
  if (costsRes.status !== 'ready' || !costsRes.data) {
    return { ...costsRes, data: null }
  }
  const byService = new Map<string, number>()
  for (const c of costsRes.data) {
    byService.set(c.service_name, (byService.get(c.service_name) ?? 0) + c.pre_tax_cost)
  }
  const colors = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)', 'var(--chart-4)']
  let i = 0
  return backendResult(Array.from(byService.entries()).map(([name, current]) => ({
        name,
        current,
        estimated: null,
        color: colors[i++ % colors.length],
      })))
}

export async function getPerfSeries(
  resourceUid?: string | null,
  metricCode: string = 'cpu_percent',
  from?: string | null,
  to?: string | null
): Promise<ServiceResult<any[]>> {
  if (isDemoMode()) return mockResult(perfSeries.map((point, idx) => ({
    ...point, timestamp: new Date(Date.now() - (perfSeries.length - idx) * 3600000).toISOString(),
  })))
  const uid = resourceUid ?? getSelectedResourceUid()
  const range = from && to ? { from, to } : getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()

  const [cpuRes, ioRes, memoryRes, sessionsRes] = await Promise.all([
    getPercentiles('cpu_percent', '1h', uid, range.from, range.to),
    getMetrics(uid, 'data_io_percent', range.from, range.to),
    getMetrics(uid, 'memory_usage_percent', range.from, range.to),
    getMetrics(uid, 'sessions_count', range.from, range.to),
  ])
  const failed = [cpuRes, ioRes, memoryRes, sessionsRes].find(result => result.status === 'error')
  if (failed) return errorResult(failed.error ?? 'Failed to load metrics')

  const cpuPoints = cpuRes.source === 'backend' && cpuRes.data ? cpuRes.data : []
  const ioPoints = ioRes.source === 'backend' && ioRes.data ? ioRes.data.points : []
  const memoryPoints = memoryRes.source === 'backend' && memoryRes.data ? memoryRes.data.points : []
  const sessionsPoints = sessionsRes.source === 'backend' && sessionsRes.data ? sessionsRes.data.points : []

  const pointMap = new Map<string, any>()

  cpuPoints.forEach((p) => {
    if (!p.PeriodStartUTC || p.Value == null || p.Method === 'not_available') return
    const percentile = Math.round(p.Percentile * 100)
    if (![50, 95, 99].includes(percentile)) return
    const existing = pointMap.get(p.PeriodStartUTC)
    if (existing) {
      existing[`p${percentile}`] = p.Value
      return
    }
    pointMap.set(p.PeriodStartUTC, {
      t: new Date(p.PeriodStartUTC).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }),
      timestamp: p.PeriodStartUTC,
      idx: pointMap.size,
      p50: percentile === 50 ? p.Value : null,
      p95: percentile === 95 ? p.Value : null,
      p99: percentile === 99 ? p.Value : null,
      io: null,
      memory: null,
      sessions: null,
      latency: null,
    })
  })

  ioPoints.forEach((p) => {
    const existing = pointMap.get(p.timestamp_utc)
    if (existing) existing.io = +p.avg.toFixed(1)
    else pointMap.set(p.timestamp_utc, { t: new Date(p.timestamp_utc).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }), timestamp: p.timestamp_utc, idx: 0, p50: null, p95: null, p99: null, io: +p.avg.toFixed(1), memory: null, sessions: null, latency: null })
  })

  memoryPoints.forEach((p) => {
    const existing = pointMap.get(p.timestamp_utc)
    if (existing) existing.memory = +p.avg.toFixed(1)
    else pointMap.set(p.timestamp_utc, { t: new Date(p.timestamp_utc).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }), timestamp: p.timestamp_utc, idx: 0, p50: null, p95: null, p99: null, io: null, memory: +p.avg.toFixed(1), sessions: null, latency: null })
  })

  sessionsPoints.forEach((p) => {
    const existing = pointMap.get(p.timestamp_utc)
    if (existing) existing.sessions = Math.round(p.avg)
    else pointMap.set(p.timestamp_utc, { t: new Date(p.timestamp_utc).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }), timestamp: p.timestamp_utc, idx: 0, p50: null, p95: null, p99: null, io: null, memory: null, sessions: Math.round(p.avg), latency: null })
  })

  return backendResult(Array.from(pointMap.values()))
}

export async function getPerfSummary(): Promise<ServiceResult<any[]>> {
  if (isDemoMode()) return mockResult(perfSummary)
  const result = await getPercentiles()
  if (result.status !== 'ready' || !result.data?.length) return { ...result, data: null }
  const summary = [50, 95, 99].flatMap(percentile => {
    const latest = result.data!.filter(row => Math.round(row.Percentile * 100) === percentile && row.Value != null && row.Method !== 'not_available').at(-1)
    return latest ? [{ label: `CPU p${percentile}`, value: `${latest.Value!.toFixed(1)}%`, tone: 'good' }] : []
  })
  return backendResult(summary)
}

export async function getReliabilityEvents(): Promise<ServiceResult<any[]>> {
  return isDemoMode() ? mockResult(reliabilityEvents) : emptyResult('no_data')
}

export async function getRecommendationSummary(): Promise<ServiceResult<RecommendationItem>> {
  if (isDemoMode()) return mockResult(recommendation as RecommendationItem)
  const uid = getSelectedResourceUid()
  if (!uid) return emptyResult('no_resource')
  const recs = await getRecommendations(uid)
  if (recs.status !== 'ready' || !recs.data?.length) {
    return { data: null, source: recs.source, status: recs.status, error: recs.error }
  }
  return backendResult(recs.data[0])
}

export async function getScenarios(): Promise<ServiceResult<ScenarioItem[]>> {
  return isDemoMode() ? mockResult(scenarios) : emptyResult('no_data')
}

export async function getPersistedScenarios(resourceUid: string): Promise<ServiceResult<PersistedScenario[]>> {
  return tryBackend(async () => {
    const items = await apiJson<PersistedScenario[]>(`/api/v1/recommendations/scenarios?resource_uid=${encodeURIComponent(resourceUid)}`)
    if (!Array.isArray(items) || items.some(item => !Array.isArray(item.candidates))) {
      throw new Error('Invalid scenarios response')
    }
    return items
  })
}

export async function getForecastSeries(): Promise<ServiceResult<any[]>> {
  return isDemoMode() ? mockResult(forecastSeries) : emptyResult('no_data')
}

export async function getForecastMeta(): Promise<ServiceResult<any>> {
  return isDemoMode() ? mockResult(forecastMeta) : emptyResult('no_data')
}

export async function getWorkloadHourly(): Promise<ServiceResult<any[]>> {
  if (isDemoMode()) return mockResult(workloadHourly)
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid || !range) return emptyResult('no_resource')
  const query = new URLSearchParams({ resource_uid: uid, period_type: '1h', from: range.from, to: range.to })
  const response = await tryBackend(() => apiJson<Array<{
    PeriodStartUTC: string
    HourOfDay: number
    WorkloadIntensity: number
    SessionsAvg: number
    AvailableMetrics: string[]
  }>>(`/api/v1/analytics/snapshots?${query}`))
  if (response.status !== 'ready' || !response.data) return { ...response, data: null }
  if (!Array.isArray(response.data) || response.data.some(row =>
    !Array.isArray(row.AvailableMetrics) || !Number.isInteger(row.HourOfDay) || row.HourOfDay < 0 || row.HourOfDay > 23
  )) return errorResult('Invalid hourly snapshots response')
  const hours = Array.from({ length: 24 }, (_, hour) => ({
    hour: `${String(hour).padStart(2, '0')}:00`,
    intensitySum: 0, intensityCount: 0, sessionsSum: 0, sessionsCount: 0,
  }))
  for (const row of response.data) {
    const entry = hours[row.HourOfDay]
    if (row.AvailableMetrics.includes('workload_intensity') && Number.isFinite(row.WorkloadIntensity)) {
      entry.intensitySum += row.WorkloadIntensity
      entry.intensityCount++
    }
    if (row.AvailableMetrics.includes('sessions_avg') && Number.isFinite(row.SessionsAvg)) {
      entry.sessionsSum += row.SessionsAvg
      entry.sessionsCount++
    }
  }
  return backendResult(hours.filter(hour => hour.intensityCount || hour.sessionsCount).map(hour => ({
    hour: hour.hour,
    intensity: hour.intensityCount ? hour.intensitySum / hour.intensityCount : null,
    concurrency: hour.sessionsCount ? hour.sessionsSum / hour.sessionsCount : null,
    reads: null,
    writes: null,
  })))
}

export async function getWorkloadClassification(): Promise<ServiceResult<WorkloadClassificationItem>> {
  if (isDemoMode()) return mockResult(workloadClassification)
  const result = await getWorkloadPattern()
  if (result.status !== 'ready' || !result.data) return { ...result, data: null }
  return backendResult({
    pattern: result.data.IntensityBand || 'Not available',
    confidence: null,
    evidence: result.data.PeakHours?.length ? [`Horas pico: ${result.data.PeakHours.join(', ')}`] : [],
    metrics: [
      { label: 'Peaks per day', value: String(result.data.PeakFrequency) },
      { label: 'Average peak duration', value: `${result.data.AvgPeakDuration} h` },
    ],
  })
}

export async function getWorkloadPatterns(): Promise<ServiceResult<string[]>> {
  return isDemoMode() ? mockResult([...workloadPatterns]) : emptyResult('no_data')
}

export async function getTopQueries(): Promise<ServiceResult<any[]>> {
  if (isDemoMode()) return mockResult(topQueries)
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid || !range) return emptyResult('no_resource')
  const query = new URLSearchParams({ resource_uid: uid, from: range.from, to: range.to, limit: '10' })
  const response = await tryBackend(() => apiJson<Array<{
    query_id: number; text: string | null; execs: number
    avg_cpu_ms: number | null; avg_dur_ms: number | null; reads: number | null
  }>>(`/api/v1/data/top-operations?${query}`))
  if (response.status !== 'ready' || !response.data) return { ...response, data: null }
  if (!Array.isArray(response.data)) return errorResult('Invalid operations response')
  return backendResult(response.data.map(row => ({
    id: String(row.query_id), text: row.text ?? 'Text not available',
    execs: row.execs, avgCpuMs: row.avg_cpu_ms,
    avgDurMs: row.avg_dur_ms, reads: row.reads,
  })))
}

export async function getWaitCategories(): Promise<ServiceResult<any[]>> {
  if (isDemoMode()) return mockResult(waitCategories)
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid || !range) return emptyResult('no_resource')
  const query = new URLSearchParams({ resource_uid: uid, from: range.from, to: range.to })
  const response = await tryBackend(() => apiJson<Array<{ name: string; total_wait_ms: number }>>(`/api/v1/data/wait-categories?${query}`))
  if (response.status !== 'ready' || !response.data) return { ...response, data: null }
  if (!Array.isArray(response.data) || response.data.some(row => !Number.isFinite(row.total_wait_ms) || row.total_wait_ms < 0)) {
    return errorResult('Invalid wait categories response')
  }
  const total = response.data.reduce((sum, row) => sum + row.total_wait_ms, 0)
  if (!total) return emptyResult('no_data')
  const colors = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)', 'var(--chart-4)', 'var(--chart-5)']
  return backendResult(response.data.map((row, index) => ({
    name: row.name, pct: +(row.total_wait_ms / total * 100).toFixed(1),
    color: colors[index % colors.length],
  })))
}

export async function getHistory(): Promise<ServiceResult<HistoryItem[]>> {
  if (isDemoMode()) return mockResult(history)
  const recsRes = await getRecommendations()
  if (recsRes.status === 'ready' && recsRes.data) {
    return backendResult(recsRes.data.map(r => ({
      id: r.id, date: r.generated, resource: r.resourceUid ?? '—', current: r.from, proposed: r.to,
      savings: `${r.savingsPct}%`, risk: r.risk, confidence: r.confidence,
      status: ({ accepted: 'Accepted', rejected: 'Rejected', dismissed: 'Archived' } as Record<string, HistoryStatus>)[r.status?.toLowerCase() ?? ''] ?? 'New',
      version: r.modelVersion, analysisPeriod: r.analysisPeriod,
    })))
  }
  return { ...recsRes, data: null }
}

export async function getDataQuality(): Promise<ServiceResult<DataQuality>> {
  if (isDemoMode()) return mockResult(dataQuality)
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid || !range) return emptyResult('no_resource')
  const query = new URLSearchParams({ resource_uid: uid, metric_code: 'cpu_percent', from: range.from, to: range.to })
  const coverage = await tryBackend(() => apiJson<{ coverage_ratio: number; gaps: number; state: string }>(`/api/v1/data/coverage?${query}`))
  if (coverage.status !== 'ready' || !coverage.data) return { ...coverage, data: null }
  if (!Number.isFinite(coverage.data.coverage_ratio)) return errorResult('Invalid coverage response')
  const ratio = coverage.data.coverage_ratio
  return backendResult({
    globalConfidence: coverage.data.state === 'sufficient' ? 'Sufficient' : 'Insufficient',
    items: [
      { label: 'CPU coverage', value: `${(ratio * 100).toFixed(1)}%`, info: '', tone: ratio >= 0.8 ? 'good' : 'warn' },
      { label: 'Intervals without readings', value: String(coverage.data.gaps), info: '', tone: coverage.data.gaps ? 'warn' : 'good' },
    ],
  })
}

// ─────────────────────────────────────────────────────────────────────────────
// Reporting (generar, consultar, cancelar y descargar reportes)
// ─────────────────────────────────────────────────────────────────────────────

export interface ReportRequest {
  resource_uid: string
  from: string
  to: string
  format: 'json' | 'csv' | 'html'
  recommendation_uids?: string[]
  include_audit?: boolean
  include_trace?: boolean
  include_evidence?: boolean
  include_risks?: boolean
  include_metrics?: boolean
  title?: string
  parameters?: Record<string, any>
}

export interface ReportResponse {
  report: {
    uid: string
    status: string
    format: 'json' | 'csv' | 'html'
    resource_uid?: string
    created_at: string
    completed_at?: string
  }
  artifact: { uid: string; storage_uri: string; mime_type: string; file_size_bytes?: number }
}

export interface ReportJob {
  job_uid: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled'
  report_uid: string | null
  artifact_uid?: string | null
  created_at: string
  started_at?: string | null
  completed_at?: string | null
  error_message?: string | null
  report_type?: string
}

export async function generateOptimizationReport(request: ReportRequest): Promise<ServiceResult<ReportResponse | null>> {
  const uid = request.resource_uid || getSelectedResourceUid()
  const range = request.from && request.to ? { from: request.from, to: request.to } : getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()
  return tryBackend(() => apiJson<ReportResponse>('/api/v1/reports/optimization', { method: 'POST', body: JSON.stringify({ resource_uid: uid, from: range.from, to: range.to, format: request.format, recommendation_uids: request.recommendation_uids, include_audit: request.include_audit, include_trace: request.include_trace, include_evidence: request.include_evidence, include_risks: request.include_risks, include_metrics: request.include_metrics, title: request.title, parameters: request.parameters }) }))
}

export async function queueOptimizationReport(request: ReportRequest): Promise<ServiceResult<ReportJob | null>> {
  const uid = request.resource_uid || getSelectedResourceUid()
  const range = request.from && request.to ? { from: request.from, to: request.to } : getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  if (!range) return missingRange()
  return tryBackend(() => apiJson<ReportJob>('/api/v1/reports', { method: 'POST', body: JSON.stringify({ resource_uid: uid, from: range.from, to: range.to, format: request.format, recommendation_uids: request.recommendation_uids, include_audit: request.include_audit, include_trace: request.include_trace, include_evidence: request.include_evidence, include_risks: request.include_risks, include_metrics: request.include_metrics, title: request.title, parameters: request.parameters }) }))
}

export async function getReport(uid: string): Promise<ServiceResult<ReportResponse | null>> {
  return tryBackend(() => apiJson<ReportResponse>(`/api/v1/reports/${uid}`))
}

export async function getReportStatus(uid: string): Promise<ServiceResult<ReportJob | null>> {
  return tryBackend(() => apiJson<ReportJob>(`/api/v1/reports/${uid}/status`))
}

export async function cancelReport(uid: string): Promise<ServiceResult<{ job_uid: string; status: string } | null>> {
  return tryBackend(() => apiJson<{ job_uid: string; status: string }>(`/api/v1/reports/${uid}/cancel`, { method: 'POST', body: JSON.stringify({}) }))
}

export async function downloadReportArtifact(uid: string): Promise<ServiceResult<Blob | null>> {
  const reportRes = await getReport(uid)
  if (reportRes.status !== 'ready' || !reportRes.data) {
    return reportRes.status === 'error'
      ? errorResult(reportRes.error ?? 'Could not retrieve the report')
      : emptyResult('no_data')
  }
  if (!reportRes.data.artifact?.storage_uri) {
    return emptyResult('no_data')
  }
  return tryBackend(async () => {
    const res = await apiFetch(`/api/v1/reports/${uid}/artifact`)
    if (!res.ok) throw new Error(res.statusText || `Error ${res.status}`)
    return await res.blob()
  })
}

// ─────────────────────────────────────────────────────────────────────────────
// Session state: resource_uid, time_range
// ─────────────────────────────────────────────────────────────────────────────

const SESSION_RESOURCE_UID = 'cco_resource_uid'
const SESSION_TIME_RANGE = 'cco_time_range'

export function getSelectedResourceUid(): string | null {
  if (typeof window === 'undefined') return null
  return sessionStorage.getItem(SESSION_RESOURCE_UID) ?? null
}

export function setSelectedResourceUid(uid: string | null): void {
  if (typeof window === 'undefined') return
  if (uid) {
    sessionStorage.setItem(SESSION_RESOURCE_UID, uid)
  } else {
    sessionStorage.removeItem(SESSION_RESOURCE_UID)
  }
}

export function getTimeRange(): { from: string; to: string } | null {
  if (typeof window === 'undefined') return null
  const raw = sessionStorage.getItem(SESSION_TIME_RANGE)
  if (!raw) return null
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

export function setTimeRange(from: string, to: string): void {
  if (typeof window === 'undefined') return
  sessionStorage.setItem(SESSION_TIME_RANGE, JSON.stringify({ from, to }))
}
