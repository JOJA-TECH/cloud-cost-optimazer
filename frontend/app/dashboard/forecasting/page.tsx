import { TrendingUp, AlertTriangle, Info } from 'lucide-react'
import { PageHeader, Panel } from '@/components/dashboard/ui'
import { Badge } from '@/components/ui/badge'
import { ForecastChart } from '@/components/charts'
import { forecastSeries, forecastMeta } from '@/lib/mock-data'

export default function ForecastingPage() {
  return (
    <div>
      <PageHeader
        title="Forecasting"
        description="Projected workload with an uncertainty band. Observed and forecast data are treated differently — a forecast is never presented as a certainty."
        actions={<Badge variant="neutral">Horizon: {forecastMeta.horizonDays} days</Badge>}
      />

      <Panel
        title="CPU utilization — observed vs forecast"
        description="Solid = observed telemetry · dashed = forecast · shaded = uncertainty band"
        note="Uncertainty widens with horizon. Beyond 14 days, forecast confidence degrades and should not drive irreversible decisions."
        action={
          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            <span className="flex items-center gap-1.5">
              <span className="h-0.5 w-4 rounded bg-[var(--chart-1)]" />
              Observed
            </span>
            <span className="flex items-center gap-1.5">
              <span className="h-0.5 w-4 rounded border-t-2 border-dashed border-[var(--chart-2)]" />
              Forecast
            </span>
          </div>
        }
      >
        <ForecastChart data={forecastSeries} height={300} />
      </Panel>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Panel title="Model error" description="Backtested accuracy over the window">
          <div className="grid grid-cols-3 gap-3">
            {forecastMeta.errors.map((e) => (
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

        <Panel title="Trend & saturation" description="What the projection implies">
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

        <Panel title="Data sufficiency" description="Guardrail for forecast validity">
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
    </div>
  )
}
