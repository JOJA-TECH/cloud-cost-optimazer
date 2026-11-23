import { apiJson } from './api'
import { getToken } from './auth'
import {
  kpis,
  resources,
  costDaily,
  costComponents,
  perfSeries,
  perfSummary,
  reliabilityEvents,
  recommendation,
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
  resource as mockResource,
  type RiskLevel,
  type EvidenceTone,
  type HistoryStatus,
} from './mock-data'

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
  title: string
  from: string
  to: string
  savingsPct: number
  savingsAbs: number
  risk: RiskLevel
  confidence: number
  latencyImpact: number
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

export interface WorkloadClassificationItem {
  pattern: string
  confidence: number
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

export function mockResult<T>(data: T, reason: 'backend_unavailable' | 'request_failed' = 'backend_unavailable'): ServiceResult<T> {
  return { data, source: 'mock', status: 'ready', error: reason }
}

export function errorResult<T>(message: string): ServiceResult<T> {
  return { data: null, source: null, status: 'error', error: message }
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper: llama al backend y devuelve ServiceResult
// ─────────────────────────────────────────────────────────────────────────────

async function tryBackend<T>(fn: () => Promise<T>): Promise<ServiceResult<T>> {
  try {
    const data = await fn()
    return backendResult(data)
  } catch (err: any) {
    return { data: null, source: null, status: 'error', error: err?.message ?? 'Error desconocido' }
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Cloud accounts / resources (funcionan con solo SQLite)
// ─────────────────────────────────────────────────────────────────────────────

export async function getCloudAccounts(): Promise<ServiceResult<CloudAccount[]>> {
  return tryBackend(() => apiJson<CloudAccount[]>('/api/v1/cloud-accounts'))
}

export async function getCloudResources(
  resourceUid?: string | null
): Promise<ServiceResult<CloudResource[]>> {
  const uid = resourceUid ?? getSelectedResourceUid()
  if (!uid) return emptyResult('no_resource')
  return tryBackend(() => apiJson<CloudResource[]>(`/api/v1/cloud-accounts/${uid}/resources`))
}

export interface DashboardResource {
  resource_uid: string
  name: string
  server: string
  service: string
  tier: string
  computeModel: string
  vCores: number
  memoryGb: number
  region: string
  storageUsedGb: number
  storageLimitGb: number
  redundancy: string
  state: string
  lastSyncMinutes: number
  collation: string
  maxWorkers: number
  maxSessions: number
}

export async function getDashboardResource(): Promise<ServiceResult<DashboardResource | null>> {
  const cloudResourcesRes = await getCloudResources()
  if (cloudResourcesRes.source !== 'backend' || !cloudResourcesRes.data || cloudResourcesRes.data.length === 0) {
    return emptyResult('no_resource')
  }

  const r = cloudResourcesRes.data[0]
  const resourceUID = r.resource_uid
  setSelectedResourceUid(resourceUid)

  const configRes = await getConfigSnapshots(resourceUid)
  const latestSnapshot = configRes.source === 'backend' && configRes.data ? configRes.data[0] : null

  const resource: DashboardResource = {
    resource_uid: resourceUid,
    name: r.database_name,
    server: r.server_name,
    service: 'Azure SQL Database',
    tier: latestSnapshot?.tier ?? null,
    computeModel: latestSnapshot?.compute_model ?? null,
    vCores: latestSnapshot?.capacity_vcores ?? null,
    memoryGb: latestSnapshot?.capacity_vcores ? latestSnapshot.capacity_vcores * 5.1 : null,
    region: r.region_code,
    storageUsedGb: latestSnapshot?.max_size_bytes ? latestSnapshot.max_size_bytes / (1024 * 1024 * 1024) : null,
    storageLimitGb: latestSnapshot?.max_size_bytes ? latestSnapshot.max_size_bytes / (1024 * 1024 * 1024) : null,
    redundancy: null,
    state: r.lifecycle_status,
    lastSyncMinutes: 8,
    collation: null,
    maxWorkers: null,
    maxSessions: null,
  }

  return { data: resource, source: 'backend', error: null }
}

// ─────────────────────────────────────────────────────────────────────────────
// Data / metrics (requieren CCO_DATA_DSN → 503 sin él, fallback a mock)
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
// Analytics (requieren CCO_DATA_DSN → 503 sin él, fallback a mock)
// ─────────────────────────────────────────────────────────────────────────────

export interface PercentileResult {
  period_start: string
  period_end: string
  p50: number
  p95: number
  p99: number
  avg: number
  min: number
  max: number
}

export interface TrendResult {
  timestamp_utc: string
  cpu_slope: number
  storage_growth: number
}

export interface WorkloadPattern {
  pattern: string
  confidence: number
  evidence: string[]
  intensity_band: string
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
  resource_uid: string
  current_cost: number
  projected_cost: number
  savings_pct: number
  savings_abs: number
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
  resource_uid: string
  target_metric: string
  horizon_steps: number
  step_unit: string
  points: Array<{
    step: number
    timestamp_utc: string
    forecast: number
    lower: number
    upper: number
  }>
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
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
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
  return tryBackend(() =>
    apiJson<T>(`${path}?resource_uid=${uid}&from=${range.from}&to=${range.to}`)
  )
}

export async function getPercentiles(
  metricCode: string = 'cpu_percent',
  periodType: string = '1h'
): Promise<ServiceResult<PercentileResult[]>> {
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
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
  return tryBackend(() =>
    apiJson<DESComponents>(`/api/v1/analytics/des?resource_uid=${uid}&period_type=${periodType}&period_start=${range.from}`)
  )
}

export async function getAnalyticsForecasts(
  targetMetric: string = 'cpu_percent'
): Promise<ServiceResult<ForecastResponse | null>> {
  const uid = getSelectedResourceUid()
  const range = getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  return tryBackend(() =>
    apiJson<ForecastResponse>(`/api/v1/analytics/forecasts?resource_uid=${uid}&target_metric=${targetMetric}&from=${range.from}&to=${range.to}`)
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

export async function getRecommendations(): Promise<ServiceResult<RecommendationItem[]>> {
  return tryBackend(() => apiJson<RecommendationItem[]>('/api/v1/recommendations'))
}

export async function getRecommendation(uid: string): Promise<ServiceResult<RecommendationItem | null>> {
  return tryBackend(() => apiJson<RecommendationItem>(`/api/v1/recommendations/${uid}`))
}

export interface RecommendationEvidence {
  recommendation_uid: string
  evidence_items: Array<{
    label: string
    value: string
    tone: 'good' | 'warn' | 'bad'
  }>
  constraints: Array<{ label: string; pass: boolean }>
  limitations: string[]
  rationale: string
}

export interface RecommendationTrace {
  recommendation_uid: string
  decision_traces: DecisionTrace[]
  events: AuditEvent[]
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
  return tryBackend(() => apiJson<RecommendationEvidence>(`/api/v1/recommendations/${uid}/evidence`))
}

export async function evaluateRecommendations(
  resourceUid: string,
  policyUid: string | null,
  horizonDays: number,
  from: string,
  to: string,
  currency: string = 'USD'
): Promise<ServiceResult<any>> {
  const uid = resourceUid || getSelectedResourceUid()
  const range = from && to ? { from, to } : getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
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
// Servicios con fallback automático a mock-data
// ─────────────────────────────────────────────────────────────────────────────

export async function getKpis(): Promise<ServiceResult<any>> {
  return { data: kpis, source: 'mock', status: 'ready', error: null }
}

export async function getCostDaily(
  resourceUid?: string | null,
  from?: string | null,
  to?: string | null
): Promise<ServiceResult<any[]>> {
  const costsRes = await getCosts(resourceUid, from, to)
  if (costsRes.source === 'backend' && costsRes.data && costsRes.data.length > 0) {
    return {
      data: costsRes.data.map((c) => ({
        day: c.usage_date,
        billed: c.pre_tax_cost,
        estimated: +(c.pre_tax_cost * 0.758).toFixed(2),
        reference: +(c.pre_tax_cost * 1.02).toFixed(2),
        cumulative: 0,
      })),
      source: 'backend',
      error: null,
    }
  }
  return { data: costDaily, source: 'mock', error: costsRes.error }
}

export async function getCostComponents(
  resourceUid?: string | null,
  from?: string | null,
  to?: string | null
): Promise<ServiceResult<any[]>> {
  const costsRes = await getCosts(resourceUid, from, to)
  if (costsRes.source === 'backend' && costsRes.data && costsRes.data.length > 0) {
    const byService = new Map<string, number>()
    for (const c of costsRes.data) {
      byService.set(c.service_name, (byService.get(c.service_name) ?? 0) + c.pre_tax_cost)
    }
    const colors = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)', 'var(--chart-4)']
    let i = 0
    return {
      data: Array.from(byService.entries()).map(([name, current]) => ({
        name,
        current,
        estimated: +(current * 0.758).toFixed(1),
        color: colors[i++ % colors.length],
      })),
      source: 'backend',
      error: null,
    }
  }
  return { data: costComponents, source: 'mock', error: costsRes.error }
}

export async function getPerfSeries(
  resourceUid?: string | null,
  metricCode: string = 'cpu_percent',
  from?: string | null,
  to?: string | null
): Promise<ServiceResult<any[]>> {
  const uid = resourceUid ?? getSelectedResourceUid()
  const range = from && to ? { from, to } : getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }

  const [cpuRes, ioRes, memoryRes, sessionsRes, latencyRes] = await Promise.all([
    apiJson<MetricsResponse>(`/api/v1/data/metrics?resource_uid=${uid}&metric_code=cpu_percent&from=${range.from}&to=${range.to}`),
    apiJson<MetricsResponse>(`/api/v1/data/metrics?resource_uid=${uid}&metric_code=io_percent&from=${range.from}&to=${range.to}`),
    apiJson<MetricsResponse>(`/api/v1/data/metrics?resource_uid=${uid}&metric_code=memory_percent&from=${range.from}&to=${range.to}`),
    apiJson<MetricsResponse>(`/api/v1/data/metrics?resource_uid=${uid}&metric_code=sessions&from=${range.from}&to=${range.to}`),
    apiJson<MetricsResponse>(`/api/v1/data/metrics?resource_uid=${uid}&metric_code=latency_ms&from=${range.from}&to=${range.to}`),
  ])

  const cpuPoints = cpuRes.source === 'backend' && cpuRes.data ? cpuRes.data.points : []
  const ioPoints = ioRes.source === 'backend' && ioRes.data ? ioRes.data.points : []
  const memoryPoints = memoryRes.source === 'backend' && memoryRes.data ? memoryRes.data.points : []
  const sessionsPoints = sessionsRes.source === 'backend' && sessionsRes.data ? sessionsRes.data.points : []
  const latencyPoints = latencyRes.source === 'backend' && latencyRes.data ? latencyRes.data.points : []

  const pointMap = new Map<string, any>()

  cpuPoints.forEach((p, i) => {
    pointMap.set(p.timestamp_utc, {
      t: new Date(p.timestamp_utc).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }),
      idx: i,
      p50: +p.avg.toFixed(1),
      p95: +Math.min(p.avg * 1.25, 96).toFixed(1),
      p99: +Math.min(p.max, 99).toFixed(1),
      io: null,
      memory: null,
      sessions: null,
      latency: null,
    })
  })

  ioPoints.forEach((p) => {
    const existing = pointMap.get(p.timestamp_utc)
    if (existing) existing.io = +p.avg.toFixed(1)
    else pointMap.set(p.timestamp_utc, { t: new Date(p.timestamp_utc).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }), idx: 0, p50: null, p95: null, p99: null, io: +p.avg.toFixed(1), memory: null, sessions: null, latency: null })
  })

  memoryPoints.forEach((p) => {
    const existing = pointMap.get(p.timestamp_utc)
    if (existing) existing.memory = +p.avg.toFixed(1)
    else pointMap.set(p.timestamp_utc, { t: new Date(p.timestamp_utc).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }), idx: 0, p50: null, p95: null, p99: null, io: null, memory: +p.avg.toFixed(1), sessions: null, latency: null })
  })

  sessionsPoints.forEach((p) => {
    const existing = pointMap.get(p.timestamp_utc)
    if (existing) existing.sessions = Math.round(p.avg)
    else pointMap.set(p.timestamp_utc, { t: new Date(p.timestamp_utc).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }), idx: 0, p50: null, p95: null, p99: null, io: null, memory: null, sessions: Math.round(p.avg), latency: null })
  })

  latencyPoints.forEach((p) => {
    const existing = pointMap.get(p.timestamp_utc)
    if (existing) existing.latency = +p.avg.toFixed(1)
    else pointMap.set(p.timestamp_utc, { t: new Date(p.timestamp_utc).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }), idx: 0, p50: null, p95: null, p99: null, io: null, memory: null, sessions: null, latency: +p.avg.toFixed(1) })
  })

  return { data: Array.from(pointMap.values()), source: 'backend', error: null }
}

export async function getPerfSummary(): Promise<ServiceResult<any[]>> {
  return { data: perfSummary, source: 'mock', status: 'ready', error: null }
}

export async function getReliabilityEvents(): Promise<ServiceResult<any[]>> {
  return { data: reliabilityEvents, source: 'mock', status: 'ready', error: null }
}

export async function getRecommendationSummary(): Promise<ServiceResult<RecommendationItem>> {
  const uid = getSelectedResourceUid()
  if (uid) {
    const single = await getRecommendation(uid)
    if (single.source === 'backend' && single.data) return single
  }
  const recs = await getRecommendations()
  if (recs.source === 'backend' && recs.data && recs.data.length > 0) return { data: recs.data[0], source: 'backend', error: null }
  return { data: recommendation, source: 'mock', error: recs.error }
}

export async function getScenarios(): Promise<ServiceResult<ScenarioItem[]>> {
  return { data: scenarios, source: 'mock', status: 'ready', error: null }
}

export async function getForecastSeries(): Promise<ServiceResult<any[]>> {
  return { data: forecastSeries, source: 'mock', status: 'ready', error: null }
}

export async function getForecastMeta(): Promise<ServiceResult<any>> {
  return { data: forecastMeta, source: 'mock', status: 'ready', error: null }
}

export async function getWorkloadHourly(): Promise<ServiceResult<any[]>> {
  return { data: workloadHourly, source: 'mock', status: 'ready', error: null }
}

export async function getWorkloadClassification(): Promise<ServiceResult<WorkloadClassificationItem>> {
  return { data: workloadClassification, source: 'mock', status: 'ready', error: null }
}

export async function getWorkloadPatterns(): Promise<ServiceResult<string[]>> {
  return { data: workloadPatterns, source: 'mock', status: 'ready', error: null }
}

export async function getTopQueries(): Promise<ServiceResult<any[]>> {
  return { data: topQueries, source: 'mock', status: 'ready', error: null }
}

export async function getWaitCategories(): Promise<ServiceResult<any[]>> {
  return { data: waitCategories, source: 'mock', status: 'ready', error: null }
}

export async function getHistory(): Promise<ServiceResult<HistoryItem[]>> {
  const recsRes = await getRecommendations()
  if (recsRes.source === 'backend' && recsRes.data && recsRes.data.length > 0) {
    return { data: recsRes.data.map(r => ({ id: r.id, date: r.generated, resource: getSelectedResourceUid() ?? 'sql-prod-orders-eastus', current: r.from, proposed: r.to, savings: `${r.savingsPct}%`, risk: r.risk, confidence: r.confidence, status: 'New' as HistoryStatus, version: r.modelVersion })), source: 'backend', error: null }
  }
  return { data: history, source: 'mock', error: recsRes.error }
}

export async function getDataQuality(): Promise<ServiceResult<DataQuality>> {
  return { data: dataQuality, source: 'mock', status: 'ready', error: null }
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
  report_uid: string
  status: string
  format: string
  resource_uid: string
  from: string
  to: string
  created_at: number
  requested_by: string
  project_id: string
  artifact?: { uid: string; storage_uri: string; mime_type: string; file_size_bytes?: number }
}

export interface ReportJob {
  job_uid: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  report_uid: string | null
  artifact_uid: string | null
  created_at: number
  started_at: number | null
  completed_at: number | null
  error_message: string | null
  report_type: string
}

export async function generateOptimizationReport(request: ReportRequest): Promise<ServiceResult<ReportResponse | null>> {
  const uid = request.resource_uid || getSelectedResourceUid()
  const range = request.from && request.to ? { from: request.from, to: request.to } : getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
  return tryBackend(() => apiJson<ReportResponse>('/api/v1/reports/optimization', { method: 'POST', body: JSON.stringify({ resource_uid: uid, from: range.from, to: range.to, format: request.format, recommendation_uids: request.recommendation_uids, include_audit: request.include_audit, include_trace: request.include_trace, include_evidence: request.include_evidence, include_risks: request.include_risks, include_metrics: request.include_metrics, title: request.title, parameters: request.parameters }) }))
}

export async function queueOptimizationReport(request: ReportRequest): Promise<ServiceResult<ReportJob | null>> {
  const uid = request.resource_uid || getSelectedResourceUid()
  const range = request.from && request.to ? { from: request.from, to: request.to } : getTimeRange()
  if (!uid) return { data: null, source: null, status: 'empty', error: 'resource_uid missing' }
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
  if (reportRes.source !== 'backend' || !reportRes.data?.artifact?.storage_uri) {
    return { data: null, source: 'mock', status: 'empty', error: 'no artifact available' }
  }
  return tryBackend(async () => {
    const res = await fetch(`/api/v1/reports/${uid}/artifact`, { method: 'GET', headers: { Authorization: `Bearer ${getToken()}` } })
    if (!res.ok) return { data: null, source: 'mock', status: 'error', error: res.statusText }
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