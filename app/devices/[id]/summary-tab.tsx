"use client"

import * as React from "react"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { TelemetryChart } from "@/components/rmm/telemetry-chart"
import { fetchAlertsByAgent, type AgentInfo, type AlertRow } from "@/lib/api"

function fmtBytes(bytes: number): string {
  if (bytes === 0) return "0 B"
  const k = 1024
  const sizes = ["B", "KB", "MB", "GB", "TB"]
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

function fmtPct(used: number, total: number): string {
  if (total === 0) return "0%"
  return `${Math.round((used / total) * 100)}%`
}

interface SummaryTabProps {
  agent: AgentInfo
}

export function SummaryTab({ agent }: SummaryTabProps) {
  const [alerts, setAlerts] = React.useState<AlertRow[]>([])

  React.useEffect(() => {
    fetchAlertsByAgent(agent.id, 5).then(setAlerts)
  }, [agent.id])

  const ramUsed = agent.totalRam - agent.freeRam
  const diskUsed = agent.diskTotal - agent.diskFree

  return (
    <div className="flex flex-col gap-4">
      {/* CPU / RAM / Disk instant values */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Card className="p-4">
          <p className="text-xs text-muted-foreground">CPU</p>
          <p className="mt-1 text-2xl font-semibold tabular-nums">
            {Math.round(agent.cpuLoad)}%
          </p>
          <p className="mt-0.5 text-xs text-muted-foreground">{agent.cpuModel}</p>
        </Card>
        <Card className="p-4">
          <p className="text-xs text-muted-foreground">RAM</p>
          <p className="mt-1 text-2xl font-semibold tabular-nums">
            {fmtPct(ramUsed, agent.totalRam)}
          </p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {fmtBytes(ramUsed)} / {fmtBytes(agent.totalRam)}
          </p>
        </Card>
        <Card className="p-4">
          <p className="text-xs text-muted-foreground">Disk</p>
          <p className="mt-1 text-2xl font-semibold tabular-nums">
            {fmtPct(diskUsed, agent.diskTotal)}
          </p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {fmtBytes(diskUsed)} / {fmtBytes(agent.diskTotal)}
          </p>
        </Card>
      </div>

      {/* Telemetry Chart */}
      <Card className="p-4">
        <TelemetryChart agentId={agent.id} />
      </Card>

      {/* Disk Partitions */}
      {agent.disks && agent.disks.length > 0 && (
        <Card className="p-4">
          <h3 className="mb-2 text-sm font-semibold">Disk Partitions</h3>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {agent.disks.map((d) => {
              const pct = d.size > 0 ? Math.round(((d.size - d.freeSpace) / d.size) * 100) : 0
              return (
                <div key={d.deviceID} className="rounded-lg border border-border p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-medium">{d.deviceID}</span>
                    <span className="text-xs text-muted-foreground">{d.filesystem}</span>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">{d.label || "—"}</p>
                  <p className="mt-1 text-xs tabular-nums">
                    {pct}% used &middot; {fmtBytes(d.size - d.freeSpace)} / {fmtBytes(d.size)}
                  </p>
                </div>
              )
            })}
          </div>
        </Card>
      )}

      {/* Network */}
      <Card className="p-4">
        <h3 className="mb-2 text-sm font-semibold">Network</h3>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
          <div className="rounded-lg border border-border p-3">
            <span className="text-xs text-muted-foreground">IP</span>
            <p className="text-sm font-medium">{agent.localIP || "—"}</p>
          </div>
          <div className="rounded-lg border border-border p-3">
            <span className="text-xs text-muted-foreground">MAC</span>
            <p className="text-sm font-medium">{agent.macAddress || "—"}</p>
          </div>
          <div className="rounded-lg border border-border p-3">
            <span className="text-xs text-muted-foreground">Gateway</span>
            <p className="text-sm font-medium">{agent.gateway || "—"}</p>
          </div>
        </div>
      </Card>

      {/* GPU */}
      {agent.gpuName && (
        <Card className="p-4">
          <h3 className="mb-2 text-sm font-semibold">Graphics</h3>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <div className="rounded-lg border border-border p-3">
              <span className="text-xs text-muted-foreground">GPU</span>
              <p className="text-sm font-medium">{agent.gpuName}</p>
            </div>
            <div className="rounded-lg border border-border p-3">
              <span className="text-xs text-muted-foreground">Driver</span>
              <p className="text-sm font-medium">{agent.gpuDriver || "—"}</p>
            </div>
          </div>
        </Card>
      )}

      {/* Recent Alerts */}
      <Card className="p-4">
        <h3 className="mb-2 text-sm font-semibold">Recent Alerts</h3>
        {alerts.length === 0 ? (
          <p className="text-sm text-muted-foreground">No recent alerts.</p>
        ) : (
          <div className="flex flex-col gap-2">
            {alerts.map((a) => (
              <div key={a.id} className="flex items-center justify-between rounded-lg border border-border p-3">
                <div className="flex items-center gap-2">
                  <Badge
                    variant="outline"
                    className={
                      a.severity === "critical"
                        ? "text-destructive"
                        : a.severity === "warning"
                          ? "text-warning"
                          : "text-info"
                    }
                  >
                    {a.severity}
                  </Badge>
                  <span className="text-sm">{a.message}</span>
                </div>
                <span className="text-xs text-muted-foreground">
                  {new Date(a.time).toLocaleString()}
                </span>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  )
}
