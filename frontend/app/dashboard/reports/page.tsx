'use client'

import { useEffect, useState } from 'react'
import {
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
  getReportStatus,
  cancelReport,
  downloadReportArtifact,
  getSelectedResourceUid,
  getTimeRange,
  type ReportRequest,
} from '@/lib/api-services'

type ReportFormat = 'json' | 'csv' | 'html'

type ReportRow = {
  kind: 'sync' | 'async'
  job_uid: string
  status: string
  report_uid: string | null
  artifact_uid: string | null
  created_at: string
  completed_at: string | null
  error_message: string | null
  format: ReportFormat
  downloadable: boolean
}

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
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const uid = getSelectedResourceUid()
    const range = getTimeRange()
    setResourceUid(uid)
    setTimeRangeState(range)
  }, [])

  async function refreshStatuses() {
    setLoadingReports(true)
    try {
      await Promise.all(reports.filter((report) => report.kind === 'async').map((report) => handleRefreshStatus(report.job_uid)))
    } finally {
      setLoadingReports(false)
    }
  }

  async function handleGenerateReport(e: React.FormEvent) {
    e.preventDefault()
    if (!resourceUid || !timeRange) return

    setGenerating(true)
    setError(null)
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
        const { report, artifact } = result.data
        setReports((prev) => [
          {
            kind: 'sync',
            job_uid: report.uid,
            status: report.status,
            report_uid: report.uid,
            artifact_uid: artifact.uid,
            created_at: report.created_at,
            completed_at: report.completed_at ?? null,
            error_message: null,
            format: report.format,
            downloadable: !!artifact.storage_uri,
          },
          ...prev,
        ])
      } else {
        setError(result.error ?? 'Could not generate the report.')
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not generate the report.')
    } finally {
      setGenerating(false)
    }
  }

  async function handleQueueReport(e: React.FormEvent) {
    e.preventDefault()
    if (!resourceUid || !timeRange) return

    setGenerating(true)
    setError(null)
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
        const data = job.data
        setReports((prev) => [
          {
            kind: 'async',
            job_uid: data.job_uid,
            status: data.status,
            report_uid: data.report_uid,
            artifact_uid: data.artifact_uid ?? null,
            created_at: data.created_at,
            completed_at: data.completed_at ?? null,
            error_message: data.error_message ?? null,
            format: format,
            downloadable: !!data.artifact_uid && !!data.report_uid,
          },
          ...prev,
        ])
      } else {
        setError(job.error ?? 'Could not queue the report.')
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not queue the report.')
    } finally {
      setGenerating(false)
    }
  }

  async function handleRefreshStatus(jobUid: string) {
    const job = await getReportStatus(jobUid)
    if (job.source === 'backend' && job.data) {
      const data = job.data
      setReports((prev) =>
        prev.map((r) =>
          r.job_uid === jobUid
            ? {
                ...r,
                status: data.status,
                report_uid: data.report_uid ?? r.report_uid,
                artifact_uid: data.artifact_uid ?? r.artifact_uid,
                completed_at: data.completed_at ?? r.completed_at,
                error_message: data.error_message ?? r.error_message,
                downloadable: !!(data.artifact_uid ?? r.artifact_uid) && !!(data.report_uid ?? r.report_uid),
              }
            : r
        )
      )
    } else {
      setError(job.error ?? 'Could not retrieve the report status.')
    }
  }

  async function handleCancel(jobUid: string) {
    const result = await cancelReport(jobUid)
    if (result.source === 'backend' && result.data) {
      const status = result.data.status
      setReports((prev) =>
        prev.map((r) =>
          r.job_uid === jobUid
            ? { ...r, status }
            : r
        )
      )
    } else {
      setError(result.error ?? 'Could not cancel the report.')
    }
  }

  async function handleDownload(jobUid: string) {
    const report = reports.find((r) => r.job_uid === jobUid)
    if (!report?.report_uid) return

    const result = await downloadReportArtifact(report.report_uid)
    if (result.status !== 'ready' || !result.data) {
      setError(result.error ?? 'Could not download the report.')
      return
    }

    const url = URL.createObjectURL(result.data)
    const a = document.createElement('a')
    a.href = url
    a.download = `report-${report.report_uid}.${report.format}`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
  }

  function formatDate(ts: string | null): string {
    if (!ts) return '—'
    return new Date(ts).toLocaleString('en-US')
  }

  function statusBadge(status: string) {
    const variants: Record<string, NonNullable<React.ComponentProps<typeof Badge>['variant']>> = {
      queued: 'warning',
      running: 'warning',
      completed: 'success',
      failed: 'danger',
      cancelled: 'neutral',
      generating: 'warning',
    }
    return <Badge variant={variants[status] || 'neutral'}>{status}</Badge>
  }

  return (
    <div>
      <PageHeader
        title="Reports"
        description="Generate optimization reports in JSON, CSV, or HTML. Reports can be generated synchronously or queued as asynchronous jobs."
      />

      {!resourceUid ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          <p className="text-sm text-muted-foreground">No resource selected</p>
          <p className="text-xs text-muted-foreground">
            Select a cloud resource from the top bar to generate reports.
          </p>
        </div>
      ) : (
        <>
          <Panel title="Generate optimization report" className="mb-4" action={<Badge variant="outline" className="text-[9px]">backend</Badge>}>
            <form onSubmit={handleGenerateReport} className="space-y-4">
              <div className="grid gap-4 md:grid-cols-3">
                <div className="flex flex-col">
                  <Label>Output format</Label>
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
                  <Label>Report title (optional)</Label>
                  <Input
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                    placeholder="e.g. Q3 2026 optimization report"
                  />
                </div>
                <div className="flex flex-col items-end">
                  <Button type="submit" disabled={generating} className="w-full">
                    {generating ? 'Generating...' : 'Generate report (synchronous)'}
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
                  Include audit
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={includeEvidence}
                    onChange={(e) => setIncludeEvidence(e.target.checked)}
                    className="rounded border-input bg-background"
                  />
                  Include evidence
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={includeRisks}
                    onChange={(e) => setIncludeRisks(e.target.checked)}
                    className="rounded border-input bg-background"
                  />
                  Include risks
                </label>
                <label className="flex items-center gap-2 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={includeMetrics}
                    onChange={(e) => setIncludeMetrics(e.target.checked)}
                    className="rounded border-input bg-background"
                  />
                  Include metrics
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
                {generating ? 'Queuing...' : 'Queue report as a job (asynchronous)'}
              </Button>
              <p className="mt-2 text-xs text-muted-foreground">
                Asynchronous jobs let you check their status and download the report when ready.
              </p>
            </form>
            {error && <p role="alert" className="mt-3 text-xs text-danger">{error}</p>}
          </Panel>

          <Panel title="Reports and jobs" className="mt-4" action={<Badge variant="outline" className="text-[9px]">Current session</Badge>}>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-semibold text-foreground">
                {reports.length} {reports.length === 1 ? 'report' : 'reports'}
              </h3>
              <Button variant="outline" size="sm" onClick={() => void refreshStatuses()} disabled={loadingReports}>
                <RefreshCw className="size-4 mr-1" />
                {loadingReports ? 'Refreshing...' : 'Refresh statuses'}
              </Button>
            </div>

            {reports.length === 0 ? (
              <div className="p-4 text-center text-xs text-muted-foreground">
                No reports generated in this session. The backend does not provide a report list.
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border text-xs text-muted-foreground">
                      <th className="px-4 py-2 text-left">Job UID</th>
                      <th className="px-4 py-2 text-left">Status</th>
                      <th className="px-4 py-2 text-left">Format</th>
                      <th className="px-4 py-2 text-left">Created</th>
                      <th className="px-4 py-2 text-left">Completed</th>
                      <th className="px-4 py-2 text-left">Error</th>
                      <th className="px-4 py-2 text-right">Actions</th>
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
                            {r.kind === 'async' && r.status === 'running' && (
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => handleRefreshStatus(r.job_uid)}
                                className="px-2"
                              >
                                <Clock className="size-3.5" />
                              </Button>
                            )}
                            {r.kind === 'async' && (r.status === 'queued' || r.status === 'running') ? (
                              <Button
                                size="sm"
                                variant="destructive"
                                onClick={() => handleCancel(r.job_uid)}
                                className="px-2"
                              >
                                <Trash2 className="size-3.5" />
                              </Button>
                            ) : r.kind === 'async' ? (
                              <Button
                                size="sm"
                                onClick={() => handleRefreshStatus(r.job_uid)}
                                className="px-2"
                              >
                                <RefreshCw className="size-3.5" />
                              </Button>
                            ) : null}
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
