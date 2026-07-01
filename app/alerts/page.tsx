"use client"

import * as React from "react"
import Link from "next/link"
import { toast } from "sonner"
import { AlertOctagon, AlertTriangle, CheckCheck, Info } from "lucide-react"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { alerts as mockAlerts, devices as mockDevices } from "@/lib/rmm-data"
import { useAlerts, useAgents, agentToDevice } from "@/lib/use-live-data"
import type { AlertRow } from "@/lib/api"
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

/** Normalise raw backend AlertRow into a display-friendly shape. */
function toDisplayAlert(a: AlertRow) {
  const sev: Severity =
    a.severity === "critical" || a.severity === "warning" ? a.severity : "info"
  return {
    id: String(a.id),
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

  // Live data
  const { alerts: liveAlerts } = useAlerts(10000)
  const { agents } = useAgents(5000)
  const liveDevices = React.useMemo(() => agents.map(agentToDevice), [agents])

  // Prefer live data, fall back to mocks
  const allAlerts = liveAlerts.length > 0 ? liveAlerts.map(toDisplayAlert) : mockAlerts
  const allDevices = liveDevices.length > 0 ? liveDevices : mockDevices

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

  // Map agent ID / device name to a device for linking
  const deviceIdByName = React.useMemo(
    () => new Map(allDevices.map((d) => [d.name, d.id])),
    [allDevices],
  )

  const acknowledgeAll = () => {
    toast.success("Alerts acknowledged", {
      description: `${filtered.length} alert event${filtered.length === 1 ? "" : "s"} marked as reviewed.`,
    })
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

            <Button variant="outline" size="sm" onClick={acknowledgeAll}>
              <CheckCheck data-icon="inline-start" />
              Acknowledge
            </Button>
          </div>
        </div>

        <ul className="divide-y divide-border">
          {filtered.length === 0 ? (
            <li className="px-4 py-8 text-center text-sm text-muted-foreground">
              No alerts match the current filter.
            </li>
          ) : (
            filtered.map((alert) => {
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
                    <div>
                      <Link
                        href={`/alerts/${alert.id}`}
                        className="text-xs font-medium text-primary hover:underline"
                      >
                        Open incident details
                      </Link>
                    </div>
                  </div>
                </li>
              )
            })
          )}
        </ul>
      </Card>
    </ConsoleShell>
  )
}
