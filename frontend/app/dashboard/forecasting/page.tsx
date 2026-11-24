'use client'

import { useEffect, useState } from 'react'
import { Info } from 'lucide-react'
import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { ForecastChart } from '@/components/charts'
import {
  getAnalyticsForecasts,
  getSelectedResourceUid,
  getTimeRange,
  type ForecastResponse,
  type ServiceResult,
} from '@/lib/api-services'

type ForecastPoint = {
  t: string
  observed: number | null
  forecast: number | null
  lower: number | null
  upper: number | null
}

export default function ForecastingPage() {
  const [resourceUid, setResourceUid] = useState<string | null>(null)
  const [timeRange, setTimeRangeState] = useState<{ from: string; to: string } | null>(null)
  const [forecastResult, setForecastResult] = useState<ServiceResult<ForecastResponse[]>>({ data: null, source: null, status: 'loading', error: null })

  const forecastSeries: ForecastPoint[] = (forecastResult.data ?? []).map((point) => ({
    t: new Date(point.PeriodStartUTC).toLocaleString(),
    observed: null,
    forecast: point.YPred,
    lower: point.YLower,
    upper: point.YUpper,
  }))
  const latestForecast = forecastResult.data?.at(-1)

  useEffect(() => {
    const uid = getSelectedResourceUid()
    const range = getTimeRange()
    setResourceUid(uid)
    setTimeRangeState(range)
  }, [])

  useEffect(() => {
    if (!resourceUid || !timeRange) {
      setForecastResult({ data: null, source: null, status: 'empty', error: 'Selecciona un recurso y un período.' })
      return
    }

    getAnalyticsForecasts('cpu_avg')
      .then((res) => setForecastResult(res))
      .catch((err: unknown) => setForecastResult({ data: null, source: null, status: 'error', error: err instanceof Error ? err.message : 'Error al consultar forecasts' }))
  }, [resourceUid, timeRange])

  return (
    <div>
      <PageHeader
        title="Forecasting"
        description="Projected workload with an uncertainty band. Observed and forecast data are treated differently — a forecast is never presented as a certainty."
        actions={
          <div className="flex items-center gap-2">
            <Badge variant="neutral">Horizon: {latestForecast ? `${latestForecast.HorizonSteps} ${latestForecast.StepUnit}` : '—'}</Badge>
            <Badge variant="outline" className="text-[9px]">{forecastResult.source}</Badge>
          </div>
        }
      />

      {forecastResult.status !== 'ready' || !forecastSeries.length ? (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-6 text-center">
          <p className="text-sm text-muted-foreground">{forecastResult.status === 'loading' ? 'Cargando forecasts...' : 'No hay forecasts disponibles'}</p>
          <p className="text-xs text-muted-foreground">
            {forecastResult.error && forecastResult.status === 'error'
              ? forecastResult.error
              : 'Verifica el recurso y la ventana seleccionados, la disponibilidad de cco_data y el servicio ML.'}
          </p>
        </div>
      ) : (
        <>
          <Panel
            title="CPU utilization — forecast"
            description="Dashed = forecast · shaded = uncertainty interval, when available"
            note="This endpoint does not include observed telemetry; forecasts must not be treated as measured utilization."
            action={<Badge variant="outline" className="text-[9px]">{forecastResult.source}</Badge>}
          >
            <ForecastChart data={forecastSeries} height={300} />
          </Panel>

          <div className="mt-4 grid gap-4 lg:grid-cols-3">
            <Panel title="Model error" description="Backtested accuracy over the window">
              <p className="mt-3 text-xs text-muted-foreground">
                Backtest metrics are not returned by this endpoint.
              </p>
            </Panel>

            <Panel title="Forecast details" description="Data returned by the backend">
              <ul className="space-y-3 text-sm">
                <li className="flex items-start gap-2">
                  <Info className="mt-0.5 size-4 shrink-0 text-primary" />
                  <span className="text-muted-foreground">Target metric: {latestForecast?.TargetMetric}</span>
                </li>
                <li className="flex items-start gap-2">
                  <Info className="mt-0.5 size-4 shrink-0 text-success" />
                  <span className="text-muted-foreground">Model version: {latestForecast?.ModelVersionID ?? 'Not specified'}</span>
                </li>
              </ul>
            </Panel>

            <Panel title="Data sufficiency" description="Guardrail for forecast validity">
              <div className="flex items-start gap-2 rounded-lg border border-warning/25 bg-warning/[0.06] p-3 text-xs text-warning">
                <Info className="mt-px size-3.5 shrink-0" />
                <span>Historical coverage and backtest results are not provided with these forecasts. Review them before making capacity decisions.</span>
              </div>
            </Panel>
          </div>
        </>
      )}
    </div>
  )
}
