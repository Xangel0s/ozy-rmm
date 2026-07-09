"use client"

import * as React from "react"
import Link from "next/link"
import { toast } from "sonner"
import { AlertOctagon, AlertTriangle, CheckCheck, Info, Loader2 } from "lucide-react"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { useAlerts, useAgents, agentToDevice } from "@/lib/use-live-data"
import { acknowledgeAlert } from "@/lib/api"
import { cn } from "@/lib/utils"

type Severity = "critical" | "warning" | "info"

const severityIcon = {
  critical: AlertOctagon,
  warning: AlertTriangle,
  info: Info,
}

const severityTone = {
  critical: "text-destructive bg-destructive/10 border-destructive/30",
  warning: "text-warning bg-warning/10 border-warning/30",
  info: "text-info bg-info/10 border-info/30",
}

function toDisplayAlert(a: { id: number; agentId: string; severity: string; message: string; time: string }) {
  const sev: Severity =
    a.severity === "critical" || a.severity === "warning" ? a.severity : "info"
  return {
    id: a.id,
    device: a.agentId,
    message: a.message,
    severity: sev,
    time: a.time ? new Date(a.time).toLocaleString() : "",
  }
}

export default function AlertsPage() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [severity, setSeverity] = React.useState<"all" | Severity>("all")
  const [acknowledging, setAcknowledging] = React.useState<number | null>(null)

  const { alerts: liveAlerts, loading } = useAlerts()
  const { agents } = useAgents()
  const liveDevices = React.useMemo(() => agents.map(agentToDevice), [agents])

  const allAlerts = React.useMemo(() => liveAlerts.map(toDisplayAlert), [liveAlerts])

  const filtered = React.useMemo(() => {
    const q = query.trim().toLowerCase()
    return allAlerts.filter((alert) => {
      const matchSeverity = severity === "all" || alert.severity === severity
      const matchQuery =
        q === "" ||
        alert.device.toLowerCase().includes(q) ||
        alert.message.toLowerCase().includes(q)
      return matchSeverity && matchQuery
    })
  }, [query, severity, allAlerts])

  const counts = React.useMemo(
    () => ({
      critical: allAlerts.filter((a) => a.severity === "critical").length,
      warning: allAlerts.filter((a) => a.severity === "warning").length,
      info: allAlerts.filter((a) => a.severity === "info").length,
    }),
    [allAlerts],
  )

  const deviceIdByName = React.useMemo(
    () => new Map(liveDevices.map((d) => [d.name, d.id])),
    [liveDevices],
  )

  const handleAcknowledge = async (alertId: number) => {
    setAcknowledging(alertId)
    const ok = await acknowledgeAlert(alertId)
    setAcknowledging(null)
    if (ok) {
      toast.success("Alert acknowledged")
    } else {
      toast.error("Failed to acknowledge alert")
    }
  }

  return (
    <ConsoleShell
      tenant={tenant}
      onTenantChange={setTenant}
      query={query}
      onQueryChange={setQuery}
      title="Alerts"
      subtitle="Prioritized event center"
    >
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Card className="gap-0 p-4">
          <span className="text-xs text-muted-foreground">Critical</span>
          <span className="mt-1 text-2xl font-semibold text-destructive tabular-nums">{counts.critical}</span>
        </Card>
        <Card className="gap-0 p-4">
          <span className="text-xs text-muted-foreground">Warning</span>
          <span className="mt-1 text-2xl font-semibold text-warning tabular-nums">{counts.warning}</span>
        </Card>
        <Card className="gap-0 p-4">
          <span className="text-xs text-muted-foreground">Info</span>
          <span className="mt-1 text-2xl font-semibold text-info tabular-nums">{counts.info}</span>
        </Card>
      </div>

      <Card className="gap-0 p-0">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3">
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-semibold">Active Events</h2>
            <Badge variant="secondary">{filtered.length} visible</Badge>
          </div>

          <div className="flex items-center gap-2">
            <div className="flex items-center rounded-lg bg-secondary p-0.5">
              {(["all", "critical", "warning", "info"] as const).map((level) => (
                <button
                  key={level}
                  type="button"
                  onClick={() => setSeverity(level)}
                  className={cn(
                    "rounded-md px-2.5 py-1 text-xs font-medium capitalize",
                    severity === level
                      ? "bg-card text-foreground ring-1 ring-border"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {level}
                </button>
              ))}
            </div>
          </div>
        </div>

        {loading && (
          <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
            <Loader2 className="size-4 animate-spin" />
            Loading alerts from agents...
          </div>
        )}

        {!loading && filtered.length === 0 && (
          <div className="px-4 py-8 text-center text-sm text-muted-foreground">
            {allAlerts.length === 0
              ? "No alerts detected. Agents will report alerts when metrics exceed thresholds."
              : "No alerts match the current filter."}
          </div>
        )}

        <ul className="divide-y divide-border">
          {filtered.map((alert) => {
            const Icon = severityIcon[alert.severity]
            const deviceId = deviceIdByName.get(alert.device)
            return (
              <li key={alert.id} className="flex items-start gap-3 px-4 py-3 hover:bg-muted/30">
                <span
                  className={cn(
                    "mt-0.5 inline-flex size-7 items-center justify-center rounded-md border",
                    severityTone[alert.severity],
                  )}
                >
                  <Icon className="size-4" />
                </span>
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <div className="flex items-center gap-2">
                    {deviceId ? (
                      <Link
                        href={`/devices/${deviceId}`}
                        className="font-mono text-xs font-semibold transition-colors hover:text-primary"
                      >
                        {alert.device}
                      </Link>
                    ) : (
                      <span className="font-mono text-xs font-semibold">{alert.device}</span>
                    )}
                    <Badge variant="outline" className="capitalize">
                      {alert.severity}
                    </Badge>
                    <span className="ml-auto text-xs text-muted-foreground">{alert.time}</span>
                  </div>
                  <p className="text-sm text-muted-foreground">{alert.message}</p>
                  <div className="flex items-center gap-3">
                    <Link
                      href={`/alerts/${alert.id}`}
                      className="text-xs font-medium text-primary hover:underline"
                    >
                      Open incident details
                    </Link>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 text-xs"
                      disabled={acknowledging === alert.id}
                      onClick={() => handleAcknowledge(alert.id)}
                    >
                      {acknowledging === alert.id ? (
                        <Loader2 className="size-3 animate-spin" />
                      ) : (
                        <CheckCheck className="size-3" />
                      )}
                      Acknowledge
                    </Button>
                  </div>
                </div>
              </li>
            )
          })}
        </ul>
      </Card>
    </ConsoleShell>
  )
}
