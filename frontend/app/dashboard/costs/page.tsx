'use client'

import { useEffect, useState } from 'react'
import { DollarSign, Tag, Calculator, TrendingDown } from 'lucide-react'
import { PageHeader, KpiCard, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { AreaTrend, Bars, Donut } from '@/components/charts'
import {
  getKpis,
  getCostDaily,
  getCostComponents,
  getSelectedResourceUid,
  getTimeRange,
  type ServiceResult,
} from '@/lib/api-services'

const valueTypes = [
  { label: 'Billed cost', desc: 'Actual usage billed by Azure', color: 'var(--chart-1)' },
  { label: 'Reference price', desc: 'List price for the same capacity', color: 'var(--chart-4)' },
  { label: 'Estimated cost', desc: 'Modeled cost for a configuration', color: 'var(--chart-2)' },
  { label: 'Simulated / counterfactual', desc: 'Replay of an alternative scenario', color: 'var(--chart-3)' },
]

type CostDay = {
  day: string
  billed: number
  estimated: number | null
  reference: number | null
  cumulative: number
}

type CostComponent = {
  name: string
  current: number
  estimated: number | null
  color: string
}

export default function CostsPage() {
  const [kpisResult, setKpisResult] = useState<ServiceResult<any>>({ data: null, source: null, status: 'loading', error: null })
  const [dailyResult, setDailyResult] = useState<ServiceResult<CostDay[]>>({ data: null, source: null, status: 'loading', error: null })
  const [componentsResult, setComponentsResult] = useState<ServiceResult<CostComponent[]>>({ data: null, source: null, status: 'loading', error: null })
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
      setDailyResult({ data: null, source: null, status: 'empty', error: 'no_resource' })
      setComponentsResult({ data: null, source: null, status: 'empty', error: 'no_resource' })
      return
    }
    setDailyResult({ data: null, source: null, status: 'loading', error: null })
    setComponentsResult({ data: null, source: null, status: 'loading', error: null })
    getCostDaily(resourceUid, timeRange.from, timeRange.to).then(setDailyResult)
    getCostComponents(resourceUid, timeRange.from, timeRange.to).then(setComponentsResult)
  }, [resourceUid, timeRange])

  useEffect(() => {
    getKpis().then((res) => {
      setKpisResult(res)
    })
  }, [])

  const costDaily = dailyResult.data ?? []
  const costComponents = componentsResult.data ?? []
  const donutData = costComponents.map((c) => ({ name: c.name, value: c.current, color: c.color }))

  const compareData = costComponents.map((c) => ({
    label: c.name,
    current: c.current,
    estimated: c.estimated,
  }))

  return (
    <div>
      <PageHeader
        title="Costs"
        description="Current and projected cost, cost components, and simulated savings. Billed, reference, estimated, and counterfactual values are kept distinct throughout."
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="neutral">Last 30 days</Badge>
          </div>
        }
      />

      {dailyResult.status !== 'ready' || !dailyResult.data ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          <p className="text-sm text-muted-foreground">
            {dailyResult.status === 'loading' ? 'Loading costs...' :
              dailyResult.status === 'error' ? 'Could not retrieve costs.' :
              dailyResult.error === 'no_resource' ? 'Select a resource and time window to view costs.' :
              'No cost data for the selected resource and time window.'}
          </p>
          {dailyResult.status === 'error' && <p className="text-xs text-muted-foreground">{dailyResult.error}</p>}
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <KpiCard label="Billed cost" value={kpisResult.data?.currentCost != null ? `$${kpisResult.data.currentCost.toFixed(2)}` : '—'} sub="selected period" delta={kpisResult.data?.currentCostDelta ?? undefined} icon={DollarSign} badge={<Badge variant="outline" className="text-[9px]">{kpisResult.source}</Badge>} />
            <KpiCard label="Reference price" value="—" sub="Not available" icon={Tag} />
            <KpiCard label="Estimated cost" value={kpisResult.data?.estimatedCost != null ? `$${kpisResult.data.estimatedCost.toFixed(2)}` : '—'} sub="Based on recommendation" tone="brand" icon={Calculator} badge={<Badge variant="outline" className="text-[9px]">{kpisResult.source}</Badge>} />
            <KpiCard label="Estimated savings" value={kpisResult.data?.estimatedSavings != null ? `$${kpisResult.data.estimatedSavings.toFixed(2)}` : '—'} sub={kpisResult.data?.savingsPct != null ? `${kpisResult.data.savingsPct}%` : 'No recommendation'} tone="success" icon={TrendingDown} badge={<Badge variant="outline" className="text-[9px]">{kpisResult.source}</Badge>} />
          </div>

          <div className="mt-4 grid gap-4 lg:grid-cols-3">
            <Panel
              title="Cost per day"
              description="Billed vs simulated daily cost"
              className="lg:col-span-2"
              action={dailyResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}
            >
              <div className="mb-3 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                <Legend color="var(--chart-1)" label="Billed" />
                {dailyResult.source === 'mock' && <Legend color="var(--chart-2)" label="Estimated (simulated)" />}
              </div>
              <Bars
                data={costDaily}
                xKey="day"
                prefix="$"
                height={240}
                bars={[
                  { key: 'billed', color: 'var(--chart-1)', name: 'Billed' },
                  ...(dailyResult.source === 'mock' ? [{ key: 'estimated', color: 'var(--chart-2)', name: 'Estimated' }] : []),
                ]}
              />
            </Panel>

            <Panel title="Cost distribution" description="Billed cost by component" action={componentsResult.status === 'ready' && componentsResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}>
              {componentsResult.status !== 'ready' || !componentsResult.data ? (
                <p className="text-xs text-muted-foreground">
                  {componentsResult.status === 'loading' ? 'Loading components...' :
                    componentsResult.status === 'error' ? `Could not retrieve components: ${componentsResult.error ?? 'Unknown error'}` :
                    'No cost components available.'}
                </p>
              ) : (
              <>
              <div className="relative">
                <Donut data={donutData} height={200} unit="" />
                <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
                  <span className="text-lg font-semibold tabular-nums text-foreground">${costComponents.reduce((s, c) => s + c.current, 0).toFixed(0)}</span>
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
              </>
              )}
            </Panel>
          </div>

          <div className="mt-4 grid gap-4 lg:grid-cols-3">
            <Panel
              title="Cumulative cost trend"
              description="Running billed total across the window"
              className="lg:col-span-2"
              action={dailyResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : <Badge variant="neutral">Billed</Badge>}
            >
              <AreaTrend data={costDaily} dataKey="cumulative" prefix="$" height={230} />
            </Panel>

            <Panel title="Current vs recommended" description="Estimated cost by component" action={componentsResult.status === 'ready' && componentsResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}>
              {componentsResult.status !== 'ready' || !componentsResult.data || componentsResult.source !== 'mock' ? (
                <p className="text-xs text-muted-foreground">No per-component estimates have been calculated by the backend.</p>
              ) : (
              <>
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
              </>
              )}
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

const costComponentsMock: CostComponent[] = [
  { name: 'Compute (vCores)', current: 372.4, estimated: 254.9, color: 'var(--chart-1)' },
  { name: 'Storage', current: 68.2, estimated: 68.2, color: 'var(--chart-2)' },
  { name: 'Backup / PITR', current: 31.6, estimated: 31.6, color: 'var(--chart-3)' },
  { name: 'I/O & egress', current: 14.0, estimated: 13.7, color: 'var(--chart-4)' },
]
