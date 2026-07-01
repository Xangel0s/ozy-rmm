"use client"

import * as React from "react"
import { PlayCircle, TerminalSquare } from "lucide-react"
import { toast } from "sonner"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { DeviceTable } from "@/components/rmm/device-table"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { devices as mockDevices, scripts, tenants } from "@/lib/rmm-data"
import { useAgents, agentToDevice } from "@/lib/use-live-data"
import { cn } from "@/lib/utils"

export default function AutomationPage() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [selected, setSelected] = React.useState<Set<string>>(new Set())
  const [activeScript, setActiveScript] = React.useState(scripts[0])

  // Live data — fall back to mocks when backend is offline
  const { agents } = useAgents(5000)
  const liveDevices = React.useMemo(
    () => (agents.length > 0 ? agents.map(agentToDevice) : []),
    [agents]
  )
  const allDevices = liveDevices.length > 0 ? liveDevices : mockDevices

  const tenantName = tenants.find((t) => t.id === tenant)?.name

  const filtered = React.useMemo(() => {
    const q = query.trim().toLowerCase()
    return allDevices.filter((d) => {
      const matchTenant = tenant === "all" || d.tenant === tenantName
      const matchQuery =
        q === "" ||
        d.name.toLowerCase().includes(q) ||
        d.ip.toLowerCase().includes(q) ||
        d.tenant.toLowerCase().includes(q)
      return matchTenant && matchQuery
    })
  }, [tenant, tenantName, query, allDevices])

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const toggleAll = (ids: string[], checked: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev)
      ids.forEach((id) => (checked ? next.add(id) : next.delete(id)))
      return next
    })

  const queueRun = () => {
    if (selected.size === 0) {
      toast.error("Select at least one endpoint", {
        description: "Pick one or more devices before queueing a script.",
      })
      return
    }

    toast.success(`${activeScript} queued`, {
      description: `Execution started on ${selected.size} selected endpoint${selected.size === 1 ? "" : "s"}.`,
    })
  }

  return (
    <ConsoleShell
      tenant={tenant}
      onTenantChange={setTenant}
      query={query}
      onQueryChange={setQuery}
      selectedCount={selected.size}
      onRunScript={queueRun}
      title="Automation / Scripts"
      subtitle={activeScript}
    >
      <Card className="gap-3 p-4">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold">Script Library</h2>
            <p className="text-xs text-muted-foreground">
              Choose a script profile and queue execution against selected endpoints.
            </p>
          </div>
          <Badge variant="secondary">{scripts.length} scripts</Badge>
        </div>

        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-3">
          {scripts.map((script) => (
            <button
              key={script}
              type="button"
              onClick={() => setActiveScript(script)}
              className={cn(
                "flex items-center justify-between rounded-lg border px-3 py-2 text-left text-sm transition-colors",
                activeScript === script
                  ? "border-primary/50 bg-primary/10 text-foreground"
                  : "border-border bg-muted/20 text-muted-foreground hover:bg-muted/50 hover:text-foreground",
              )}
            >
              <span className="truncate">{script}</span>
              <TerminalSquare className="size-3.5 shrink-0" />
            </button>
          ))}
        </div>

        <div className="flex items-center justify-end">
          <Button size="sm" onClick={queueRun}>
            <PlayCircle data-icon="inline-start" />
            Queue {activeScript}
          </Button>
        </div>
      </Card>

      <DeviceTable
        devices={filtered}
        selected={selected}
        onToggle={toggle}
        onToggleAll={toggleAll}
      />
    </ConsoleShell>
  )
}
