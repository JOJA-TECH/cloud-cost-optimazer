import { Activity, BadgeDollarSign, GitCompareArrows, ShieldAlert, FileSearch } from 'lucide-react'

const features = [
  {
    icon: Activity,
    title: 'Workload-aware analysis',
    body: 'Analyze how demand behaves over time — peaks, percentiles, variability, and trend — instead of relying on averages alone.',
  },
  {
    icon: BadgeDollarSign,
    title: 'Cost intelligence',
    body: 'Understand current cost, projected cost, cost components, and simulated savings, with billed and counterfactual values kept distinct.',
  },
  {
    icon: GitCompareArrows,
    title: 'Scenario simulation',
    body: 'Compare alternative Azure SQL configurations and their estimated cost and performance impact before making a decision.',
  },
  {
    icon: ShieldAlert,
    title: 'Risk-aware recommendations',
    body: 'Evaluate peaks, variability, trend, headroom, SLA history, and uncertainty — risk and confidence are shown next to every saving.',
  },
  {
    icon: FileSearch,
    title: 'Evidence-backed decisions',
    body: 'Every recommendation carries quantitative evidence and traceable reasoning you can inspect and audit later.',
  },
]

export function Features() {
  return (
    <section className="mx-auto max-w-6xl px-4 py-16 sm:px-6 sm:py-20">
      <div className="max-w-2xl">
        <span className="text-xs font-semibold uppercase tracking-widest text-primary">
          What the platform does
        </span>
        <h2 className="mt-3 text-balance text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
          Optimize cost without blindly trading away performance.
        </h2>
      </div>

      <div className="mt-10 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {features.map((f) => (
          <div
            key={f.title}
            className="group rounded-xl border border-border bg-card p-5 transition-colors hover:border-primary/30"
          >
            <span className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <f.icon className="size-4.5" />
            </span>
            <h3 className="mt-4 text-sm font-semibold text-foreground">{f.title}</h3>
            <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{f.body}</p>
          </div>
        ))}
        <div className="flex flex-col justify-center rounded-xl border border-dashed border-border bg-muted/40 p-5">
          <p className="text-sm font-medium text-foreground">Decision, not just data.</p>
          <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">
            Data → Insight → Scenario → Risk → Recommendation. The whole chain stays visible end to
            end.
          </p>
        </div>
      </div>
    </section>
  )
}
