'use client'

import { Fragment, useState } from 'react'
import { ChevronDown, ArrowRight } from 'lucide-react'
import { PageHeader, Panel, RiskBadge } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { history, type HistoryStatus } from '@/lib/mock-data'

const statuses: (HistoryStatus | 'All')[] = ['All', 'New', 'Reviewed', 'Accepted', 'Rejected', 'Archived']

const statusVariant: Record<HistoryStatus, 'default' | 'neutral' | 'success' | 'warning' | 'danger'> = {
  New: 'default',
  Reviewed: 'neutral',
  Accepted: 'success',
  Rejected: 'danger',
  Archived: 'neutral',
}

export default function HistoryPage() {
  const [filter, setFilter] = useState<(typeof statuses)[number]>('All')
  const [open, setOpen] = useState<string | null>('rec-2041')

  const rows = history.filter((h) => filter === 'All' || h.status === filter)

  return (
    <div>
      <PageHeader
        title="History"
        description="Every recommendation is traceable and auditable — with the configuration, savings, risk, confidence, status, and the model/rule version used at the time."
      />

      <div className="mb-4 flex flex-wrap items-center gap-1.5">
        {statuses.map((s) => (
          <button
            key={s}
            onClick={() => setFilter(s)}
            className={cn(
              'rounded-md border px-2.5 py-1 text-xs font-medium transition-colors',
              filter === s
                ? 'border-primary bg-primary text-primary-foreground'
                : 'border-border bg-card text-muted-foreground hover:text-foreground',
            )}
          >
            {s}
          </button>
        ))}
      </div>

      <Panel bodyClassName="p-0" className="overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] text-sm">
            <thead>
              <tr className="border-b border-border text-xs text-muted-foreground">
                <th className="px-5 py-2.5 text-left font-medium">Date</th>
                <th className="px-3 py-2.5 text-left font-medium">Resource</th>
                <th className="px-3 py-2.5 text-left font-medium">Change</th>
                <th className="px-3 py-2.5 text-right font-medium">Savings</th>
                <th className="px-3 py-2.5 text-left font-medium">Risk</th>
                <th className="px-3 py-2.5 text-right font-medium">Conf.</th>
                <th className="px-3 py-2.5 text-left font-medium">Status</th>
                <th className="px-5 py-2.5" />
              </tr>
            </thead>
            <tbody>
              {rows.map((h) => (
                <Fragment key={h.id}>
                  <tr
                    className="cursor-pointer border-b border-border hover:bg-muted/40"
                    onClick={() => setOpen(open === h.id ? null : h.id)}
                  >
                    <td className="px-5 py-3 tabular-nums text-muted-foreground">{h.date}</td>
                    <td className="px-3 py-3 font-mono text-xs text-foreground">{h.resource}</td>
                    <td className="px-3 py-3">
                      <span className="flex items-center gap-1.5 text-foreground">
                        {h.current}
                        <ArrowRight className="size-3 text-primary" />
                        {h.proposed}
                      </span>
                    </td>
                    <td className="px-3 py-3 text-right tabular-nums text-foreground">{h.savings}</td>
                    <td className="px-3 py-3">{h.risk === 'None' ? <span className="text-muted-foreground">—</span> : <RiskBadge risk={h.risk} />}</td>
                    <td className="px-3 py-3 text-right tabular-nums text-muted-foreground">{h.confidence}%</td>
                    <td className="px-3 py-3">
                      <Badge variant={statusVariant[h.status]}>{h.status}</Badge>
                    </td>
                    <td className="px-5 py-3 text-right">
                      <ChevronDown className={cn('inline size-4 text-muted-foreground transition-transform', open === h.id && 'rotate-180')} />
                    </td>
                  </tr>
                  {open === h.id && (
                    <tr className="border-b border-border bg-muted/30">
                      <td colSpan={8} className="px-5 py-4">
                        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                          <Detail label="Recommendation ID" value={h.id} mono />
                          <Detail label="Model / rule version" value={h.version} mono />
                          <Detail label="Analysis period" value="Last 30 days" />
                          <Detail label="Evidence snapshot" value="Frozen at generation" />
                        </div>
                        <p className="mt-3 text-xs text-muted-foreground">
                          Opening a past recommendation replays the evidence exactly as it was when
                          generated — CPU percentiles, headroom, SLA history, and cost source — so
                          the decision remains reproducible and auditable.
                        </p>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>
    </div>
  )
}

function Detail({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2">
      <p className="text-[11px] text-muted-foreground">{label}</p>
      <p className={cn('mt-0.5 text-xs font-medium text-foreground', mono && 'font-mono')}>{value}</p>
    </div>
  )
}
