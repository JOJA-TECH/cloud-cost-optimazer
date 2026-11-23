import { apiJson } from './api'
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
// Helper: llama al backend y si falla devuelve null (el llamador decide fallback)
// ─────────────────────────────────────────────────────────────────────────────

async function tryBackend<T>(fn: () => Promise<T>): Promise<T | null> {
  try {
    return await fn()
  } catch {
    return null
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Contexto de sesión: recurso seleccionado y ventana temporal
// ─────────────────────────────────────────────────────────────────────────────

export interface TimeRange {
  from: string
  to: string
}

const SESSION_RESOURCE_UID = 'cco_selected_resource_uid'
const SESSION_TIME_RANGE = 'cco_selected_time_range'

// Ventana por defecto: últimos 30 días
function defaultTimeRange(): TimeRange {
  const to = new Date()
  const from = new Date(to)
  from.setDate(to.getDate() - 30)
  return {
    from: from.toISOString(),
    to: to.toISOString(),
  }
}

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

export function getTimeRange(): TimeRange {
  if (typeof window === 'undefined') return defaultTimeRange()
  const raw = sessionStorage.getItem(SESSION_TIME_RANGE)
  if (!raw) return defaultTimeRange()
  try {
    const parsed = JSON.parse(raw) as TimeRange
    if (parsed.from && parsed.to) return parsed
  } catch {}
  return defaultTimeRange()
}

export function setTimeRange(from: string, to: string): void {
  if (typeof window === 'undefined') return
  sessionStorage.setItem(SESSION_TIME_RANGE, JSON.stringify({ from, to }))
}

// Formatea una fecha a RFC3339 UTC para enviar al backend
export function toRfc3339(date: Date | string): string {
  const d = typeof date === 'string' ? new Date(date) : date
  return d.toISOString()
}

// ─────────────────────────────────────────────────────────────────────────────
// Cloud accounts / resources (funcionan con solo SQLite)
// ─────────────────────────────────────────────────────────────────────────────

export async function getCloudAccounts(): Promise<CloudAccount[]> {
  return tryBackend(() => apiJson<CloudAccount[]>('/api/v1/cloud-accounts')) ?? []
}

export async function getCloudResources(accountId?: string): Promise<CloudResource[]> {
  const accounts = accountId
    ? [{ cloud_account_id: accountId } as CloudAccount]
    : await getCloudAccounts()
  if (accounts.length === 0) return []

  const id = accountId ?? accounts[0].cloud_account_id
  const data = await tryBackend(() =>
    apiJson<{ resources: CloudResource[] }>(`/api/v1/cloud-accounts/${id}/resources`)
  )
  return data?.resources ?? []
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

export async function getDashboardResource(): Promise<DashboardResource | null> {
  try {
    const cloudResources = await getCloudResources()
    if (cloudResources.length > 0) {
      const r = cloudResources[0]
      const resource: DashboardResource = {
        resource_uid: r.resource_uid,
        name: r.database_name,
        server: r.server_name,
        service: 'Azure SQL Database',
        tier: 'General Purpose',
        computeModel: 'Provisioned',
        vCores: 8,
        memoryGb: 40.8,
        region: r.region_code,
        storageUsedGb: 214,
        storageLimitGb: 512,
        redundancy: 'Zone-redundant',
        state: r.lifecycle_status,
        lastSyncMinutes: 8,
        collation: 'SQL_Latin1_General_CP1_CI_AS',
        maxWorkers: 800,
        maxSessions: 2400,
      }
      // Guardar el resource_uid seleccionado en sesión para que las páginas lo usen
      setSelectedResourceUid(r.resource_uid)
      return resource
    }
  } catch {}
  return null
}

// ─────────────────────────────────────────────────────────────────────────────
// Data / metrics (requieren CCO_DATA_DSN → 503 sin él, fallback a mock)
// ─────────────────────────────────────────────────────────────────────────────

export async function getMetrics(
  resourceUid?: string | null,
  metricCode?: string | null,
  from?: string | null,
  to?: string | null
): Promise<MetricsResponse | null> {
  const uid = resourceUid ?? getSelectedResourceUid()
  const range = from && to ? { from, to } : getTimeRange()
  if (!uid || !metricCode) return null
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
): Promise<CostRecord[]> {
  const uid = resourceUid ?? getSelectedResourceUid()
  const range = from && to ? { from, to } : getTimeRange()
  if (!uid) return []
  const data = await tryBackend(() =>
    apiJson<CostRecord[]>(
      `/api/v1/data/costs?resource_uid=${uid}&from=${range.from}&to=${range.to}`
    )
  )
  return data ?? []
}

export async function getConfigSnapshots(
  resourceUid?: string | null
): Promise<ConfigSnapshot[]> {
  const uid = resourceUid ?? getSelectedResourceUid()
  if (!uid) return []
  const data = await tryBackend(() =>
    apiJson<ConfigSnapshot[]>(`/api/v1/data/config-snapshots?resource_uid=${uid}`)
  )
  return data ?? []
}

// ─────────────────────────────────────────────────────────────────────────────
// Recommendations (requieren CCO_DATA_DSN → 503 sin él, fallback a mock)
// ─────────────────────────────────────────────────────────────────────────────

export async function getRecommendations(): Promise<RecommendationItem[]> {
  const data = await tryBackend(() => apiJson<RecommendationItem[]>('/api/v1/recommendations'))
  return data ?? []
}

export async function getRecommendation(uid: string): Promise<RecommendationItem | null> {
  return tryBackend(() => apiJson<RecommendationItem>(`/api/v1/recommendations/${uid}`))
}

// ─────────────────────────────────────────────────────────────────────────────
// Servicios con fallback automático a mock-data
// ─────────────────────────────────────────────────────────────────────────────

export async function getKpis() {
  return kpis
}

export async function getCostDaily(
  resourceUid?: string | null,
  from?: string | null,
  to?: string | null
) {
  const costs = await getCosts(resourceUid, from, to)
  if (costs.length > 0) {
    // Mapear registros de costo al formato de costDaily
    return costs.map((c) => ({
      day: c.usage_date,
      billed: c.pre_tax_cost,
      estimated: +(c.pre_tax_cost * 0.758).toFixed(2),
      reference: +(c.pre_tax_cost * 1.02).toFixed(2),
      cumulative: 0,
    }))
  }
  return costDaily
}

export async function getCostComponents(
  resourceUid?: string | null,
  from?: string | null,
  to?: string | null
) {
  const costs = await getCosts(resourceUid, from, to)
  if (costs.length > 0) {
    const byService = new Map<string, number>()
    for (const c of costs) {
      byService.set(c.service_name, (byService.get(c.service_name) ?? 0) + c.pre_tax_cost)
    }
    const colors = ['var(--chart-1)', 'var(--chart-2)', 'var(--chart-3)', 'var(--chart-4)']
    let i = 0
    return Array.from(byService.entries()).map(([name, current]) => ({
      name,
      current,
      estimated: +(current * 0.758).toFixed(1),
      color: colors[i++ % colors.length],
    }))
  }
  return costComponents
}

export async function getPerfSeries(
  resourceUid?: string | null,
  metricCode: string = 'cpu_percent',
  from?: string | null,
  to?: string | null
) {
  // Intenta obtener métricas reales del backend
  const metrics = await getMetrics(resourceUid, metricCode, from, to)
  if (metrics?.points.length) {
    return metrics.points.map((p, i) => ({
      t: new Date(p.timestamp_utc).toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' }),
      idx: i,
      p50: +p.avg.toFixed(1),
      p95: +Math.min(p.avg * 1.25, 96).toFixed(1),
      p99: +Math.min(p.max, 99).toFixed(1),
      io: +(20 + p.avg * 0.3).toFixed(1),
      memory: +(58 + p.avg * 0.2).toFixed(1),
      sessions: Math.round(180 + p.avg * 4),
      latency: +(4.2 + p.avg * 0.1).toFixed(1),
    }))
  }
  return perfSeries
}

export async function getPerfSummary() {
  return perfSummary
}

export async function getReliabilityEvents() {
  return reliabilityEvents
}

export async function getRecommendationSummary(): Promise<RecommendationItem> {
  const recs = await getRecommendations()
  if (recs.length > 0) return recs[0]
  const single = await getRecommendation('rec-2041')
  if (single) return single
  return recommendation
}

export async function getScenarios(): Promise<ScenarioItem[]> {
  return scenarios
}

export async function getForecastSeries() {
  return forecastSeries
}

export async function getForecastMeta() {
  return forecastMeta
}

export async function getWorkloadHourly() {
  return workloadHourly
}

export async function getWorkloadClassification(): Promise<WorkloadClassificationItem> {
  return workloadClassification
}

export async function getWorkloadPatterns() {
  return workloadPatterns
}

export async function getTopQueries() {
  return topQueries
}

export async function getWaitCategories() {
  return waitCategories
}

export async function getHistory(): Promise<HistoryItem[]> {
  const data = await tryBackend(() => apiJson<HistoryItem[]>('/api/v1/recommendations'))
  if (data?.length) {
    return data.map((r) => ({
      id: r.id,
      date: r.generated,
      resource: getSelectedResourceUid() ?? 'sql-prod-orders-eastus',
      current: r.from,
      proposed: r.to,
      savings: `${r.savingsPct}%`,
      risk: r.risk,
      confidence: r.confidence,
      status: 'New' as HistoryStatus,
      version: r.modelVersion,
    }))
  }
  return history
}

export async function getDataQuality(): Promise<DataQuality> {
  return dataQuality
}
