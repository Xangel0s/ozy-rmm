"use client"

import * as React from "react"
import Link from "next/link"
import { useParams } from "next/navigation"
import { ArrowLeft, ShieldAlert, Siren, TerminalSquare } from "lucide-react"
import { toast } from "sonner"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { alerts, devices } from "@/lib/rmm-data"

export default function AlertDetailPage() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const params = useParams<{ id: string }>()
  const id = params?.id

  const incident = alerts.find((item) => item.id === id)
  const relatedDevice = incident ? devices.find((device) => device.name === incident.device) : undefined

  if (!incident) {
    return (
      <ConsoleShell
        tenant={tenant}
        onTenantChange={setTenant}
        query={query}
        onQueryChange={setQuery}
        title="Incident Details"
        subtitle="Incident not found"
      >
        <Card className="gap-3 p-5">
          <h2 className="text-base font-semibold">Incident not found</h2>
          <p className="text-sm text-muted-foreground">
            This incident does not exist anymore or the ID is not valid.
          </p>
          <div>
            <Link href="/alerts" className="text-sm font-medium text-primary hover:underline">
              Return to alerts
            </Link>
          </div>
        </Card>
      </ConsoleShell>
    )
  }

  return (
    <ConsoleShell
      tenant={tenant}
      onTenantChange={setTenant}
      query={query}
      onQueryChange={setQuery}
      title={`Incident ${incident.id.toUpperCase()}`}
      subtitle={incident.device}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Link href="/alerts" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-4" />
          Back to alerts
        </Link>

        <div className="flex items-center gap-2">
          {relatedDevice && (
            <Link
              href={`/devices/${relatedDevice.id}`}
              className="inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline"
            >
              <TerminalSquare className="size-4" />
              Open device
            </Link>
          )}
          <Button
            size="sm"
            onClick={() =>
              toast.success(`Incident ${incident.id.toUpperCase()} acknowledged`, {
                description: "Escalation flow has been paused.",
              })
            }
          >
            Acknowledge
          </Button>
        </div>
      </div>

      <Card className="gap-3 p-4">
        <div className="flex items-center gap-2">
          <Siren className="size-4 text-destructive" />
          <h2 className="text-sm font-semibold">Summary</h2>
          <Badge variant="outline" className="capitalize">
            {incident.severity}
          </Badge>
          <span className="ml-auto text-xs text-muted-foreground">Detected {incident.time}</span>
        </div>
        <p className="text-sm text-muted-foreground">{incident.message}</p>
      </Card>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1fr_1fr]">
        <Card className="gap-3 p-4">
          <h2 className="text-sm font-semibold">Response Runbook</h2>
          <ol className="space-y-2 text-sm text-muted-foreground">
            <li className="rounded-lg border border-border p-2">1. Validate current endpoint telemetry and confirm scope.</li>
            <li className="rounded-lg border border-border p-2">2. Execute remediation script and monitor heartbeat recovery.</li>
            <li className="rounded-lg border border-border p-2">3. Document root cause and close incident after stabilization.</li>
          </ol>
        </Card>

        <Card className="gap-3 p-4">
          <h2 className="text-sm font-semibold">Timeline</h2>
          <ul className="space-y-2 text-sm text-muted-foreground">
            <li className="rounded-lg border border-border p-2">{incident.time}: Alert triggered by policy engine.</li>
            <li className="rounded-lg border border-border p-2">1m later: Automated enrichment attached endpoint metadata.</li>
            <li className="rounded-lg border border-border p-2">Current: Awaiting operator action and acknowledgement.</li>
          </ul>
          {relatedDevice && (
            <div className="rounded-lg border border-border p-3">
              <p className="text-xs text-muted-foreground">Affected endpoint</p>
              <Link href={`/devices/${relatedDevice.id}`} className="font-medium text-primary hover:underline">
                {relatedDevice.name}
              </Link>
            </div>
          )}
          {!relatedDevice && (
            <div className="rounded-lg border border-border p-3">
              <p className="text-xs text-muted-foreground">Affected endpoint</p>
              <p className="font-medium">{incident.device}</p>
            </div>
          )}
        </Card>
      </div>

      <Card className="gap-2 p-4">
        <div className="flex items-center gap-2">
          <ShieldAlert className="size-4 text-warning" />
          <h2 className="text-sm font-semibold">Operator Notes</h2>
        </div>
        <p className="text-sm text-muted-foreground">
          This incident detail page is now part of the RMM workflow: triage, endpoint pivoting, and acknowledgement.
        </p>
      </Card>
    </ConsoleShell>
  )
}
