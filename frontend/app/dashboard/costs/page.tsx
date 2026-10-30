import { DollarSign, Tag, Calculator, TrendingDown } from 'lucide-react'
import { PageHeader, KpiCard, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { AreaTrend, Bars, Donut } from '@/components/charts'
import { costDaily, costComponents } from '@/lib/mock-data'

const valueTypes = [
  { label: 'Billed cost', desc: 'Actual usage billed by Azure', color: 'var(--chart-1)' },
  { label: 'Reference price', desc: 'List price for the same capacity', color: 'var(--chart-4)' },
  { label: 'Estimated cost', desc: 'Modeled cost for a configuration', color: 'var(--chart-2)' },
  { label: 'Simulated / counterfactual', desc: 'Replay of an alternative scenario', color: 'var(--chart-3)' },
]

const compareData = [
  { label: 'Compute', current: 372.4, estimated: 254.9 },
  { label: 'Storage', current: 68.2, estimated: 68.2 },
  { label: 'Backup', current: 31.6, estimated: 31.6 },
  { label: 'I/O', current: 14.0, estimated: 13.7 },
]

const donutData = costComponents.map((c) => ({ name: c.name, value: c.current, color: c.color }))

export default function CostsPage() {
  return (
    <div>
      <PageHeader
        title="Costs"
        description="Current and projected cost, cost components, and simulated savings. Billed, reference, estimated, and counterfactual values are kept distinct throughout."
        actions={<Badge variant="neutral">Last 30 days</Badge>}
      />

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <KpiCard label="Billed cost" value="$486.20" sub="this period" delta={4.8} icon={DollarSign} />
        <KpiCard label="Reference price" value="$495.90" sub="list, same capacity" icon={Tag} />
        <KpiCard label="Estimated cost" value="$368.40" sub="4 vCores, simulated" tone="brand" icon={Calculator} />
        <KpiCard label="Estimated savings" value="$117.80" sub="24.2% / month" tone="success" icon={TrendingDown} />
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Panel
          title="Cost per day"
          description="Billed vs simulated daily cost"
          className="lg:col-span-2"
          note="Simulated values replay the workload against the 4-vCore configuration. They are not billed amounts."
        >
          <div className="mb-3 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <Legend color="var(--chart-1)" label="Billed" />
            <Legend color="var(--chart-2)" label="Estimated (simulated)" />
          </div>
          <Bars
            data={costDaily}
            xKey="day"
            prefix="$"
            height={240}
            bars={[
              { key: 'billed', color: 'var(--chart-1)', name: 'Billed' },
              { key: 'estimated', color: 'var(--chart-2)', name: 'Estimated' },
            ]}
          />
        </Panel>

        <Panel title="Cost distribution" description="Billed cost by component">
          <div className="relative">
            <Donut data={donutData} height={200} unit="" />
            <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
              <span className="text-lg font-semibold tabular-nums text-foreground">$486</span>
              <span className="text-[11px] text-muted-foreground">/ month</span>
            </div>
          </div>
          <ul className="mt-3 space-y-2">
            {costComponents.map((c) => (
              <li key={c.name} className="flex items-center gap-2 text-xs">
                <span className="size-2 rounded-full" style={{ background: c.color }} />
                <span className="text-muted-foreground">{c.name}</span>
                <span className="ml-auto font-medium tabular-nums text-foreground">
                  ${c.current.toFixed(1)}
                </span>
              </li>
            ))}
          </ul>
        </Panel>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Panel
          title="Cumulative cost trend"
          description="Running billed total across the window"
          className="lg:col-span-2"
          action={<Badge variant="neutral">Billed</Badge>}
        >
          <AreaTrend data={costDaily} dataKey="cumulative" prefix="$" height={230} />
        </Panel>

        <Panel title="Current vs recommended" description="Estimated cost by component">
          <div className="mb-3 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
            <Legend color="var(--chart-1)" label="Current" />
            <Legend color="var(--chart-2)" label="Recommended" />
          </div>
          <Bars
            data={compareData}
            xKey="label"
            prefix="$"
            height={220}
            bars={[
              { key: 'current', color: 'var(--chart-1)', name: 'Current' },
              { key: 'estimated', color: 'var(--chart-2)', name: 'Recommended' },
            ]}
          />
        </Panel>
      </div>

      <Panel title="Value types" description="These must never be read as interchangeable" className="mt-4">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {valueTypes.map((v) => (
            <div key={v.label} className="rounded-lg border border-border bg-background p-3">
              <div className="flex items-center gap-2">
                <span className="size-2.5 rounded-sm" style={{ background: v.color }} />
                <p className="text-xs font-semibold text-foreground">{v.label}</p>
              </div>
              <p className="mt-1.5 text-xs text-muted-foreground">{v.desc}</p>
            </div>
          ))}
        </div>
      </Panel>
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
