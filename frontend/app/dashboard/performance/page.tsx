'use client'

import { useState } from 'react'
import { PageHeader, Panel, toneText } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { AreaTrend, MultiLine } from '@/components/charts'
import { perfSeries, perfSummary, reliabilityEvents } from '@/lib/mock-data'

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

export default function PerformancePage() {
  const [hours, setHours] = useState(72)
  const [active, setActive] = useState<Record<string, boolean>>({ p50: true, p95: true, p99: true })

  const data = perfSeries.slice(-hours)
  const shownLines = cpuLines.filter((l) => active[l.key])

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
                className={cn(
                  'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                  hours === r.hours ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {r.label}
              </button>
            ))}
          </div>
        }
      />

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-8">
        {perfSummary.map((m) => (
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
        action={
          <div className="flex items-center gap-1">
            {cpuLines.map((l) => (
              <button
                key={l.key}
                onClick={() => setActive((s) => ({ ...s, [l.key]: !s[l.key] }))}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium transition-colors',
                  active[l.key]
                    ? 'border-border bg-background text-foreground'
                    : 'border-transparent text-muted-foreground opacity-50',
                )}
              >
                <span className="size-2 rounded-full" style={{ background: l.color }} />
                {l.name.replace('CPU ', '')}
              </button>
            ))}
          </div>
        }
      >
        <MultiLine
          data={data}
          xKey="t"
          yDomain={[0, 100]}
          unit="%"
          height={300}
          lines={shownLines.length ? shownLines : cpuLines}
          thresholds={[
            { y: 75, color: 'var(--warning)', label: 'warning' },
            { y: 90, color: 'var(--danger)', label: 'critical' },
          ]}
        />
      </Panel>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Panel title="I/O utilization" description="Percent of provisioned throughput">
          <AreaTrend data={data} dataKey="io" xKey="t" unit="%" color="var(--chart-2)" height={190} />
        </Panel>
        <Panel title="Active sessions" description="Concurrent sessions (max 2400)">
          <AreaTrend data={data} dataKey="sessions" xKey="t" color="var(--chart-1)" height={190} />
        </Panel>
        <Panel title="Query latency" description="Average response time (ms)">
          <AreaTrend data={data} dataKey="latency" xKey="t" unit=" ms" color="var(--chart-3)" height={190} />
        </Panel>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Panel title="Memory utilization" description="Working set as percent of provisioned">
          <AreaTrend data={data} dataKey="memory" xKey="t" unit="%" color="var(--chart-5)" height={190} />
        </Panel>
        <Panel title="Reliability events" description="Errors, timeouts, and contention (30d)">
          <div className="grid grid-cols-2 gap-3">
            {reliabilityEvents.map((e) => (
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
    </div>
  )
}
