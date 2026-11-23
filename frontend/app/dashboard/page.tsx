'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { CheckCircle2, DollarSign, TrendingDown, Calculator, ShieldCheck, Gauge, ArrowRight } from 'lucide-react'
import { PageHeader, KpiCard, Panel, ConfidenceBar } from '@/components/dashboard/ui'
import { RecommendationSummary } from '@/components/dashboard/recommendation-summary'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { AreaTrend, MultiLine } from '@/components/charts'
import {
  getKpis,
  getCostDaily,
  getPerfSeries,
  getDataQuality,
  getRecommendationSummary,
  getSelectedResourceUid,
  getTimeRange,
} from '@/lib/api-services'

type CostDay = {
  day: string
  billed: number
  estimated: number
  reference: number
  cumulative: number
}

type PerfPoint = {
  t: string
  idx: number
  p50: number
  p95: number
  p99: number
  io: number
  memory: number
  sessions: number
  latency: number
}

export default function OverviewPage() {
  const [kpis, setKpis] = useState<any>(kpisMock)
  const [kpisSource, setKpisSource] = useState<'mock' | 'backend'>('mock')
  const [costDaily, setCostDaily] = useState<CostDay[]>([])
  const [costsLoaded, setCostsLoaded] = useState(false)
  const [costsSource, setCostsSource] = useState<'mock' | 'backend'>('mock')
  const [perfSeries, setPerfSeries] = useState<PerfPoint[]>([])
  const [perfLoaded, setPerfLoaded] = useState(false)
  const [perfSource, setPerfSource] = useState<'mock' | 'backend'>('mock')
  const [dataQuality, setDataQuality] = useState<any>(dataQualityMock)
  const [dataQualitySource, setDataQualitySource] = useState<'mock' | 'backend'>('mock')
  const [rec, setRec] = useState<any>(recMock)
  const [recSource, setRecSource] = useState<'mock' | 'backend'>('mock')
  const [resourceUid, setResourceUid] = useState<string | null>(null)
  const [timeRange, setTimeRangeState] = useState<{ from: string; to: string } | null>(null)

  useEffect(() => {
    const uid = getSelectedResourceUid()
    const range = getTimeRange()
    setResourceUid(uid)
    setTimeRangeState(range)
  }, [])

  useEffect(() => {
    if (!resourceUid || !timeRange) return

    getCostDaily(resourceUid, timeRange.from, timeRange.to).then((res) => {
      setCostDaily(res.data ?? [])
      setCostsLoaded(!!res.data && res.data.length > 0)
      setCostsSource(res.source)
    })
    getPerfSeries(resourceUid, 'cpu_percent', timeRange.from, timeRange.to)
      .then((res) => {
        setPerfSeries(res.data ?? [])
        setPerfLoaded(!!res.data && res.data.length > 0)
        setPerfSource(res.source)
      })
  }, [resourceUid, timeRange])

  useEffect(() => {
    getKpis().then((res) => {
      setKpis(res.data ?? kpisMock)
      setKpisSource(res.source)
    })
    getDataQuality().then((res) => {
      setDataQuality(res.data ?? dataQualityMock)
      setDataQualitySource(res.source)
    })
    getRecommendationSummary().then((res) => {
      setRec(res.data ?? recMock)
      setRecSource(res.source)
    })
  }, [])

  const anyDataLoaded = costsLoaded || perfLoaded
  const anyDataReal = costsSource === 'backend' || perfSource === 'backend'

  return (
    <div>
      <PageHeader
        title="Overview"
        description="Current state, cost, utilization, risk, and the optimization opportunity for the selected Azure SQL database — with the evidence behind it."
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="success" className="h-7 px-2.5">
              <CheckCircle2 className="size-3.5" />
              {kpis.status}
            </Badge>
            <Badge variant="outline" className="h-7 px-2.5 text-[10px]">
              {anyDataReal ? 'Datos reales' : 'Demo'}
            </Badge>
          </div>
        }
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
        <KpiCard label="Resource status" value="Healthy" sub="Azure SQL Database" tone="success" icon={CheckCircle2} />
        <KpiCard label="Current cost" value={`$${kpis.currentCost.toFixed(2)}`} sub="/ month" delta={kpis.currentCostDelta} icon={DollarSign} />
        <KpiCard label="Estimated cost" value={`$${kpis.estimatedCost.toFixed(2)}`} sub="simulated / mo" tone="brand" icon={Calculator} />
        <KpiCard label="Estimated savings" value={`${kpis.savingsPct}%`} sub={`$${kpis.estimatedSavings.toFixed(2)} / mo`} tone="success" icon={TrendingDown} />
        <KpiCard label="Risk" value={kpis.risk} sub="within constraints" tone="success" icon={ShieldCheck} />
        <KpiCard label="Confidence" value={`${kpis.confidence}%`} sub="high coverage" tone="brand" icon={Gauge} />
      </div>

      <div className="mt-4">
        <RecommendationSummary rec={rec} />
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Panel
          title="Cumulative cost"
          description="Billed usage across the analysis window"
          className="lg:col-span-2"
          action={<Badge variant="neutral">Billed</Badge>}
        >
          {!costsLoaded ? (
            <div className="h-64 flex items-center justify-center rounded-lg border border-dashed bg-muted/50 p-4 text-xs text-muted-foreground">
              No hay datos de costo disponibles para el recurso seleccionado.
            </div>
          ) : (
            <AreaTrend data={costDaily} dataKey="cumulative" prefix="$" height={230} />
          )}
        </Panel>

        <Panel title="Data quality" description="Signals feeding the analysis">
          <div className="space-y-3">
            <div className="flex items-center justify-between rounded-lg border border-border bg-background px-3 py-2">
              <span className="text-xs text-muted-foreground">Global confidence</span>
              <Badge variant="success">{dataQuality.globalConfidence}</Badge>
              <Badge variant="outline" className="text-[9px]">{dataQualitySource}</Badge>
            </div>
            {dataQuality.items.slice(0, 5).map((item: any) => (
              <div key={item.label} className="flex items-center justify-between text-xs">
                <span className="text-muted-foreground">{item.label}</span>
                <span className="font-medium tabular-nums text-foreground">{item.value}</span>
              </div>
            ))}
            <Button variant="outline" size="sm" className="w-full" nativeButton={false} render={<Link href="/dashboard/diagnostics" />}>
              View diagnostics
              <ArrowRight />
            </Button>
          </div>
        </Panel>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Panel
          title="CPU utilization percentiles"
          description="Average alone does not drive the decision — peaks do"
          className="lg:col-span-2"
          note="Observed telemetry sampled at 5-minute resolution over the last 72 hours."
        >
          <div className="mb-3 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <Legend color="var(--chart-3)" label="p50" />
            <Legend color="var(--chart-2)" label="p95" />
            <Legend color="var(--chart-1)" label="p99" />
          </div>
          {!perfLoaded ? (
            <div className="h-[230px] flex items-center justify-center rounded-lg border border-dashed bg-muted/50 p-4 text-xs text-muted-foreground">
              No hay datos de rendimiento disponibles para el recurso seleccionado.
            </div>
          ) : (
            <MultiLine
              data={perfSeries}
              xKey="t"
              yDomain={[0, 100]}
              unit="%"
              height={230}
              thresholds={[{ y: 85, color: 'var(--warning)', label: 'warning' }]}
              lines={[
                { key: 'p50', color: 'var(--chart-3)', name: 'CPU p50' },
                { key: 'p95', color: 'var(--chart-2)', name: 'CPU p95' },
                { key: 'p99', color: 'var(--chart-1)', name: 'CPU p99' },
              ]}
            />
          )}
        </Panel>

        <Panel title="Opportunity snapshot" description="What a right-size would change">
          <div className="space-y-4">
            <div className="rounded-lg border border-border bg-background p-3">
              <p className="text-xs text-muted-foreground">Compute</p>
              <p className="mt-1 flex items-center gap-2 text-sm font-semibold text-foreground">
                8 vCores <ArrowRight className="size-3.5 text-primary" /> <span className="text-primary">4 vCores</span>
              </p>
            </div>
            <ConfidenceBar value={rec.confidence} />
            <div className="rounded-lg bg-muted/60 p-3 text-xs text-muted-foreground">
              Projected CPU p99 at target capacity stays near 58% — below saturation, with headroom
              for observed peaks.
            </div>
            <Button size="sm" className="w-full" nativeButton={false} render={<Link href="/dashboard/scenarios" />}>
              Compare scenarios
              <ArrowRight />
            </Button>
          </div>
        </Panel>
      </div>
    </div>
  )
}

function Legend({ color, label }: { color: string; label: string }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className="size-2 rounded-full" style={{ background: color }} />
      {label}
    </span>
  )
}

// Fallback mock values (used until the backend returns data, or as default if backend is unavailable)
const kpisMock = {
  status: 'Healthy',
  currentCost: 486.2,
  currentCostDelta: 4.8,
  estimatedCost: 368.4,
  estimatedSavings: 117.8,
  savingsPct: 24.2,
  risk: 'Low',
  confidence: 91,
}

const costDailyMock: CostDay[] = [
  { day: 'Aug 1', billed: 16.8, estimated: 12.7, reference: 17.1, cumulative: 16.8 },
]

const perfSeriesMock: PerfPoint[] = [
  { t: '00:00', idx: 0, p50: 12, p95: 21, p99: 26, io: 24, memory: 58, sessions: 180, latency: 4.2 },
]

const dataQualityMock = {
  globalConfidence: 'High',
  items: [
    { label: 'Last collected', value: '8 min ago', info: '', tone: 'good' },
    { label: 'Historical duration', value: '30 days', info: '', tone: 'good' },
    { label: 'Sampling interval', value: '5 min', info: '', tone: 'good' },
    { label: 'Coverage', value: '96%', info: '', tone: 'good' },
    { label: 'Missing metrics', value: 'None', info: '', tone: 'good' },
  ],
}

const recMock = {
  id: 'rec-2041',
  title: 'Recommendation available',
  from: '8 vCores',
  to: '4 vCores',
  savingsPct: 24.2,
  savingsAbs: 117.8,
  risk: 'Low',
  confidence: 91,
  latencyImpact: 2.1,
  analysisPeriod: 'Last 30 days',
  generated: 'Today, 14:32',
  modelVersion: 'rightsizer v1.4.2 · ruleset 2024.11',
  evidence: [
    { label: 'CPU p95', value: '28%', tone: 'good' },
    { label: 'CPU p99', value: '41%', tone: 'good' },
    { label: 'I/O p95', value: 'Below threshold', tone: 'good' },
    { label: 'Workload trend', value: 'Stable', tone: 'good' },
    { label: 'Headroom', value: 'Sufficient', tone: 'good' },
    { label: 'SLA (30d)', value: '99.97%', tone: 'good' },
  ],
  constraints: [
    { label: 'p95 latency within threshold', pass: true },
    { label: 'No SLA violations in analysis window', pass: true },
    { label: 'Sufficient CPU headroom at projected peak', pass: true },
    { label: 'I/O within provisioned limits', pass: true },
  ],
  limitations: [],
  rationale: '',
}
