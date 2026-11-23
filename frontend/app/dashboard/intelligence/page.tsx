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
        const data = cloudResources.source === 'backend' ? cloudResources.data : []
        setResources(data)
        const selected = data[0]?.resource_uid ?? ''
        setResourceUID(selected)
        if (!selected) setPageError('No hay recursos cloud accesibles. Verifica el módulo Cloud, el binding de datos y el permiso resources:read.')
      } catch (error) {
        if (active) setPageError(error instanceof Error ? error.message : 'No se pudieron consultar recursos del backend.')
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
      setPageError(error instanceof Error ? error.message : 'No se pudo completar el análisis.')
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
      setPageError('Revisa el nombre, la versión (entero ≥ 1), los umbrales opcionales entre 0 y 1 y la cobertura CPU p95 mayor que 0 y hasta 100%.')
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
      setPageError(error instanceof Error ? error.message : 'No se pudo crear la política.')
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
        throw new Error(job.error || `El job terminó con estado ${job.status}.`)
      }
      if (job.status !== 'completed') {
        setPageError(`El job ${job.id} continúa en ${job.status}. Puedes volver a consultar la recomendación cuando termine.`)
        return
      }
      const updated = await loadAnalyticsWorkspace(resourceUID, window)
      setWorkspace(updated)
      const latestResult = updated.recommendations.find((item) => !previousRecommendationUIDs.has(item.recommendation_uid))
        ?? updated.recommendations[0]
        ?? null
      setGenerated(latestResult)
      if (!latestResult) setPageError('El job finalizó, pero no se encontró una recomendación persistida para el recurso.')
    } catch (error) {
      setPageError(error instanceof Error ? error.message : 'El backend no pudo generar la recomendación.')
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
        title="IA y analítica"
        description="La analítica del backend produce evidencia; su motor de reglas aplica políticas versionadas. No se ejecutan cambios en Azure."
        actions={<Badge variant="neutral"><BrainCircuit className="mr-1 size-3" />Backend · Analytics + Recommendation</Badge>}
      />

      <Panel title="Recurso y ventana temporal" description="Los datos y las fechas se consultan y procesan en los módulos existentes del backend.">
        <div className="grid gap-3 md:grid-cols-[minmax(0,2fr)_1fr_1fr_auto] md:items-end">
          <label className="space-y-1 text-xs text-muted-foreground">
            <span>Base de datos</span>
            <select value={resourceUID} onChange={(event) => setResourceUID(event.target.value)} disabled={loadingResources || resources.length === 0} className="h-10 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground">
              {resources.length === 0 && <option value="">Sin recursos accesibles</option>}
              {resources.map((resource) => <option key={resource.resource_uid} value={resource.resource_uid}>{resource.server_name}/{resource.database_name} · {resource.region_code}</option>)}
            </select>
          </label>
          <label className="space-y-1 text-xs text-muted-foreground">
            <span>Desde (UTC)</span>
            <input type="date" value={fromDate} onChange={(event) => setFromDate(event.target.value)} className="h-10 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
          </label>
          <label className="space-y-1 text-xs text-muted-foreground">
            <span>Hasta (UTC)</span>
            <input type="date" value={toDate} onChange={(event) => setToDate(event.target.value)} className="h-10 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
          </label>
          <Button onClick={runAnalysis} disabled={!resourceUID || !window || computing || loadingResources}>
            {computing ? <LoaderCircle className="mr-2 size-4 animate-spin" /> : <RefreshCw className="mr-2 size-4" />}
            {computing ? 'Analizando…' : 'Calcular evidencia'}
          </Button>
        </div>
        {!window && <p className="mt-2 text-xs text-danger">Selecciona fechas válidas; la fecha inicial debe ser anterior o igual a la fecha final.</p>}
        {activeResource && <p className="mt-3 break-all text-[11px] text-muted-foreground">resource_uid: {activeResource.resource_uid}</p>}
      </Panel>

      {pageError && <Notice kind="error" text={pageError} />}
      {computeMessages.length > 0 && <Notice kind="warning" text={`El backend reportó ${computeMessages.length} operación(es) incompleta(s): ${computeMessages.join(' · ')}`} />}
      {workspace?.errors.map((message) => <Notice key={message} kind="warning" text={message} />)}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="CPU p95 observado" value={formatPercent(cpuP95)} detail="Percentil 95 exacto guardado por Analytics" />
        <MetricCard label="CPU p99 observado" value={formatPercent(cpuP99)} detail="Percentil 99; no se deriva del promedio" />
        <MetricCard label="I/O p95 observado" value={formatPercent(ioP95)} detail="No disponible si falta muestra exacta" />
        <MetricCard label="Snapshots analíticos" value={workspace ? String(workspace.snapshots.length) : '—'} detail="Ventanas horarias en el período" />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Panel title="Evidencia calculada" description="Valores y método proceden de analytics; campos ausentes no se estiman localmente.">
          {!workspace ? <Loading /> : (
            <div className="space-y-3">
              <EvidenceRow label="CPU promedio (último snapshot)" value={formatPercent(latest(workspace.snapshots)?.CPUAvg)} />
              <EvidenceRow label="CPU máximo observado" value={formatPercent(latest(workspace.snapshots)?.CPUMax)} />
              <EvidenceRow label="Headroom de capacidad" value={formatPercent(workspace.capacity?.HeadroomPct)} />
              <EvidenceRow label="Utilización pico" value={formatPercent(workspace.capacity?.PeakUtilization)} />
              <EvidenceRow label="Costo mensual observado" value={money(workspace.costs?.ActualMonthlyCost, workspace.costs?.Currency)} />
              <EvidenceRow label="Proyección mensual" value={money(workspace.costs?.ProjectedMonthly, workspace.costs?.Currency)} />
              <EvidenceRow label="DES · capacidad" value={formatNumber(workspace.des?.CapacityE)} />
              <EvidenceRow label="DES · costo" value={formatNumber(workspace.des?.CostE)} />
              <EvidenceRow label="DES · rendimiento" value={formatNumber(workspace.des?.PerformanceE)} />
              <EvidenceRow label="Patrón de carga" value={workspace.workload?.IntensityBand ?? 'Sin evidencia'} />
              <EvidenceRow label="Tendencias disponibles" value={workspace.trends.map((trend) => `${trend.MetricCode ?? 'métrica'}: ${formatNumber(trend.Slope)}/h`).join(' · ') || 'Sin evidencia'} />
              <EvidenceRow label="Último cálculo" value={latest(workspace.snapshots)?.ComputedAt ? new Date(latest(workspace.snapshots)!.ComputedAt!).toLocaleString() : 'No calculado'} />
            </div>
          )}
        </Panel>

        <Panel title="Forecast" description="Opcional; debe provenir del proveedor ML conectado a analytics.MLPort.">
          {!workspace ? <Loading /> : workspace.forecasts.length > 0 ? (
            <div className="space-y-2">
              {workspace.forecasts.slice(0, 5).map((forecast) => <EvidenceRow key={forecast.ForecastUID ?? `${forecast.PeriodStartUTC}-${forecast.TargetMetric}`} label={`${forecast.TargetMetric ?? 'métrica'} · ${forecast.StepUnit ?? ''}`} value={`${formatNumber(forecast.YPred)}${forecast.YLower == null || forecast.YUpper == null ? '' : ` (${formatNumber(forecast.YLower)}–${formatNumber(forecast.YUpper)})`}`} />)}
            </div>
          ) : (
            <div className="flex gap-2 rounded-lg border border-border bg-muted/30 p-3 text-sm text-muted-foreground">
              <CircleHelp className="mt-0.5 size-4 shrink-0 text-primary" />
              No hay forecast persistido para este recurso y período. El backend tiene el contrato ML, pero requiere un proveedor productivo configurado; no se fabrica una predicción de respaldo.
            </div>
          )}
        </Panel>
      </div>

      <div className="grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
        <Panel title="Motor de Recommendation" description="Seleccione una política explícita; el resultado se guarda y exige revisión humana.">
          <div className="grid gap-3 sm:grid-cols-[1fr_auto] sm:items-end">
            <label className="space-y-1 text-xs text-muted-foreground">
              <span>Política habilitada</span>
              <select value={selectedPolicy} onChange={(event) => setSelectedPolicy(event.target.value)} className="h-10 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground">
                <option value="">Selecciona una política</option>
                {(workspace?.policies ?? []).map((policy) => <option key={policy.policy_uid} value={policy.policy_uid}>{policy.name} · v{policy.version}</option>)}
              </select>
            </label>
            <Button onClick={requestRecommendation} disabled={!selectedPolicy || !window || generating || !resourceUID}>
              {generating ? <LoaderCircle className="mr-2 size-4 animate-spin" /> : <Sparkles className="mr-2 size-4" />}
              {generating ? 'Evaluando…' : 'Generar recomendación'}
            </Button>
          </div>
          {(!workspace || workspace.policies.length === 0) && (
            <div className="mt-3 rounded-lg border border-warning/25 bg-warning/[0.06] p-3 text-xs text-muted-foreground">
              No hay políticas habilitadas. Cree una política versionada abajo; el formulario no activa reducción de capacidad ni cambio de modelo salvo que usted lo permita.
            </div>
          )}
          {jobStatus && <p className="mt-2 text-xs text-muted-foreground">Job {jobStatus.id} · {jobStatus.status}{jobStatus.error ? ` · ${jobStatus.error}` : ''}</p>}
          {latestRecommendation && <RecommendationCard recommendation={latestRecommendation} />}
          {!latestRecommendation && workspace?.recommendations.length === 0 && <p className="mt-3 text-sm text-muted-foreground">Aún no hay una recomendación persistida para este recurso.</p>}
        </Panel>

        <Panel title="Crear política versionada" description="Los umbrales son decisiones del equipo; no se inventan por el modelo.">
          <div className="space-y-3">
            <label className="block space-y-1 text-xs text-muted-foreground">Nombre
              <input value={newPolicyName} onChange={(event) => setNewPolicyName(event.target.value)} placeholder="Ej. right-sizing conservador" className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
            </label>
            <label className="block space-y-1 text-xs text-muted-foreground">Versión
              <input type="number" min="1" value={newPolicyVersion} onChange={(event) => setNewPolicyVersion(event.target.value)} className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
            </label>
              <label className="flex items-start gap-2 text-xs text-muted-foreground">
                <input type="checkbox" checked={allowScaleDown} onChange={(event) => setAllowScaleDown(event.target.checked)} className="mt-0.5" />
                Permitir que las reglas evalúen reducciones (no aplica cambios automáticamente).
              </label>
            <label className="block space-y-1 text-xs text-muted-foreground">Cobertura mínima horaria de CPU p95 (%)
              <input type="number" min="0.01" max="100" step="0.1" required value={minCPUCoverage} onChange={(event) => setMinCPUCoverage(event.target.value)} className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
              <span className="block text-[11px]">Debe ser mayor que 0 y hasta 100. Sin cobertura suficiente, el motor se abstiene.</span>
            </label>
            <div className="grid grid-cols-2 gap-2">
              <label className="space-y-1 text-xs text-muted-foreground">Riesgo máximo (0–1)
                <input type="number" min="0" max="1" step="0.01" value={maxRisk} onChange={(event) => setMaxRisk(event.target.value)} placeholder="Opcional" className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
              </label>
              <label className="space-y-1 text-xs text-muted-foreground">Confianza mínima (0–1)
                <input type="number" min="0" max="1" step="0.01" value={minConfidence} onChange={(event) => setMinConfidence(event.target.value)} placeholder="Opcional" className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground" />
              </label>
            </div>
            <Button variant="outline" onClick={createPolicy} disabled={savingPolicy || !newPolicyName.trim()} className="w-full">
              {savingPolicy ? <LoaderCircle className="mr-2 size-4 animate-spin" /> : <ShieldCheck className="mr-2 size-4" />}
              Guardar política en backend
            </Button>
          </div>
        </Panel>
      </div>

      <div className="flex items-start gap-2 rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
        <Database className="mt-0.5 size-4 shrink-0 text-primary" />
        <span>Las observaciones y evidencias viven en `cco_data`; autenticación, permisos, jobs y auditoría se mantienen en sus módulos del backend. Una recomendación es declarativa: la aprobación humana no ejecuta cambios en Azure.</span>
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
  return value == null || !Number.isFinite(value) ? 'Sin evidencia' : `${value.toFixed(1)}%`
}

function formatNumber(value?: number | null) {
  return value == null || !Number.isFinite(value) ? '—' : value.toFixed(2)
}

function money(value?: number, currency?: string) {
  if (value == null || !Number.isFinite(value)) return 'Sin evidencia'
  return new Intl.NumberFormat('es-MX', { style: 'currency', currency: currency || 'USD', maximumFractionDigits: 2 }).format(value)
}

function MetricCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return <div className="rounded-xl border border-border bg-card p-4 shadow-sm"><p className="text-xs text-muted-foreground">{label}</p><p className="mt-2 text-xl font-semibold tabular-nums text-foreground">{value}</p><p className="mt-1 text-[11px] text-muted-foreground">{detail}</p></div>
}

function EvidenceRow({ label, value }: { label: string; value: string }) {
  return <div className="flex items-start justify-between gap-3 border-b border-border/70 pb-2 text-xs last:border-0 last:pb-0"><span className="text-muted-foreground">{label}</span><span className="max-w-[60%] text-right font-medium text-foreground">{value}</span></div>
}

function Loading() {
  return <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" />Cargando desde el backend…</div>
}

function Notice({ kind, text }: { kind: 'error' | 'warning'; text: string }) {
  return <div className={`flex items-start gap-2 rounded-lg border p-3 text-xs ${kind === 'error' ? 'border-danger/25 bg-danger/[0.06] text-danger' : 'border-warning/25 bg-warning/[0.06] text-warning'}`}><AlertTriangle className="mt-0.5 size-3.5 shrink-0" />{text}</div>
}

function RecommendationCard({ recommendation }: { recommendation: BackendRecommendation }) {
  const saving = recommendation.estimated_saving
  const configValue = (config: Record<string, unknown>) => String(config.SKUName ?? config.sku_name ?? config.ComputeModel ?? config.compute_model ?? 'Configuración no disponible')
  return (
    <div className="mt-4 rounded-lg border border-border bg-background p-4">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div><p className="text-sm font-semibold text-foreground">{recommendation.action.description || recommendation.action.type}</p><p className="mt-1 text-xs text-muted-foreground">{configValue(recommendation.current_configuration)} → {configValue(recommendation.proposed_configuration)}</p></div>
        <Badge variant={recommendation.generation_state === 'complete' ? 'success' : 'warning'}>{recommendation.generation_state}</Badge>
      </div>
      <div className="mt-3 grid grid-cols-3 gap-2 text-center">
        <div className="rounded-md bg-muted/40 p-2"><p className="text-sm font-semibold">{money(saving?.monthly_amount, saving?.currency)}</p><p className="text-[10px] text-muted-foreground">ahorro potencial/mes</p></div>
        <div className="rounded-md bg-muted/40 p-2"><p className="text-sm font-semibold">{recommendation.risk?.level ?? '—'}</p><p className="text-[10px] text-muted-foreground">riesgo</p></div>
        <div className="rounded-md bg-muted/40 p-2"><p className="text-sm font-semibold">{recommendation.confidence ? `${(recommendation.confidence.score * 100).toFixed(0)}%` : '—'}</p><p className="text-[10px] text-muted-foreground">confianza</p></div>
      </div>
      <p className="mt-3 text-xs leading-relaxed text-muted-foreground">{recommendation.rationale?.summary || 'El backend no proporcionó una explicación adicional.'}</p>
      {recommendation.rationale?.factors?.length > 0 && <ul className="mt-2 space-y-1">{recommendation.rationale.factors.map((factor, index) => <li key={`${factor}-${index}`} className="flex gap-2 text-[11px] text-muted-foreground"><Check className="mt-0.5 size-3 shrink-0 text-primary" />{factor}</li>)}</ul>}
      <p className="mt-3 text-[10px] text-muted-foreground">ID {recommendation.recommendation_uid} · política v{recommendation.policy_version} · requiere revisión humana{recommendation.audit_status === 'audit_pending' ? ' · auditoría pendiente' : ''}</p>
    </div>
  )
}
