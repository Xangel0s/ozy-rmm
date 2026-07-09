"use client"

import * as React from "react"
import { subDays, formatISO } from "date-fns"
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts"
import { useAgentTelemetry } from "@/lib/use-live-data"
import type { TelemetryBucket } from "@/lib/api"

type Range = "24h" | "7d" | "30d"

interface TelemetryChartProps {
  agentId: string
}

const ranges: { label: Range; interval: string }[] = [
  { label: "24h", interval: "hour" },
  { label: "7d", interval: "hour" },
  { label: "30d", interval: "day" },
]

function rangeConfig(range: Range): { from: string; to: string; interval: string } {
  const to = new Date()
  const days = range === "24h" ? 1 : range === "7d" ? 7 : 30
  const from = subDays(to, days)
  const interval = range === "30d" ? "day" : "hour"

  return {
    from: formatISO(from),
    to: formatISO(to),
    interval,
  }
}

function customTooltip({ active, payload, label }: any) {
  if (!active || !payload?.length) return null

  return (
    <div className="rounded-lg border border-border bg-card px-3 py-2 text-xs shadow-lg">
      <p className="mb-1 font-medium text-muted-foreground">{label}</p>
      {payload.map((entry: any) => (
        <p key={entry.name} style={{ color: entry.color }}>
          {entry.name}: {Math.round(entry.value)}%
        </p>
      ))}
    </div>
  )
}

export function TelemetryChart({ agentId }: TelemetryChartProps) {
  const [range, setRange] = React.useState<Range>("24h")
  const { from, to, interval } = rangeConfig(range)

  const { buckets, loading } = useAgentTelemetry(agentId, from, to, interval)

  if (loading) {
    return (
      <div className="flex h-48 items-center justify-center text-sm text-muted-foreground">
        Loading telemetry...
      </div>
    )
  }

  if (buckets.length === 0) {
    return (
      <div className="flex h-48 items-center justify-center text-sm text-muted-foreground">
        No telemetry data for this period
      </div>
    )
  }

  return (
    <div>
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold">Resource Usage</h3>
        <div className="flex items-center rounded-lg bg-secondary p-0.5">
          {ranges.map(({ label }) => (
            <button
              key={label}
              type="button"
              onClick={() => setRange(label)}
              className={
                range === label
                  ? "rounded-md bg-card px-2.5 py-1 text-xs font-medium text-foreground ring-1 ring-border"
                  : "rounded-md px-2.5 py-1 text-xs font-medium text-muted-foreground hover:text-foreground"
              }
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      <div className="h-48 w-full">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={buckets} margin={{ top: 4, right: 4, bottom: 4, left: -16 }}>
            <defs>
              <linearGradient id="cpuGrad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="var(--color-cpu)" stopOpacity={0.3} />
                <stop offset="95%" stopColor="var(--color-cpu)" stopOpacity={0} />
              </linearGradient>
              <linearGradient id="ramGrad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="var(--color-ram)" stopOpacity={0.3} />
                <stop offset="95%" stopColor="var(--color-ram)" stopOpacity={0} />
              </linearGradient>
              <linearGradient id="diskGrad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor="var(--color-disk)" stopOpacity={0.3} />
                <stop offset="95%" stopColor="var(--color-disk)" stopOpacity={0} />
              </linearGradient>
            </defs>
            <CartesianGrid strokeDasharray="3 3" className="stroke-border/50" />
            <XAxis
              dataKey="bucket"
              tick={{ fontSize: 11 }}
              className="text-muted-foreground"
              tickFormatter={(v: string) => {
                const d = new Date(v)
                return range === "24h"
                  ? `${d.getHours().toString().padStart(2, "0")}:00`
                  : `${d.getMonth() + 1}/${d.getDate()}`
              }}
            />
            <YAxis
              domain={[0, 100]}
              tick={{ fontSize: 11 }}
              className="text-muted-foreground"
              tickFormatter={(v: number) => `${v}%`}
            />
            <Tooltip content={customTooltip} />
            <Area
              type="monotone"
              dataKey="cpu_avg"
              name="CPU"
              stroke="var(--color-cpu)"
              fill="url(#cpuGrad)"
              connectNulls
              strokeWidth={1.5}
            />
            <Area
              type="monotone"
              dataKey="ram_avg"
              name="RAM"
              stroke="var(--color-ram)"
              fill="url(#ramGrad)"
              connectNulls
              strokeWidth={1.5}
            />
            <Area
              type="monotone"
              dataKey="disk_avg"
              name="Disk"
              stroke="var(--color-disk)"
              fill="url(#diskGrad)"
              connectNulls
              strokeWidth={1.5}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>

      <div className="mt-2 flex items-center gap-4 text-xs text-muted-foreground">
        <span className="flex items-center gap-1">
          <span className="inline-block size-2.5 rounded-full" style={{ backgroundColor: "var(--color-cpu)" }} />
          CPU
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block size-2.5 rounded-full" style={{ backgroundColor: "var(--color-ram)" }} />
          RAM
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block size-2.5 rounded-full" style={{ backgroundColor: "var(--color-disk)" }} />
          Disk
        </span>
      </div>
    </div>
  )
}
