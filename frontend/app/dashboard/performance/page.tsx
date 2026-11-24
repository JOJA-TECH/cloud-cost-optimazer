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
  timestamp: string
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
    if (!resourceUid || !timeRange) {
      setDataResult({ data: null, source: null, status: 'empty', error: 'Selecciona un recurso y un período.' })
      return
    }

    getPerfSeries(resourceUid, 'cpu_percent', timeRange.from, timeRange.to)
      .then((res) => setDataResult(res))
      .catch((err) => setDataResult({ data: null, source: null, status: 'error', error: err?.message ?? 'Error desconocido' }))

    getPerfSummary().then((res) => setPerfSummaryResult(res))
    getReliabilityEvents().then((res) => setReliabilityEventsResult(res))
  }, [resourceUid, timeRange])

  const end = timeRange ? new Date(timeRange.to).getTime() : NaN
  const shownData = dataResult.data?.filter((point) => {
    const timestamp = new Date(point.timestamp).getTime()
    return timestamp >= end - hours * 60 * 60 * 1000 && timestamp <= end
  }) ?? []
  const shownLines = cpuLines.filter((line) => active[line.key])

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
                onClick={() => setHours(r.hours)}
                aria-pressed={hours === r.hours}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium transition-colors',
                  hours === r.hours
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

      {!shownData.length || dataResult.status !== 'ready' ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          {dataResult.status === 'loading' ? (
            <div className="text-xs text-muted-foreground">Cargando datos de rendimiento...</div>
          ) : dataResult.status === 'empty' ? (
            <p className="text-sm text-muted-foreground">{dataResult.error ?? 'No hay datos de rendimiento disponibles para el recurso seleccionado.'}</p>
          ) : (
            <p className="text-sm text-muted-foreground">No hay datos de rendimiento disponibles para el recurso seleccionado.</p>
          )}
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-8">
            <div className="col-span-full"><Badge variant="outline" className="text-[9px]">Resumen: {perfSummaryResult.source ?? 'sin datos'}</Badge></div>
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
            description="Estimated CPU percentiles · threshold lines mark warning and critical"
            className="mt-4"
            note="p50 and p95 are estimates derived from the average; p99 is derived from the maximum. They are not measured percentiles."
            action={
              <div className="flex items-center gap-2">
                {cpuLines.map((line) => (
                  <button
                    key={line.key}
                    type="button"
                    aria-pressed={active[line.key]}
                    onClick={() => setActive((current) => ({ ...current, [line.key]: !current[line.key] }))}
                    className={cn('rounded border px-2 py-1 text-[10px]', active[line.key] ? 'border-primary text-foreground' : 'border-border text-muted-foreground')}
                  >
                    {line.name}
                  </button>
                ))}
                <Badge variant="outline" className="text-[9px]">{dataResult.source}</Badge>
              </div>
            }
          >
            <MultiLine
              data={shownData}
              xKey="t"
              yDomain={[0, 100]}
              unit="%"
              height={300}
              lines={shownLines}
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
            <Panel title="Reliability events" description="Errors, timeouts, and contention (30d)" action={<Badge variant="outline" className="text-[9px]">{reliabilityEventsResult.source ?? 'sin datos'}</Badge>}>
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
