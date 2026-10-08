import Link from 'next/link'
import { ArrowRight, Database, LineChart, TrendingUp, SlidersHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DashboardPreview } from './dashboard-preview'

const chips = [
  { icon: Database, label: 'Azure SQL Database' },
  { icon: LineChart, label: 'Workload Analytics' },
  { icon: TrendingUp, label: 'Forecasting' },
  { icon: SlidersHorizontal, label: 'Right-sizing' },
]

export function Hero() {
  return (
    <section id="product" className="relative overflow-hidden">
      <div
        className="pointer-events-none absolute inset-x-0 top-0 h-[420px] opacity-70"
        style={{
          background:
            'radial-gradient(60% 100% at 50% 0%, color-mix(in oklab, var(--primary) 12%, transparent), transparent)',
        }}
      />
      <div className="mx-auto max-w-6xl px-4 pt-14 pb-10 sm:px-6 sm:pt-20 lg:pt-24">
        <div className="mx-auto max-w-3xl text-center">
          <span className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1 text-xs font-medium text-muted-foreground">
            <span className="size-1.5 rounded-full bg-success" />
            Release 1.0 · Focused on Azure SQL Database
          </span>
          <h1 className="mt-5 text-balance text-4xl font-semibold tracking-tight text-foreground sm:text-5xl lg:text-[3.4rem] lg:leading-[1.05]">
            Optimize Azure SQL costs with{' '}
            <span className="text-primary">workload-aware intelligence</span>.
          </h1>
          <p className="mx-auto mt-5 max-w-2xl text-pretty text-base text-muted-foreground sm:text-lg">
            Understand what your database is doing, what it costs, what is likely to happen next, and
            which capacity option delivers the best balance between cost, performance, and risk.
          </p>
          <div className="mt-7 flex flex-col items-center justify-center gap-3 sm:flex-row">
            <Button size="lg" className="h-11 px-5 text-sm" render={<Link href="/dashboard" />}>
              Explore the platform
              <ArrowRight />
            </Button>
            <Button
              size="lg"
              variant="outline"
              className="h-11 px-5 text-sm"
              render={<Link href="#how-it-works" />}
            >
              See how it works
            </Button>
          </div>
          <div className="mt-8 flex flex-wrap items-center justify-center gap-2">
            {chips.map((c) => (
              <span
                key={c.label}
                className="inline-flex items-center gap-1.5 rounded-full border border-border bg-card/60 px-3 py-1 text-xs text-muted-foreground"
              >
                <c.icon className="size-3.5 text-primary" />
                {c.label}
              </span>
            ))}
          </div>
        </div>

        <div className="relative mt-12 sm:mt-16">
          <DashboardPreview />
        </div>
      </div>
    </section>
  )
}
