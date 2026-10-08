'use client'

import { useState } from 'react'
import { ChevronDown, Database, Server, HardDrive, Shield, RefreshCw, CheckCircle2 } from 'lucide-react'
import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { resource, dataQuality } from '@/lib/mock-data'

const summary = [
  { icon: Database, label: 'Service', value: resource.service },
  { icon: Server, label: 'Tier', value: `${resource.tier} · ${resource.computeModel}` },
  { icon: Server, label: 'Compute', value: `${resource.vCores} vCores · ${resource.memoryGb} GB` },
  { icon: HardDrive, label: 'Storage', value: `${resource.storageUsedGb} / ${resource.storageLimitGb} GB` },
  { icon: Shield, label: 'Redundancy', value: resource.redundancy },
  { icon: CheckCircle2, label: 'State', value: resource.state },
]

const technical = [
  { label: 'Server', value: resource.server },
  { label: 'Region', value: resource.region },
  { label: 'Collation', value: resource.collation },
  { label: 'Max workers', value: String(resource.maxWorkers) },
  { label: 'Max sessions', value: String(resource.maxSessions) },
  { label: 'Storage limit', value: `${resource.storageLimitGb} GB` },
  { label: 'Compute model', value: resource.computeModel },
  { label: 'Service tier', value: resource.tier },
]

export default function ConfigurationPage() {
  const [showTech, setShowTech] = useState(false)

  return (
    <div>
      <PageHeader
        title="Configuration"
        description="The current operational configuration of the selected Azure SQL database. Executive summary first; deeper API-level detail is available on demand."
        actions={
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <RefreshCw className="size-3.5" />
            Last updated: {resource.lastSyncMinutes} min ago
          </span>
        }
      />

      <div className="grid gap-4 lg:grid-cols-3">
        <Panel title="Current configuration" description={resource.name} className="lg:col-span-2">
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

        <Panel title="Data quality" description="Signals feeding the analysis">
          <div className="mb-3 flex items-center justify-between rounded-lg border border-border bg-background px-3 py-2.5">
            <span className="text-xs text-muted-foreground">Global confidence</span>
            <Badge variant="success">{dataQuality.globalConfidence}</Badge>
          </div>
          <ul className="space-y-1">
            {dataQuality.items.map((item) => (
              <li key={item.label} className="group flex items-center justify-between rounded-md px-2 py-1.5 hover:bg-muted/60">
                <span className="flex items-center gap-1.5 text-xs text-muted-foreground" title={item.info}>
                  {item.label}
                </span>
                <span className="text-xs font-medium tabular-nums text-foreground">{item.value}</span>
              </li>
            ))}
          </ul>
          <div className="mt-3 rounded-lg bg-muted/60 p-3 text-xs text-muted-foreground">
            Release 1.0 analyzes Azure SQL Database and does not modify production resources.
            Multi-cloud support is a roadmap concept.
          </div>
        </Panel>
      </div>
    </div>
  )
}
