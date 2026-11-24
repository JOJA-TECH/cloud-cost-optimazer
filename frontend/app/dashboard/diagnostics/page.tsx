'use client'

import { useEffect, useState } from 'react'
import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { Donut } from '@/components/charts'
import {
  getTopQueries,
  getWaitCategories,
  getSelectedResourceUid,
  getTimeRange,
  type ServiceResult,
} from '@/lib/api-services'

export default function DiagnosticsPage() {
  const [topQueries, setTopQueries] = useState<{ id: string; text: string; execs: number; avgCpuMs: number; avgDurMs: number; reads: number }[]>(topQueriesMock)
  const [waitCategories, setWaitCategories] = useState<{ name: string; pct: number; color: string }[]>(waitCategoriesMock)
  const [resourceUid, setResourceUid] = useState<string | null>(null)
  const [timeRange, setTimeRangeState] = useState<{ from: string; to: string } | null>(null)
  const [topQueriesResult, setTopQueriesResult] = useState<ServiceResult<typeof topQueriesMock>>({ data: null, source: null, status: 'loading', error: null })
  const [waitCategoriesResult, setWaitCategoriesResult] = useState<ServiceResult<typeof waitCategoriesMock>>({ data: null, source: null, status: 'loading', error: null })

  useEffect(() => {
    const uid = getSelectedResourceUid()
    const range = getTimeRange()
    setResourceUid(uid)
    setTimeRangeState(range)
  }, [])

  useEffect(() => {
    if (!resourceUid || !timeRange) return

    getTopQueries().then((res) => {
      setTopQueries(res.data ?? topQueriesMock)
      setTopQueriesResult(res)
    })
    getWaitCategories().then((res) => {
      setWaitCategories(res.data ?? waitCategoriesMock)
      setWaitCategoriesResult(res)
    })
  }, [resourceUid, timeRange])

  return (
    <div>
      <PageHeader
        title="Diagnostics"
        description="Technical detail supporting the recommendation — top operations and wait categories. Denser than the executive views, but kept readable."
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="neutral">Query Store · last 30 days</Badge>
            <Badge variant="outline" className="text-[9px]">{topQueriesResult.source}</Badge>
          </div>
        }
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <Panel title="Top operations" description="Ranked by aggregate impact" className="lg:col-span-2" bodyClassName="p-0" action={<Badge variant="outline" className="text-[9px]">{topQueriesResult.source}</Badge>}>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[640px] text-sm">
              <thead>
                <tr className="border-b border-border text-xs text-muted-foreground">
                  <th className="px-5 py-2.5 text-left font-medium">Query</th>
                  <th className="px-3 py-2.5 text-right font-medium">Execs</th>
                  <th className="px-3 py-2.5 text-right font-medium">Avg CPU</th>
                  <th className="px-3 py-2.5 text-right font-medium">Avg dur</th>
                  <th className="px-5 py-2.5 text-right font-medium">Logical reads</th>
                </tr>
              </thead>
              <tbody>
                {topQueries.map((q) => (
                  <tr key={q.id} className="border-b border-border last:border-0 hover:bg-muted/40">
                    <td className="px-5 py-3">
                      <div className="flex items-center gap-2">
                        <Badge variant="neutral" className="font-mono">{q.id}</Badge>
                        <code className="max-w-[240px] truncate font-mono text-xs text-foreground">
                          {q.text}
                        </code>
                      </div>
                    </td>
                    <td className="px-3 py-3 text-right tabular-nums text-muted-foreground">
                      {q.execs.toLocaleString()}
                    </td>
                    <td className="px-3 py-3 text-right tabular-nums text-foreground">{q.avgCpuMs} ms</td>
                    <td className="px-3 py-3 text-right tabular-nums text-foreground">{q.avgDurMs} ms</td>
                    <td className="px-5 py-3 text-right tabular-nums text-muted-foreground">
                      {q.reads.toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Panel>

        <Panel title="Wait categories" description="Where time is actually spent" action={<Badge variant="outline" className="text-[9px]">{waitCategoriesResult.source}</Badge>}>
          <div className="relative">
            <Donut data={waitCategories} height={180} />
            <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
              <span className="text-lg font-semibold tabular-nums text-foreground">{waitCategories[0]?.pct ?? 34}%</span>
              <span className="text-[11px] text-muted-foreground">CPU-bound</span>
            </div>
          </div>
          <ul className="mt-3 space-y-2">
            {waitCategories.map((w) => (
              <li key={w.name} className="flex items-center gap-2 text-xs">
                <span className="size-2 rounded-full" style={{ background: w.color }} />
                <span className="text-muted-foreground">{w.name}</span>
                <span className="ml-auto font-medium tabular-nums text-foreground">{w.pct}%</span>
              </li>
            ))}
          </ul>
        </Panel>
      </div>

      <Panel title="How diagnostics support the recommendation" className="mt-4" note="Technical detail supporting the recommendation — top operations and wait categories. Denser than the executive views, but kept readable.">
        <div className="grid gap-3 sm:grid-cols-3">
          <Insight title="CPU-bound, not I/O-bound" body="34% of waits are CPU, 22% PAGEIOLATCH. vCore reduction stays viable because I/O is not the bottleneck at peak." />
          <Insight title="Concentrated cost" body="Q3 and Q4 dominate CPU per execution. Their pattern is periodic, matching the workload classification." />
          <Insight title="Low contention" body="Lock waits at 16% and only 3 deadlocks in 30 days indicate headroom is real, not masked by blocking." />
        </div>
      </Panel>
    </div>
  )
}

function Insight({ title, body }: { title: string; body: string }) {
  return (
    <div className="rounded-lg border border-border bg-background p-3">
      <p className="text-xs font-semibold text-foreground">{title}</p>
      <p className="mt-1 text-xs text-muted-foreground">{body}</p>
    </div>
  )
}

const topQueriesMock = [
  { id: 'Q1', text: 'SELECT ... FROM orders o JOIN order_items ...', execs: 184203, avgCpuMs: 42, avgDurMs: 61, reads: 1240 },
  { id: 'Q2', text: 'UPDATE inventory SET qty = qty - @n WHERE ...', execs: 96541, avgCpuMs: 18, avgDurMs: 24, reads: 88 },
  { id: 'Q3', text: 'SELECT TOP 100 ... FROM audit_log WHERE ...', execs: 12044, avgCpuMs: 210, avgDurMs: 512, reads: 41200 },
  { id: 'Q4', text: 'EXEC sp_refresh_customer_summary', execs: 2880, avgCpuMs: 640, avgDurMs: 1180, reads: 96400 },
  { id: 'Q5', text: 'INSERT INTO events (…) VALUES (…)', execs: 421900, avgCpuMs: 6, avgDurMs: 9, reads: 12 },
]

const waitCategoriesMock = [
  { name: 'CPU', pct: 34, color: 'var(--chart-1)' },
  { name: 'PAGEIOLATCH', pct: 22, color: 'var(--chart-2)' },
  { name: 'LCK_M_X (locks)', pct: 16, color: 'var(--chart-3)' },
  { name: 'WRITELOG', pct: 14, color: 'var(--chart-4)' },
  { name: 'Network I/O', pct: 8, color: 'var(--chart-5)' },
  { name: 'Other', pct: 6, color: '#cbd5e1' },
]
