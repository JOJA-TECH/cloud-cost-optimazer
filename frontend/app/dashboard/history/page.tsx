'use client'

import { Fragment, useEffect, useState } from 'react'
import { ChevronDown, ArrowRight } from 'lucide-react'
import { PageHeader, Panel, RiskBadge } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import {
  getHistory,
  getAuditEvents,
  getSelectedResourceUid,
  getTimeRange,
} from '@/lib/api-services'
import type { HistoryItem, HistoryStatus } from '@/lib/api-services'

const statuses: (HistoryStatus | 'All')[] = ['All', 'New', 'Reviewed', 'Accepted', 'Rejected', 'Archived']

const statusVariant: Record<HistoryStatus, 'default' | 'neutral' | 'success' | 'warning' | 'danger'> = {
  New: 'default',
  Reviewed: 'neutral',
  Accepted: 'success',
  Rejected: 'danger',
  Archived: 'neutral',
}

export default function HistoryPage() {
  const [filter, setFilter] = useState<(typeof statuses)[number]>('All')
  const [open, setOpen] = useState<string | null>(null)
  const [rows, setRows] = useState<HistoryItem[]>(historyMock)
  const [auditEvents, setAuditEvents] = useState<AuditEventRow[]>([])
  const [resourceUid, setResourceUid] = useState<string | null>(null)
  const [historyLoaded, setHistoryLoaded] = useState(false)
  const [auditLoaded, setAuditLoaded] = useState(false)
  const [timeRange, setTimeRangeState] = useState<{ from: string; to: string } | null>(null)

  useEffect(() => {
    const uid = getSelectedResourceUid()
    const range = getTimeRange()
    setResourceUid(uid)
    setTimeRangeState(range)
  }, [])

  useEffect(() => {
    if (!resourceUid || !timeRange) return

    // Cargar historial de recomendaciones
    getHistory().then((h) => {
      setRows(h)
      setHistoryLoaded(true)
      if (!open || !h.find((x) => x.id === open)) {
        setOpen(h[0]?.id ?? null)
      }
    })

    // Cargar eventos de audit para este recurso
    getAuditEvents({
      entity_type: 'recommendation',
      from: timeRange.from,
      to: timeRange.to,
      limit: 50,
    }).then((res) => {
      if (res) {
        setAuditEvents(
          res.items.map((e) => ({
            event_id: e.event_id,
            occurred_at: e.occurred_at,
            action: e.action,
            actor_type: e.actor.actor_type,
            actor_user_id: e.actor.user_id ?? '—',
            actor_auth_method: e.actor.auth_method ?? '—',
            correlation_id: e.trace.correlation_id,
            operation_id: e.trace.operation_id ?? '—',
            resource_type: e.resource.type ?? '—',
            resource_id: e.resource.id ?? '—',
            details: e.metadata ?? {},
          }))
        )
      }
      setAuditLoaded(true)
    })
  }, [resourceUid, timeRange])

  const filtered = rows.filter((h) => filter === 'All' || h.status === filter)

  return (
    <div>
      <PageHeader
        title="History"
        description="Historial de recomendaciones y traza de auditoría. El historial muestra decisiones de right-sizing generadas; la traza de auditoría registra quién tomó cada decisión y cuándo."
      />

      {/* Filtro de estado */}
      <div className="mb-4 flex flex-wrap items-center gap-1.5">
        {statuses.map((s) => (
          <button
            key={s}
            onClick={() => setFilter(s)}
            className={cn(
              'rounded-md border px-2.5 py-1 text-xs font-medium transition-colors',
              filter === s
                ? 'border-primary bg-primary text-primary-foreground'
                : 'border-border bg-card text-muted-foreground hover:text-foreground',
            )}
          >
            {s}
          </button>
        ))}
      </div>

      {/* Sección 1: Historial de recomendaciones */}
      <Panel
        title="Historial de recomendaciones"
        description="Cada recomendación generada con su configuración, ahorro, riesgo, confianza y estado."
        className="mb-4"
        action={<Badge variant="outline" className="text-[9px]">{historyLoaded ? 'Datos reales' : 'Demo'}</Badge>}
      >
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] text-sm">
            <thead>
              <tr className="border-b border-border text-xs text-muted-foreground">
                <th className="px-5 py-2.5 text-left font-medium">Date</th>
                <th className="px-3 py-2.5 text-left font-medium">Resource</th>
                <th className="px-3 py-2.5 text-left font-medium">Change</th>
                <th className="px-3 py-2.5 text-right font-medium">Savings</th>
                <th className="px-3 py-2.5 text-left font-medium">Risk</th>
                <th className="px-3 py-2.5 text-right font-medium">Conf.</th>
                <th className="px-3 py-2.5 text-left font-medium">Status</th>
                <th className="px-5 py-2.5" />
              </tr>
            </thead>
            <tbody>
              {filtered.map((h) => (
                <Fragment key={h.id}>
                  <tr
                    className="cursor-pointer border-b border-border hover:bg-muted/40"
                    onClick={() => setOpen(open === h.id ? null : h.id)}
                  >
                    <td className="px-5 py-3 tabular-nums text-muted-foreground">{h.date}</td>
                    <td className="px-3 py-3 font-mono text-xs text-foreground">{h.resource}</td>
                    <td className="px-3 py-3">
                      <span className="flex items-center gap-1.5 text-foreground">
                        {h.current}
                        <ArrowRight className="size-3 text-primary" />
                        {h.proposed}
                      </span>
                    </td>
                    <td className="px-3 py-3 text-right tabular-nums text-foreground">{h.savings}</td>
                    <td className="px-3 py-3">{h.risk === 'None' ? <span className="text-muted-foreground">—</span> : <RiskBadge risk={h.risk} />}</td>
                    <td className="px-3 py-3 text-right tabular-nums text-muted-foreground">{h.confidence}%</td>
                    <td className="px-3 py-3">
                      <Badge variant={statusVariant[h.status]}>{h.status}</Badge>
                    </td>
                    <td className="px-5 py-3 text-right">
                      <ChevronDown className={cn('inline size-4 text-muted-foreground transition-transform', open === h.id && 'rotate-180')} />
                    </td>
                  </tr>
                  {open === h.id && (
                    <tr className="border-b border-border bg-muted/30">
                      <td colSpan={8} className="px-5 py-4">
                        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                          <Detail label="Recommendation ID" value={h.id} mono />
                          <Detail label="Model / rule version" value={h.version} mono />
                          <Detail label="Analysis period" value={h.analysisPeriod ?? 'Last 30 days'} />
                          <Detail label="Evidence snapshot" value="Frozen at generation" />
                        </div>
                        <p className="mt-3 text-xs text-muted-foreground">
                          Opening a past recommendation replays the evidence exactly as it was when
                          generated — CPU percentiles, headroom, SLA history, and cost source — so
                          the decision remains reproducible and auditable.
                        </p>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={8} className="px-5 py-8 text-center text-xs text-muted-foreground">
                    No hay recomendaciones para mostrar.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Panel>

      {/* Sección 2: Traza de auditoría */}
      <Panel
        title="Traza de auditoría (Audit Trail)"
        description="Eventos de auditoría registrados para decisiones de recomendación: quién, cuándo, qué acción y evidencia asociada."
        className="mt-4"
        action={<Badge variant="outline" className="text-[9px]">{auditLoaded ? 'Datos reales' : 'Demo'}</Badge>}
      >
        {!auditLoaded ? (
          <div className="p-4 text-center text-xs text-muted-foreground">
            Cargando traza de auditoría...
          </div>
        ) : auditEvents.length === 0 ? (
          <div className="p-4 text-center text-xs text-muted-foreground">
            No hay eventos de auditoría para el recurso y ventana seleccionados.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[960px] text-sm">
              <thead>
                <tr className="border-b border-border text-xs text-muted-foreground">
                  <th className="px-5 py-2.5 text-left font-medium">Fecha</th>
                  <th className="px-3 py-2.5 text-left font-medium">Acción</th>
                  <th className="px-3 py-2.5 text-left font-medium">Actor</th>
                  <th className="px-3 py-2.5 text-left font-medium">Método</th>
                  <th className="px-3 py-2.5 text-left font-medium">Correlation ID</th>
                  <th className="px-3 py-2.5 text-left font-medium">Operation ID</th>
                  <th className="px-5 py-2.5 text-left font-medium">Recurso</th>
                </tr>
              </thead>
              <tbody>
                {auditEvents.map((e) => (
                  <tr key={e.event_id} className="border-b border-border last:border-0 hover:bg-muted/40 cursor-pointer">
                    <td className="px-5 py-3 tabular-nums text-xs text-muted-foreground font-mono">
                      {e.occurred_at}
                    </td>
                    <td className="px-3 py-3 text-xs text-foreground">{e.action}</td>
                    <td className="px-3 py-3 text-xs font-mono text-foreground">{e.actor_user_id}</td>
                    <td className="px-3 py-3 text-xs text-muted-foreground">{e.actor_auth_method}</td>
                    <td className="px-3 py-3 text-xs font-mono text-foreground">
                      {e.correlation_id.slice(0, 8)}...
                    </td>
                    <td className="px-3 py-3 text-xs font-mono text-foreground">
                      {e.operation_id === '—' ? '—' : e.operation_id.slice(0, 8) + '...'}
                    </td>
                    <td className="px-3 py-3 text-xs text-muted-foreground">
                      {e.resource_type} / {e.resource_id}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>
    </div>
  )
}

function Detail({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2">
      <p className="text-[11px] text-muted-foreground">{label}</p>
      <p className={cn('mt-0.5 text-xs font-medium text-foreground', mono && 'font-mono')}>{value}</p>
    </div>
  )
}

const historyMock: HistoryItem[] = [
  { id: 'rec-2041', date: '2026-09-13', resource: 'sql-prod-orders-eastus', current: '8 vCores', proposed: '4 vCores', savings: '24.2%', risk: 'Low', confidence: 91, status: 'New', version: 'v1.4.2', analysisPeriod: 'Last 30 days' },
  { id: 'rec-1990', date: '2026-08-30', resource: 'sql-prod-billing-eastus', current: '6 vCores', proposed: '4 vCores', savings: '18.6%', risk: 'Low', confidence: 88, status: 'Accepted', version: 'v1.4.1', analysisPeriod: 'Last 30 days' },
  { id: 'rec-1943', date: '2026-08-14', resource: 'sql-prod-analytics-westus', current: '16 vCores', proposed: '16 vCores', savings: '—', risk: 'None', confidence: 62, status: 'Reviewed', version: 'v1.4.1', analysisPeriod: 'Last 30 days' },
  { id: 'rec-1902', date: '2026-07-29', resource: 'sql-prod-orders-eastus', current: '8 vCores', proposed: '6 vCores', savings: '11.9%', risk: 'Medium', confidence: 71, status: 'Rejected', version: 'v1.4.0', analysisPeriod: 'Last 30 days' },
  { id: 'rec-1855', date: '2026-07-11', resource: 'sql-prod-identity-eastus', current: '4 vCores', proposed: '—', savings: '—', risk: 'None', confidence: 41, status: 'Archived', version: 'v1.3.9', analysisPeriod: 'Last 30 days' },
]

type AuditEventRow = {
  event_id: number
  occurred_at: string
  action: string
  actor_type: string
  actor_user_id: string
  actor_auth_method: string
  correlation_id: string
  operation_id: string
  resource_type: string
  resource_id: string
  details: Record<string, any>
}
