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
  estimated: number
  reference: number
  cumulative: number
}

type CostComponent = {
  name: string
  current: number
  estimated: number
  color: string
}

export default function CostsPage() {
  const [kpis, setKpis] = useState<any>(kpisMock)
  const [costDaily, setCostDaily] = useState<CostDay[]>([])
  const [costComponents, setCostComponents] = useState<CostComponent[]>([])
  const [resourceUid, setResourceUid] = useState<string | null>(null)
  const [timeRange, setTimeRangeState] = useState<{ from: string; to: string } | null>(null)
  const [costsLoaded, setCostsLoaded] = useState(false)
  const [costsSource, setCostsSource] = useState<'mock' | 'backend'>('mock')

  useEffect(() => {
    const uid = getSelectedResourceUid()
    const range = getTimeRange()
    setResourceUid(uid)
    setTimeRangeState(range)
  }, [])

  useEffect(() => {
    if (!resourceUid || !timeRange) return

    getCostDaily(resourceUid, timeRange.from, timeRange.to)
      .then((res) => {
        setCostDaily(res.data ?? [])
        setCostsLoaded(!!res.data && res.data.length > 0)
        setCostsSource(res.source)
      })
      .catch(() => setCostsLoaded(false))

    getCostComponents(resourceUid, timeRange.from, timeRange.to)
      .then((res) => {
        setCostComponents(res.data ?? costComponentsMock)
      })
      .catch(() => setCostComponents(costComponentsMock))

    getKpis().then((res) => {
      setKpis(res.data ?? kpisMock)
    })
  }, [resourceUid, timeRange])

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
            <Badge variant="outline" className="text-[9px]">{costsSource}</Badge>
          </div>
        }
      />

      {!costsLoaded ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          <p className="text-sm text-muted-foreground">Datos insuficientes</p>
          <p className="text-xs text-muted-foreground">
            No hay registros de costo para el recurso y ventana seleccionados. Verifique que exista
            datos en cco_data y que CCO_DATA_DSN esté configurado en el backend.
          </p>
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <KpiCard label="Billed cost" value={`$${kpis.currentCost.toFixed(2)}`} sub="this period" delta={kpis.currentCostDelta} icon={DollarSign} />
            <KpiCard label="Reference price" value={`$${(kpis.currentCost * 1.02).toFixed(2)}`} sub="list, same capacity" icon={Tag} />
            <KpiCard label="Estimated cost" value={`$${kpis.estimatedCost.toFixed(2)}`} sub="4 vCores, simulated" tone="brand" icon={Calculator} />
            <KpiCard label="Estimated savings" value={`$${kpis.estimatedSavings.toFixed(2)}`} sub={`${kpis.savingsPct}% / month`} tone="success" icon={TrendingDown} />
          </div>

          <div className="mt-4 grid gap-4 lg:grid-cols-3">
            <Panel
              title="Cost per day"
              description="Billed vs simulated daily cost"
              className="lg:col-span-2"
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
