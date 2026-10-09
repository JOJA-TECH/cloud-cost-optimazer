import Link from 'next/link'
import { ArrowRight, CircleCheck, FileSearch } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { EvidenceDot, RiskBadge, toneText } from './ui'
import { recommendation } from '@/lib/mock-data'

export function RecommendationSummary() {
  const r = recommendation
  return (
    <div className="rounded-xl border border-border bg-card shadow-sm">
      <div className="flex flex-col gap-4 p-5 lg:flex-row lg:items-start">
        <div className="flex-1">
          <div className="flex items-center gap-2">
            <span className="flex size-8 items-center justify-center rounded-lg bg-success/10 text-success">
              <CircleCheck className="size-4.5" />
            </span>
            <div>
              <h2 className="text-sm font-semibold text-foreground">{r.title}</h2>
              <p className="text-xs text-muted-foreground">
                Decision support · does not modify the production resource
              </p>
            </div>
          </div>

          <div className="mt-4 flex flex-wrap items-center gap-x-6 gap-y-3">
            <div>
              <p className="text-xs text-muted-foreground">Recommended configuration</p>
              <p className="mt-0.5 flex items-center gap-2 text-lg font-semibold text-foreground">
                {r.from}
                <ArrowRight className="size-4 text-primary" />
                <span className="text-primary">{r.to}</span>
              </p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Estimated savings</p>
              <p className="mt-0.5 text-lg font-semibold tabular-nums text-success">
                {r.savingsPct}%
              </p>
            </div>
            <div className="flex items-center gap-4">
              <div>
                <p className="text-xs text-muted-foreground">Risk</p>
                <div className="mt-1">
                  <RiskBadge risk={r.risk} />
                </div>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Confidence</p>
                <p className="mt-0.5 text-lg font-semibold tabular-nums text-foreground">
                  {r.confidence}%
                </p>
              </div>
            </div>
          </div>

          <div className="mt-4 flex flex-wrap gap-2">
            <Button size="lg" nativeButton={false} render={<Link href="/dashboard/recommendations" />}>
              Review scenario
              <ArrowRight />
            </Button>
            <Button size="lg" variant="outline" nativeButton={false} render={<Link href="/dashboard/recommendations" />}>
              <FileSearch />
              View evidence
            </Button>
          </div>
        </div>

        <div className="w-full shrink-0 rounded-lg border border-border bg-background p-4 lg:w-72">
          <p className="mb-2 text-xs font-medium text-foreground">Evidence</p>
          <ul className="flex flex-col gap-2">
            {r.evidence.map((e) => (
              <li key={e.label} className="flex items-center gap-2 text-xs">
                <EvidenceDot tone={e.tone} />
                <span className="text-muted-foreground">{e.label}</span>
                <span className={`ml-auto font-medium tabular-nums ${toneText(e.tone)}`}>
                  {e.value}
                </span>
              </li>
            ))}
          </ul>
          <Badge variant="neutral" className="mt-3 w-full justify-center">
            Analysis period: {r.analysisPeriod}
          </Badge>
        </div>
      </div>
    </div>
  )
}
