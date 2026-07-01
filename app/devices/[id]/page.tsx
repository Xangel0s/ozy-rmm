"use client"

import * as React from "react"
import Link from "next/link"
import { useParams } from "next/navigation"
import {
  ArrowLeft,
  Cpu,
  HardDrive,
  MemoryStick,
  Monitor,
  ShieldAlert,
  TerminalSquare,
} from "lucide-react"
import { toast } from "sonner"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { alerts, deviceBackups, devices, osLabels } from "@/lib/rmm-data"

export default function DeviceDetailPage() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const params = useParams<{ id: string }>()
  const id = params?.id
  const device = devices.find((item) => item.id === id)

  if (!device) {
    return (
      <ConsoleShell
        tenant={tenant}
        onTenantChange={setTenant}
        query={query}
        onQueryChange={setQuery}
        title="Device Details"
        subtitle="Endpoint not found"
      >
        <Card className="gap-3 p-5">
          <h2 className="text-base font-semibold">Endpoint not found</h2>
          <p className="text-sm text-muted-foreground">
            The selected endpoint does not exist or was removed from inventory.
          </p>
          <div>
            <Link href="/devices" className="text-sm font-medium text-primary hover:underline">
              Return to device list
            </Link>
          </div>
        </Card>
      </ConsoleShell>
    )
  }

  const relatedAlerts = alerts.filter((alert) => alert.device === device.name)
  const backups = deviceBackups.filter((backup) => backup.deviceId === device.id)

  return (
    <ConsoleShell
      tenant={tenant}
      onTenantChange={setTenant}
      query={query}
      onQueryChange={setQuery}
      title={device.name}
      subtitle={`${device.tenant} - ${device.ip}`}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Link href="/devices" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-4" />
          Back to devices
        </Link>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              toast.success(`Terminal session queued for ${device.name}`, {
                description: "Session broker is preparing secure shell access.",
              })
            }
          >
            <TerminalSquare data-icon="inline-start" />
            Terminal
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              toast.success(`Remote control requested for ${device.name}`, {
                description: "Operator session will open after endpoint confirmation.",
              })
            }
          >
            <Monitor data-icon="inline-start" />
            Remote Control
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              toast.success(`Collect Diagnostics queued for ${device.name}`, {
                description: "Execution will start on the next heartbeat.",
              })
            }
          >
            <TerminalSquare data-icon="inline-start" />
            Run Diagnostics
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Card className="gap-1 p-4">
          <span className="text-xs text-muted-foreground">CPU</span>
          <span className="text-2xl font-semibold tabular-nums">{device.cpu}%</span>
          <span className="text-xs text-muted-foreground">Current utilization</span>
          <Cpu className="mt-2 size-4 text-muted-foreground" />
        </Card>
        <Card className="gap-1 p-4">
          <span className="text-xs text-muted-foreground">RAM</span>
          <span className="text-2xl font-semibold tabular-nums">{device.ram}%</span>
          <span className="text-xs text-muted-foreground">Memory pressure</span>
          <MemoryStick className="mt-2 size-4 text-muted-foreground" />
        </Card>
        <Card className="gap-1 p-4">
          <span className="text-xs text-muted-foreground">Disk</span>
          <span className="text-2xl font-semibold tabular-nums">{device.disk}%</span>
          <span className="text-xs text-muted-foreground">Storage usage</span>
          <HardDrive className="mt-2 size-4 text-muted-foreground" />
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1fr_360px]">
        <Card className="gap-3 p-4">
          <h2 className="text-sm font-semibold">Endpoint Profile</h2>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <div className="rounded-lg border border-border p-3">
              <p className="text-xs text-muted-foreground">Hostname</p>
              <p className="font-medium">{device.name}</p>
            </div>
            <div className="rounded-lg border border-border p-3">
              <p className="text-xs text-muted-foreground">Operating system</p>
              <p className="font-medium">{osLabels[device.os]}</p>
            </div>
            <div className="rounded-lg border border-border p-3">
              <p className="text-xs text-muted-foreground">Status</p>
              <Badge className="mt-1 capitalize">{device.status}</Badge>
            </div>
            <div className="rounded-lg border border-border p-3">
              <p className="text-xs text-muted-foreground">Last check-in</p>
              <p className="font-medium">{device.lastSync}</p>
            </div>
          </div>
        </Card>

        <Card className="gap-3 p-4">
          <h2 className="text-sm font-semibold">Active Incidents</h2>
          {relatedAlerts.length === 0 ? (
            <p className="text-sm text-muted-foreground">No active incidents for this endpoint.</p>
          ) : (
            <ul className="space-y-2">
              {relatedAlerts.map((alert) => (
                <li key={alert.id} className="rounded-lg border border-border p-3">
                  <div className="flex items-center gap-2">
                    <ShieldAlert className="size-4 text-warning" />
                    <Badge variant="outline" className="capitalize">
                      {alert.severity}
                    </Badge>
                    <span className="ml-auto text-xs text-muted-foreground">{alert.time}</span>
                  </div>
                  <p className="mt-1 text-sm text-muted-foreground">{alert.message}</p>
                  <Link
                    href={`/alerts/${alert.id}`}
                    className="mt-2 inline-block text-xs font-medium text-primary hover:underline"
                  >
                    Open incident
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <Card className="gap-3 p-4">
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-sm font-semibold">Backups</h2>
          <div className="flex items-center gap-2">
            <Badge variant="secondary">{backups.length} records</Badge>
            <Link href="/backups" className="text-xs font-medium text-primary hover:underline">
              Open backups module
            </Link>
          </div>
        </div>

        {backups.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No backup records available for this endpoint.
          </p>
        ) : (
          <div className="space-y-2">
            {backups.map((backup) => (
              <div
                key={backup.id}
                className="grid grid-cols-1 gap-2 rounded-lg border border-border p-3 md:grid-cols-[1fr_auto_auto]"
              >
                <div>
                  <p className="font-medium">{backup.name}</p>
                  <p className="text-xs text-muted-foreground">{backup.location}</p>
                  <p className="text-xs text-muted-foreground">{backup.createdAt}</p>
                </div>
                <div className="flex items-center gap-2 md:justify-self-end">
                  <Badge variant="outline" className="capitalize">
                    {backup.type}
                  </Badge>
                  <Badge variant="outline" className="capitalize">
                    {backup.status}
                  </Badge>
                </div>
                <div className="text-sm font-medium md:justify-self-end">{backup.size}</div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </ConsoleShell>
  )
}
