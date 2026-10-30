'use client'

import {
  Area,
  ComposedChart,
  Line,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

const data = Array.from({ length: 24 }, (_, h) => {
  const curve = Math.max(0, Math.sin(((h - 6) / 13) * Math.PI))
  const p50 = 11 + curve * 12
  return {
    t: `${String(h).padStart(2, '0')}:00`,
    p50: +p50.toFixed(1),
    p95: +(p50 + 18 + curve * 22).toFixed(1),
    p99: +Math.min(p50 + 30 + curve * 34, 99).toFixed(1),
  }
})

export function NaiveVsAwareChart() {
  return (
    <ResponsiveContainer width="100%" height={220}>
      <ComposedChart data={data} margin={{ top: 6, right: 8, left: -10, bottom: 0 }}>
        <defs>
          <linearGradient id="p99fill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="var(--chart-1)" stopOpacity={0.22} />
            <stop offset="100%" stopColor="var(--chart-1)" stopOpacity={0.02} />
          </linearGradient>
        </defs>
        <XAxis
          dataKey="t"
          stroke="var(--muted-foreground)"
          fontSize={10}
          tickLine={false}
          axisLine={false}
          minTickGap={30}
        />
        <YAxis
          stroke="var(--muted-foreground)"
          fontSize={10}
          tickLine={false}
          axisLine={false}
          width={34}
          unit="%"
          domain={[0, 100]}
        />
        <Tooltip
          contentStyle={{
            borderRadius: 8,
            border: '1px solid var(--border)',
            fontSize: 12,
            background: 'var(--popover)',
          }}
        />
        <ReferenceLine
          y={22}
          stroke="var(--muted-foreground)"
          strokeDasharray="5 5"
          label={{ value: 'avg 22%', position: 'insideTopRight', fontSize: 10, fill: 'var(--muted-foreground)' }}
        />
        <Area type="monotone" dataKey="p99" name="p99" stroke="var(--chart-1)" strokeWidth={2} fill="url(#p99fill)" />
        <Line type="monotone" dataKey="p95" name="p95" stroke="var(--chart-2)" strokeWidth={2} dot={false} />
        <Line type="monotone" dataKey="p50" name="p50" stroke="var(--chart-3)" strokeWidth={1.6} dot={false} />
      </ComposedChart>
    </ResponsiveContainer>
  )
}
