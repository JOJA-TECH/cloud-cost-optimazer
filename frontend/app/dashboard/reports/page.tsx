'use client'

import { useEffect, useState } from 'react'
import {
  ArrowRight,
  Clock,
  Download,
  FileText,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  generateOptimizationReport,
  queueOptimizationReport,
  getReport,
  getReportStatus,
  cancelReport,
  downloadReportArtifact,
  getSelectedResourceUid,
  getTimeRange,
  type ReportRequest,
} from '@/lib/api-services'

type ReportFormat = 'json' | 'csv' | 'html'

const formatOptions: Array<{ value: ReportFormat; label: string }> = [
  { value: 'json', label: 'JSON' },
  { value: 'csv', label: 'CSV' },
  { value: 'html', label: 'HTML' },
]

export default function ReportsPage() {
  const [resourceUid, setResourceUid] = useState<string | null>(null)
  const [timeRange, setTimeRangeState] = useState<{ from: string; to: string } | null>(null)
  const [format, setFormat] = useState<ReportFormat>('json')
  const [title, setTitle] = useState('')
  const [includeAudit, setIncludeAudit] = useState(false)
  const [includeEvidence, setIncludeEvidence] = useState(true)
  const [includeRisks, setIncludeRisks] = useState(true)
  const [includeMetrics, setIncludeMetrics] = useState(true)
  const [generating, setGenerating] = useState(false)
  const [reports, setReports] = useState<ReportRow[]>([])
  const [loadingReports, setLoadingReports] = useState(false)

  useEffect(() => {
    const uid = getSelectedResourceUid()
    const range = getTimeRange()
    setResourceUid(uid)
    setTimeRangeState(range)
    loadReports()
  }, [])

  async function loadReports() {
    setLoadingReports(true)
    try {
      setReports([])
    } catch {
      // Ignorar errores al cargar
    } finally {
      setLoadingReports(false)
    }
  }

  async function handleGenerateReport(e: React.FormEvent) {
    e.preventDefault()
    if (!resourceUid || !timeRange) return

    setGenerating(true)
    try {
      const request: ReportRequest = {
        resource_uid: resourceUid,
        from: timeRange.from,
        to: timeRange.to,
        format,
        title: title || undefined,
        include_audit: includeAudit,
        include_evidence: includeEvidence,
        include_risks: includeRisks,
        include_metrics: includeMetrics,
      }

      const result = await generateOptimizationReport(request)
      if (result.source === 'backend' && result.data) {
        setReports((prev) => [
          {
            job_uid: result.data.report_uid,
            status: result.data.status,
            report_uid: result.data.report_uid,
            artifact_uid: result.data.artifact?.uid ?? null,
            created_at: result.data.created_at,
            started_at: null,
            completed_at: null,
            error_message: null,
            report_type: result.data.format,
            format: result.data.format,
            downloadable: !!result.data.artifact?.storage_uri,
          },
          ...prev,
        ])
      }
    } catch {
      // Error manejado por tryBackend
    } finally {
      setGenerating(false)
    }
  }

  async function handleQueueReport(e: React.FormEvent) {
    e.preventDefault()
    if (!resourceUid || !timeRange) return

    setGenerating(true)
    try {
      const request: ReportRequest = {
        resource_uid: resourceUid,
        from: timeRange.from,
        to: timeRange.to,
        format,
        title: title || undefined,
        include_audit: includeAudit,
        include_evidence: includeEvidence,
        include_risks: includeRisks,
        include_metrics: includeMetrics,
      }

      const job = await queueOptimizationReport(request)
      if (job.source === 'backend' && job.data) {
        setReports((prev) => [
          {
            job_uid: job.data.job_uid,
            status: job.data.status,
            report_uid: job.data.report_uid ?? null,
            artifact_uid: job.data.artifact_uid ?? null,
            created_at: job.data.created_at,
            started_at: job.data.started_at ?? null,
            completed_at: job.data.completed_at ?? null,
            error_message: job.data.error_message ?? null,
            report_type: job.data.report_type,
            format: format,
            downloadable: !!job.data.artifact_uid,
          },
          ...prev,
        ])
      }
    } catch {
      // Error manejado por tryBackend
    } finally {
      setGenerating(false)
    }
  }

  async function handleRefreshStatus(jobUid: string) {
    const job = await getReportStatus(jobUid)
    if (job.source === 'backend' && job.data) {
      setReports((prev) =>
        prev.map((r) =>
          r.job_uid === jobUid
            ? {
                ...r,
                status: job.data.status,
                report_uid: job.data.report_uid ?? r.report_uid,
                artifact_uid: job.data.artifact_uid ?? r.artifact_uid,
                completed_at: job.data.completed_at ?? r.completed_at,
                error_message: job.data.error_message ?? r.error_message,
                downloadable: !!job.data.artifact_uid,
              }
            : r
        )
      )
    }
  }

  async function handleCancel(jobUid: string) {
    const result = await cancelReport(jobUid)
    if (result && result.source === 'backend' && result.data) {
      setReports((prev) =>
        prev.map((r) =>
          r.job_uid === jobUid
            ? { ...r, status: result.data.status }
            : r
        )
      )
    }
  }

  async function handleDownload(jobUid: string) {
    const report = reports.find((r) => r.job_uid === jobUid)
    if (!report?.report_uid) return

    const blob = await downloadReportArtifact(report.report_uid)
    if (!blob) return

    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `report-${jobUid}.${format}`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  function formatDate(ts: number | null): string {
    if (!ts) return '—'
    return new Date(ts).toLocaleString()
  }

  function statusBadge(status: string) {
    const variants: Record<string, string> = {
      pending: 'warning',
      running: 'warning',
      completed: 'success',
      failed: 'destructive',
      cancelled: 'neutral',
    }
    return <Badge variant={variants[status] || 'neutral'}>{status}</Badge>
  }

  return (
    <div>
      <PageHeader
        title="Reports"
        description="Genera reportes de optimización en formato JSON, CSV o HTML. Los reportes pueden generarse síncronamente o encolarse como jobs asíncronos."
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="outline" className="text-[9px]">{resourceUid ? 'Datos reales' : 'Demo'}</Badge>
          </div>
        }
      />

      {!resourceUid ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          <p className="text-sm text-muted-foreground">No hay recurso seleccionado</p>
          <p className="text-xs text-muted-foreground">
            Selecciona un recurso cloud desde el barra superior para generar reportes.
          </p>
        </div>
      ) : (
        <>
          <Panel title="Generar reporte de optimización" className="mb-4">
            <form onSubmit={handleGenerateReport} className="space-y-4">
              <div className="grid gap-4 md:grid-cols-3">
                <div className="flex flex-col">
                  <Label>Formato de salida</Label>
                  <select
                    value={format}
                    onChange={(e) => setFormat(e.target.value as ReportFormat)}
                    className="border border-input bg-background px-3 py-2 text-sm rounded-md focus:ring-2 focus:ring-ring"
                  >
                    {formatOptions.map((f) => (
                      <option key={f.value} value={f.value}>
                        {f.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="flex flex-col">
                  <Label>Título del reporte (opcional)</Label>
                  <Input
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                    placeholder="Ej. Reporte de optimización Q3 2026"
                  />
                </div>
                <div className="flex flex-col items-end">
                  <Button type="submit" disabled={generating} className="w-full">
                    {generating ? 'Generando...' : 'Generar reporte (síncrono)'}
                  </Button>
                </div>
              </div>

              <div className="grid gap-2 md:grid-cols-4">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={includeAudit}
                    onChange={(e) => setIncludeAudit(e.target.checked)}
                    className="rounded border-input bg-background"
                  />
                  Incluir auditoría
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={includeEvidence}
                    onChange={(e) => setIncludeEvidence(e.target.checked)}
                    className="rounded border-input bg-background"
                  />
                  Incluir evidencia
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={includeRisks}
                    onChange={(e) => setIncludeRisks(e.target.checked)}
                    className="rounded border-input bg-background"
                  />
                  Incluir riesgos
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={includeMetrics}
                    onChange={(e) => setIncludeMetrics(e.target.checked)}
                    className="rounded border-input bg-background"
                  />
                  Incluir métricas
                </label>
              </div>
            </form>

            <form onSubmit={handleQueueReport} className="mt-4 border-t border-border pt-4">
              <Button
                type="submit"
                variant="outline"
                disabled={generating}
                className="w-full"
              >
                {generating ? 'Encolando...' : 'Encolar reporte como job (asíncrono)'}
              </Button>
              <p className="mt-2 text-xs text-muted-foreground">
                Los jobs asíncronos permiten consultar su estado y descargar el artefacto cuando estén listos.
              </p>
            </form>
          </Panel>

          <Panel title="Reportes y jobs" className="mt-4">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-foreground">
                {reports.length} reporte{reports.length !== 1 ? 's' : ''}
              </h3>
              <Button variant="outline" size="sm" onClick={loadReports}>
                <RefreshCw className="size-4 mr-1" />
                Actualizar
              </Button>
            </div>

            {reports.length === 0 ? (
              <div className="p-4 text-center text-xs text-muted-foreground">
                No hay reportes generados todavía.
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border text-xs text-muted-foreground">
                      <th className="px-4 py-2 text-left">Job UID</th>
                      <th className="px-4 py-2 text-left">Estado</th>
                      <th className="px-4 py-2 text-left">Formato</th>
                      <th className="px-4 py-2 text-left">Creado</th>
                      <th className="px-4 py-2 text-left">Completado</th>
                      <th className="px-4 py-2 text-left">Error</th>
                      <th className="px-4 py-2 text-right">Acciones</th>
                    </tr>
                  </thead>
                  <tbody>
                    {reports.map((r) => (
                      <tr
                        key={r.job_uid}
                        className="border-b border-border last:border-0 hover:bg-muted/40"
                      >
                        <td className="px-4 py-3 text-xs font-mono text-foreground">
                          {r.job_uid.slice(0, 8)}...
                        </td>
                        <td className="px-4 py-3 text-xs">{statusBadge(r.status)}</td>
                        <td className="px-4 py-3 text-xs text-muted-foreground">
                          {r.format.toUpperCase()}
                        </td>
                        <td className="px-4 py-3 text-xs text-muted-foreground font-mono">
                          {formatDate(r.created_at)}
                        </td>
                        <td className="px-4 py-3 text-xs text-muted-foreground font-mono">
                          {formatDate(r.completed_at)}
                        </td>
                        <td className="px-4 py-3 text-xs text-muted-foreground">
                          {r.error_message ?? '—'}
                        </td>
                        <td className="px-4 py-3 text-right">
                          <div className="flex items-center gap-1">
                            {r.status === 'running' && (
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => handleRefreshStatus(r.job_uid)}
                                className="px-2"
                              >
                                <Clock className="size-3.5" />
                              </Button>
                            )}
                            {r.status === 'pending' || r.status === 'running' ? (
                              <Button
                                size="sm"
                                variant="destructive"
                                onClick={() => handleCancel(r.job_uid)}
                                className="px-2"
                              >
                                <Trash2 className="size-3.5" />
                              </Button>
                            ) : (
                              <Button
                                size="sm"
                                onClick={() => handleRefreshStatus(r.job_uid)}
                                className="px-2"
                              >
                                <RefreshCw className="size-3.5" />
                              </Button>
                            )}
                            {r.downloadable ? (
                              <Button
                                size="sm"
                                variant="default"
                                onClick={() => handleDownload(r.job_uid)}
                                className="px-2"
                              >
                                <Download className="size-3.5" />
                              </Button>
                            ) : (
                              <Button size="sm" disabled className="px-2 opacity-50">
                                <FileText className="size-3.5" />
                              </Button>
                            )}
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Panel>
        </>
      )}
    </div>
  )
}
