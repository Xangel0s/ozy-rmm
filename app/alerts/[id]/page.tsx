"use client"

import * as React from "react"
import Link from "next/link"
import { useParams } from "next/navigation"
import { ArrowLeft, Loader2, ShieldAlert, Siren, TerminalSquare } from "lucide-react"
import { toast } from "sonner"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { fetchAlert, fetchAgent, acknowledgeAlert } from "@/lib/api"
import type { AlertRow, AgentInfo } from "@/lib/api"

type Severity = "critical" | "warning" | "info"

export default function AlertDetailPage() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [alert, setAlert] = React.useState<AlertRow | null>(null)
  const [agent, setAgent] = React.useState<AgentInfo | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [acknowledging, setAcknowledging] = React.useState(false)
  const params = useParams<{ id: string }>()
  const id = params?.id

  React.useEffect(() => {
    if (!id) return
    let cancelled = false

    async function load() {
      setLoading(true)
      const alertData = await fetchAlert(id!)
      if (cancelled) return

      if (!alertData) {
        setAlert(null)
        setLoading(false)
        return
      }

      setAlert(alertData)

      // Fetch the related agent for extra info
      const agentData = await fetchAgent(alertData.agentId)
      if (!cancelled && agentData) {
        setAgent(agentData)
      }
      if (!cancelled) setLoading(false)
    }

    load()
    return () => { cancelled = true }
  }, [id])

  const handleAcknowledge = async () => {
    if (!alert) return
    setAcknowledging(true)
    const ok = await acknowledgeAlert(alert.id)
    setAcknowledging(false)
    if (ok) {
      toast.success(`Incident #${alert.id} acknowledged`)
    } else {
      toast.error("Failed to acknowledge incident")
    }
  }

  const sev: Severity = alert
    ? (alert.severity === "critical" || alert.severity === "warning" ? alert.severity : "info")
    : "info"

  return (
    <ConsoleShell
      tenant={tenant}
      onTenantChange={setTenant}
      query={query}
      onQueryChange={setQuery}
      title={alert ? `Incident #${alert.id}` : "Incident Details"}
      subtitle={agent?.hostname || alert?.agentId || ""}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Link href="/alerts" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-4" />
          Back to alerts
        </Link>

        <div className="flex items-center gap-2">
          {agent && (
            <Link
              href={`/devices/${agent.id}`}
              className="inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline"
            >
              <TerminalSquare className="size-4" />
              Open device
            </Link>
          )}
          {alert && (
            <Button
              size="sm"
              disabled={acknowledging}
              onClick={handleAcknowledge}
            >
              {acknowledging ? <Loader2 className="size-4 animate-spin" /> : <ShieldAlert className="size-4" />}
              Acknowledge
            </Button>
          )}
        </div>
      </div>

      {loading && (
        <Card className="flex items-center justify-center gap-2 p-8 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" />
          Loading incident data...
        </Card>
      )}

      {!loading && !alert && (
        <Card className="gap-3 p-5">
          <h2 className="text-base font-semibold">Incident not found</h2>
          <p className="text-sm text-muted-foreground">
            This incident does not exist or you do not have access.
          </p>
          <div>
            <Link href="/alerts" className="text-sm font-medium text-primary hover:underline">
              Return to alerts
            </Link>
          </div>
        </Card>
      )}

      {!loading && alert && (
        <>
          <Card className="gap-3 p-4">
            <div className="flex items-center gap-2">
              <Siren className="size-4 text-destructive" />
              <h2 className="text-sm font-semibold">Summary</h2>
              <Badge variant="outline" className="capitalize">
                {sev}
              </Badge>
              <span className="ml-auto text-xs text-muted-foreground">
                Detected {alert.time ? new Date(alert.time).toLocaleString() : "unknown"}
              </span>
            </div>
            <p className="text-sm text-muted-foreground">{alert.message}</p>
          </Card>

          <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1fr_1fr]">
            <Card className="gap-3 p-4">
              <h2 className="text-sm font-semibold">Affected Endpoint</h2>
              {agent ? (
                <div className="rounded-lg border border-border p-3">
                  <div className="flex items-center justify-between">
                    <div>
                      <p className="font-medium">{agent.hostname}</p>
                      <p className="text-xs text-muted-foreground">{agent.os}</p>
                    </div>
                    <Badge variant={agent.status === "online" ? "default" : "secondary"}>
                      {agent.status}
                    </Badge>
                  </div>
                  <div className="mt-3 grid grid-cols-3 gap-2 text-xs">
                    <div>
                      <span className="text-muted-foreground">CPU</span>
                      <p className="font-medium">{Math.round(agent.cpuLoad)}%</p>
                    </div>
                    <div>
                      <span className="text-muted-foreground">RAM</span>
                      <p className="font-medium">
                        {agent.totalRam > 0
                          ? `${Math.round(((agent.totalRam - agent.freeRam) / agent.totalRam) * 100)}%`
                          : "N/A"}
                      </p>
                    </div>
                    <div>
                      <span className="text-muted-foreground">Disk</span>
                      <p className="font-medium">
                        {agent.diskTotal > 0
                          ? `${Math.round(((agent.diskTotal - agent.diskFree) / agent.diskTotal) * 100)}%`
                          : "N/A"}
                      </p>
                    </div>
                  </div>
                  <Link
                    href={`/devices/${agent.id}`}
                    className="mt-3 inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
                  >
                    <TerminalSquare className="size-3" />
                    Open device details
                  </Link>
                </div>
              ) : (
                <div className="rounded-lg border border-border p-3">
                  <p className="text-xs text-muted-foreground">Agent ID</p>
                  <p className="font-mono font-medium">{alert.agentId}</p>
                </div>
              )}
            </Card>

            <Card className="gap-3 p-4">
              <h2 className="text-sm font-semibold">Response Runbook</h2>
              <ol className="space-y-2 text-sm text-muted-foreground">
                <li className="rounded-lg border border-border p-2">
                  1. Validate current endpoint telemetry and confirm scope.
                </li>
                <li className="rounded-lg border border-border p-2">
                  2. Execute remediation script and monitor heartbeat recovery.
                </li>
                <li className="rounded-lg border border-border p-2">
                  3. Document root cause and close incident after stabilization.
                </li>
              </ol>
            </Card>
          </div>

          <Card className="gap-2 p-4">
            <div className="flex items-center gap-2">
              <ShieldAlert className="size-4 text-warning" />
              <h2 className="text-sm font-semibold">Operator Notes</h2>
            </div>
            <p className="text-sm text-muted-foreground">
              Incident #{alert.id} triggered by {alert.severity} alert from agent {alert.agentId}.
              {alert.message}
            </p>
          </Card>
        </>
      )}
    </ConsoleShell>
  )
}
