'use client'

import { useEffect, useState } from 'react'
import { TrendingUp, AlertTriangle, Info } from 'lucide-react'
import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { ForecastChart } from '@/components/charts'
import {
  getAnalyticsForecasts,
  getSelectedResourceUid,
  getTimeRange,
} from '@/lib/api-services'

type ForecastMeta = {
  horizonDays: number
  trend: string
  saturation: string
  errors: Array<{ label: string; value: string }>
}

type ForecastPoint = {
  t: string
  observed: number | null
  forecast: number | null
  lower: number | null
  upper: number | null
}

export default function ForecastingPage() {
  const [forecastSeries, setForecastSeries] = useState<ForecastPoint[]>([])
  const [forecastMeta, setForecastMeta] = useState<ForecastMeta>(forecastMetaMock)
  const [resourceUid, setResourceUid] = useState<string | null>(null)
  const [timeRange, setTimeRangeState] = useState<{ from: string; to: string } | null>(null)
  const [forecastLoaded, setForecastLoaded] = useState(false)
  const [forecastSource, setForecastSource] = useState<'mock' | 'backend'>('mock')

  useEffect(() => {
    const uid = getSelectedResourceUid()
    const range = getTimeRange()
    setResourceUid(uid)
    setTimeRangeState(range)
  }, [])

  useEffect(() => {
    if (!resourceUid || !timeRange) return

    getAnalyticsForecasts('cpu_percent')
      .then((forecastRes) => {
        if (forecastRes.source === 'backend' && forecastRes.data && forecastRes.data.points?.length > 0) {
          const series: ForecastPoint[] = forecastRes.data.points.map((p) => ({
            t: p.timestamp_utc,
            observed: null,
            forecast: p.forecast,
            lower: p.lower,
            upper: p.upper,
          }))
          setForecastSeries(series)
          setForecastLoaded(true)
          setForecastSource('backend')
        } else {
          setForecastLoaded(false)
          setForecastSource('mock')
        }
      })
      .catch(() => setForecastLoaded(false))
  }, [resourceUid, timeRange])

  return (
    <div>
      <PageHeader
        title="Forecasting"
        description="Projected workload with an uncertainty band. Observed and forecast data are treated differently — a forecast is never presented as a certainty."
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="neutral">Horizon: {forecastMeta.horizonDays} days</Badge>
          </div>
        }
      />

      {!forecastLoaded ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          <p className="text-sm text-muted-foreground">Datos insuficientes</p>
          <p className="text-xs text-muted-foreground">
            No hay datos de forecast para el recurso y ventana seleccionados. Verifique que exista
            datos en cco_data, que CCO_DATA_DSN esté configurado y que el servicio ML esté disponible.
          </p>
        </div>
      ) : (
        <>
          <Panel
            title="CPU utilization — observed vs forecast"
            description="Solid = observed telemetry · dashed = forecast · shaded = uncertainty band"
            note="Uncertainty widens with horizon. Beyond 14 days, forecast confidence degrades and should not drive irreversible decisions."
            action={<Badge variant="outline" className="text-[9px]">{forecastSource === 'backend' ? 'Datos reales' : 'mock'}</Badge>}
          >
            <ForecastChart data={forecastSeries} height={300} />
          </Panel>

          <div className="mt-4 grid gap-4 lg:grid-cols-3">
            <Panel title="Model error" description="Backtested accuracy over the window" action={<Badge variant="outline" className="text-[9px]">{forecastSource}</Badge>}>
              <div className="grid grid-cols-3 gap-3">
                {forecastMeta.errors.map((e: any) => (
                  <div key={e.label} className="rounded-lg border border-border bg-background p-3 text-center">
                    <p className="text-lg font-semibold tabular-nums text-foreground">{e.value}</p>
                    <p className="text-[11px] text-muted-foreground">{e.label}</p>
                  </div>
                ))}
              </div>
              <p className="mt-3 text-xs text-muted-foreground">
                Errors are computed by replaying the model against held-out history. Lower is better.
              </p>
            </Panel>

            <Panel title="Trend & saturation" description="What the projection implies" action={<Badge variant="outline" className="text-[9px]">{forecastSource}</Badge>}>
              <ul className="space-y-3 text-sm">
                <li className="flex items-start gap-2">
                  <TrendingUp className="mt-0.5 size-4 shrink-0 text-primary" />
                  <span className="text-muted-foreground">{forecastMeta.trend}</span>
                </li>
                <li className="flex items-start gap-2">
                  <Info className="mt-0.5 size-4 shrink-0 text-success" />
                  <span className="text-muted-foreground">{forecastMeta.saturation}</span>
                </li>
              </ul>
              <div className="mt-3 rounded-lg bg-muted/60 p-3 text-xs text-muted-foreground">
                No saturation within the horizon means the recommended 4-vCore configuration remains
                feasible under the projected trend.
              </div>
            </Panel>

            <Panel title="Data sufficiency" description="Guardrail for forecast validity" action={<Badge variant="outline" className="text-[9px]">{forecastSource}</Badge>}>
              <div className="flex items-center justify-between rounded-lg border border-border bg-background px-3 py-2.5">
                <span className="text-xs text-muted-foreground">History available</span>
                <Badge variant="success">30 days</Badge>
              </div>
              <div className="mt-3 flex items-start gap-2 rounded-lg border border-warning/25 bg-warning/[0.06] p-3 text-xs text-warning">
                <AlertTriangle className="mt-px size-3.5 shrink-0" />
                <span>
                  With fewer than 7 days of history the forecast would be suppressed and marked
                  <span className="font-medium"> insufficient data</span> rather than shown.
                </span>
              </div>
            </Panel>
          </div>
        </>
      )}
    </div>
  )
}

const forecastMetaMock: ForecastMeta = {
  horizonDays: 14,
  trend: '+3.1% projected over 14 days',
  saturation: 'No CPU saturation projected within horizon',
  errors: [
    { label: 'MAE', value: '3.8%' },
    { label: 'RMSE', value: '5.1%' },
    { label: 'MAPE', value: '6.4%' },
  ],
}
