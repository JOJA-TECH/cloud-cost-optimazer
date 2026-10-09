'use client'

import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ComposedChart,
  Line,
  LineChart,
  Pie,
  PieChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

const axisProps = {
  stroke: 'var(--muted-foreground)',
  fontSize: 11,
  tickLine: false,
  axisLine: false,
} as const

function TooltipBox({
  active,
  payload,
  label,
  unit = '',
  prefix = '',
}: {
  active?: boolean
  payload?: any[]
  label?: string | number
  unit?: string
  prefix?: string
}) {
  if (!active || !payload || payload.length === 0) return null
  return (
    <div className="rounded-lg border border-border bg-popover px-3 py-2 text-xs shadow-md">
      {label !== undefined && (
        <p className="mb-1 font-medium text-foreground">{label}</p>
      )}
      <div className="flex flex-col gap-0.5">
        {payload
          .filter((p) => p.value !== null && p.value !== undefined)
          .map((p, i) => (
            <div key={i} className="flex items-center gap-2">
              <span
                className="size-2 rounded-full"
                style={{ background: p.color || p.stroke || p.fill }}
              />
              <span className="text-muted-foreground">{p.name}</span>
              <span className="ml-auto font-medium tabular-nums text-foreground">
                {prefix}
                {typeof p.value === 'number' ? p.value.toLocaleString() : p.value}
                {unit}
              </span>
            </div>
          ))}
      </div>
    </div>
  )
}

export function AreaTrend({
  data,
  dataKey,
  xKey = 'day',
  color = 'var(--chart-1)',
  height = 220,
  unit = '',
  prefix = '',
  showAxes = true,
}: {
  data: any[]
  dataKey: string
  xKey?: string
  color?: string
  height?: number
  unit?: string
  prefix?: string
  showAxes?: boolean
}) {
  const gid = `area-${dataKey}`
  return (
    <ResponsiveContainer width="100%" height={height}>
      <AreaChart data={data} margin={{ top: 6, right: 6, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity={0.28} />
            <stop offset="100%" stopColor={color} stopOpacity={0.02} />
          </linearGradient>
        </defs>
        {showAxes && <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />}
        {showAxes && <XAxis dataKey={xKey} {...axisProps} minTickGap={24} />}
        {showAxes && <YAxis {...axisProps} width={40} />}
        <Tooltip content={<TooltipBox unit={unit} prefix={prefix} />} />
        <Area
          type="monotone"
          dataKey={dataKey}
          name={dataKey}
          stroke={color}
          strokeWidth={2}
          fill={`url(#${gid})`}
        />
      </AreaChart>
    </ResponsiveContainer>
  )
}

export function MultiLine({
  data,
  xKey,
  lines,
  height = 260,
  unit = '',
  thresholds = [],
  yDomain,
}: {
  data: any[]
  xKey: string
  lines: { key: string; color: string; name: string; dashed?: boolean }[]
  height?: number
  unit?: string
  thresholds?: { y: number; color: string; label: string }[]
  yDomain?: [number, number]
}) {
  return (
    <ResponsiveContainer width="100%" height={height}>
      <LineChart data={data} margin={{ top: 6, right: 10, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
        <XAxis dataKey={xKey} {...axisProps} minTickGap={28} />
        <YAxis {...axisProps} width={40} domain={yDomain} unit={unit} />
        <Tooltip content={<TooltipBox unit={unit} />} />
        {thresholds.map((t, i) => (
          <ReferenceLine
            key={i}
            y={t.y}
            stroke={t.color}
            strokeDasharray="4 4"
            strokeOpacity={0.7}
            label={{ value: t.label, position: 'right', fontSize: 10, fill: t.color }}
          />
        ))}
        {lines.map((l) => (
          <Line
            key={l.key}
            type="monotone"
            dataKey={l.key}
            name={l.name}
            stroke={l.color}
            strokeWidth={l.key.includes('p99') ? 2.4 : 2}
            strokeDasharray={l.dashed ? '5 4' : undefined}
            dot={false}
            activeDot={{ r: 3 }}
          />
        ))}
      </LineChart>
    </ResponsiveContainer>
  )
}

export function Bars({
  data,
  xKey,
  bars,
  height = 240,
  unit = '',
  prefix = '',
  stacked = false,
}: {
  data: any[]
  xKey: string
  bars: { key: string; color: string; name: string }[]
  height?: number
  unit?: string
  prefix?: string
  stacked?: boolean
}) {
  return (
    <ResponsiveContainer width="100%" height={height}>
      <BarChart data={data} margin={{ top: 6, right: 6, left: 0, bottom: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
        <XAxis dataKey={xKey} {...axisProps} minTickGap={8} />
        <YAxis {...axisProps} width={40} />
        <Tooltip cursor={{ fill: 'var(--muted)' }} content={<TooltipBox unit={unit} prefix={prefix} />} />
        {bars.map((b) => (
          <Bar
            key={b.key}
            dataKey={b.key}
            name={b.name}
            fill={b.color}
            stackId={stacked ? 'a' : undefined}
            radius={stacked ? [0, 0, 0, 0] : [4, 4, 0, 0]}
            maxBarSize={stacked ? 44 : 28}
          />
        ))}
      </BarChart>
    </ResponsiveContainer>
  )
}

export function ForecastChart({ data, height = 280 }: { data: any[]; height?: number }) {
  return (
    <ResponsiveContainer width="100%" height={height}>
      <ComposedChart data={data} margin={{ top: 6, right: 10, left: 0, bottom: 0 }}>
        <defs>
          <linearGradient id="band" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--chart-2)" stopOpacity={0.18} />
            <stop offset="100%" stopColor="var(--chart-2)" stopOpacity={0.02} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" vertical={false} />
        <XAxis dataKey="t" {...axisProps} minTickGap={24} />
        <YAxis {...axisProps} width={40} unit="%" />
        <Tooltip content={<TooltipBox unit="%" />} />
        <Area
          type="monotone"
          dataKey="upper"
          name="Upper bound"
          stroke="none"
          fill="url(#band)"
          connectNulls
        />
        <Area
          type="monotone"
          dataKey="lower"
          name="Lower bound"
          stroke="none"
          fill="var(--background)"
          fillOpacity={1}
          connectNulls
        />
        <Line
          type="monotone"
          dataKey="observed"
          name="Observed"
          stroke="var(--chart-1)"
          strokeWidth={2.2}
          dot={false}
          connectNulls
        />
        <Line
          type="monotone"
          dataKey="forecast"
          name="Forecast"
          stroke="var(--chart-2)"
          strokeWidth={2.2}
          strokeDasharray="5 4"
          dot={false}
          connectNulls
        />
      </ComposedChart>
    </ResponsiveContainer>
  )
}

export function Donut({
  data,
  height = 220,
  unit = '%',
}: {
  data: { name: string; pct?: number; value?: number; color: string }[]
  height?: number
  unit?: string
}) {
  return (
    <ResponsiveContainer width="100%" height={height}>
      <PieChart>
        <Tooltip content={<TooltipBox unit={unit} />} />
        <Pie
          data={data}
          dataKey={data[0]?.pct !== undefined ? 'pct' : 'value'}
          nameKey="name"
          innerRadius="58%"
          outerRadius="86%"
          paddingAngle={2}
          stroke="var(--card)"
          strokeWidth={2}
        >
          {data.map((d, i) => (
            <Cell key={i} fill={d.color} />
          ))}
        </Pie>
      </PieChart>
    </ResponsiveContainer>
  )
}
