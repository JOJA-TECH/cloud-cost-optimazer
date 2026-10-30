import Link from 'next/link'
import { CheckCircle2, DollarSign, TrendingDown, Calculator, ShieldCheck, Gauge, ArrowRight } from 'lucide-react'
import { PageHeader, KpiCard, Panel, ConfidenceBar } from '@/components/dashboard/ui'
import { RecommendationSummary } from '@/components/dashboard/recommendation-summary'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { AreaTrend, MultiLine } from '@/components/charts'
import { kpis, costDaily, perfSeries, dataQuality } from '@/lib/mock-data'

export default function OverviewPage() {
  return (
    <div>
      <PageHeader
        title="Overview"
        description="Current state, cost, utilization, risk, and the optimization opportunity for the selected Azure SQL database — with the evidence behind it."
        actions={
          <Badge variant="success" className="h-7 px-2.5">
            <CheckCircle2 className="size-3.5" />
            {kpis.status}
          </Badge>
        }
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 xl:grid-cols-6">
        <KpiCard label="Resource status" value="Healthy" sub="Azure SQL Database" tone="success" icon={CheckCircle2} />
        <KpiCard label="Current cost" value="$486.20" sub="/ month" delta={kpis.currentCostDelta} icon={DollarSign} />
        <KpiCard label="Estimated cost" value="$368.40" sub="simulated / mo" tone="brand" icon={Calculator} />
        <KpiCard label="Estimated savings" value="24.2%" sub="$117.80 / mo" tone="success" icon={TrendingDown} />
        <KpiCard label="Risk" value="Low" sub="within constraints" tone="success" icon={ShieldCheck} />
        <KpiCard label="Confidence" value="91%" sub="high coverage" tone="brand" icon={Gauge} />
      </div>

      <div className="mt-4">
        <RecommendationSummary />
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Panel
          title="Cumulative cost"
          description="Billed usage across the analysis window"
          className="lg:col-span-2"
          action={<Badge variant="neutral">Billed</Badge>}
        >
          <AreaTrend data={costDaily} dataKey="cumulative" prefix="$" height={230} />
        </Panel>

        <Panel title="Data quality" description="Signals feeding the analysis">
          <div className="space-y-3">
            <div className="flex items-center justify-between rounded-lg border border-border bg-background px-3 py-2">
              <span className="text-xs text-muted-foreground">Global confidence</span>
              <Badge variant="success">{dataQuality.globalConfidence}</Badge>
            </div>
            {dataQuality.items.slice(0, 5).map((item) => (
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
        </Panel>

        <Panel title="Opportunity snapshot" description="What a right-size would change">
          <div className="space-y-4">
            <div className="rounded-lg border border-border bg-background p-3">
              <p className="text-xs text-muted-foreground">Compute</p>
              <p className="mt-1 flex items-center gap-2 text-sm font-semibold text-foreground">
                8 vCores <ArrowRight className="size-3.5 text-primary" /> <span className="text-primary">4 vCores</span>
              </p>
            </div>
            <ConfidenceBar value={91} />
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
