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
} from '@/lib/api-services'

type WorkloadClassification = {
  pattern: string
  confidence: number
  evidence: string[]
  metrics: Array<{ label: string; value: string }>
}

export default function WorkloadPage() {
  const [workloadHourly, setWorkloadHourly] = useState<{ hour: string; intensity: number; concurrency: number; reads: number; writes: number }[]>([])
  const [cls, setCls] = setClassificationState(workloadClassificationMock)
  const [workloadPatterns, setWorkloadPatterns] = useState<string[]>(workloadPatternsMock)
  const [patternLoaded, setPatternLoaded] = useState(false)
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
    getWorkloadHourly()
      .then(setWorkloadHourly)
      .catch(() => setWorkloadHourly([]))
    getWorkloadClassification()
      .then(setCls)
      .catch(() => setCls(workloadClassificationMock))
    getWorkloadPatterns()
      .then(setWorkloadPatterns)
      .catch(() => setWorkloadPatterns(workloadPatternsMock))
    getWorkloadPattern()
      .then((p) => {
        if (p && p.data) {
          setCls({
            pattern: p.data.pattern,
            confidence: p.data.confidence,
            evidence: p.data.evidence,
            metrics: cls.metrics,
          })
          setPatternLoaded(true)
        }
      })
      .catch(() => setPatternLoaded(false))
  }, [resourceUid, timeRange, cls.metrics])

  return (
    <div>
      <PageHeader
        title="Workload analysis"
        description="How demand actually behaves over time — the central intelligence behind every optimization. Classification, peaks, variability, and trend drive the recommendation."
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="text-[9px]">{patternLoaded ? 'Datos reales' : 'Demo'}</Badge>
          </div>
        }
      />

      {!patternLoaded ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          <p className="text-sm text-muted-foreground">Datos insuficientes</p>
          <p className="text-xs text-muted-foreground">
            No hay datos de patrones de carga para el recurso y ventana seleccionados. Verifique que exista
            datos en cco_data y que CCO_DATA_DSN esté configurado en el backend.
          </p>
        </div>
      ) : (
        <>
          <div className="grid gap-4 lg:grid-cols-3">
            <Panel className="lg:col-span-1" title="Workload pattern" description="Classified from 30 days of telemetry">
              <div className="rounded-xl border border-primary/25 bg-primary/[0.05] p-4">
                <div className="flex items-center gap-2">
                  <span className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <Activity className="size-5" />
                  </span>
                  <div>
                    <p className="text-lg font-semibold text-foreground">{cls.pattern}</p>
                    <p className="text-xs text-muted-foreground">Classification</p>
                  </div>
                  <Badge variant="default" className="ml-auto">
                    {cls.confidence}% conf.
                  </Badge>
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
            </Panel>

            <Panel className="lg:col-span-2" title="Workload intensity by hour" description="Aggregate demand across a representative day">
              <AreaTrend data={workloadHourly} dataKey="intensity" xKey="hour" unit="%" height={200} color="var(--chart-1)" />
              <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-3">
                {cls.metrics.map((m: any) => (
                  <div key={m.label} className="rounded-lg border border-border bg-background p-3">
                    <p className="text-[11px] text-muted-foreground">{m.label}</p>
                    <p className="mt-0.5 text-sm font-semibold text-foreground">{m.value}</p>
                  </div>
                ))}
              </div>
            </Panel>
          </div>

          <div className="mt-4 grid gap-4 lg:grid-cols-2">
            <Panel title="Concurrency by hour" description="Peak concurrent sessions during business windows">
              <Bars
                data={workloadHourly}
                xKey="hour"
                height={220}
                bars={[{ key: 'concurrency', color: 'var(--chart-2)', name: 'Concurrency' }]}
              />
            </Panel>
            <Panel title="Read / write mix" description="Share of operations by hour (%)">
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

function setClassificationState(_: any) {
  return useState<any>(workloadClassificationMock)
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
