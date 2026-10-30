'use client'

import { AreaTrend, Bars, Donut } from '@/components/charts'
import { Badge } from '@/components/ui/badge'
import { costDaily, waitCategories, workloadHourly } from '@/lib/mock-data'

const scenarioBars = [
  { label: 'Current', cost: 486 },
  { label: 'Cand. A', cost: 368 },
  { label: 'Cand. B', cost: 276 },
]

export function ShowcaseCharts() {
  return (
    <div className="grid gap-3 lg:grid-cols-3">
      <Panel title="Cumulative cost" tag="billed" className="lg:col-span-2">
        <AreaTrend data={costDaily} dataKey="cumulative" prefix="$" height={150} />
      </Panel>

      <Panel title="Risk & confidence">
        <div className="flex h-[150px] flex-col justify-center gap-4">
          <Gauge label="Confidence" value={91} tone="var(--chart-1)" note="High coverage" />
          <div className="flex items-center justify-between rounded-lg border border-border bg-card px-3 py-2.5">
            <span className="text-xs text-muted-foreground">Risk</span>
            <Badge variant="success">Low</Badge>
          </div>
        </div>
      </Panel>

      <Panel title="Workload intensity by hour">
        <Bars
          data={workloadHourly}
          xKey="hour"
          height={150}
          unit="%"
          bars={[{ key: 'intensity', color: 'var(--chart-2)', name: 'Intensity' }]}
        />
      </Panel>

      <Panel title="Scenario cost">
        <Bars
          data={scenarioBars}
          xKey="label"
          height={150}
          prefix="$"
          bars={[{ key: 'cost', color: 'var(--chart-1)', name: 'Est. cost' }]}
        />
      </Panel>

      <Panel title="Wait categories">
        <div className="relative h-[150px]">
          <Donut data={waitCategories} height={150} />
        </div>
      </Panel>
    </div>
  )
}

function Panel({
  title,
  tag,
  className,
  children,
}: {
  title: string
  tag?: string
  className?: string
  children: React.ReactNode
}) {
  return (
    <div className={`rounded-xl border border-border bg-card p-3 ${className ?? ''}`}>
      <div className="mb-1 flex items-center justify-between">
        <p className="text-xs font-medium text-foreground">{title}</p>
        {tag && <span className="text-[10px] text-muted-foreground">{tag}</span>}
      </div>
      {children}
    </div>
  )
}

function Gauge({ label, value, tone, note }: { label: string; value: number; tone: string; note: string }) {
  return (
    <div>
      <div className="flex items-baseline justify-between">
        <span className="text-xs text-muted-foreground">{label}</span>
        <span className="text-sm font-semibold tabular-nums text-foreground">{value}%</span>
      </div>
      <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
        <div className="h-full rounded-full" style={{ width: `${value}%`, background: tone }} />
      </div>
      <p className="mt-1 text-[10px] text-muted-foreground">{note}</p>
    </div>
  )
}
