'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { AlertTriangle, Check, Clock3, RefreshCw, X } from 'lucide-react'
import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  decideBackendRecommendation,
  listBackendRecommendations,
  type BackendRecommendation,
} from '@/lib/ai-services'

function formatMoney(value: number, currency: string) {
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(value)
  } catch {
    return `${value.toFixed(2)} ${currency}`
  }
}

function basisLabel(value: string) {
  if (value === 'actual_cost_daily') return 'Costo observado normalizado'
  if (value === 'price_catalog') return 'Precio de lista del catálogo'
  if (value === 'scenario_simulation') return 'Simulación de escenario'
  return value || 'No especificada'
}

export default function RecommendationsPage() {
  const [items, setItems] = useState<BackendRecommendation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busyUid, setBusyUid] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setItems(await listBackendRecommendations())
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'No se pudieron cargar las recomendaciones.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  async function decide(uid: string, action: 'accept' | 'reject') {
    setBusyUid(uid)
    setError(null)
    try {
      await decideBackendRecommendation(uid, action)
      await refresh()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'No se pudo guardar la decisión humana.')
    } finally {
      setBusyUid(null)
    }
  }

  return (
    <div>
      <PageHeader
        title="Revisión de recomendaciones"
        description="Decisiones persistidas por el motor del backend. Revisa evidencia, bases de costo y limitaciones antes de aceptar o rechazar; esta pantalla nunca cambia recursos en Azure."
        actions={
          <Button variant="outline" size="sm" onClick={() => void refresh()} disabled={loading}>
            <RefreshCw className="mr-2 size-3.5" /> Actualizar
          </Button>
        }
      />

      {error && (
        <div role="alert" className="mb-4 rounded-lg border border-danger/30 bg-danger/5 p-3 text-sm text-danger">
          {error}
        </div>
      )}

      {loading && items.length === 0 ? (
        <Panel><p className="text-sm text-muted-foreground">Consultando recomendaciones persistidas…</p></Panel>
      ) : items.length === 0 ? (
        <Panel title="No hay recomendaciones persistidas">
          <p className="text-sm text-muted-foreground">
            No se muestran datos de ejemplo. Calcula analítica y encola una evaluación desde Intelligence para generar una decisión trazable.
          </p>
          <Link href="/dashboard/intelligence" className={buttonVariants({ className: 'mt-4' })}>Ir a Intelligence</Link>
        </Panel>
      ) : (
        <div className="space-y-4">
          {items.map((item) => {
            const isReviewable = item.status === 'proposed' || item.status === 'under_review'
            const isChange = item.action.type !== 'no_action'
            const amount = item.estimated_saving.monthly_amount
            return (
              <Panel key={item.recommendation_uid}>
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <div className="flex flex-wrap items-center gap-2">
                      <h2 className="text-base font-semibold text-foreground">{item.action.description || item.action.type}</h2>
                      <Badge variant={item.status === 'accepted' ? 'success' : 'neutral'}>{item.status}</Badge>
                      <Badge variant="outline">Riesgo {item.risk.level}</Badge>
                      <Badge variant="outline">{item.generation_state}</Badge>
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Recurso {item.resource_uid} · Política v{item.policy_version} · {new Date(item.created_at).toLocaleString()}
                    </p>
                  </div>
                  {isReviewable && (
                    <div className="flex gap-2">
                      <Button size="sm" onClick={() => void decide(item.recommendation_uid, 'accept')} disabled={busyUid !== null}>
                        <Check className="mr-1.5 size-3.5" />{isChange ? 'Aceptar recomendación' : 'Confirmar revisión'}
                      </Button>
                      <Button size="sm" variant="outline" onClick={() => void decide(item.recommendation_uid, 'reject')} disabled={busyUid !== null}>
                        <X className="mr-1.5 size-3.5" /> Rechazar
                      </Button>
                    </div>
                  )}
                </div>

                <div className="mt-4 grid gap-3 md:grid-cols-3">
                  <section className="rounded-lg border border-border bg-background p-3">
                    <p className="text-xs text-muted-foreground">Configuración observada</p>
                    <p className="mt-1 font-medium text-foreground">{String(item.current_configuration.SKUName ?? item.current_configuration.sku_name ?? 'No disponible')}</p>
                  </section>
                  <section className="rounded-lg border border-border bg-background p-3">
                    <p className="text-xs text-muted-foreground">Configuración propuesta</p>
                    <p className="mt-1 font-medium text-foreground">{String(item.proposed_configuration.SKUName ?? item.proposed_configuration.sku_name ?? 'No disponible')}</p>
                  </section>
                  <section className="rounded-lg border border-border bg-background p-3">
                    <p className="text-xs text-muted-foreground">Ahorro potencial estimado / mes</p>
                    <p className="mt-1 font-semibold tabular-nums text-foreground">{formatMoney(amount, item.estimated_saving.currency)}</p>
                    <p className="text-xs text-muted-foreground">No representa ahorro realizado</p>
                  </section>
                </div>

                <div className="mt-4 grid gap-4 lg:grid-cols-2">
                  <section>
                    <h3 className="text-sm font-semibold text-foreground">Explicación y evidencia</h3>
                    <p className="mt-1 text-sm text-muted-foreground">{item.rationale.summary}</p>
                    {item.rationale.factors.length > 0 && (
                      <ul className="mt-2 space-y-1">
                        {item.rationale.factors.map((factor, index) => (
                          <li key={`${item.recommendation_uid}-${index}`} className="flex items-start gap-2 text-xs text-muted-foreground">
                            <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-warning" />{factor}
                          </li>
                        ))}
                      </ul>
                    )}
                  </section>
                  <section>
                    <h3 className="text-sm font-semibold text-foreground">Bases y supuestos económicos</h3>
                    <dl className="mt-2 space-y-1 text-xs">
                      <div className="flex justify-between gap-3"><dt className="text-muted-foreground">Base actual</dt><dd className="text-right text-foreground">{basisLabel(item.estimated_saving.current_cost_basis)}</dd></div>
                      <div className="flex justify-between gap-3"><dt className="text-muted-foreground">Base propuesta</dt><dd className="text-right text-foreground">{basisLabel(item.estimated_saving.proposed_cost_basis)}</dd></div>
                      <div className="flex justify-between gap-3"><dt className="text-muted-foreground">Método</dt><dd className="text-right text-foreground">{item.estimated_saving.method}</dd></div>
                    </dl>
                    <ul className="mt-2 space-y-1">
                      {item.estimated_saving.comparison_assumptions.map((assumption) => (
                        <li key={assumption} className="text-xs text-muted-foreground">• {assumption}</li>
                      ))}
                    </ul>
                  </section>
                </div>

                <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-border pt-3 text-xs text-muted-foreground">
                  <span className="inline-flex items-center gap-1.5"><Clock3 className="size-3.5" /> {item.evidence_count} referencias de evidencia</span>
                  <span>{item.audit_status === 'audit_pending' ? 'Auditoría pendiente de reconciliación' : 'Toda decisión de aplicación queda fuera de este flujo'}</span>
                  {!isChange && <span>Sin cambio de configuración para aprobar</span>}
                </div>
              </Panel>
            )
          })}
        </div>
      )}
    </div>
  )
}
