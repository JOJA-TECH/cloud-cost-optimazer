'use client'

import { useEffect, useState } from 'react'
import {
  ArrowRight,
  ChevronDown,
  CircleCheck,
  Check,
  ShieldCheck,
  Sparkles,
  Clock,
  AlertTriangle,
  Ban,
  Send,
} from 'lucide-react'
import { PageHeader, Panel, RiskBadge, ConfidenceBar, EvidenceDot, toneText } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { getRecommendationSummary } from '@/lib/api-services'
import type { RecommendationItem } from '@/lib/api-services'

export default function RecommendationsPage() {
  const [r, setR] = useState<RecommendationItem>(recMock)
  const [showWhy, setShowWhy] = useState(true)

  useEffect(() => {
    getRecommendationSummary().then(setR)
  }, [])

  return (
    <div>
      <PageHeader
        title="Recommendation detail"
        description="A decision-support recommendation grounded in calculated evidence. The AI layer explains this evidence — it does not invent the capacity decision or change production."
        actions={<Badge variant="neutral"><Clock className="size-3" />{r.generated}</Badge>}
      />

      <div className="grid gap-4 lg:grid-cols-3">
        {/* Main detail */}
        <div className="space-y-4 lg:col-span-2">
          <Panel>
            <div className="flex flex-wrap items-start justify-between gap-4">
              <div className="flex items-center gap-3">
                <span className="flex size-10 items-center justify-center rounded-xl bg-success/10 text-success">
                  <CircleCheck className="size-5" />
                </span>
                <div>
                  <p className="text-xs text-muted-foreground">Recommended configuration</p>
                  <p className="flex items-center gap-2 text-xl font-semibold text-foreground">
                    {r.from}
                    <ArrowRight className="size-4 text-primary" />
                    <span className="text-primary">{r.to}</span>
                  </p>
                </div>
              </div>
              <div className="grid grid-cols-3 gap-4 text-center">
                <Metric label="Savings" value={`${r.savingsPct}%`} tone="success" />
                <Metric label="Latency" value={`+${r.latencyImpact}%`} />
                <Metric label="Confidence" value={`${r.confidence}%`} tone="brand" />
              </div>
            </div>
          </Panel>

          <div className="grid gap-4 sm:grid-cols-2">
            <Panel title="Constraints evaluated" description="All must pass for feasibility">
              <ul className="space-y-2.5">
                {r.constraints.map((c) => (
                  <li key={c.label} className="flex items-start gap-2 text-sm">
                    <span className="mt-0.5 flex size-4 items-center justify-center rounded-full bg-success/15 text-success">
                      <Check className="size-3" />
                    </span>
                    <span className="text-muted-foreground">{c.label}</span>
                  </li>
                ))}
              </ul>
            </Panel>

            <Panel title="Evidence" description="Calculated from observed telemetry">
              <ul className="space-y-2.5">
                {r.evidence.map((e) => (
                  <li key={e.label} className="flex items-center gap-2 text-sm">
                    <EvidenceDot tone={e.tone} />
                    <span className="text-muted-foreground">{e.label}</span>
                    <span className={cn('ml-auto font-medium tabular-nums', toneText(e.tone))}>{e.value}</span>
                  </li>
                ))}
              </ul>
            </Panel>
          </div>

          <Panel
            title="Why this recommendation?"
            action={
              <button
                onClick={() => setShowWhy((v) => !v)}
                className="inline-flex items-center gap-1 text-xs font-medium text-primary"
              >
                {showWhy ? 'Hide' : 'Show'}
                <ChevronDown className={cn('size-4 transition-transform', showWhy && 'rotate-180')} />
              </button>
            }
          >
            {showWhy && (
              <div className="space-y-3">
                <p className="text-sm leading-relaxed text-muted-foreground">{r.rationale}</p>
                <div className="grid gap-2 sm:grid-cols-2">
                  <Grounded label="Analysis period" value={r.analysisPeriod} />
                  <Grounded label="Generated" value={r.generated} />
                  <Grounded label="Model / ruleset" value={r.modelVersion} />
                  <Grounded label="Resource" value="sql-prod-orders-eastus" />
                </div>
              </div>
            )}
          </Panel>

          <Panel title="Limitations" description="Stated explicitly with every recommendation">
            <ul className="space-y-2">
              {r.limitations.map((l) => (
                <li key={l} className="flex items-start gap-2 text-sm text-muted-foreground">
                  <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-warning" />
                  {l}
                </li>
              ))}
            </ul>
          </Panel>
        </div>

        {/* Sidebar: risk/confidence + AI + no-rec */}
        <div className="space-y-4">
          <Panel title="Risk & confidence" description="Evaluated independently">
            <div className="space-y-4">
              <div className="flex items-center justify-between rounded-lg border border-border bg-background px-3 py-2.5">
                <span className="flex items-center gap-2 text-sm text-muted-foreground">
                  <ShieldCheck className="size-4 text-success" />
                  Risk
                </span>
                <RiskBadge risk={r.risk} />
              </div>
              <p className="text-xs text-muted-foreground">
                Considers proximity to limits, peak magnitude and frequency, variability, trend,
                headroom, and SLA history.
              </p>
              <div className="border-t border-border pt-4">
                <ConfidenceBar value={r.confidence} />
                <p className="mt-2 text-xs text-muted-foreground">
                  High confidence: sufficient historical coverage, complete CPU/I/O metrics, stable
                  workload pattern, validated scenario.
                </p>
              </div>
            </div>
          </Panel>

          <AssistantPanel />

          <Panel title="Other outcomes are valid" description="The system can decline to recommend">
            <div className="space-y-3">
              <div className="rounded-lg border border-border bg-background p-3">
                <div className="mb-1 flex items-center gap-2">
                  <Ban className="size-3.5 text-muted-foreground" />
                  <p className="text-xs font-semibold text-foreground">No recommendation</p>
                </div>
                <p className="text-xs text-muted-foreground">
                  sql-prod-analytics-westus — current capacity already balanced; no feasible saving.
                </p>
              </div>
              <div className="rounded-lg border border-border bg-background p-3">
                <div className="mb-1 flex items-center gap-2">
                  <AlertTriangle className="size-3.5 text-warning" />
                  <p className="text-xs font-semibold text-foreground">Insufficient data</p>
                </div>
                <p className="text-xs text-muted-foreground">
                  sql-prod-identity-eastus — 41% confidence, &lt;7 days history. Collecting more
                  telemetry before advising.
                </p>
              </div>
            </div>
          </Panel>
        </div>
      </div>
    </div>
  )
}

function Metric({ label, value, tone }: { label: string; value: string; tone?: 'success' | 'brand' }) {
  const color = tone === 'success' ? 'text-success' : tone === 'brand' ? 'text-primary' : 'text-foreground'
  return (
    <div>
      <p className={cn('text-lg font-semibold tabular-nums', color)}>{value}</p>
      <p className="text-[11px] text-muted-foreground">{label}</p>
    </div>
  )
}

function Grounded({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-border bg-background px-3 py-2">
      <p className="text-[11px] text-muted-foreground">{label}</p>
      <p className="mt-0.5 text-xs font-medium text-foreground">{value}</p>
    </div>
  )
}

function AssistantPanel() {
  const [question, setQuestion] = useState<string | null>(null)

  return (
    <Panel
      title="Ask about this recommendation"
      description="Explanation layer over calculated evidence"
      action={<Sparkles className="size-4 text-primary" />}
    >
      <div className="flex flex-col gap-2">
        {assistantSuggestionsMock.map((q) => (
          <button
            key={q}
            onClick={() => setQuestion(q)}
            className={cn(
              'rounded-lg border px-3 py-2 text-left text-xs transition-colors',
              question === q
                ? 'border-primary/40 bg-primary/5 text-foreground'
                : 'border-border bg-background text-muted-foreground hover:border-primary/30 hover:text-foreground',
            )}
          >
            {q}
          </button>
        ))}
      </div>

      {question && (
        <div className="mt-3 rounded-lg bg-muted/60 p-3">
          <div className="mb-1.5 flex items-center gap-1.5 text-[11px] font-medium text-primary">
            <Sparkles className="size-3" />
            Grounded in visible evidence
          </div>
          <p className="text-xs leading-relaxed text-foreground">{assistantAnswersMock[question]}</p>
        </div>
      )}

      <div className="mt-3 flex items-center gap-2 rounded-lg border border-border bg-background px-3 py-2 text-xs text-muted-foreground">
        <input
          className="min-w-0 flex-1 bg-transparent outline-none placeholder:text-muted-foreground"
          placeholder="Ask a question…"
          readOnly
        />
        <Send className="size-3.5 text-muted-foreground" />
      </div>
    </Panel>
  )
}

const recMock: RecommendationItem = {
  id: 'rec-2041',
  title: 'Recommendation available',
  from: '8 vCores',
  to: '4 vCores',
  savingsPct: 24.2,
  savingsAbs: 117.8,
  risk: 'Low',
  confidence: 91,
  latencyImpact: 2.1,
  analysisPeriod: 'Last 30 days',
  generated: 'Today, 14:32',
  modelVersion: 'rightsizer v1.4.2 · ruleset 2024.11',
  evidence: [
    { label: 'CPU p95', value: '28%', tone: 'good' },
    { label: 'CPU p99', value: '41%', tone: 'good' },
    { label: 'I/O p95', value: 'Below threshold', tone: 'good' },
    { label: 'Workload trend', value: 'Stable', tone: 'good' },
    { label: 'Headroom', value: 'Sufficient', tone: 'good' },
    { label: 'SLA (30d)', value: '99.97%', tone: 'good' },
  ],
  constraints: [
    { label: 'p95 latency within threshold', pass: true },
    { label: 'No SLA violations in analysis window', pass: true },
    { label: 'Sufficient CPU headroom at projected peak', pass: true },
    { label: 'I/O within provisioned limits', pass: true },
  ],
  limitations: [],
  rationale: '',
}

const assistantSuggestionsMock = [
  'Why is this recommendation considered low risk?',
  'What evidence supports the proposed configuration?',
  'What could happen if workload increases?',
  'Which metric is limiting the recommendation?',
]

const assistantAnswersMock: Record<string, string> = {
  'Why is this recommendation considered low risk?':
    'Risk is Low because projected CPU p99 at 4 vCores stays near 58% — well below saturation — with sufficient headroom for observed peaks, a stable trend, low variability outside business hours, and no SLA violations in the 30-day window. These are calculated signals shown on the Recommendation and Risk views; the assistant only explains them.',
  'What evidence supports the proposed configuration?':
    'The scenario replay shows CPU p95 at 28% and p99 at 41% on current capacity, I/O p95 below threshold, storage growth low, and historical SLA compliance of 99.97%. Simulated on 4 vCores these constraints still pass, which is why Candidate A is marked feasible and recommended.',
  'What could happen if workload increases?':
    'Forecasting projects +3.1% over the next 14 days with no CPU saturation in the horizon. If demand grew beyond the upper uncertainty band, p99 headroom on 4 vCores would tighten first; the system would lower confidence and could withdraw the recommendation rather than keep it.',
  'Which metric is limiting the recommendation?':
    'CPU p99 at peak is the binding constraint. Candidate B (2 vCores) pushes p99 to ~81% and moves SLA to Warning, which is why it is not recommended despite larger nominal savings. Memory (68%) is the second-closest signal to its threshold.',
}


