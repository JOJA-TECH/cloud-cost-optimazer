'use client'

import { ArrowDownRight, CircleCheck, ShieldCheck, TrendingDown } from 'lucide-react'
import { AreaTrend, MultiLine } from '@/components/charts'
import { Badge } from '@/components/ui/badge'
import { costDaily, perfSeries } from '@/lib/mock-data'

export function DashboardPreview() {
  return (
    <div className="overflow-hidden rounded-2xl border border-border bg-card shadow-xl shadow-primary/5">
      {/* window chrome */}
      <div className="flex items-center gap-2 border-b border-border bg-muted/50 px-4 py-2.5">
        <span className="size-2.5 rounded-full bg-border" />
        <span className="size-2.5 rounded-full bg-border" />
        <span className="size-2.5 rounded-full bg-border" />
        <div className="ml-3 flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">Overview</span>
          <span>·</span>
          <span>sql-prod-orders-eastus</span>
        </div>
        <Badge variant="neutral" className="ml-auto hidden sm:inline-flex">
          Azure SQL · East US
        </Badge>
      </div>

      <div className="grid gap-3 p-3 sm:p-4">
        {/* KPI strip */}
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <MiniKpi label="Current cost" value="$486.20" sub="+4.8% MoM" tone="muted" />
          <MiniKpi label="Estimated cost" value="$368.40" sub="/ month" tone="brand" />
          <MiniKpi
            label="Savings"
            value="24.2%"
            sub="$117.80 / mo"
            tone="success"
            icon={<TrendingDown className="size-3.5" />}
          />
          <MiniKpi label="Confidence" value="91%" sub="Low risk" tone="muted" />
        </div>

        <div className="grid gap-3 lg:grid-cols-5">
          {/* recommendation card */}
          <div className="rounded-xl border border-border bg-background p-4 lg:col-span-2">
            <div className="mb-3 flex items-center gap-2">
              <span className="flex size-7 items-center justify-center rounded-lg bg-success/10 text-success">
                <CircleCheck className="size-4" />
              </span>
              <div>
                <p className="text-xs font-medium text-foreground">Recommendation</p>
                <p className="text-[11px] text-muted-foreground">Right-size compute</p>
              </div>
              <Badge variant="success" className="ml-auto">
                Low risk
              </Badge>
            </div>
            <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
              8 vCores
              <ArrowDownRight className="size-4 text-primary" />
              4 vCores
            </div>
            <div className="mt-3 space-y-1.5 text-[11px]">
              <EvidenceRow label="CPU p95" value="28%" />
              <EvidenceRow label="CPU p99" value="41%" />
              <EvidenceRow label="Trend" value="Stable" />
              <EvidenceRow label="Headroom" value="Sufficient" />
            </div>
            <div className="mt-3 flex items-center gap-1.5 rounded-md bg-muted px-2 py-1.5 text-[11px] text-muted-foreground">
              <ShieldCheck className="size-3.5 text-primary" />
              Backed by 30 days of workload evidence
            </div>
          </div>

          {/* charts */}
          <div className="grid gap-3 lg:col-span-3">
            <div className="rounded-xl border border-border bg-background p-3">
              <div className="mb-1 flex items-center justify-between">
                <p className="text-xs font-medium text-foreground">CPU utilization</p>
                <div className="flex items-center gap-2 text-[10px] text-muted-foreground">
                  <Legend color="var(--chart-3)" label="p50" />
                  <Legend color="var(--chart-2)" label="p95" />
                  <Legend color="var(--chart-1)" label="p99" />
                </div>
              </div>
              <MultiLine
                height={116}
                data={perfSeries.slice(0, 24)}
                xKey="t"
                yDomain={[0, 100]}
                unit="%"
                lines={[
                  { key: 'p50', color: 'var(--chart-3)', name: 'p50' },
                  { key: 'p95', color: 'var(--chart-2)', name: 'p95' },
                  { key: 'p99', color: 'var(--chart-1)', name: 'p99' },
                ]}
              />
            </div>
            <div className="rounded-xl border border-border bg-background p-3">
              <div className="mb-1 flex items-center justify-between">
                <p className="text-xs font-medium text-foreground">Estimated daily cost</p>
                <span className="text-[10px] text-muted-foreground">simulated</span>
              </div>
              <AreaTrend
                height={92}
                data={costDaily}
                dataKey="estimated"
                prefix="$"
                color="var(--chart-1)"
                showAxes={false}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function MiniKpi({
  label,
  value,
  sub,
  tone,
  icon,
}: {
  label: string
  value: string
  sub: string
  tone: 'muted' | 'brand' | 'success'
  icon?: React.ReactNode
}) {
  const valueTone =
    tone === 'success' ? 'text-success' : tone === 'brand' ? 'text-primary' : 'text-foreground'
  return (
    <div className="rounded-xl border border-border bg-background p-3">
      <p className="text-[11px] text-muted-foreground">{label}</p>
      <p className={`mt-1 flex items-center gap-1 text-lg font-semibold tabular-nums ${valueTone}`}>
        {icon}
        {value}
      </p>
      <p className="text-[10px] text-muted-foreground">{sub}</p>
    </div>
  )
}

function EvidenceRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium tabular-nums text-foreground">{value}</span>
    </div>
  )
}

function Legend({ color, label }: { color: string; label: string }) {
  return (
    <span className="flex items-center gap-1">
      <span className="size-1.5 rounded-full" style={{ background: color }} />
      {label}
    </span>
  )
}
