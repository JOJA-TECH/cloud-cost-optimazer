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
  type RecommendationItem,
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
  p50: number | null
  p95: number | null
  p99: number | null
  io: number | null
  memory: number | null
  sessions: number | null
  latency: number | null
}

type DataResult<T> = {
  data: T | null
  source: 'backend' | 'mock' | null
  status: 'loading' | 'ready' | 'empty' | 'error'
  error: string | null
}

export default function OverviewPage() {
  const [kpisResult, setKpisResult] = useState<DataResult<any>>({ data: null, source: null, status: 'loading', error: null })
  const [costDailyResult, setCostDailyResult] = useState<DataResult<CostDay[]>>({ data: null, source: null, status: 'loading', error: null })
  const [perfSeriesResult, setPerfSeriesResult] = useState<DataResult<PerfPoint[]>>({ data: null, source: null, status: 'loading', error: null })
  const [dataQualityResult, setDataQualityResult] = useState<DataResult<any>>({ data: null, source: null, status: 'loading', error: null })
  const [recResult, setRecResult] = useState<DataResult<RecommendationItem>>({ data: null, source: null, status: 'loading', error: null })
  const [resourceUid, setResourceUid] = useState<string | null>(null)
  const [timeRange, setTimeRangeState] = useState<{ from: string; to: string } | null>(null)

  useEffect(() => {
    const uid = getSelectedResourceUid()
    const range = getTimeRange()
    setResourceUid(uid)
    setTimeRangeState(range)
  }, [])

  useEffect(() => {
    if (!resourceUid || !timeRange) {
      setCostDailyResult({ data: null, source: null, status: 'empty', error: 'no_resource' })
      setPerfSeriesResult({ data: null, source: null, status: 'empty', error: 'no_resource' })
      return
    }

    setCostDailyResult({ data: null, source: null, status: 'loading', error: null })
    setPerfSeriesResult({ data: null, source: null, status: 'loading', error: null })
    getCostDaily(resourceUid, timeRange.from, timeRange.to)
      .then((res) => setCostDailyResult(res))
      .catch((err) => setCostDailyResult({ data: null, source: null, status: 'error', error: err?.message ?? 'Error cargando costos' }))

    getPerfSeries(resourceUid, 'cpu_percent', timeRange.from, timeRange.to)
      .then((res) => setPerfSeriesResult(res))
      .catch((err) => setPerfSeriesResult({ data: null, source: null, status: 'error', error: err?.message ?? 'Error cargando rendimiento' }))
  }, [resourceUid, timeRange])

  useEffect(() => {
    getKpis().then((res) => setKpisResult(res))
    getDataQuality().then((res) => setDataQualityResult(res))
    getRecommendationSummary().then((res) => setRecResult(res))
  }, [])

  const currentRecommendation = recResult.status === 'ready' ? recResult.data : null

  return (
    <div>
      <PageHeader
        title="Overview"
        description="Current state, cost, utilization, risk, and the optimization opportunity for the selected Azure SQL database — with the evidence behind it."
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="success" className="h-7 px-2.5">
              <CheckCircle2 className="size-3.5" />
              {kpisResult.data?.status ?? 'Estado no disponible'}
            </Badge>
            <Badge variant="outline" className="text-[9px]">{kpisResult.source}</Badge>
          </div>
        }
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
        <KpiCard label="Resource status" value={kpisResult.data?.status ?? '—'} sub="Azure SQL Database" tone="success" icon={CheckCircle2} badge={<Badge variant="outline" className="text-[9px]">{kpisResult.source}</Badge>} />
        <KpiCard label="Current cost" value={kpisResult.data?.currentCost != null ? `$${kpisResult.data.currentCost.toFixed(2)}` : '—'} sub="selected period" delta={kpisResult.data?.currentCostDelta ?? undefined} icon={DollarSign} badge={<Badge variant="outline" className="text-[9px]">{kpisResult.source}</Badge>} />
        <KpiCard label="Estimated cost" value={kpisResult.data?.estimatedCost != null ? `$${kpisResult.data.estimatedCost.toFixed(2)}` : '—'} sub="according to recommendation" tone="brand" icon={Calculator} badge={<Badge variant="outline" className="text-[9px]">{kpisResult.source}</Badge>} />
        <KpiCard label="Estimated savings" value={kpisResult.data?.savingsPct != null ? `${kpisResult.data.savingsPct}%` : '—'} sub={kpisResult.data?.estimatedSavings != null ? `$${kpisResult.data.estimatedSavings.toFixed(2)} / mes` : 'Sin recomendación'} tone="success" icon={TrendingDown} badge={<Badge variant="outline" className="text-[9px]">{kpisResult.source}</Badge>} />
        <KpiCard label="Risk" value={kpisResult.data?.risk ?? '—'} sub="within constraints" tone="success" icon={ShieldCheck} badge={<Badge variant="outline" className="text-[9px]">{kpisResult.source}</Badge>} />
        <KpiCard label="Confidence" value={kpisResult.data?.confidence != null ? `${kpisResult.data.confidence}%` : '—'} sub="recommendation confidence" tone="brand" icon={Gauge} badge={<Badge variant="outline" className="text-[9px]">{kpisResult.source}</Badge>} />
      </div>

      <div className="mt-4">
        {currentRecommendation ? (
          <RecommendationSummary rec={currentRecommendation} recSource={recResult.source} />
        ) : (
          <div className="rounded-xl border border-border bg-card p-5 text-sm text-muted-foreground">
            {recResult.status === 'loading' ? 'Cargando recomendación...' :
              recResult.status === 'error' ? `No fue posible consultar la recomendación: ${recResult.error ?? 'Error desconocido'}` :
              recResult.error === 'no_resource' ? 'Selecciona un recurso para consultar recomendaciones.' :
              'No hay recomendaciones disponibles para el recurso seleccionado.'}
          </div>
        )}
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Panel
          title="Cumulative cost"
          description="Billed usage across the analysis window"
          className="lg:col-span-2"
          action={costDailyResult.status === 'ready' && costDailyResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}
        >
          {!costDailyResult.data || costDailyResult.status !== 'ready' ? (
            <div className="h-64 flex items-center justify-center rounded-lg border border-dashed bg-muted/50 p-4 text-xs text-muted-foreground">
              {costDailyResult.status === 'empty' && <p className="text-xs text-muted-foreground">No hay datos de costo disponibles para el recurso seleccionado.</p>}
              {costDailyResult.status === 'error' && <p className="text-xs text-muted-foreground">{costDailyResult.error}</p>}
            </div>
          ) : (
            <AreaTrend data={costDailyResult.data} dataKey="cumulative" prefix="$" height={230} />
          )}
        </Panel>

        <Panel title="Data quality" description="Signals feeding the analysis" action={dataQualityResult.status === 'ready' && dataQualityResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}>
          <div className="space-y-3">
            {dataQualityResult.status !== 'ready' && <p className="text-xs text-muted-foreground">{dataQualityResult.status === 'loading' ? 'Cargando cobertura...' : dataQualityResult.status === 'error' ? dataQualityResult.error : 'No hay información de cobertura disponible.'}</p>}
            {dataQualityResult.data && (
            <>
            <div className="flex items-center justify-between rounded-lg border border-border bg-background px-3 py-2">
              <span className="text-xs text-muted-foreground">Global confidence</span>
              <Badge variant="success">{dataQualityResult.data.globalConfidence}</Badge>
            </div>
            {dataQualityResult.data?.items.slice(0, 5).map((item: any) => (
              <div key={item.label} className="flex items-center justify-between text-xs">
                <span className="text-muted-foreground">{item.label}</span>
                <span className="font-medium tabular-nums text-foreground">{item.value}</span>
              </div>
            ))}
            </>
            )}
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
           note="Percentiles calculated by backend analytics for the selected time window."
           action={perfSeriesResult.status === 'ready' && perfSeriesResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}
        >
          <div className="mb-3 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <Legend color="var(--chart-3)" label="p50" />
            <Legend color="var(--chart-2)" label="p95" />
            <Legend color="var(--chart-1)" label="p99" />
          </div>
          {!perfSeriesResult.data || perfSeriesResult.status !== 'ready' || !perfSeriesResult.data.some(p => p.p50 !== null) ? (
            <div className="h-[230px] flex items-center justify-center rounded-lg border border-dashed bg-muted/50 p-4 text-xs text-muted-foreground">
              {!perfSeriesResult.data && perfSeriesResult.status === 'loading' && <p className="text-xs text-muted-foreground">Cargando datos de rendimiento...</p>}
              {perfSeriesResult.status === 'empty' && <p className="text-xs text-muted-foreground">No hay datos de rendimiento disponibles para el recurso seleccionado.</p>}
              {perfSeriesResult.status === 'error' && <p className="text-xs text-muted-foreground">{perfSeriesResult.error}</p>}
              {perfSeriesResult.status === 'ready' && <p>No hay percentiles de CPU calculados para el período seleccionado.</p>}
            </div>
          ) : (
            <MultiLine
              data={perfSeriesResult.data}
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
          {!currentRecommendation ? (
            <p className="text-sm text-muted-foreground">
              {recResult.status === 'loading' ? 'Cargando oportunidad...' :
                recResult.status === 'error' ? 'No fue posible consultar la oportunidad.' :
                'No hay una oportunidad evaluada para el recurso seleccionado.'}
            </p>
          ) : (
          <div className="space-y-4">
            <div className="rounded-lg border border-border bg-background p-3">
              <p className="text-xs text-muted-foreground">Compute</p>
              <p className="mt-1 flex items-center gap-2 text-sm font-semibold text-foreground">
                {currentRecommendation.from} <ArrowRight className="size-3.5 text-primary" /> <span className="text-primary">{currentRecommendation.to}</span>
              </p>
            </div>
            <ConfidenceBar value={currentRecommendation.confidence} />
            {recResult.source === 'mock' && <Badge variant="outline" className="text-[9px]">Mock</Badge>}
            <div className="rounded-lg bg-muted/60 p-3 text-xs text-muted-foreground">
              Consulta la evidencia y los escenarios antes de tomar una decisión de capacidad.
            </div>
            <Button size="sm" className="w-full" nativeButton={false} render={<Link href="/dashboard/scenarios" />}>
              Compare scenarios
              <ArrowRight />
            </Button>
          </div>
          )}
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

const recMock: RecommendationItem = {
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
