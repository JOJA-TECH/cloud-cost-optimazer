import { Plug, Eye, TrendingUp, SlidersHorizontal, CircleCheck } from 'lucide-react'

const steps = [
  {
    icon: Plug,
    title: 'Connect',
    body: 'Securely connect Azure resources and collect relevant metrics.',
  },
  {
    icon: Eye,
    title: 'Observe',
    body: 'Analyze historical workload, utilization, cost, performance, and configuration.',
  },
  {
    icon: TrendingUp,
    title: 'Forecast',
    body: 'Estimate future workload and identify possible saturation or growth.',
  },
  {
    icon: SlidersHorizontal,
    title: 'Simulate',
    body: 'Compare alternative capacity configurations and their estimated cost/performance impact.',
  },
  {
    icon: CircleCheck,
    title: 'Recommend',
    body: 'Present the best feasible option with savings, risk, confidence, and evidence.',
  },
]

export function HowItWorks() {
  return (
    <section id="how-it-works" className="border-y border-border bg-card">
      <div className="mx-auto max-w-6xl px-4 py-16 sm:px-6 sm:py-20">
        <div className="max-w-2xl">
          <span className="text-xs font-semibold uppercase tracking-widest text-primary">
            How it works
          </span>
          <h2 className="mt-3 text-balance text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
            From raw telemetry to a defensible decision.
          </h2>
        </div>

        <ol className="mt-12 grid gap-6 md:grid-cols-5">
          {steps.map((s, i) => (
            <li key={s.title} className="relative">
              <div className="flex items-center gap-3 md:block">
                <span className="relative z-10 flex size-10 items-center justify-center rounded-xl border border-primary/20 bg-primary/10 text-primary">
                  <s.icon className="size-5" />
                </span>
                {i < steps.length - 1 && (
                  <span className="hidden h-px flex-1 bg-gradient-to-r from-primary/30 to-transparent md:absolute md:left-10 md:top-5 md:block md:w-[calc(100%-2rem)]" />
                )}
              </div>
              <div className="mt-3 flex items-center gap-2">
                <span className="text-xs font-semibold tabular-nums text-primary">
                  {String(i + 1).padStart(2, '0')}
                </span>
                <h3 className="text-sm font-semibold text-foreground">{s.title}</h3>
              </div>
              <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{s.body}</p>
            </li>
          ))}
        </ol>
      </div>
    </section>
  )
}
