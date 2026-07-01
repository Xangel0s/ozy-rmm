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
import { deviceBackups, devices, tenants } from "@/lib/rmm-data"

export default function BackupsPage() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [statusFilter, setStatusFilter] = React.useState<"all" | "completed" | "running" | "failed">(
    "all",
  )

  const tenantName = tenants.find((t) => t.id === tenant)?.name

  const records = React.useMemo(() => {
    const q = query.trim().toLowerCase()

    return deviceBackups
      .map((backup) => ({
        backup,
        device: devices.find((device) => device.id === backup.deviceId),
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
  }, [query, statusFilter, tenant, tenantName])

  const stats = React.useMemo(
    () => ({
      completed: records.filter((r) => r.backup.status === "completed").length,
      running: records.filter((r) => r.backup.status === "running").length,
      failed: records.filter((r) => r.backup.status === "failed").length,
    }),
    [records],
  )

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

            <Button
              size="sm"
              onClick={() =>
                toast.success("New backup job queued", {
                  description: "Policy execution started for selected tenant scope.",
                })
              }
            >
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
                  <TableHead>Tamano</TableHead>
                  <TableHead>Cron</TableHead>
                  <TableHead>Ultima ejecucion</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {records.map(({ backup, device }) => (
                  <TableRow key={backup.id}>
                    <TableCell className="font-medium">{backup.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{backup.location}</TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Database className="size-3.5 text-muted-foreground" />
                        <Link href={`/devices/${device.id}`} className="font-medium text-primary hover:underline">
                          {device.name}
                        </Link>
                      </div>
                    </TableCell>
                    <TableCell>{device.tenant}</TableCell>
                    <TableCell>
                      <Badge variant="outline" className="capitalize">
                        {backup.type}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Badge
                          variant="outline"
                          className={
                            backup.status === "failed"
                              ? "capitalize text-destructive"
                              : backup.status === "running"
                                ? "capitalize text-info"
                                : "capitalize text-success"
                          }
                        >
                          {backup.status}
                        </Badge>
                        {backup.status === "failed" && <CircleAlert className="size-4 text-destructive" />}
                      </div>
                    </TableCell>
                    <TableCell className="font-medium">{backup.size}</TableCell>
                    <TableCell className="font-mono text-xs">{backup.cron}</TableCell>
                    <TableCell className="text-muted-foreground">{backup.createdAt}</TableCell>
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
