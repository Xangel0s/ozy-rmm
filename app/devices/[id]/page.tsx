"use client"

import * as React from "react"
import Link from "next/link"
import { useParams } from "next/navigation"
import {
  ArrowLeft,
  Cpu,
  HardDrive,
  MemoryStick,
  TerminalSquare,
  Clock,
  Server,
  Layers,
  Package,
  StickyNote,
  Boxes,
  Bug,
  Shield,
  Puzzle,
  Activity,
} from "lucide-react"
import { toast } from "sonner"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { useAgents } from "@/lib/use-live-data"
import { RemoteTerminal } from "@/components/rmm/remote-terminal"
import { SummaryTab } from "./summary-tab"
import { SoftwareTab } from "./software-tab"
import { NotesTab } from "./notes-tab"
import { AssetsTab } from "./assets-tab"
import { DebugTab } from "./debug-tab"
import { AuditTab } from "./audit-tab"
import { PatchesTab } from "./patches-tab"
import { ChecksTab } from "./checks-tab"
import { fetchAlertsByAgent } from "@/lib/api"
import type { DeviceSummary, AlertSummary } from "./types"

export default function DeviceDetailPage() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [showTerminal, setShowTerminal] = React.useState(false)
  const [activeTab, setActiveTab] = React.useState("summary")
  const [recentAlerts, setRecentAlerts] = React.useState<AlertSummary[]>([])

  const params = useParams<{ id: string }>()
  const id = params?.id

  const { agents } = useAgents(5000)

  // Fetch recent alerts for this agent
  React.useEffect(() => {
    if (!id) return
    let cancelled = false
    fetchAlertsByAgent(id, 10).then((data) => {
      if (!cancelled) {
        setRecentAlerts(data.map((a) => ({
          id: String(a.id),
          message: a.message,
          severity: a.severity,
          time: a.time,
        })))
      }
    })
    return () => { cancelled = true }
  }, [id])

  const device = React.useMemo((): DeviceSummary | null => {
    const agent = agents.find((a) => a.id === id)
    if (!agent) return null
    return {
      id: agent.id,
      hostname: agent.hostname,
      status: agent.status === "online" ? "online" : "offline",
      lastSeen: agent.lastSeen || "",
      os: agent.os || "",
      kernelVersion: agent.kernelVersion || "",
      agentVersion: agent.agentVersion || "",
      uptime: agent.uptime || "",
      cpuModel: agent.cpuModel || "",
      numCPU: agent.numCPU || 0,
      cpuLoad: agent.cpuLoad || 0,
      totalRAM: agent.totalRam || 0,
      freeRam: agent.freeRam || 0,
      gpuName: agent.gpuName || "",
      gpuDriver: agent.gpuDriver || "",
      vendor: agent.vendor || "",
      model: agent.model || "",
      serialNumber: agent.serialNumber || "",
      localIP: agent.localIP || "",
      macAddress: agent.macAddress || "",
      gateway: agent.gateway || "",
      disks: (agent.disks || []).map((d) => ({
        deviceID: d.deviceID,
        size: d.size,
        freeSpace: d.freeSpace,
        label: d.label || d.deviceID,
        filesystem: d.filesystem || "",
      })),
      recentAlerts: [],
      recentBackups: [],
    }
  }, [agents, id])

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

  const ramUsed = device.totalRAM > 0
    ? Math.round(((device.totalRAM - device.freeRam) / device.totalRAM) * 100)
    : 0

  return (
    <ConsoleShell
      tenant={tenant}
      onTenantChange={setTenant}
      query={query}
      onQueryChange={setQuery}
      title={device.hostname}
      subtitle={`${device.vendor && device.model ? `${device.vendor} ${device.model}` : device.os} - ${device.localIP || "No IP"}`}
    >
      {/* Header Actions */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Link href="/devices" className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="size-4" />
          Back to devices
        </Link>
        <div className="flex items-center gap-2">
          <Button
            variant={showTerminal ? "default" : "outline"}
            size="sm"
            onClick={() => {
              setShowTerminal((prev) => !prev)
              if (!showTerminal) {
                toast.success(`Opening terminal session for ${device.hostname}`, {
                  description: "Connecting interactive shell connection.",
                })
              }
            }}
          >
            <TerminalSquare className="size-4" />
            {showTerminal ? "Close Terminal" : "Terminal"}
          </Button>
        </div>
      </div>

      {/* Terminal */}
      {showTerminal && (
        <div className="my-2">
          <RemoteTerminal agentId={device.id} />
        </div>
      )}

      {/* Resource Cards */}
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
        <Card className="gap-1 p-4">
          <span className="text-xs text-muted-foreground">Status</span>
          <Badge variant={device.status === "online" ? "default" : "secondary"} className="w-fit capitalize">
            {device.status}
          </Badge>
          <span className="text-xs text-muted-foreground">{device.lastSeen ? `Last seen ${device.lastSeen}` : "Unknown"}</span>
        </Card>
        <Card className="gap-1 p-4">
          <span className="text-xs text-muted-foreground">CPU</span>
          <span className="text-2xl font-semibold tabular-nums">{device.cpuLoad}%</span>
          <span className="text-xs text-muted-foreground truncate">{device.cpuModel || "Utilization"}</span>
          <Cpu className="mt-2 size-4 text-muted-foreground" />
        </Card>
        <Card className="gap-1 p-4">
          <span className="text-xs text-muted-foreground">RAM</span>
          <span className="text-2xl font-semibold tabular-nums">{ramUsed}%</span>
          <span className="text-xs text-muted-foreground">
            {device.totalRAM > 0
              ? `${((device.totalRAM - device.freeRam) / 1073741824).toFixed(1)} / ${(device.totalRAM / 1073741824).toFixed(1)} GB`
              : "Memory pressure"}
          </span>
          <MemoryStick className="mt-2 size-4 text-muted-foreground" />
        </Card>
        <Card className="gap-1 p-4">
          <span className="text-xs text-muted-foreground">Uptime</span>
          <span className="text-2xl font-semibold tabular-nums">{device.uptime || "N/A"}</span>
          <span className="text-xs text-muted-foreground">{device.numCPU ? `${device.numCPU} CPU cores` : "Since last boot"}</span>
          <Clock className="mt-2 size-4 text-muted-foreground" />
        </Card>
      </div>

      {/* Tab Container */}
      <Tabs defaultValue="summary" value={activeTab} onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="summary">
            <Server className="mr-1.5 size-3.5" />
            Summary
          </TabsTrigger>
          <TabsTrigger value="software">
            <Package className="mr-1.5 size-3.5" />
            Software
          </TabsTrigger>
          <TabsTrigger value="patches">
            <Shield className="mr-1.5 size-3.5" />
            Patches
          </TabsTrigger>
          <TabsTrigger value="checks">
            <Activity className="mr-1.5 size-3.5" />
            Checks
          </TabsTrigger>
          <TabsTrigger value="notes">
            <StickyNote className="mr-1.5 size-3.5" />
            Notes
          </TabsTrigger>
          <TabsTrigger value="assets">
            <Boxes className="mr-1.5 size-3.5" />
            Assets
          </TabsTrigger>
          <TabsTrigger value="debug">
            <Bug className="mr-1.5 size-3.5" />
            Debug
          </TabsTrigger>
          <TabsTrigger value="audit">
            <Layers className="mr-1.5 size-3.5" />
            Audit
          </TabsTrigger>
        </TabsList>

        <TabsContent value="summary">
          <SummaryTab device={device} recentAlerts={recentAlerts} backupsNotImplemented />
        </TabsContent>
        <TabsContent value="software">
          <SoftwareTab agentId={device.id} />
        </TabsContent>
        <TabsContent value="patches">
          <PatchesTab agentId={device.id} />
        </TabsContent>
        <TabsContent value="checks">
          <ChecksTab agentId={device.id} />
        </TabsContent>
        <TabsContent value="notes">
          <NotesTab agentId={device.id} />
        </TabsContent>
        <TabsContent value="assets">
          <AssetsTab agent={device} />
        </TabsContent>
        <TabsContent value="debug">
          <DebugTab agentId={device.id} />
        </TabsContent>
        <TabsContent value="audit">
          <AuditTab agentId={device.id} />
        </TabsContent>
      </Tabs>
    </ConsoleShell>
  )
}
