// Demonstration / sample data for the Cloud Database Cost Optimizer prototype.
// All values are illustrative and do not represent a real Azure subscription.

export const resource = {
  name: 'sql-prod-orders-eastus',
  server: 'cdco-prod.database.windows.net',
  service: 'Azure SQL Database',
  tier: 'General Purpose',
  computeModel: 'Provisioned',
  vCores: 8,
  memoryGb: 40.8,
  region: 'East US',
  storageUsedGb: 214,
  storageLimitGb: 512,
  redundancy: 'Zone-redundant',
  state: 'Online',
  lastSyncMinutes: 8,
  collation: 'SQL_Latin1_General_CP1_CI_AS',
  maxWorkers: 800,
  maxSessions: 2400,
}

export const resources = [
  { id: 'orders', name: 'sql-prod-orders-eastus', region: 'East US', status: 'Healthy' },
  { id: 'billing', name: 'sql-prod-billing-eastus', region: 'East US', status: 'Warning' },
  { id: 'analytics', name: 'sql-prod-analytics-westus', region: 'West US', status: 'Healthy' },
  { id: 'identity', name: 'sql-prod-identity-eastus', region: 'East US', status: 'Insufficient data' },
]

export const kpis = {
  status: 'Healthy',
  currentCost: 486.2,
  currentCostDelta: 4.8,
  estimatedCost: 368.4,
  estimatedSavings: 117.8,
  savingsPct: 24.2,
  risk: 'Low' as RiskLevel,
  confidence: 91,
}

export type RiskLevel = 'Low' | 'Medium' | 'High' | 'None'

export const recommendation = {
  id: 'rec-2041',
  title: 'Recommendation available',
  from: '8 vCores',
  to: '4 vCores',
  savingsPct: 24.2,
  savingsAbs: 117.8,
  risk: 'Low' as RiskLevel,
  confidence: 91,
  latencyImpact: 2.1,
  analysisPeriod: 'Last 30 days',
  generated: 'Today, 14:32',
  modelVersion: 'rightsizer v1.4.2 · ruleset 2024.11',
  evidence: [
    { label: 'CPU p95', value: '28%', tone: 'good' as EvidenceTone },
    { label: 'CPU p99', value: '41%', tone: 'good' as EvidenceTone },
    { label: 'I/O p95', value: 'Below threshold', tone: 'good' as EvidenceTone },
    { label: 'Workload trend', value: 'Stable', tone: 'good' as EvidenceTone },
    { label: 'Headroom', value: 'Sufficient', tone: 'good' as EvidenceTone },
    { label: 'SLA (30d)', value: '99.97%', tone: 'good' as EvidenceTone },
  ],
  constraints: [
    { label: 'p95 latency within threshold', pass: true },
    { label: 'No SLA violations in analysis window', pass: true },
    { label: 'Sufficient CPU headroom at projected peak', pass: true },
    { label: 'I/O within provisioned limits', pass: true },
  ],
  limitations: [
    'Historical coverage is 96% of the analysis window.',
    'Forecast uncertainty increases beyond 14 days.',
  ],
  rationale:
    'Although average CPU utilization is low, the workload shows stable demand, moderate peak frequency, and sufficient headroom. The simulated 4-vCore configuration maintains the defined performance constraints while reducing estimated monthly cost. This is a decision-support recommendation and does not modify the production resource.',
}

export type EvidenceTone = 'good' | 'warn' | 'bad'

// ---- Cost series (30 days) -------------------------------------------------
function day(i: number) {
  const d = new Date(2026, 7, 15)
  d.setDate(d.getDate() + i)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

export const costDaily = Array.from({ length: 30 }, (_, i) => {
  const weekend = [0, 6].includes(new Date(2026, 7, 15 + i).getDay())
  const base = 16.2 + Math.sin(i / 3) * 0.9 + (weekend ? -3.1 : 0.6)
  const billed = +(base + (i > 20 ? 0.5 : 0)).toFixed(2)
  return {
    day: day(i),
    billed,
    estimated: +(billed * 0.758).toFixed(2),
    reference: +(billed * 1.02).toFixed(2),
    cumulative: 0,
  }
}).map((d, i, arr) => {
  const cum = arr.slice(0, i + 1).reduce((s, x) => s + x.billed, 0)
  return { ...d, cumulative: +cum.toFixed(2) }
})

export const costComponents = [
  { name: 'Compute (vCores)', current: 372.4, estimated: 254.9, color: 'var(--chart-1)' },
  { name: 'Storage', current: 68.2, estimated: 68.2, color: 'var(--chart-2)' },
  { name: 'Backup / PITR', current: 31.6, estimated: 31.6, color: 'var(--chart-3)' },
  { name: 'I/O & egress', current: 14.0, estimated: 13.7, color: 'var(--chart-4)' },
]

export const scenarios = [
  {
    id: 'current',
    label: 'Current',
    capacity: '8 vCores',
    vCores: 8,
    cost: 486,
    savings: null as number | null,
    cpuP99: 41,
    latency: 'baseline',
    latencyPct: 0,
    risk: 'None' as RiskLevel,
    sla: 'Pass' as 'Pass' | 'Warning' | 'Fail',
    confidence: null as number | null,
    feasible: true,
    recommended: false,
  },
  {
    id: 'a',
    label: 'Candidate A',
    capacity: '4 vCores',
    vCores: 4,
    cost: 368,
    savings: 24.2,
    cpuP99: 58,
    latency: '+2.1%',
    latencyPct: 2.1,
    risk: 'Low' as RiskLevel,
    sla: 'Pass' as const,
    confidence: 91,
    feasible: true,
    recommended: true,
  },
  {
    id: 'b',
    label: 'Candidate B',
    capacity: '2 vCores',
    vCores: 2,
    cost: 276,
    savings: 43.2,
    cpuP99: 81,
    latency: '+9.7%',
    latencyPct: 9.7,
    risk: 'Medium' as RiskLevel,
    sla: 'Warning' as const,
    confidence: 74,
    feasible: true,
    recommended: false,
  },
]

// ---- Performance time series (72 hourly points) ---------------------------
export const perfSeries = Array.from({ length: 72 }, (_, i) => {
  const h = i % 24
  const businessCurve = Math.max(0, Math.sin(((h - 6) / 13) * Math.PI))
  const p50 = 12 + businessCurve * 14 + (h > 9 && h < 18 ? 3 : 0)
  const p95 = p50 + 9 + businessCurve * 6
  const p99 = p95 + 6 + businessCurve * 5
  return {
    t: `${String(h).padStart(2, '0')}:00`,
    idx: i,
    p50: +p50.toFixed(1),
    p95: +Math.min(p95, 96).toFixed(1),
    p99: +Math.min(p99, 99).toFixed(1),
    io: +(20 + businessCurve * 28 + 4).toFixed(1),
    memory: +(58 + businessCurve * 10).toFixed(1),
    sessions: Math.round(180 + businessCurve * 640),
    latency: +(4.2 + businessCurve * 3.6).toFixed(1),
  }
})

export const perfSummary = [
  { label: 'CPU p50', value: '18%', tone: 'good' as EvidenceTone },
  { label: 'CPU p95', value: '28%', tone: 'good' as EvidenceTone },
  { label: 'CPU p99', value: '41%', tone: 'good' as EvidenceTone },
  { label: 'I/O p95', value: '52%', tone: 'good' as EvidenceTone },
  { label: 'Memory', value: '68%', tone: 'warn' as EvidenceTone },
  { label: 'Peak sessions', value: '820 / 2400', tone: 'good' as EvidenceTone },
  { label: 'Avg latency', value: '5.1 ms', tone: 'good' as EvidenceTone },
  { label: 'SLA compliance', value: '99.97%', tone: 'good' as EvidenceTone },
]

export const reliabilityEvents = [
  { label: 'Deadlocks (30d)', value: 3, tone: 'good' as EvidenceTone },
  { label: 'Timeouts (30d)', value: 11, tone: 'warn' as EvidenceTone },
  { label: 'Errors (30d)', value: 0, tone: 'good' as EvidenceTone },
  { label: 'Throttling events', value: 0, tone: 'good' as EvidenceTone },
]

// ---- Workload -------------------------------------------------------------
export const workloadHourly = Array.from({ length: 24 }, (_, h) => {
  const businessCurve = Math.max(0, Math.sin(((h - 6) / 13) * Math.PI))
  return {
    hour: `${String(h).padStart(2, '0')}`,
    intensity: +(14 + businessCurve * 62).toFixed(1),
    concurrency: Math.round(60 + businessCurve * 420),
    reads: +(40 + businessCurve * 45).toFixed(0),
    writes: +(60 - businessCurve * 45).toFixed(0),
  }
})

export const workloadClassification = {
  pattern: 'Periodic',
  confidence: 89,
  evidence: [
    'Predictable daily peaks between 09:00–17:00',
    'Stable baseline outside business hours',
    'Low variability outside peak windows',
    'No sustained upward drift across 30 days',
  ],
  metrics: [
    { label: 'Peak frequency', value: 'Moderate' },
    { label: 'Peak duration', value: '~6.5 h/day' },
    { label: 'Variability (CV)', value: '0.34' },
    { label: 'Growth trend', value: '+1.2% / 30d' },
    { label: 'Idle windows', value: '22:00–06:00' },
    { label: 'Read / write mix', value: '58% / 42%' },
  ],
}

export const workloadPatterns = [
  'Stable',
  'Periodic',
  'Bursty',
  'Increasing',
  'CPU Intensive',
  'I/O Intensive',
  'Mixed',
] as const

// ---- Forecasting ----------------------------------------------------------
export const forecastSeries = Array.from({ length: 44 }, (_, i) => {
  const observed = i < 30
  const trend = 26 + i * 0.28 + Math.sin(i / 2.4) * 4.5
  if (observed) {
    return {
      t: `D${i - 29}`,
      day: i - 29,
      observed: +trend.toFixed(1),
      forecast: null as number | null,
      lower: null as number | null,
      upper: null as number | null,
    }
  }
  const f = 26 + i * 0.34 + Math.sin(i / 2.4) * 3
  const spread = 2 + (i - 29) * 0.9
  return {
    t: `+${i - 29}`,
    day: i - 29,
    observed: null as number | null,
    forecast: +f.toFixed(1),
    lower: +(f - spread).toFixed(1),
    upper: +(f + spread).toFixed(1),
  }
})

export const forecastMeta = {
  horizonDays: 14,
  trend: '+3.1% projected over 14 days',
  saturation: 'No CPU saturation projected within horizon',
  errors: [
    { label: 'MAE', value: '3.8%' },
    { label: 'RMSE', value: '5.1%' },
    { label: 'MAPE', value: '6.4%' },
  ],
}

// ---- Diagnostics ----------------------------------------------------------
export const topQueries = [
  { id: 'Q1', text: 'SELECT ... FROM orders o JOIN order_items ...', execs: 184203, avgCpuMs: 42, avgDurMs: 61, reads: 1240 },
  { id: 'Q2', text: 'UPDATE inventory SET qty = qty - @n WHERE ...', execs: 96541, avgCpuMs: 18, avgDurMs: 24, reads: 88 },
  { id: 'Q3', text: 'SELECT TOP 100 ... FROM audit_log WHERE ...', execs: 12044, avgCpuMs: 210, avgDurMs: 512, reads: 41200 },
  { id: 'Q4', text: 'EXEC sp_refresh_customer_summary', execs: 2880, avgCpuMs: 640, avgDurMs: 1180, reads: 96400 },
  { id: 'Q5', text: 'INSERT INTO events (…) VALUES (…)', execs: 421900, avgCpuMs: 6, avgDurMs: 9, reads: 12 },
]

export const waitCategories = [
  { name: 'CPU', pct: 34, color: 'var(--chart-1)' },
  { name: 'PAGEIOLATCH', pct: 22, color: 'var(--chart-2)' },
  { name: 'LCK_M_X (locks)', pct: 16, color: 'var(--chart-3)' },
  { name: 'WRITELOG', pct: 14, color: 'var(--chart-4)' },
  { name: 'Network I/O', pct: 8, color: 'var(--chart-5)' },
  { name: 'Other', pct: 6, color: '#cbd5e1' },
]

// ---- Data quality ---------------------------------------------------------
export const dataQuality = {
  globalConfidence: 'High',
  items: [
    { label: 'Last collected', value: '8 min ago', info: 'Most recent successful metric collection.', tone: 'good' as EvidenceTone },
    { label: 'Historical duration', value: '30 days', info: 'Length of observed history feeding the analysis.', tone: 'good' as EvidenceTone },
    { label: 'Sampling interval', value: '5 min', info: 'Resolution of collected telemetry.', tone: 'good' as EvidenceTone },
    { label: 'Coverage', value: '96%', info: 'Share of expected samples actually collected.', tone: 'good' as EvidenceTone },
    { label: 'Missing metrics', value: 'None', info: 'Required signals with no data in the window.', tone: 'good' as EvidenceTone },
    { label: 'Cost source', value: 'Billed', info: 'Cost derived from billed usage, not list price.', tone: 'good' as EvidenceTone },
    { label: 'Experimental validation', value: 'Available', info: 'A controlled scenario replay validated the estimate.', tone: 'good' as EvidenceTone },
  ],
}

// ---- History --------------------------------------------------------------
export type HistoryStatus = 'New' | 'Reviewed' | 'Accepted' | 'Rejected' | 'Archived'

export const history: {
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
}[] = [
  { id: 'rec-2041', date: '2026-09-13', resource: 'sql-prod-orders-eastus', current: '8 vCores', proposed: '4 vCores', savings: '24.2%', risk: 'Low', confidence: 91, status: 'New', version: 'v1.4.2' },
  { id: 'rec-1990', date: '2026-08-30', resource: 'sql-prod-billing-eastus', current: '6 vCores', proposed: '4 vCores', savings: '18.6%', risk: 'Low', confidence: 88, status: 'Accepted', version: 'v1.4.1' },
  { id: 'rec-1943', date: '2026-08-14', resource: 'sql-prod-analytics-westus', current: '16 vCores', proposed: '16 vCores', savings: '—', risk: 'None', confidence: 62, status: 'Reviewed', version: 'v1.4.1' },
  { id: 'rec-1902', date: '2026-07-29', resource: 'sql-prod-orders-eastus', current: '8 vCores', proposed: '6 vCores', savings: '11.9%', risk: 'Medium', confidence: 71, status: 'Rejected', version: 'v1.4.0' },
  { id: 'rec-1855', date: '2026-07-11', resource: 'sql-prod-identity-eastus', current: '4 vCores', proposed: '—', savings: '—', risk: 'None', confidence: 41, status: 'Archived', version: 'v1.3.9' },
]

export const assistantSuggestions = [
  'Why is this recommendation considered low risk?',
  'What evidence supports the proposed configuration?',
  'What could happen if workload increases?',
  'Which metric is limiting the recommendation?',
]

export const assistantAnswers: Record<string, string> = {
  'Why is this recommendation considered low risk?':
    'Risk is Low because projected CPU p99 at 4 vCores stays near 58% — well below saturation — with sufficient headroom for observed peaks, a stable trend, low variability outside business hours, and no SLA violations in the 30-day window. These are calculated signals shown on the Recommendation and Risk views; the assistant only explains them.',
  'What evidence supports the proposed configuration?':
    'The scenario replay shows CPU p95 at 28% and p99 at 41% on current capacity, I/O p95 below threshold, storage growth low, and historical SLA compliance of 99.97%. Simulated on 4 vCores these constraints still pass, which is why Candidate A is marked feasible and recommended.',
  'What could happen if workload increases?':
    'Forecasting projects +3.1% over the next 14 days with no CPU saturation in the horizon. If demand grew beyond the upper uncertainty band, p99 headroom on 4 vCores would tighten first; the system would lower confidence and could withdraw the recommendation rather than keep it.',
  'Which metric is limiting the recommendation?':
    'CPU p99 at peak is the binding constraint. Candidate B (2 vCores) pushes p99 to ~81% and moves SLA to Warning, which is why it is not recommended despite larger nominal savings. Memory (68%) is the second-closest signal to its threshold.',
}
