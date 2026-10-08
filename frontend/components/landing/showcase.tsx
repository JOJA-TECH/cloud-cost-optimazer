import Link from 'next/link'
import {
  ArrowRight,
  LayoutDashboard,
  DollarSign,
  Gauge,
  Activity,
  GitCompareArrows,
  CircleCheck,
  TrendingUp,
  Stethoscope,
  History,
  Settings,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { LogoMark } from '@/components/logo'
import { ShowcaseCharts } from './showcase-charts'

const nav = [
  { icon: LayoutDashboard, label: 'Overview', active: true },
  { icon: DollarSign, label: 'Costs' },
  { icon: Gauge, label: 'Performance' },
  { icon: Activity, label: 'Workload' },
  { icon: GitCompareArrows, label: 'Scenarios' },
  { icon: CircleCheck, label: 'Recommendations' },
  { icon: TrendingUp, label: 'Forecasting' },
  { icon: Stethoscope, label: 'Diagnostics' },
  { icon: History, label: 'History' },
  { icon: Settings, label: 'Configuration' },
]

export function Showcase() {
  return (
    <section className="mx-auto max-w-6xl px-4 py-16 sm:px-6 sm:py-20">
      <div className="mx-auto max-w-2xl text-center">
        <span className="text-xs font-semibold uppercase tracking-widest text-primary">
          The product
        </span>
        <h2 className="mt-3 text-balance text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
          See the decision, not just the data.
        </h2>
        <p className="mt-4 text-pretty text-muted-foreground">
          A working analytics surface for Azure SQL: costs, performance percentiles, workload
          classification, scenarios, and traceable recommendations in one place.
        </p>
      </div>

      <div className="mt-10 overflow-hidden rounded-2xl border border-border bg-card shadow-xl shadow-primary/5">
        <div className="flex">
          {/* sidebar */}
          <aside className="hidden w-52 shrink-0 flex-col bg-sidebar p-3 md:flex">
            <div className="flex items-center gap-2 px-2 py-2">
              <LogoMark className="size-6" />
              <span className="text-xs font-semibold text-white">Cost Optimizer</span>
            </div>
            <nav className="mt-3 flex flex-col gap-0.5">
              {nav.map((n) => (
                <span
                  key={n.label}
                  className={`flex items-center gap-2.5 rounded-md px-2.5 py-2 text-xs font-medium ${
                    n.active
                      ? 'bg-sidebar-primary/20 text-white'
                      : 'text-sidebar-foreground'
                  }`}
                >
                  <n.icon className="size-4" />
                  {n.label}
                </span>
              ))}
            </nav>
          </aside>

          {/* content */}
          <div className="min-w-0 flex-1 bg-background">
            <div className="flex items-center gap-3 border-b border-border px-4 py-3">
              <span className="text-sm font-semibold text-foreground">Overview</span>
              <span className="hidden rounded-md border border-border bg-card px-2 py-0.5 text-xs text-muted-foreground sm:inline">
                sql-prod-orders-eastus
              </span>
              <span className="ml-auto flex items-center gap-1.5 text-xs text-muted-foreground">
                <span className="size-1.5 rounded-full bg-success" />
                Updated 8 min ago
              </span>
            </div>
            <div className="p-4">
              <ShowcaseCharts />
            </div>
          </div>
        </div>
      </div>

      <div className="mt-8 flex justify-center">
        <Button size="lg" className="h-11 px-5 text-sm" render={<Link href="/dashboard" />}>
          Open the live dashboard
          <ArrowRight />
        </Button>
      </div>
    </section>
  )
}
