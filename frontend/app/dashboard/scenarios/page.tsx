'use client'

import { useState } from 'react'
import Link from 'next/link'
import { ArrowRight, FileSearch, Play, CheckCircle2, AlertTriangle } from 'lucide-react'
import { PageHeader, Panel, RiskBadge } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Bars } from '@/components/charts'
import { cn } from '@/lib/utils'
import { scenarios } from '@/lib/mock-data'

const rows: { key: string; label: string; render: (s: (typeof scenarios)[number]) => React.ReactNode }[] = [
  { key: 'capacity', label: 'Capacity', render: (s) => s.capacity },
  { key: 'cost', label: 'Estimated cost', render: (s) => `$${s.cost}` },
  { key: 'savings', label: 'Savings', render: (s) => (s.savings ? `${s.savings}%` : '—') },
  { key: 'cpu', label: 'CPU expected', render: (s) => `${s.cpuP99}% p99` },
  { key: 'latency', label: 'Latency', render: (s) => s.latency },
  { key: 'risk', label: 'Risk', render: (s) => (s.risk === 'None' ? '—' : <RiskBadge risk={s.risk} />) },
  {
    key: 'sla',
    label: 'SLA',
    render: (s) => (
      <Badge variant={s.sla === 'Pass' ? 'success' : s.sla === 'Warning' ? 'warning' : 'danger'}>
        {s.sla}
      </Badge>
    ),
  },
  { key: 'confidence', label: 'Confidence', render: (s) => (s.confidence ? `${s.confidence}%` : '—') },
]

const costBars = scenarios.map((s) => ({ label: s.label, cost: s.cost }))

export default function ScenariosPage() {
  const [selected, setSelected] = useState('a')

  return (
    <div>
      <PageHeader
        title="Scenario analysis"
        description="Compare alternative Azure SQL configurations and their estimated cost, performance, and risk before deciding. Infeasible candidates are never promoted to a recommendation."
      />

      <Panel bodyClassName="p-0" className="overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[640px] text-sm">
            <thead>
              <tr className="border-b border-border">
                <th className="px-5 py-3 text-left text-xs font-medium text-muted-foreground">Metric</th>
                {scenarios.map((s) => (
                  <th key={s.id} className="px-5 py-3 text-left">
                    <button
                      onClick={() => s.id !== 'current' && setSelected(s.id)}
                      className={cn(
                        'flex flex-col items-start gap-1 rounded-md px-2 py-1.5 text-left transition-colors',
                        s.id !== 'current' && 'hover:bg-muted',
                        selected === s.id && s.id !== 'current' && 'bg-primary/10',
                      )}
                    >
                      <span className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
                        {s.label}
                        {s.recommended && <CheckCircle2 className="size-3.5 text-success" />}
                        {s.id === 'b' && <AlertTriangle className="size-3.5 text-warning" />}
                      </span>
                      <span className="text-xs font-normal text-muted-foreground">{s.capacity}</span>
                    </button>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.key} className="border-b border-border last:border-0">
                  <td className="px-5 py-3 text-xs font-medium text-muted-foreground">{row.label}</td>
                  {scenarios.map((s) => (
                    <td
                      key={s.id}
                      className={cn(
                        'px-5 py-3 text-sm tabular-nums text-foreground',
                        selected === s.id && s.id !== 'current' && 'bg-primary/[0.04]',
                        s.id === 'b' && (row.key === 'sla' || row.key === 'cpu') && 'font-medium',
                      )}
                    >
                      {row.render(s)}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Panel title="Estimated monthly cost" description="Lower is cheaper — but not always feasible" className="lg:col-span-2">
          <Bars data={costBars} xKey="label" prefix="$" height={230} bars={[{ key: 'cost', color: 'var(--chart-1)', name: 'Est. cost' }]} />
        </Panel>

        <Panel title="Selected scenario">
          {(() => {
            const s = scenarios.find((x) => x.id === selected)!
            return (
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-semibold text-foreground">{s.label}</p>
                    <p className="text-xs text-muted-foreground">{s.capacity}</p>
                  </div>
                  {s.recommended ? (
                    <Badge variant="success">Recommended</Badge>
                  ) : (
                    <Badge variant="warning">Higher risk</Badge>
                  )}
                </div>
                <dl className="space-y-2 text-xs">
                  <Row label="Estimated cost" value={`$${s.cost} / mo`} />
                  <Row label="Savings" value={s.savings ? `${s.savings}%` : '—'} />
                  <Row label="CPU p99 (projected)" value={`${s.cpuP99}%`} />
                  <Row label="Latency impact" value={s.latency} />
                  <Row label="Confidence" value={s.confidence ? `${s.confidence}%` : '—'} />
                </dl>
                {s.id === 'b' && (
                  <div className="flex items-start gap-2 rounded-lg bg-warning/10 p-3 text-xs text-warning">
                    <AlertTriangle className="mt-px size-3.5 shrink-0" />
                    Projected p99 of 81% pushes SLA to Warning. Weaker feasibility — not recommended
                    despite larger nominal savings.
                  </div>
                )}
                <div className="flex flex-col gap-2">
                  <Button size="sm" nativeButton={false} render={<Link href="/dashboard/recommendations" />}>
                    Review recommendation
                    <ArrowRight />
                  </Button>
                  <div className="grid grid-cols-2 gap-2">
                    <Button size="sm" variant="outline">
                      <Play />
                      Simulate
                    </Button>
                    <Button size="sm" variant="outline" nativeButton={false} render={<Link href="/dashboard/recommendations" />}>
                      <FileSearch />
                      Evidence
                    </Button>
                  </div>
                </div>
              </div>
            )
          })()}
        </Panel>
      </div>
    </div>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="font-medium tabular-nums text-foreground">{value}</dd>
    </div>
  )
}
