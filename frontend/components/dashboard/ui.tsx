import { Info, TrendingDown, TrendingUp, type LucideIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type { EvidenceTone, RiskLevel } from '@/lib/mock-data'

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string
  description?: string
  actions?: React.ReactNode
}) {
  return (
    <div className="mb-5 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-foreground">{title}</h1>
        {description && <p className="mt-1 max-w-2xl text-sm text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  )
}

export function Panel({
  title,
  description,
  action,
  note,
  className,
  bodyClassName,
  children,
}: {
  title?: string
  description?: string
  action?: React.ReactNode
  note?: string
  className?: string
  bodyClassName?: string
  children: React.ReactNode
}) {
  return (
    <section
      className={cn('rounded-xl border border-border bg-card shadow-sm', className)}
    >
      {(title || action) && (
        <header className="flex items-start justify-between gap-3 px-5 pt-4 pb-3">
          <div>
            {title && <h2 className="text-sm font-semibold text-foreground">{title}</h2>}
            {description && (
              <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>
            )}
          </div>
          {action}
        </header>
      )}
      <div className={cn('px-5 pb-5', !title && 'pt-5', bodyClassName)}>{children}</div>
      {note && (
        <div className="flex items-center gap-1.5 border-t border-border bg-muted/40 px-5 py-2 text-xs text-muted-foreground">
          <Info className="size-3.5 shrink-0 text-primary" />
          {note}
        </div>
      )}
    </section>
  )
}

export function KpiCard({
  label,
  value,
  sub,
  delta,
  tone = 'default',
  icon: Icon,
  badge,
}: {
  label: string
  value: string
  sub?: string
  delta?: number
  tone?: 'default' | 'brand' | 'success' | 'warning' | 'danger'
  icon?: LucideIcon
  badge?: React.ReactNode
}) {
  const valueColor =
    tone === 'brand'
      ? 'text-primary'
      : tone === 'success'
        ? 'text-success'
        : tone === 'warning'
          ? 'text-warning'
          : tone === 'danger'
            ? 'text-danger'
            : 'text-foreground'
  return (
    <div className="rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="flex items-center justify-between">
        <p className="text-xs font-medium text-muted-foreground">{label}</p>
        {Icon && <Icon className="size-4 text-muted-foreground" />}
        {badge}
      </div>
      <p className={cn('mt-2 text-2xl font-semibold tracking-tight tabular-nums', valueColor)}>
        {value}
      </p>
      <div className="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
        {typeof delta === 'number' && (
          <span
            className={cn(
              'inline-flex items-center gap-0.5 font-medium',
              delta > 0 ? 'text-warning' : 'text-success',
            )}
          >
            {delta > 0 ? <TrendingUp className="size-3" /> : <TrendingDown className="size-3" />}
            {delta > 0 ? '+' : ''}
            {delta}%
          </span>
        )}
        {sub && <span>{sub}</span>}
      </div>
    </div>
  )
}

export function RiskBadge({ risk }: { risk: RiskLevel }) {
  const map = {
    Low: { variant: 'success' as const, label: 'Low' },
    Medium: { variant: 'warning' as const, label: 'Medium' },
    High: { variant: 'danger' as const, label: 'High' },
    None: { variant: 'neutral' as const, label: '—' },
  }
  const cfg = map[risk]
  return <Badge variant={cfg.variant}>{cfg.label} risk</Badge>
}

export function ConfidenceBar({
  value,
  label = 'Confidence',
}: {
  value: number
  label?: string
}) {
  const tone = value >= 80 ? 'var(--chart-1)' : value >= 60 ? 'var(--warning)' : 'var(--danger)'
  return (
    <div>
      <div className="flex items-baseline justify-between">
        <span className="text-xs text-muted-foreground">{label}</span>
        <span className="text-sm font-semibold tabular-nums text-foreground">{value}%</span>
      </div>
      <div className="mt-1.5 h-2 overflow-hidden rounded-full bg-muted">
        <div className="h-full rounded-full transition-all" style={{ width: `${value}%`, background: tone }} />
      </div>
    </div>
  )
}

export function EvidenceDot({ tone }: { tone: EvidenceTone }) {
  const color =
    tone === 'good' ? 'bg-success' : tone === 'warn' ? 'bg-warning' : 'bg-danger'
  return <span className={cn('size-1.5 rounded-full', color)} />
}

export function toneText(tone: EvidenceTone) {
  return tone === 'good' ? 'text-success' : tone === 'warn' ? 'text-warning' : 'text-danger'
}
