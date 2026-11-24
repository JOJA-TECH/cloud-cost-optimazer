'use client'

import { Activity, CheckCircle2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { AreaTrend, Bars } from '@/components/charts'
import {
  getWorkloadHourly,
  getWorkloadClassification,
  getWorkloadPatterns,
  getWorkloadPattern,
  getSelectedResourceUid,
  getTimeRange,
  type ServiceResult,
} from '@/lib/api-services'

type WorkloadClassification = {
  pattern: string
  confidence: number | null
  evidence: string[]
  metrics: Array<{ label: string; value: string }>
}

export default function WorkloadPage() {
  const [workloadHourly, setWorkloadHourly] = useState<{ hour: string; intensity: number | null; concurrency: number | null; reads: number | null; writes: number | null }[]>([])
  const [clsResult, setClsResult] = useState<ServiceResult<WorkloadClassification>>({ data: null, source: null, status: 'loading', error: null })
  const [workloadPatterns, setWorkloadPatterns] = useState<string[]>([])
  const [hourlyResult, setHourlyResult] = useState<ServiceResult<typeof workloadHourly>>({ data: null, source: null, status: 'loading', error: null })
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
      setClsResult({ data: null, source: null, status: 'empty', error: 'no_resource' })
      return
    }
    getWorkloadHourly()
      .then((res) => { setWorkloadHourly(res.data ?? []); setHourlyResult(res) })
    getWorkloadClassification()
      .then(setClsResult)
    getWorkloadPatterns()
      .then((res) => setWorkloadPatterns(res.data ?? []))
  }, [resourceUid, timeRange])

  const cls = clsResult.data
  return (
    <div>
      <PageHeader
        title="Workload analysis"
        description="How demand actually behaves over time — the central intelligence behind every optimization. Classification, peaks, variability, and trend drive the recommendation."
        actions={
          <div className="flex items-center gap-2">
            {clsResult.status === 'ready' && clsResult.source === 'mock' && <Badge variant="outline" className="text-[9px]">Mock</Badge>}
          </div>
        }
      />

      {(!cls || clsResult.status !== 'ready') && hourlyResult.status !== 'ready' ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          <p className="text-sm text-muted-foreground">{clsResult.status === 'loading' ? 'Cargando clasificación...' : clsResult.status === 'error' ? 'No fue posible consultar el patrón de carga.' : 'Datos insuficientes'}</p>
          <p className="text-xs text-muted-foreground">
            {clsResult.status === 'error' ? clsResult.error : 'No hay datos de patrones de carga para el recurso y ventana seleccionados.'}
          </p>
        </div>
      ) : (
        <>
          <div className="grid gap-4 lg:grid-cols-3">
            <Panel className="lg:col-span-1" title="Workload pattern" description="Classified from selected telemetry" action={clsResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}>
              {cls ? (<>
              <div className="rounded-xl border border-primary/25 bg-primary/[0.05] p-4">
                <div className="flex items-center gap-2">
                  <span className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <Activity className="size-5" />
                  </span>
                  <div>
                    <p className="text-lg font-semibold text-foreground">{cls.pattern}</p>
                    <p className="text-xs text-muted-foreground">Classification</p>
                  </div>
                    {cls.confidence != null && <Badge variant="default" className="ml-auto">
                      {cls.confidence}% conf.
                    </Badge>}
                </div>
                <ul className="mt-4 space-y-2">
                  {cls.evidence.map((e: string) => (
                    <li key={e} className="flex items-start gap-2 text-xs text-muted-foreground">
                      <CheckCircle2 className="mt-px size-3.5 shrink-0 text-success" />
                      {e}
                    </li>
                  ))}
                </ul>
              </div>

              <div className="mt-4">
                <p className="mb-2 text-xs font-medium text-muted-foreground">Supported classifications</p>
                <div className="flex flex-wrap gap-1.5">
                  {workloadPatterns.map((p: string) => (
                    <Badge key={p} variant={p === cls.pattern ? 'solid' : 'neutral'}>
                      {p}
                    </Badge>
                  ))}
                </div>
              </div>
              </>) : <p className="text-xs text-muted-foreground">No hay clasificación calculada para este período.</p>}
            </Panel>

            <Panel className="lg:col-span-2" title="Workload intensity by hour" description="Aggregate demand across a representative day" action={hourlyResult.status === 'ready' && hourlyResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}>
              {hourlyResult.status === 'ready' && workloadHourly.some(p => p.intensity !== null) ? (<>
              <AreaTrend data={workloadHourly} dataKey="intensity" xKey="hour" unit={hourlyResult.source === 'mock' ? '%' : ' ops/min'} height={200} color="var(--chart-1)" />
              <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3">
                {cls?.metrics.map((m: any) => (
                  <div key={m.label} className="rounded-lg border border-border bg-background p-3">
                    <p className="text-[11px] text-muted-foreground">{m.label}</p>
                    <p className="mt-0.5 text-sm font-semibold text-foreground">{m.value}</p>
                  </div>
                ))}
              </div>
              </>) : <p className="text-xs text-muted-foreground">No hay datos horarios disponibles para este recurso.</p>}
            </Panel>
          </div>

          <div className="mt-4 grid gap-4 lg:grid-cols-2">
            <Panel title="Concurrency by hour" description={hourlyResult.source === 'mock' ? 'Peak concurrent sessions' : 'Average active sessions by hour'} action={hourlyResult.status === 'ready' && hourlyResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}>
              {hourlyResult.status === 'ready' && workloadHourly.some(p => p.concurrency !== null) ? (
              <Bars
                data={workloadHourly}
                xKey="hour"
                height={220}
                bars={[{ key: 'concurrency', color: 'var(--chart-2)', name: 'Concurrency' }]}
              />
              ) : <p className="text-xs text-muted-foreground">No hay datos horarios disponibles.</p>}
            </Panel>
            <Panel title="Read / write mix" description="Share of operations by hour (%)" action={hourlyResult.status === 'ready' && hourlyResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}>
              {hourlyResult.status === 'ready' && workloadHourly.some(p => p.reads !== null && p.writes !== null) ? (<>
              <div className="mb-3 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                <Legend color="var(--chart-1)" label="Reads" />
                <Legend color="var(--chart-4)" label="Writes" />
              </div>
              <Bars
                data={workloadHourly}
                xKey="hour"
                height={200}
                unit="%"
                stacked
                bars={[
                  { key: 'reads', color: 'var(--chart-1)', name: 'Reads' },
                  { key: 'writes', color: 'var(--chart-4)', name: 'Writes' },
                ]}
              />
              </>) : <p className="text-xs text-muted-foreground">No hay lecturas y escrituras medidas para este período; las métricas de I/O no representan la mezcla de operaciones.</p>}
            </Panel>
          </div>
        </>
      )}
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

const workloadClassificationMock: WorkloadClassification = {
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

const workloadPatternsMock = [
  'Stable',
  'Periodic',
  'Bursty',
  'Increasing',
  'CPU Intensive',
  'I/O Intensive',
  'Mixed',
]
