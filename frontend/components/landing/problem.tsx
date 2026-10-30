import { AlertTriangle, ArrowRight, Check, X } from 'lucide-react'
import { NaiveVsAwareChart } from './problem-chart'

const naiveSteps = ['Average CPU', 'Reduce capacity']
const awareSteps = [
  'Historical behavior',
  'Peaks',
  'Percentiles',
  'Trend',
  'Forecast',
  'Scenarios',
  'Risk',
  'Recommendation',
]

export function Problem() {
  return (
    <section id="insights" className="border-y border-border bg-card">
      <div className="mx-auto max-w-6xl px-4 py-16 sm:px-6 sm:py-20">
        <div className="mx-auto max-w-2xl text-center">
          <span className="text-xs font-semibold uppercase tracking-widest text-primary">
            The core problem
          </span>
          <h2 className="mt-3 text-balance text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
            Low average utilization does not always mean you can safely scale down.
          </h2>
          <p className="mt-4 text-pretty text-muted-foreground">
            A database can average low CPU yet still hit frequent peaks, trend upward, and leave
            little headroom. Averaging those signals away is how right-sizing quietly breaks SLAs.
          </p>
        </div>

        <div className="mt-12 grid gap-6 lg:grid-cols-5">
          <div className="lg:col-span-3">
            <div className="rounded-xl border border-border bg-background p-5">
              <div className="mb-4 flex items-center justify-between">
                <p className="text-sm font-semibold text-foreground">
                  Same average CPU, very different risk
                </p>
                <span className="inline-flex items-center gap-1.5 rounded-md bg-warning/12 px-2 py-1 text-xs font-medium text-warning">
                  <AlertTriangle className="size-3.5" />
                  Peaks hidden by the mean
                </span>
              </div>
              <NaiveVsAwareChart />
              <p className="mt-3 text-xs text-muted-foreground">
                The dashed line is the 22% average. The p95 and p99 bands regularly exceed it — that
                is the demand a right-sizing decision actually has to survive.
              </p>
            </div>
          </div>

          <div className="grid gap-4 lg:col-span-2">
            <div className="rounded-xl border border-danger/25 bg-danger/[0.04] p-5">
              <div className="mb-3 flex items-center gap-2">
                <span className="flex size-6 items-center justify-center rounded-md bg-danger/10 text-danger">
                  <X className="size-3.5" />
                </span>
                <p className="text-sm font-semibold text-foreground">Naive optimization</p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {naiveSteps.map((s, i) => (
                  <span key={s} className="flex items-center gap-2">
                    <span className="rounded-md border border-border bg-card px-2.5 py-1 text-xs text-foreground">
                      {s}
                    </span>
                    {i < naiveSteps.length - 1 && (
                      <ArrowRight className="size-3.5 text-muted-foreground" />
                    )}
                  </span>
                ))}
              </div>
              <p className="mt-3 text-xs text-muted-foreground">
                Collapses everything into one number and hopes the peaks never matter.
              </p>
            </div>

            <div className="rounded-xl border border-primary/25 bg-primary/[0.04] p-5">
              <div className="mb-3 flex items-center gap-2">
                <span className="flex size-6 items-center justify-center rounded-md bg-success/10 text-success">
                  <Check className="size-3.5" />
                </span>
                <p className="text-sm font-semibold text-foreground">Workload-aware optimization</p>
              </div>
              <div className="flex flex-wrap items-center gap-1.5">
                {awareSteps.map((s, i) => (
                  <span key={s} className="flex items-center gap-1.5">
                    <span className="rounded-md border border-primary/20 bg-card px-2 py-1 text-xs text-foreground">
                      {s}
                    </span>
                    {i < awareSteps.length - 1 && (
                      <ArrowRight className="size-3 text-primary/60" />
                    )}
                  </span>
                ))}
              </div>
              <p className="mt-3 text-xs text-muted-foreground">
                Keeps the signal that actually determines whether a smaller tier is safe.
              </p>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
