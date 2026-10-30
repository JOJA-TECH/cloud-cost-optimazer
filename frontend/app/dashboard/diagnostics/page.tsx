import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { Donut } from '@/components/charts'
import { topQueries, waitCategories } from '@/lib/mock-data'

export default function DiagnosticsPage() {
  return (
    <div>
      <PageHeader
        title="Diagnostics"
        description="Technical detail supporting the recommendation — top operations and wait categories. Denser than the executive views, but kept readable."
        actions={<Badge variant="neutral">Query Store · last 30 days</Badge>}
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <Panel title="Top operations" description="Ranked by aggregate impact" className="lg:col-span-2" bodyClassName="p-0">
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

        <Panel title="Wait categories" description="Where time is actually spent">
          <div className="relative">
            <Donut data={waitCategories} height={180} />
            <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
              <span className="text-lg font-semibold tabular-nums text-foreground">34%</span>
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

      <Panel
        title="How diagnostics support the recommendation"
        className="mt-4"
        note="This technical detail explains the recommendation; it does not, on its own, trigger a capacity change."
      >
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
