'use client'

import { useEffect, useState } from 'react'
import { ChevronDown, Database, Server, HardDrive, Shield, RefreshCw, CheckCircle2 } from 'lucide-react'
import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import {
  getDashboardResource,
  getDataQuality,
  getConfigSnapshots,
  getSelectedResourceUid,
  type DashboardResource,
  type DataQuality,
  type ServiceResult,
} from '@/lib/api-services'

type ConfigSnapshot = {
  observed_at_utc: string
  sku_name: string
  tier: string
  family: string
  capacity_vcores: number
  compute_model: string
  max_size_bytes: number
  auto_pause_delay_minutes: number | null
  status: string
  free_limit: boolean
}

export default function ConfigurationPage() {
  const [showTech, setShowTech] = useState(false)
  const [resource, setResource] = useState<DashboardResource | null>(null)
  const [dataQualityResult, setDataQualityResult] = useState<ServiceResult<DataQuality>>({ data: null, source: null, status: 'loading', error: null })
  const [configSnapshots, setConfigSnapshots] = useState<ConfigSnapshot[]>([])
  const [configLoaded, setConfigLoaded] = useState(false)
  const [resourceUid, setResourceUid] = useState<string | null>(null)

  useEffect(() => {
    const uid = getSelectedResourceUid()
    setResourceUid(uid)
    getDashboardResource().then((res) => {
      if (res.source === 'backend' && res.data) {
        setResource(res.data)
      }
    })
    getDataQuality().then(setDataQualityResult)
    if (uid) {
      getConfigSnapshots(uid)
        .then((sRes) => {
          if (sRes.source === 'backend' && sRes.data) {
            setConfigSnapshots(sRes.data)
            setConfigLoaded(true)
          }
        })
        .catch(() => setConfigLoaded(false))
    }
  }, [])

  const summary = resource ? [
    { icon: Database, label: 'Service', value: resource.service ?? '—' },
    { icon: Server, label: 'Tier', value: `${resource.tier ?? '—'} · ${resource.computeModel ?? '—'}` },
    { icon: Server, label: 'Compute', value: `${resource.vCores ?? '—'} vCores · ${resource.memoryGb ?? '—'} GB` },
    { icon: HardDrive, label: 'Storage', value: `${resource.storageUsedGb ?? '—'} / ${resource.storageLimitGb ?? '—'} GB` },
    { icon: Shield, label: 'Redundancy', value: resource.redundancy ?? '—' },
    { icon: CheckCircle2, label: 'State', value: resource.state ?? '—' },
  ] : []

  const technical = resource ? [
    { label: 'Server', value: resource.server },
    { label: 'Region', value: resource.region },
    { label: 'Collation', value: resource.collation ?? '—' },
    { label: 'Max workers', value: resource.maxWorkers ? String(resource.maxWorkers) : '—' },
    { label: 'Max sessions', value: resource.maxSessions ? String(resource.maxSessions) : '—' },
    { label: 'Storage limit', value: resource.storageLimitGb ? `${resource.storageLimitGb} GB` : '—' },
    { label: 'Compute model', value: resource.computeModel ?? '—' },
    { label: 'Service tier', value: resource.tier ?? '—' },
  ] : []

  return (
    <div>
      <PageHeader
        title="Configuration"
        description="The current operational configuration of the selected Azure SQL database. Executive summary first; deeper API-level detail is available on demand."
        actions={
          <div className="flex items-center gap-2">
            <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <RefreshCw className="size-3.5" />
              Last updated: {resource?.lastSyncMinutes ?? 0} min ago
            </span>
          </div>
        }
      />

      {!resource ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          <p className="text-sm text-muted-foreground">Datos insuficientes</p>
          <p className="text-xs text-muted-foreground">
            No hay configuración de recurso disponible. Verifique que exista un recurso cloud
            registrado y que CCO_DATA_DSN esté configurado en el backend.
          </p>
        </div>
      ) : (
        <>
          <div className="grid gap-4 lg:grid-cols-3">
            <Panel title="Current configuration" description={resource?.name ?? '—'} className="lg:col-span-2" action={resource.resource_uid === 'demo-resource' ? <Badge variant="outline">Mock</Badge> : undefined}>
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {summary.map((s) => (
                  <div key={s.label} className="rounded-lg border border-border bg-background p-3">
                    <div className="flex items-center gap-2 text-muted-foreground">
                      <s.icon className="size-4" />
                      <span className="text-xs">{s.label}</span>
                    </div>
                    <p className="mt-1.5 text-sm font-semibold text-foreground">{s.value}</p>
                  </div>
                ))}
              </div>

              <div className="mt-4">
                <button
                  onClick={() => setShowTech((v) => !v)}
                  className="flex w-full items-center justify-between rounded-lg border border-border bg-background px-4 py-3 text-sm font-medium text-foreground transition-colors hover:bg-muted"
                >
                  <span>Technical details</span>
                  <ChevronDown className={cn('size-4 text-muted-foreground transition-transform', showTech && 'rotate-180')} />
                </button>
                {showTech && (
                  <div className="mt-2 overflow-hidden rounded-lg border border-border">
                    <dl className="divide-y divide-border">
                      {technical.map((t) => (
                        <div key={t.label} className="flex items-center justify-between px-4 py-2.5">
                          <dt className="text-xs text-muted-foreground">{t.label}</dt>
                          <dd className="font-mono text-xs text-foreground">{t.value}</dd>
                        </div>
                      ))}
                    </dl>
                  </div>
                )}
              </div>
            </Panel>

            <Panel title="Data quality" description="Signals feeding the analysis" action={dataQualityResult.status === 'ready' && dataQualityResult.source === 'mock' ? <Badge variant="outline">Mock</Badge> : undefined}>
              {dataQualityResult.data ? (<>
              <div className="mb-3 flex items-center justify-between rounded-lg border border-border bg-background px-3 py-2.5">
                <span className="text-xs text-muted-foreground">Global confidence</span>
                <Badge variant="success">{dataQualityResult.data.globalConfidence}</Badge>
              </div>
              <ul className="space-y-1">
                {dataQualityResult.data.items.map((item) => (
                  <li key={item.label} className="group flex items-center justify-between rounded-md px-2 py-1.5 hover:bg-muted/60">
                    <span className="flex items-center gap-1.5 text-xs text-muted-foreground" title={item.info}>
                      {item.label}
                    </span>
                    <span className="text-xs font-medium tabular-nums text-foreground">{item.value}</span>
                  </li>
                ))}
              </ul>
              </>) : <p className="text-xs text-muted-foreground">{dataQualityResult.status === 'loading' ? 'Cargando cobertura...' : dataQualityResult.status === 'error' ? dataQualityResult.error : 'No hay información de cobertura disponible.'}</p>}
              <div className="mt-3 rounded-lg bg-muted/60 p-3 text-xs text-muted-foreground">
                Release 1.0 analyzes Azure SQL Database and does not modify production resources.
                Multi-cloud support is a roadmap concept.
              </div>
            </Panel>
          </div>

          <Panel
            title="Configuration snapshots"
            description="Historial de configuración (ARM) del recurso, más reciente primero"
            className="mt-4"
          >
            {!configLoaded ? (
              <div className="p-4 text-center text-xs text-muted-foreground">
                No hay snapshots de configuración disponibles para el recurso seleccionado.
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border text-xs text-muted-foreground">
                      <th className="px-4 py-2 text-left">Observed at</th>
                      <th className="px-4 py-2 text-left">SKU</th>
                      <th className="px-4 py-2 text-left">Tier</th>
                      <th className="px-4 py-2 text-left">vCores</th>
                      <th className="px-4 py-2 text-left">Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {configSnapshots.map((s) => (
                      <tr key={s.observed_at_utc} className="border-b border-border last:border-0">
                        <td className="px-4 py-2 font-mono text-xs">{s.observed_at_utc}</td>
                        <td className="px-4 py-2 text-xs">{s.sku_name}</td>
                        <td className="px-4 py-2 text-xs">{s.tier}</td>
                        <td className="px-4 py-2 text-xs">{s.capacity_vcores}</td>
                        <td className="px-4 py-2 text-xs">{s.status}</td>
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

const resourceMock: DashboardResource = {
  resource_uid: 'mock-resource',
  name: 'sql-prod-orders-eastus',
  server: 'cdco-prod.database.windows.net',
  service: 'Azure SQL Database',
  tier: 'General Purpose',
  computeModel: 'Provisioned',
  vCores: 8,
  memoryGb: 40.8,
  region: 'East US',
  storageUsedGb: 214,
  storageLimitGb: 512,
  redundancy: 'Zone-redundant',
  state: 'Online',
  lastSyncMinutes: 8,
  collation: 'SQL_Latin1_General_CP1_CI_AS',
  maxWorkers: 800,
  maxSessions: 2400,
}

const dataQualityMock = {
  globalConfidence: 'High',
  items: [
    { label: 'Last collected', value: '8 min ago', info: 'Most recent successful metric collection.', tone: 'good' },
    { label: 'Historical duration', value: '30 days', info: 'Length of observed history feeding the analysis.', tone: 'good' },
    { label: 'Sampling interval', value: '5 min', info: 'Resolution of collected telemetry.', tone: 'good' },
    { label: 'Coverage', value: '96%', info: 'Share of expected samples actually collected.', tone: 'good' },
    { label: 'Missing metrics', value: 'None', info: 'Required signals with no data in the window.', tone: 'good' },
    { label: 'Cost source', value: 'Billed', info: 'Cost derived from billed usage, not list price.', tone: 'good' },
    { label: 'Experimental validation', value: 'Available', info: 'A controlled scenario replay validated the estimate.', tone: 'good' },
  ],
}
