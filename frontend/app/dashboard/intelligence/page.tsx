'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { AlertTriangle, BrainCircuit, Check, CircleHelp, Database, LoaderCircle, RefreshCw, ShieldCheck, Sparkles } from 'lucide-react'
import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { getCloudResources, type CloudResource } from '@/lib/api-services'
import {
  computeAnalytics,
  enqueueBackendRecommendation,
  getBackendJobStatus,
  loadAnalyticsWorkspace,
  type AnalyticsWorkspace,
  type BackendJobStatus,
  type BackendRecommendation,
  type OptimizationPolicy,
} from '@/lib/ai-services'
import { apiJson } from '@/lib/api'

const dateInput = (date: Date) => date.toISOString().slice(0, 10)
const initialTo = new Date()
const initialFrom = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)

export default function IntelligencePage() {
  const [resources, setResources] = useState<CloudResource[]>([])
  const [resourceUID, setResourceUID] = useState('')
  const [fromDate, setFromDate] = useState(dateInput(initialFrom))
  const [toDate, setToDate] = useState(dateInput(initialTo))
  const [workspace, setWorkspace] = useState<AnalyticsWorkspace | null>(null)
  const [selectedPolicy, setSelectedPolicy] = useState('')
  const [loadingResources, setLoadingResources] = useState(true)
  const [computing, setComputing] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [pageError, setPageError] = useState('')
  const [computeMessages, setComputeMessages] = useState<string[]>([])
  const [newPolicyName, setNewPolicyName] = useState('')
  const [newPolicyVersion, setNewPolicyVersion] = useState('1')
  const [allowScaleDown, setAllowScaleDown] = useState(false)
  const [maxRisk, setMaxRisk] = useState('')
  const [minConfidence, setMinConfidence] = useState('')
  const [minCPUCoverage, setMinCPUCoverage] = useState('95')
  const [savingPolicy, setSavingPolicy] = useState(false)
  const [generated, setGenerated] = useState<BackendRecommendation | null>(null)
  const [jobStatus, setJobStatus] = useState<BackendJobStatus | null>(null)

  const window = useMemo(() => {
    if (!fromDate || !toDate || fromDate > toDate) return null
    const from = new Date(`${fromDate}T00:00:00.000Z`)
    const to = new Date(`${toDate}T23:59:59.000Z`)
    if (!Number.isFinite(from.getTime()) || !Number.isFinite(to.getTime())) return null
    return { from: from.toISOString(), to: to.toISOString() }
  }, [fromDate, toDate])

  const refreshWorkspace = useCallback(async (uid = resourceUID) => {
    if (!uid || !window) return
    const result = await loadAnalyticsWorkspace(uid, window)
    setWorkspace(result)
    setSelectedPolicy((current) => current || result.policies[0]?.policy_uid || '')
  }, [resourceUID, window])

  useEffect(() => {
    let active = true
    async function loadResources() {
      setLoadingResources(true)
      setPageError('')
      try {
        const cloudResources = await getCloudResources()
        if (!active) return
        const data = cloudResources.source === 'backend' ? (cloudResources.data ?? []) : []
        setResources(data)
        const selected = data[0]?.resource_uid ?? ''
        setResourceUID(selected)
        if (!selected) setPageError('No accessible cloud resources. Check the Cloud module, data binding, and resources:read permission.')
      } catch (error) {
        if (active) setPageError(error instanceof Error ? error.message : 'Could not retrieve resources from the backend.')
      } finally {
        if (active) setLoadingResources(false)
      }
    }
    void loadResources()
    return () => { active = false }
  }, [])

  useEffect(() => {
    setGenerated(null)
    if (resourceUID && window) void refreshWorkspace(resourceUID)
    else setWorkspace(null)
  }, [resourceUID, refreshWorkspace, window])

  const runAnalysis = async () => {
    if (!resourceUID || !window) return
    setComputing(true)
    setPageError('')
    setComputeMessages([])
    setGenerated(null)
    try {
      const failures = await computeAnalytics(resourceUID, window)
      setComputeMessages(failures)
      await refreshWorkspace(resourceUID)
      if (failures.length === 0) setPageError('')
    } catch (error) {
      setPageError(error instanceof Error ? error.message : 'Could not complete the analysis.')
    } finally {
      setComputing(false)
    }
  }

  const createPolicy = async () => {
    const name = newPolicyName.trim()
    const version = Number(newPolicyVersion)
    const riskValue = maxRisk === '' ? undefined : Number(maxRisk)
    const confidenceValue = minConfidence === '' ? undefined : Number(minConfidence)
    const coverageValue = Number(minCPUCoverage)
    if (!name || !Number.isInteger(version) || version < 1 ||
      (riskValue !== undefined && (!Number.isFinite(riskValue) || riskValue < 0 || riskValue > 1)) ||
      (confidenceValue !== undefined && (!Number.isFinite(confidenceValue) || confidenceValue < 0 || confidenceValue > 1)) ||
      !Number.isFinite(coverageValue) || coverageValue <= 0 || coverageValue > 100) {
      setPageError('Check the name, version (integer ≥ 1), optional thresholds between 0 and 1, and CPU p95 coverage above 0 and at most 100%.')
      return
    }
    const constraints: Record<string, unknown> = {
      allow_scale_down: allowScaleDown,
      allow_compute_model_change: false,
      min_cpu_p95_coverage_pct: coverageValue,
    }
    if (riskValue !== undefined) constraints.max_risk_score = riskValue
    if (confidenceValue !== undefined) constraints.min_confidence = confidenceValue
    setSavingPolicy(true)
    setPageError('')
    try {
      const created = await apiJson<OptimizationPolicy>('/api/v1/recommendations/policies', {
        method: 'POST',
        body: JSON.stringify({ name, version, enabled: true, constraints }),
      })
      setWorkspace((current) => current ? { ...current, policies: [created, ...current.policies] } : current)
      setSelectedPolicy(created.policy_uid)
      setNewPolicyName('')
    } catch (error) {
      setPageError(error instanceof Error ? error.message : 'Could not create the policy.')
    } finally {
      setSavingPolicy(false)
    }
  }

  const requestRecommendation = async () => {
    if (!resourceUID || !selectedPolicy || !window) return
    setGenerating(true)
    setPageError('')
    setJobStatus(null)
    const previousRecommendationUIDs = new Set(workspace?.recommendations.map((item) => item.recommendation_uid) ?? [])
    try {
      let job = await enqueueBackendRecommendation({
        resource_uid: resourceUID,
        policy_uid: selectedPolicy,
        from: window.from,
        to: window.to,
        horizon_days: 7,
        currency: 'USD',
      })
      setJobStatus(job)
      for (let attempt = 0; attempt < 120 && (job.status === 'queued' || job.status === 'running'); attempt += 1) {
        await new Promise((resolve) => setTimeout(resolve, 1500))
        job = await getBackendJobStatus(job.id)
        setJobStatus(job)
      }
      if (job.status === 'failed' || job.status === 'cancelled') {
        throw new Error(job.error || `The job ended with status ${job.status}.`)
      }
      if (job.status !== 'completed') {
        setPageError(`Job ${job.id} is still ${job.status}. Check the recommendation again when it finishes.`)
        return
      }
      const updated = await loadAnalyticsWorkspace(resourceUID, window)
      setWorkspace(updated)
      const latestResult = updated.recommendations.find((item) => !previousRecommendationUIDs.has(item.recommendation_uid))
        ?? updated.recommendations[0]
        ?? null
      setGenerated(latestResult)
      if (!latestResult) setPageError('The job finished, but no persisted recommendation was found for the resource.')
    } catch (error) {
      setPageError(error instanceof Error ? error.message : 'The backend could not generate the recommendation.')
    } finally {
      setGenerating(false)
    }
  }

  const activeResource = resources.find((resource) => resource.resource_uid === resourceUID)
  const cpuP95 = percentileValue(workspace?.cpuPercentiles ?? [], 95)
  const cpuP99 = percentileValue(workspace?.cpuPercentiles ?? [], 99)
  const ioP95 = percentileValue(workspace?.ioPercentiles ?? [], 95)
  const latestRecommendation = generated ?? workspace?.recommendations[0] ?? null

  return (
    <div className="space-y-4">
      <PageHeader
        title="AI and analytics"
        description="Backend analytics produces evidence; its rules engine applies versioned policies. No changes are made in Azure."
        actions={<Badge variant="neutral"><BrainCircuit className="mr-1 size-3" />Backend · Analytics + Recommendation</Badge>}
      />

      <Panel title="Resource and time window" description="Data and dates are queried and processed by the existing backend modules.">
        <div className="grid gap-3 md:grid-cols-[minmax(0,2fr)_1fr_1fr_auto] md:items-end">
          <label className="space-y-1 text-xs text-muted-foreground">
            <span>Database</span>
            <select value={resourceUID} onChange={(event) => setResourceUID(event.target.value)} disabled={loadingResources || resources.length === 0} className="h-10 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground">
              {resources.length === 0 && <option value="">No accessible resources</option>}
              {resources.map((resource) => <option key={resource.resource_uid} value={resource.resource_uid}>{resource.server_name}/{resource.database_name} · {resource.region_code}</option>)}
            </select>
          </label>
          <label className="space-y-1 text-xs text-muted-foreground">
            <span>From (UTC)</span>
            <input type="date" value={fromDate} onChange={(event) => setFromDate(event.target.value)} className="h-10 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
          </label>
          <label className="space-y-1 text-xs text-muted-foreground">
            <span>To (UTC)</span>
            <input type="date" value={toDate} onChange={(event) => setToDate(event.target.value)} className="h-10 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
          </label>
          <Button onClick={runAnalysis} disabled={!resourceUID || !window || computing || loadingResources}>
            {computing ? <LoaderCircle className="mr-2 size-4 animate-spin" /> : <RefreshCw className="mr-2 size-4" />}
            {computing ? 'Analyzing…' : 'Calculate evidence'}
          </Button>
        </div>
        {!window && <p className="mt-2 text-xs text-danger">Select valid dates; the start date must be on or before the end date.</p>}
        {activeResource && <p className="mt-3 break-all text-[11px] text-muted-foreground">resource_uid: {activeResource.resource_uid}</p>}
      </Panel>

      {pageError && <Notice kind="error" text={pageError} />}
        {computeMessages.length > 0 && <Notice kind="warning" text={`The backend reported ${computeMessages.length} incomplete ${computeMessages.length === 1 ? 'operation' : 'operations'}: ${computeMessages.join(' · ')}`} />}
      {workspace?.errors.map((message) => <Notice key={message} kind="warning" text={message} />)}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="Observed CPU p95" value={formatPercent(cpuP95)} detail="Exact 95th percentile stored by Analytics" />
        <MetricCard label="Observed CPU p99" value={formatPercent(cpuP99)} detail="99th percentile; not derived from the average" />
        <MetricCard label="Observed I/O p95" value={formatPercent(ioP95)} detail="Unavailable if the exact sample is missing" />
        <MetricCard label="Analytics snapshots" value={workspace ? String(workspace.snapshots.length) : '—'} detail="Hourly windows in the selected period" />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Panel title="Calculated evidence" description="Values and methods come from analytics; missing fields are not estimated locally.">
          {!workspace ? <Loading /> : (
            <div className="space-y-3">
              <EvidenceRow label="Average CPU (latest snapshot)" value={formatPercent(latest(workspace.snapshots)?.CPUAvg)} />
              <EvidenceRow label="Maximum observed CPU" value={formatPercent(latest(workspace.snapshots)?.CPUMax)} />
              <EvidenceRow label="Capacity headroom" value={formatPercent(workspace.capacity?.HeadroomPct)} />
              <EvidenceRow label="Peak utilization" value={formatPercent(workspace.capacity?.PeakUtilization)} />
              <EvidenceRow label="Observed monthly cost" value={money(workspace.costs?.ActualMonthlyCost, workspace.costs?.Currency)} />
              <EvidenceRow label="Monthly projection" value={money(workspace.costs?.ProjectedMonthly, workspace.costs?.Currency)} />
              <EvidenceRow label="DES · capacity" value={formatNumber(workspace.des?.CapacityE)} />
              <EvidenceRow label="DES · cost" value={formatNumber(workspace.des?.CostE)} />
              <EvidenceRow label="DES · performance" value={formatNumber(workspace.des?.PerformanceE)} />
              <EvidenceRow label="Workload pattern" value={workspace.workload?.IntensityBand ?? 'No evidence'} />
              <EvidenceRow label="Available trends" value={workspace.trends.map((trend) => `${trend.MetricCode ?? 'metric'}: ${formatNumber(trend.Slope)}/h`).join(' · ') || 'No evidence'} />
              <EvidenceRow label="Last calculation" value={latest(workspace.snapshots)?.ComputedAt ? new Date(latest(workspace.snapshots)!.ComputedAt!).toLocaleString('en-US') : 'Not calculated'} />
            </div>
          )}
        </Panel>

        <Panel title="Forecast" description="Optional; must come from the ML provider connected to analytics.MLPort.">
          {!workspace ? <Loading /> : workspace.forecasts.length > 0 ? (
            <div className="space-y-2">
              {workspace.forecasts.slice(0, 5).map((forecast) => <EvidenceRow key={forecast.ForecastUID ?? `${forecast.PeriodStartUTC}-${forecast.TargetMetric}`} label={`${forecast.TargetMetric ?? 'metric'} · ${forecast.StepUnit ?? ''}`} value={`${formatNumber(forecast.YPred)}${forecast.YLower == null || forecast.YUpper == null ? '' : ` (${formatNumber(forecast.YLower)}–${formatNumber(forecast.YUpper)})`}`} />)}
            </div>
          ) : (
            <div className="flex gap-2 rounded-lg border border-border bg-muted/30 p-3 text-sm text-muted-foreground">
              <CircleHelp className="mt-0.5 size-4 shrink-0 text-primary" />
              No persisted forecast for this resource and period. The backend supports an ML provider, but a production provider must be configured; no fallback prediction is fabricated.
            </div>
          )}
        </Panel>
      </div>

      <div className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
        <Panel title="Recommendation engine" description="Select a policy explicitly; the result is saved and requires human review.">
          <div className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
            <label className="space-y-1 text-xs text-muted-foreground">
              <span>Enabled policy</span>
              <select value={selectedPolicy} onChange={(event) => setSelectedPolicy(event.target.value)} className="h-10 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground">
                <option value="">Select a policy</option>
                {(workspace?.policies ?? []).map((policy) => <option key={policy.policy_uid} value={policy.policy_uid}>{policy.name} · v{policy.version}</option>)}
              </select>
            </label>
            <Button onClick={requestRecommendation} disabled={!selectedPolicy || !window || generating || !resourceUID}>
              {generating ? <LoaderCircle className="mr-2 size-4 animate-spin" /> : <Sparkles className="mr-2 size-4" />}
              {generating ? 'Evaluating…' : 'Generate recommendation'}
            </Button>
          </div>
          {(!workspace || workspace.policies.length === 0) && (
            <div className="mt-3 rounded-lg border border-warning/25 bg-warning/[0.06] p-3 text-xs text-muted-foreground">
              No enabled policies. Create a versioned policy below; the form does not enable capacity reductions or model changes unless you allow them.
            </div>
          )}
          {jobStatus && <p className="mt-2 text-xs text-muted-foreground">Job {jobStatus.id} · {jobStatus.status}{jobStatus.error ? ` · ${jobStatus.error}` : ''}</p>}
          {latestRecommendation && <RecommendationCard recommendation={latestRecommendation} />}
          {!latestRecommendation && workspace?.recommendations.length === 0 && <p className="mt-3 text-sm text-muted-foreground">No persisted recommendation for this resource yet.</p>}
        </Panel>

        <Panel title="Create a versioned policy" description="Thresholds are team decisions; the model does not invent them.">
          <div className="space-y-3">
            <label className="block space-y-1 text-xs text-muted-foreground">Name
              <input value={newPolicyName} onChange={(event) => setNewPolicyName(event.target.value)} placeholder="e.g. conservative right-sizing" className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
            </label>
            <label className="block space-y-1 text-xs text-muted-foreground">Version
              <input type="number" min="1" value={newPolicyVersion} onChange={(event) => setNewPolicyVersion(event.target.value)} className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
            </label>
              <label className="flex items-start gap-2 text-xs text-muted-foreground">
                <input type="checkbox" checked={allowScaleDown} onChange={(event) => setAllowScaleDown(event.target.checked)} className="mt-0.5" />
                Allow rules to evaluate capacity reductions (changes are not applied automatically).
              </label>
            <label className="block space-y-1 text-xs text-muted-foreground">Minimum hourly CPU p95 coverage (%)
              <input type="number" min="0.01" max="100" step="0.1" required value={minCPUCoverage} onChange={(event) => setMinCPUCoverage(event.target.value)} className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
              <span className="block text-[11px]">Must be above 0 and at most 100. The engine abstains without sufficient coverage.</span>
            </label>
            <div className="grid grid-cols-2 gap-2">
              <label className="space-y-1 text-xs text-muted-foreground">Maximum risk (0–1)
                <input type="number" min="0" max="1" step="0.01" value={maxRisk} onChange={(event) => setMaxRisk(event.target.value)} placeholder="Optional" className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
              </label>
              <label className="space-y-1 text-xs text-muted-foreground">Minimum confidence (0–1)
                <input type="number" min="0" max="1" step="0.01" value={minConfidence} onChange={(event) => setMinConfidence(event.target.value)} placeholder="Optional" className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
              </label>
            </div>
            <Button variant="outline" onClick={createPolicy} disabled={savingPolicy || !newPolicyName.trim()} className="w-full">
              {savingPolicy ? <LoaderCircle className="mr-2 size-4 animate-spin" /> : <ShieldCheck className="mr-2 size-4" />}
              Save policy to backend
            </Button>
          </div>
        </Panel>
      </div>

      <div className="flex items-start gap-2 rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
        <Database className="mt-0.5 size-4 shrink-0 text-primary" />
        <span>Observations and evidence are stored in `cco_data`; authentication, permissions, jobs, and auditing remain in their backend modules. Recommendations are declarative: human approval does not execute changes in Azure.</span>
      </div>
    </div>
  )
}

function percentileValue(rows: AnalyticsWorkspace['cpuPercentiles'], target: number): number | null {
  const exact = rows.find((row) => Math.abs((row.Percentile ?? -1) - target) < 0.0001 && row.Value != null)
  return exact?.Value ?? null
}

function latest(rows: AnalyticsWorkspace['snapshots']) {
  return rows.reduce((current, row) => !current || (row.PeriodStartUTC ?? '') > (current.PeriodStartUTC ?? '') ? row : current, undefined as AnalyticsWorkspace['snapshots'][number] | undefined)
}

function formatPercent(value?: number | null) {
  return value == null || !Number.isFinite(value) ? 'No evidence' : `${value.toFixed(1)}%`
}

function formatNumber(value?: number | null) {
  return value == null || !Number.isFinite(value) ? '—' : value.toFixed(2)
}

function money(value?: number, currency?: string) {
  if (value == null || !Number.isFinite(value)) return 'No evidence'
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: currency || 'USD', maximumFractionDigits: 2 }).format(value)
}

function MetricCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return <div className="rounded-xl border border-border bg-card p-4 shadow-sm"><p className="text-xs text-muted-foreground">{label}</p><p className="mt-2 text-xl font-semibold tabular-nums text-foreground">{value}</p><p className="mt-1 text-[11px] text-muted-foreground">{detail}</p></div>
}

function EvidenceRow({ label, value }: { label: string; value: string }) {
  return <div className="flex items-start justify-between gap-3 border-b border-border/70 pb-2 text-xs last:border-0 last:pb-0"><span className="text-muted-foreground">{label}</span><span className="max-w-[60%] text-right font-medium text-foreground">{value}</span></div>
}

function Loading() {
  return <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" />Loading from the backend…</div>
}

function Notice({ kind, text }: { kind: 'error' | 'warning'; text: string }) {
  return <div className={`flex items-start gap-2 rounded-lg border p-3 text-xs ${kind === 'error' ? 'border-danger/25 bg-danger/[0.06] text-danger' : 'border-warning/25 bg-warning/[0.06] text-warning'}`}><AlertTriangle className="mt-0.5 size-3.5 shrink-0" />{text}</div>
}

function RecommendationCard({ recommendation }: { recommendation: BackendRecommendation }) {
  const saving = recommendation.estimated_saving
  const configValue = (config: Record<string, unknown>) => String(config.SKUName ?? config.sku_name ?? config.ComputeModel ?? config.compute_model ?? 'Configuration unavailable')
  return (
    <div className="mt-4 rounded-lg border border-border bg-background p-4">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div><p className="text-sm font-semibold text-foreground">{recommendation.action.description || recommendation.action.type}</p><p className="mt-1 text-xs text-muted-foreground">{configValue(recommendation.current_configuration)} → {configValue(recommendation.proposed_configuration)}</p></div>
        <Badge variant={recommendation.generation_state === 'complete' ? 'success' : 'warning'}>{recommendation.generation_state}</Badge>
      </div>
      <div className="mt-3 grid grid-cols-3 gap-2 text-center">
        <div className="rounded-md bg-muted/40 p-2"><p className="text-sm font-semibold">{money(saving?.monthly_amount, saving?.currency)}</p><p className="text-[10px] text-muted-foreground">potential savings/month</p></div>
        <div className="rounded-md bg-muted/40 p-2"><p className="text-sm font-semibold">{recommendation.risk?.level ?? '—'}</p><p className="text-[10px] text-muted-foreground">risk</p></div>
        <div className="rounded-md bg-muted/40 p-2"><p className="text-sm font-semibold">{recommendation.confidence ? `${(recommendation.confidence.score * 100).toFixed(0)}%` : '—'}</p><p className="text-[10px] text-muted-foreground">confidence</p></div>
      </div>
      <p className="mt-3 text-xs leading-relaxed text-muted-foreground">{recommendation.rationale?.summary || 'The backend did not provide an additional explanation.'}</p>
      {recommendation.rationale?.factors?.length > 0 && <ul className="mt-2 space-y-1">{recommendation.rationale.factors.map((factor, index) => <li key={`${factor}-${index}`} className="flex gap-2 text-[11px] text-muted-foreground"><Check className="mt-0.5 size-3 shrink-0 text-primary" />{factor}</li>)}</ul>}
      <p className="mt-3 text-[10px] text-muted-foreground">ID {recommendation.recommendation_uid} · policy v{recommendation.policy_version} · requires human review{recommendation.audit_status === 'audit_pending' ? ' · audit pending' : ''}</p>
    </div>
  )
}
