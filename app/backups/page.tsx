"use client"

import * as React from "react"
import Link from "next/link"
import { ArchiveRestore, CircleAlert, Database, PlayCircle } from "lucide-react"
import { toast } from "sonner"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { deviceBackups, devices as mockDevices, tenants } from "@/lib/rmm-data"
import { useBackups, useAgents, agentToDevice } from "@/lib/use-live-data"
import { runBackup } from "@/lib/api"
import type { BackupJob } from "@/lib/api"

/** Format bytes into a human-readable string */
function fmtBytes(bytes: number): string {
  if (bytes === 0) return "0 B"
  const k = 1024
  const sizes = ["B", "KB", "MB", "GB", "TB"]
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

export default function BackupsPage() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [statusFilter, setStatusFilter] = React.useState<"all" | "completed" | "running" | "failed">(
    "all",
  )

  // Live data
  const { backups: liveBackups } = useBackups(15000)
  const { agents } = useAgents(5000)
  const liveDevices = React.useMemo(() => agents.map(agentToDevice), [agents])

  // Use live or mock depending on backend availability
  const isLive = liveBackups.length > 0
  const tenantName = tenants.find((t) => t.id === tenant)?.name

  // Build unified records for the table
  const records = React.useMemo(() => {
    if (isLive) {
      // Live path: join backup_jobs with live agents
      const q = query.trim().toLowerCase()
      const deviceMap = new Map(liveDevices.map((d) => [d.id, d]))

      return liveBackups
        .filter((b) => {
          const matchStatus = statusFilter === "all" || b.status === statusFilter
          const matchQuery =
            q === "" ||
            b.name.toLowerCase().includes(q) ||
            b.location.toLowerCase().includes(q) ||
            b.agentId.toLowerCase().includes(q)
          return matchStatus && matchQuery
        })
        .map((b) => ({
          id: String(b.id),
          name: b.name,
          location: b.location,
          deviceId: b.agentId,
          deviceName: deviceMap.get(b.agentId)?.name ?? b.agentId,
          deviceTenant: deviceMap.get(b.agentId)?.tenant ?? "Live Agent",
          type: b.type,
          status: b.status as "completed" | "running" | "failed" | "pending",
          size: fmtBytes(b.sizeBytes),
          cron: b.cron,
          executedAt: b.executedAt ? new Date(b.executedAt).toLocaleString() : "—",
        }))
    }

    // Mock path
    const q = query.trim().toLowerCase()
    return deviceBackups
      .map((backup) => ({
        backup,
        device: mockDevices.find((d) => d.id === backup.deviceId),
      }))
      .filter((entry) => {
        if (!entry.device) return false
        const matchTenant = tenant === "all" || entry.device.tenant === tenantName
        const matchStatus = statusFilter === "all" || entry.backup.status === statusFilter
        const matchQuery =
          q === "" ||
          entry.backup.name.toLowerCase().includes(q) ||
          entry.backup.location.toLowerCase().includes(q) ||
          entry.device.name.toLowerCase().includes(q)
        return matchTenant && matchStatus && matchQuery
      })
      .map(({ backup, device }) => ({
        id: backup.id,
        name: backup.name,
        location: backup.location,
        deviceId: device!.id,
        deviceName: device!.name,
        deviceTenant: device!.tenant,
        type: backup.type,
        status: backup.status as "completed" | "running" | "failed" | "pending",
        size: backup.size,
        cron: backup.cron,
        executedAt: backup.createdAt,
      }))
  }, [isLive, liveBackups, liveDevices, deviceBackups, mockDevices, query, statusFilter, tenant, tenantName])

  const stats = React.useMemo(
    () => ({
      completed: records.filter((r) => r.status === "completed").length,
      running: records.filter((r) => r.status === "running").length,
      failed: records.filter((r) => r.status === "failed").length,
    }),
    [records],
  )

  const handleRunBackup = async () => {
    if (agents.length > 0) {
      await runBackup(agents[0].id)
      toast.success("Manual backup queued", {
        description: `Backup job dispatched to ${agents[0].hostname}.`,
      })
    } else {
      toast.success("New backup job queued", {
        description: "Policy execution started for selected tenant scope.",
      })
    }
  }

  return (
    <ConsoleShell
      tenant={tenant}
      onTenantChange={setTenant}
      query={query}
      onQueryChange={setQuery}
      title="Backups"
      subtitle="Centralized backup operations"
      showSearch
    >
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Card className="gap-1 p-4">
          <span className="text-xs text-muted-foreground">Completed</span>
          <span className="text-2xl font-semibold text-success tabular-nums">{stats.completed}</span>
        </Card>
        <Card className="gap-1 p-4">
          <span className="text-xs text-muted-foreground">Running</span>
          <span className="text-2xl font-semibold text-info tabular-nums">{stats.running}</span>
        </Card>
        <Card className="gap-1 p-4">
          <span className="text-xs text-muted-foreground">Failed</span>
          <span className="text-2xl font-semibold text-destructive tabular-nums">{stats.failed}</span>
        </Card>
      </div>

      <Card className="gap-0 p-0">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3">
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-semibold">Backup Jobs</h2>
            <Badge variant="secondary">{records.length} visible</Badge>
          </div>

          <div className="flex items-center gap-2">
            <div className="flex items-center rounded-lg bg-secondary p-0.5">
              {(["all", "completed", "running", "failed"] as const).map((status) => (
                <button
                  key={status}
                  type="button"
                  onClick={() => setStatusFilter(status)}
                  className={
                    statusFilter === status
                      ? "rounded-md bg-card px-2.5 py-1 text-xs font-medium capitalize text-foreground ring-1 ring-border"
                      : "rounded-md px-2.5 py-1 text-xs font-medium capitalize text-muted-foreground hover:text-foreground"
                  }
                >
                  {status}
                </button>
              ))}
            </div>

            <Button size="sm" onClick={handleRunBackup}>
              <PlayCircle data-icon="inline-start" />
              Run Backup
            </Button>
          </div>
        </div>

        {records.length === 0 ? (
          <div className="px-4 py-8 text-center">
            <ArchiveRestore className="mx-auto mb-2 size-5 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">No backup records for this filter.</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead>Job</TableHead>
                  <TableHead>Destino</TableHead>
                  <TableHead>Dispositivo</TableHead>
                  <TableHead>Cliente</TableHead>
                  <TableHead>Tipo</TableHead>
                  <TableHead>Estado</TableHead>
                  <TableHead>Tamaño</TableHead>
                  <TableHead>Cron</TableHead>
                  <TableHead>Última ejecución</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {records.map((r) => (
                  <TableRow key={r.id}>
                    <TableCell className="font-medium">{r.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{r.location}</TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Database className="size-3.5 text-muted-foreground" />
                        <Link href={`/devices/${r.deviceId}`} className="font-medium text-primary hover:underline">
                          {r.deviceName}
                        </Link>
                      </div>
                    </TableCell>
                    <TableCell>{r.deviceTenant}</TableCell>
                    <TableCell>
                      <Badge variant="outline" className="capitalize">
                        {r.type}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Badge
                          variant="outline"
                          className={
                            r.status === "failed"
                              ? "capitalize text-destructive"
                              : r.status === "running"
                                ? "capitalize text-info"
                                : "capitalize text-success"
                          }
                        >
                          {r.status}
                        </Badge>
                        {r.status === "failed" && <CircleAlert className="size-4 text-destructive" />}
                      </div>
                    </TableCell>
                    <TableCell className="font-medium">{r.size}</TableCell>
                    <TableCell className="font-mono text-xs">{r.cron}</TableCell>
                    <TableCell className="text-muted-foreground">{r.executedAt}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </Card>
    </ConsoleShell>
  )
}
