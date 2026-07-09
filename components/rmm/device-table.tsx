"use client"

import * as React from "react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import {
  ArrowDown,
  ArrowUp,
  ChevronsUpDown,
  Eye,
  Monitor,
  Shield,
  MonitorPlay,
  MoreHorizontal,
  RotateCw,
  ScanLine,
  TerminalSquare,
} from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { OsIcon, StatusDot, UsageBar, usageTone } from "@/components/rmm/indicators"
import { Sparkline } from "@/components/rmm/sparkline"
import { osLabels, type Device } from "@/lib/types"
import { cn } from "@/lib/utils"

type SortKey = "name" | "tenant" | "status" | "cpu" | "ram" | "disk"

const statusRank: Record<Device["status"], number> = { warning: 0, offline: 1, online: 2 }

function UsageCell({
  value,
  trend,
}: {
  value: number
  trend: number[]
}) {
  const tone = usageTone(value)
  return (
    <div className="flex items-center gap-2">
      <div className="flex w-14 flex-col gap-1">
        <span
          className={cn(
            "text-xs font-medium tabular-nums",
            tone === "destructive" && "text-destructive",
            tone === "warning" && "text-warning",
          )}
        >
          {value}%
        </span>
        <UsageBar value={value} tone={tone} />
      </div>
      <Sparkline data={trend} tone={tone} />
    </div>
  )
}

export function DeviceTable({
  devices,
  selected,
  onToggle,
  onToggleAll,
}: {
  devices: Device[]
  selected: Set<string>
  onToggle: (id: string) => void
  onToggleAll: (ids: string[], checked: boolean) => void
}) {
  const router = useRouter()
  const [sortKey, setSortKey] = React.useState<SortKey>("status")
  const [dir, setDir] = React.useState<"asc" | "desc">("asc")
  const [ctxMenu, setCtxMenu] = React.useState<{ x: number; y: number; device: Device } | null>(
    null,
  )

  React.useEffect(() => {
    if (!ctxMenu) return

    const close = () => setCtxMenu(null)
    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") close()
    }

    window.addEventListener("click", close)
    window.addEventListener("keydown", onEscape)
    return () => {
      window.removeEventListener("click", close)
      window.removeEventListener("keydown", onEscape)
    }
  }, [ctxMenu])

  const sorted = React.useMemo(() => {
    const list = [...devices]
    list.sort((a, b) => {
      let cmp = 0
      if (sortKey === "status") cmp = statusRank[a.status] - statusRank[b.status]
      else if (sortKey === "name") cmp = a.name.localeCompare(b.name)
      else if (sortKey === "tenant") cmp = a.tenant.localeCompare(b.tenant)
      else cmp = a[sortKey] - b[sortKey]
      return dir === "asc" ? cmp : -cmp
    })
    return list
  }, [devices, sortKey, dir])

  const toggleSort = (key: SortKey) => {
    if (key === sortKey) setDir((d) => (d === "asc" ? "desc" : "asc"))
    else {
      setSortKey(key)
      setDir("asc")
    }
  }

  const ids = sorted.map((d) => d.id)
  const allSelected = ids.length > 0 && ids.every((id) => selected.has(id))
  const someSelected = ids.some((id) => selected.has(id))

  const SortHead = ({ label, k, className }: { label: string; k: SortKey; className?: string }) => (
    <TableHead className={className}>
      <button
        type="button"
        onClick={() => toggleSort(k)}
        className="inline-flex items-center gap-1 text-muted-foreground transition-colors hover:text-foreground"
      >
        {label}
        {sortKey === k ? (
          dir === "asc" ? (
            <ArrowUp className="size-3" />
          ) : (
            <ArrowDown className="size-3" />
          )
        ) : (
          <ChevronsUpDown className="size-3 opacity-40" />
        )}
      </button>
    </TableHead>
  )

  const runQuickAction = (action: string, device: Device) => {
    setCtxMenu(null)

    if (action === "details") {
      router.push(`/devices/${device.id}`)
      return
    }

    if (action === "terminal") {
      router.push(`/devices/${device.id}?tab=terminal`)
      return
    }

    if (action === "remote-control") {
      toast.success(`Remote control session requested for ${device.name}`, {
        description: "Interactive desktop relay will open when endpoint accepts the session.",
      })
      return
    }

    if (action === "script") {
      toast.success(`Collect Diagnostics queued for ${device.name}`, {
        description: "The endpoint will execute the script on next heartbeat.",
      })
      return
    }

    if (action === "restart") {
      toast.success(`Agent restart requested on ${device.name}`, {
        description: "The endpoint will reconnect automatically after restart.",
      })
      return
    }

    if (action === "scan") {
      toast.success(`Vulnerability scan started on ${device.name}`)
      return
    }

    if (action === "isolate") {
      toast.warning(`Isolation mode enabled for ${device.name}`, {
        description: "Network access will be restricted except to management plane.",
      })
    }
  }

  const openContextMenu = (event: React.MouseEvent, device: Device) => {
    event.preventDefault()
    const width = 220
    const height = 300
    const x = Math.min(event.clientX, window.innerWidth - width - 8)
    const y = Math.min(event.clientY, window.innerHeight - height - 8)
    setCtxMenu({ x: Math.max(8, x), y: Math.max(8, y), device })
  }

  return (
    <Card className="relative gap-0 p-0">
      <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
        <div className="flex items-center gap-2">
          <h2 className="text-sm font-semibold">Monitored Endpoints</h2>
          <span className="rounded-full bg-secondary px-2 py-0.5 text-xs font-medium text-muted-foreground tabular-nums">
            {devices.length}
          </span>
        </div>
        {someSelected && (
          <span className="text-xs text-muted-foreground">
            {[...selected].filter((id) => ids.includes(id)).length} selected
          </span>
        )}
      </div>

      <div className="overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              <TableHead className="w-10 pl-4">
                <Checkbox
                  checked={allSelected}
                  indeterminate={someSelected && !allSelected}
                  onCheckedChange={(checked) => onToggleAll(ids, checked === true)}
                  aria-label="Select all devices"
                />
              </TableHead>
              <SortHead label="Device" k="name" />
              <SortHead label="Client" k="tenant" className="hidden lg:table-cell" />
              <SortHead label="Status" k="status" />
              <SortHead label="CPU" k="cpu" />
              <SortHead label="RAM" k="ram" className="hidden md:table-cell" />
              <SortHead label="Disk" k="disk" className="hidden md:table-cell" />
              <TableHead className="hidden xl:table-cell">Last Sync</TableHead>
              <TableHead className="w-10 pr-4 text-right">
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sorted.map((d) => {
              const isSelected = selected.has(d.id)
              const offline = d.status === "offline"
              return (
                <TableRow
                  key={d.id}
                  data-state={isSelected ? "selected" : undefined}
                  className="group"
                  onContextMenu={(event) => openContextMenu(event, d)}
                >
                  <TableCell className="pl-4">
                    <Checkbox
                      checked={isSelected}
                      onCheckedChange={() => onToggle(d.id)}
                      aria-label={`Select ${d.name}`}
                    />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2.5">
                      <OsIcon os={d.os} />
                      <div className="flex flex-col leading-tight">
                        <Link
                          href={`/devices/${d.id}`}
                          className="font-medium transition-colors hover:text-primary"
                        >
                          {d.name}
                        </Link>
                        <span className="font-mono text-xs text-muted-foreground">
                          {osLabels[d.os]} · {d.ip}
                        </span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="hidden text-muted-foreground lg:table-cell">
                    {d.tenant}
                  </TableCell>
                  <TableCell>
                    <StatusDot status={d.status} />
                  </TableCell>
                  <TableCell>
                    {offline ? (
                      <span className="text-xs text-muted-foreground">—</span>
                    ) : (
                      <UsageCell value={d.cpu} trend={d.cpuTrend} />
                    )}
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    {offline ? (
                      <span className="text-xs text-muted-foreground">—</span>
                    ) : (
                      <UsageCell value={d.ram} trend={d.ramTrend} />
                    )}
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    <UsageCell value={d.disk} trend={d.diskTrend} />
                  </TableCell>
                  <TableCell className="hidden text-xs text-muted-foreground xl:table-cell">
                    {d.lastSync}
                  </TableCell>
                  <TableCell className="pr-4 text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            className="opacity-0 transition-opacity group-hover:opacity-100 data-[popup-open]:opacity-100"
                            aria-label={`Actions for ${d.name}`}
                          />
                        }
                      >
                        <MoreHorizontal />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-48">
                        <DropdownMenuLabel>{d.name}</DropdownMenuLabel>
                        <DropdownMenuSeparator />
                        <DropdownMenuGroup>
                          <DropdownMenuItem onClick={() => runQuickAction("script", d)}>
                            <TerminalSquare />
                            Trigger Script
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => runQuickAction("terminal", d)}>
                            <MonitorPlay />
                            Remote Terminal
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => runQuickAction("remote-control", d)}>
                            <Monitor />
                            Remote Control
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => runQuickAction("restart", d)}>
                            <RotateCw />
                            Restart Agent
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => runQuickAction("details", d)}>
                            <Eye />
                            View Details
                          </DropdownMenuItem>
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>

      {ctxMenu && (
        <div
          className="fixed z-50 min-w-52 rounded-lg border border-border bg-popover p-1 shadow-2xl"
          style={{ top: ctxMenu.y, left: ctxMenu.x }}
          onClick={(event) => event.stopPropagation()}
        >
          <div className="px-2 py-1.5 text-xs font-semibold text-muted-foreground">
            {ctxMenu.device.name}
          </div>
          <button
            type="button"
            onClick={() => runQuickAction("details", ctxMenu.device)}
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
          >
            <Eye className="size-4" />
            View Details
          </button>
          <button
            type="button"
            onClick={() => runQuickAction("terminal", ctxMenu.device)}
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
          >
            <MonitorPlay className="size-4" />
            Terminal Session
          </button>
          <button
            type="button"
            onClick={() => runQuickAction("remote-control", ctxMenu.device)}
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
          >
            <Monitor className="size-4" />
            Remote Control
          </button>
          <button
            type="button"
            onClick={() => runQuickAction("script", ctxMenu.device)}
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
          >
            <TerminalSquare className="size-4" />
            Collect Diagnostics
          </button>
          <button
            type="button"
            onClick={() => runQuickAction("restart", ctxMenu.device)}
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
          >
            <RotateCw className="size-4" />
            Restart Agent
          </button>
          <button
            type="button"
            onClick={() => runQuickAction("scan", ctxMenu.device)}
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
          >
            <ScanLine className="size-4" />
            Vulnerability Scan
          </button>
          <button
            type="button"
            onClick={() => runQuickAction("isolate", ctxMenu.device)}
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-destructive hover:bg-destructive/10"
          >
            <Shield className="size-4" />
            Isolate Endpoint
          </button>
        </div>
      )}
    </Card>
  )
}
