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
import { isDemoMode } from '@/lib/demo-mode'
import {
  getRecommendationSummary,
  getRecommendation,
  getRecommendationEvidence,
  getRecommendationTrace,
  acceptRecommendation,
  rejectRecommendation,
  dismissRecommendation,
  applyRecommendation,
  getSelectedResourceUid,
  type RecommendationItem,
  type RecommendationTrace,
  type ServiceResult,
} from '@/lib/api-services'

type EvidenceItem = {
  label: string
  value: string
  tone?: 'good' | 'warn' | 'bad'
}

export default function RecommendationsPage() {
  const [r, setR] = useState<RecommendationItem | null>(null)
  const [resourceUid, setResourceUid] = useState<string | null>(null)
  const [evidence, setEvidence] = useState<EvidenceItem[]>([])
  const [trace, setTrace] = useState<RecommendationTrace | null>(null)
  const [recSource, setRecSource] = useState<'mock' | 'backend' | null>(null)
  const [recStatus, setRecStatus] = useState<ServiceResult<RecommendationItem>['status']>('loading')
  const [recError, setRecError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const [actionPending, setActionPending] = useState(false)
  const [demo, setDemo] = useState(false)

  useEffect(() => { setDemo(isDemoMode()) }, [])

  useEffect(() => {
    const uid = getSelectedResourceUid()
    setResourceUid(uid)
    getRecommendationSummary().then((res) => {
      setRecSource(res.source)
      setRecStatus(res.status)
      setRecError(res.error)
      if (res.data) {
        setR(res.data)
        setEvidence(res.data.evidence)
      } else {
        setR(null)
        setEvidence([])
      }
    })
  }, [])

  useEffect(() => {
    if (recSource !== 'backend' || !resourceUid || !r || r.resourceUid !== resourceUid) return

    getRecommendationEvidence(r.id)
      .then((e) => {
        if (e.source === 'backend' && e.data) {
          setEvidence(e.data.evidence_items)
        }
      })
      .catch(() => {})

    getRecommendationTrace(r.id)
      .then((t) => {
        if (t.source === 'backend' && t.data) {
          setTrace(t.data)
        }
      })
      .catch(() => setTrace(null))
  }, [resourceUid, recSource, r?.id, r?.resourceUid])

  async function recordDecision(action: (uid: string) => Promise<ServiceResult<unknown>>) {
    if (recSource !== 'backend' || !r?.resourceUid || r.resourceUid !== resourceUid || actionPending) return
    setActionPending(true)
    setActionError(null)
    try {
      const result = await action(r.id)
      if (result.status !== 'ready') {
        setActionError(result.error ?? 'Could not record the decision.')
        return
      }
      const updated = await getRecommendation(r.id)
      if (updated.status === 'ready' && updated.data) {
        setR(updated.data)
        const updatedTrace = await getRecommendationTrace(r.id)
        if (updatedTrace.status === 'ready' && updatedTrace.data) setTrace(updatedTrace.data)
      } else {
        setActionError(updated.error ?? 'Decision recorded, but the view could not be refreshed.')
      }
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'Could not record the decision.')
    } finally {
      setActionPending(false)
    }
  }

  const canDecide = recSource === 'backend' && r?.resourceUid === resourceUid && !actionPending

  if (!r || recStatus !== 'ready') {
    return (
      <div>
        <PageHeader title="Recommendation detail" description="Recommendations calculated for the selected resource." />
        <Panel>
          <p className="text-sm text-muted-foreground">
            {recStatus === 'loading' ? 'Loading recommendation...' :
              recStatus === 'error' ? `Could not retrieve the recommendation: ${recError ?? 'Unknown error'}` :
              recError === 'no_resource' ? 'Select a resource to view recommendations.' :
              'No recommendations available for the selected resource.'}
          </p>
        </Panel>
      </div>
    )
  }

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
          <Panel action={recSource === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}>
            {r.status && <Badge variant="outline" className="mb-3">Status: {r.status}</Badge>}
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
                <Metric label="Latency" value={r.latencyImpact === null ? '—' : `+${r.latencyImpact}%`} />
                <Metric label="Confidence" value={`${r.confidence}%`} tone="brand" />
              </div>
            </div>
          </Panel>

          <div className="grid gap-4 sm:grid-cols-2">
            <Panel title="Constraints evaluated" description="All must pass for feasibility" action={recSource === 'mock' && r.constraints.length ? <Badge variant="outline">Mock</Badge> : undefined}>
              <ul className="space-y-2.5">
                {r.constraints.map((c) => (
                  <li key={c.label} className="flex items-start gap-2 text-sm">
                    <span className={cn('mt-0.5 flex size-4 items-center justify-center rounded-full', c.pass ? 'bg-success/15 text-success' : 'bg-warning/15 text-warning')}>
                      {c.pass ? <Check className="size-3" /> : <AlertTriangle className="size-3" />}
                    </span>
                    <span className="text-muted-foreground">{c.label}</span>
                  </li>
                ))}
                {r.constraints.length === 0 && <li className="text-xs text-muted-foreground">No evaluated constraints were provided.</li>}
              </ul>
            </Panel>

            <Panel title="Evidence" description="Available evidence for this recommendation" action={recSource === 'mock' && evidence.length ? <Badge variant="outline">Mock</Badge> : undefined}>
              <ul className="space-y-2.5">
                {evidence.map((e) => (
                  <li key={e.label} className="flex items-center gap-2 text-sm">
                    {e.tone ? <EvidenceDot tone={e.tone} /> : <span className="size-2 shrink-0 rounded-full bg-muted-foreground" />}
                    <span className="text-muted-foreground">{e.label}</span>
                    <span className={cn('ml-auto font-medium tabular-nums', e.tone ? toneText(e.tone) : 'text-foreground')}>{e.value}</span>
                  </li>
                ))}
                {evidence.length === 0 && <li className="text-xs text-muted-foreground">No evidence available for this recommendation.</li>}
              </ul>
            </Panel>

            <Panel
              title="Why this recommendation?"
              action={recSource === 'mock' && r.rationale ? <Badge variant="outline">Mock</Badge> : undefined}
            >
              <div className="space-y-3">
                  <p className="text-sm leading-relaxed text-muted-foreground">{r.rationale}</p>
                  <div className="grid gap-2 sm:grid-cols-2">
                    <Grounded label="Analysis period" value={r.analysisPeriod} />
                    <Grounded label="Generated" value={r.generated} />
                    <Grounded label="Model / ruleset" value={r.modelVersion} />
                    <Grounded label="Resource" value={r.resourceUid ?? 'Ejemplo'} />
                  </div>
              </div>
            </Panel>

            <Panel title="Limitations" description="Stated explicitly with every recommendation" action={recSource === 'mock' && r.limitations.length ? <Badge variant="outline">Mock</Badge> : undefined}>
              <ul className="space-y-2">
                {r.limitations.map((l: string) => (
                  <li key={l} className="flex items-start gap-2 text-sm text-muted-foreground">
                    <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-warning" />
                    {l}
                  </li>
                ))}
                {r.limitations.length === 0 && <li className="text-xs text-muted-foreground">No limitations were provided.</li>}
              </ul>
            </Panel>

            {/* Decision actions */}
            <Panel title="Decision actions" description="Record your decision on this recommendation" action={<Badge variant="outline" className="text-[9px]">{recSource}</Badge>}>
              <div className="grid grid-cols-2 gap-3">
                <Button
                  size="sm"
                  variant="default"
                  onClick={() => void recordDecision(acceptRecommendation)}
                  disabled={!canDecide}
                >
                  Accept
                </Button>
                <Button
                  size="sm"
                  variant="destructive"
                  onClick={() => void recordDecision(rejectRecommendation)}
                  disabled={!canDecide}
                >
                  Reject
                </Button>
              </div>
              <div className="mt-2 grid grid-cols-2 gap-3">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => void recordDecision(dismissRecommendation)}
                  disabled={!canDecide}
                >
                  Dismiss
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => void recordDecision(applyRecommendation)}
                  disabled={!canDecide}
                >
                  Apply (record only)
                </Button>
              </div>
              {recSource !== 'backend' && (
                <p className="mt-2 text-xs text-muted-foreground">
                  Decision buttons are disabled without a backend connection.
                </p>
              )}
              {actionError && <p role="alert" className="mt-2 text-xs text-danger">{actionError}</p>}
            </Panel>
          </div>
        </div>

        {/* Sidebar: risk/confidence + AI + no-rec */}
        <div className="space-y-4 lg:col-span-1 lg:col-start-3">
            <Panel title="Risk & confidence" description="Evaluated independently" action={recSource === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}>
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
                    Confidence is an estimate from the engine; review the evidence before making a decision.
                  </p>
                </div>
              </div>
            </Panel>

            {demo && <AssistantPanel />}

            {demo && <Panel title="Other outcomes are valid" description="Ejemplos ilustrativos" action={<Badge variant="outline" className="text-[9px]">mock</Badge>}>
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
            </Panel>}

            {trace && (
              <Panel title="Audit trace" description="Recommendation decision lineage">
                <div className="space-y-2">
                  <div className="flex items-center justify-between text-xs">
                    <span>Status: {trace.status}</span>
                    <Badge variant={trace.status === 'accepted' || trace.status === 'applied' ? 'success' : trace.status === 'rejected' ? 'danger' : 'outline'}>
                      {trace.decision}
                    </Badge>
                  </div>
                  <p className="text-xs text-muted-foreground">Risk: {trace.risk.level} · Confidence: {Math.round(trace.confidence.score * 100)}%</p>
                  {trace.events.length === 0 && <p className="text-xs text-muted-foreground">No audit events recorded.</p>}
                  {trace.events.map((event) => (
                    <div key={event.event_id} className="flex gap-2 border-t border-border pt-2 text-xs text-muted-foreground">
                      <span className="font-mono">{event.event_id}</span>
                      <span>{event.action}</span>
                      <span>{event.occurred_at}</span>
                    </div>
                  ))}
                </div>
              </Panel>
            )}
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
      description="Illustrative examples; they do not query the current recommendation"
      action={<Badge variant="outline" className="text-[9px]">mock</Badge>}
    >
      <div className="flex flex-col gap-2">
        {Object.values(assistantSuggestionsMock).map((q) => (
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
            Sample response, not generated from current data
          </div>
          <p className="text-xs leading-relaxed text-foreground">{assistantSuggestionsMock[question]}</p>
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

const assistantSuggestionsMock: Record<string, string> = {
  'Why is this recommendation considered low risk?':
    'Risk is Low because projected CPU p99 at 4 vCores stays near 58% — well below saturation — with sufficient headroom for observed peaks, a stable trend, low variability outside business hours, and no SLA violations in the 30-day window. These are calculated signals shown on the Recommendation and Risk views; the assistant only explains them.',
  'What evidence supports the proposed configuration?':
    'The scenario replay shows CPU p95 at 28% and p99 at 41% on current capacity, I/O p95 below threshold, storage growth low, and historical SLA compliance of 99.97%. Simulated on 4 vCores these constraints still pass, which is why Candidate A is marked feasible and recommended.',
  'What could happen if workload increases?':
    'Forecasting projects +3.1% over the next 14 days with no CPU saturation in the horizon. If demand grew beyond the upper uncertainty band, p99 headroom on 4 vCores would tighten first; the system would lower confidence and could withdraw the recommendation rather than keep it.',
  'Which metric is limiting the recommendation?':
    'CPU p99 at peak is the binding constraint. Candidate B (2 vCores) pushes p99 to ~81% and moves SLA to Warning, which is why it is not recommended despite larger nominal savings. Memory (68%) is the second-closest signal to its threshold.',
}
