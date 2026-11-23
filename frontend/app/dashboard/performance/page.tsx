'use client'

import { useEffect, useState } from 'react'
import { PageHeader, Panel, toneText } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { AreaTrend, MultiLine } from '@/components/charts'
import {
  getPerfSeries,
  getPerfSummary,
  getReliabilityEvents,
  getSelectedResourceUid,
  getTimeRange,
} from '@/lib/api-services'

const ranges = [
  { label: '24h', hours: 24 },
  { label: '48h', hours: 48 },
  { label: '72h', hours: 72 },
]

const cpuLines = [
  { key: 'p50', color: 'var(--chart-3)', name: 'CPU p50' },
  { key: 'p95', color: 'var(--chart-2)', name: 'CPU p95' },
  { key: 'p99', color: 'var(--chart-1)', name: 'CPU p99' },
]

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

type DataResult<T> = {
  data: T | null
  source: 'backend' | 'mock' | null
  status: 'loading' | 'ready' | 'empty' | 'error'
  error: string | null
}

export default function PerformancePage() {
  const [hours, setHours] = useState(72)
  const [active, setActive] = useState<Record<string, boolean>>({ p50: true, p95: true, p99: true })
  const [dataResult, setDataResult] = useState<DataResult<PerfPoint[]>>({ data: null, source: null, status: 'loading', error: null })
  const [perfSummaryResult, setPerfSummaryResult] = useState<DataResult<any[]>>({ data: null, source: null, status: 'loading', error: null })
  const [reliabilityEventsResult, setReliabilityEventsResult] = useState<DataResult<any[]>>({ data: null, source: null, status: 'loading', error: null })
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

    getPerfSeries(resourceUid, 'cpu_percent', timeRange.from, timeRange.to)
      .then((res) => setDataResult(res))
      .catch((err) => setDataResult({ data: null, source: null, status: 'error', error: err?.message ?? 'Error desconocido' }))

    getPerfSummary().then((res) => setPerfSummaryResult(res))
    getReliabilityEvents().then((res) => setReliabilityEventsResult(res))
  }, [resourceUid, timeRange])

  const shownData = dataResult.data ? dataResult.data.slice(-hours) : []

  return (
    <div>
      <PageHeader
        title="Performance"
        description="CPU percentiles, I/O, memory, sessions, latency, and reliability over time. Percentiles are emphasized over averages — the peaks are what a capacity change has to survive."
        actions={
          <div className="flex items-center gap-1 rounded-lg border border-border bg-card p-0.5">
            {ranges.map((r) => (
              <button
                key={r.label}
                onClick={() => setActive((s) => ({ ...s, [r.label]: !s[r.label] }))}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium transition-colors',
                  active[r.label]
                    ? 'border-border bg-background text-foreground'
                    : 'border-transparent text-muted-foreground opacity-50',
                )}
              >
                {r.label}
              </button>
            ))}
          </div>
        }
      />

      {!dataResult.data || dataResult.status !== 'ready' ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          {dataResult.status === 'loading' ? (
            <div className="text-xs text-muted-foreground">Cargando datos de rendimiento...</div>
          ) : dataResult.status === 'empty' ? (
            <p className="text-sm text-muted-foreground">No hay datos de rendimiento disponibles para el recurso seleccionado.</p>
          ) : (
            <p className="text-sm text-muted-foreground">No hay datos de rendimiento disponibles para el recurso seleccionado.</p>
          )}
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-8">
            {perfSummaryResult.data?.map((m: any) => (
              <div key={m.label} className="rounded-xl border border-border bg-card p-3 shadow-sm">
                <p className="text-[11px] text-muted-foreground">{m.label}</p>
                <p className={cn('mt-1 text-base font-semibold tabular-nums', toneText(m.tone))}>
                  {m.value}
                </p>
              </div>
            ))}
          </div>

          <Panel
            title="CPU utilization over time"
            description="Toggle percentiles · threshold lines mark warning and critical"
            className="mt-4"
            note="Observed telemetry at 5-minute resolution. The average is intentionally not the headline metric."
            action={perfSource === 'backend' ? <Badge variant="outline" className="text-[9px]">{perfSource}</Badge> : undefined}
          >
            <MultiLine
              data={shownData}
              xKey="t"
              yDomain={[0, 100]}
              unit="%"
              height={300}
              lines={shownLines.length ? shownData : cpuLines}
              thresholds={[
                { y: 75, color: 'var(--warning)', label: 'warning' },
                { y: 90, color: 'var(--danger)', label: 'critical' },
              ]}
            />
          </Panel>

          <div className="mt-4 grid gap-4 lg:grid-cols-3">
            <Panel title="I/O utilization" description="Percent of provisioned throughput">
              <AreaTrend data={shownData} dataKey="io" xKey="t" unit="%" color="var(--chart-2)" height={190} />
            </Panel>
            <Panel title="Active sessions" description="Concurrent sessions (max 2400)">
              <AreaTrend data={shownData} dataKey="sessions" xKey="t" color="var(--chart-1)" height={190} />
            </Panel>
            <Panel title="Query latency" description="Average response time (ms)">
              <AreaTrend data={shownData} dataKey="latency" xKey="t" unit=" ms" color="var(--chart-3)" height={190} />
            </Panel>
          </div>

          <div className="mt-4 grid gap-4 lg:grid-cols-2">
            <Panel title="Memory utilization" description="Working set as percent of provisioned">
              <AreaTrend data={shownData} dataKey="memory" xKey="t" unit="%" color="var(--chart-5)" height={190} />
            </Panel>
            <Panel title="Reliability events" description="Errors, timeouts, and contention (30d)" action={perfLoaded ? <Badge variant="outline" className="text-[9px]">{perfSource}</Badge> : undefined}>
              <div className="grid grid-cols-2 gap-3">
                {reliabilityEventsResult.data?.map((e: any) => (
                  <div key={e.label} className="rounded-lg border border-border bg-background p-3">
                    <div className="flex items-center justify-between">
                      <p className="text-xs text-muted-foreground">{e.label}</p>
                      <span className={cn('size-1.5 rounded-full', e.tone === 'good' ? 'bg-success' : e.tone === 'warn' ? 'bg-warning' : 'bg-danger')} />
                    </div>
                    <p className={cn('mt-1 text-xl font-semibold tabular-nums', toneText(e.tone))}>{e.value}</p>
                  </div>
                ))}
              </div>
              <div className="mt-3 flex items-center gap-2 rounded-lg bg-muted/60 p-3 text-xs text-muted-foreground">
                <Badge variant="success">SLA 99.97%</Badge>
                No SLA violations recorded in the analysis window.
              </div>
            </Panel>
          </div>
        </>
      )}
    </div>
  )
}

const perfSeriesMock: PerfPoint[] = [
  { t: '00:00', idx: 0, p50: 12, p95: 21, p99: 26, io: 24, memory: 58, sessions: 180, latency: 4.2 },
]

const perfSummaryMock = [
  { label: 'CPU p50', value: '18%', tone: 'good' },
  { label: 'CPU p95', value: '28%', tone: 'good' },
  { label: 'CPU p99', value: '41%', tone: 'good' },
  { label: 'I/O p95', value: '52%', tone: 'good' },
  { label: 'Memory', value: '68%', tone: 'warn' },
  { label: 'Peak sessions', value: '820 / 2400', tone: 'good' },
  { label: 'Avg latency', value: '5.1 ms', tone: 'good' },
  { label: 'SLA compliance', value: '99.97%', tone: 'good' },
]

const reliabilityEventsMock = [
  { label: 'Deadlocks (30d)', value: 3, tone: 'good' },
  { label: 'Timeouts (30d)', value: 11, tone: 'warn' },
  { label: 'Errors (30d)', value: 0, tone: 'good' },
  { label: 'Throttling events', value: 0, tone: 'good' },
]