"use client"

import * as React from "react"
import { toast } from "sonner"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { DeviceTable } from "@/components/rmm/device-table"
import { KpiCards } from "@/components/rmm/kpi-cards"
import { Card } from "@/components/ui/card"
import { devices as mockDevices, tenants } from "@/lib/rmm-data"
import { useAgents, agentToDevice } from "@/lib/use-live-data"

export default function DevicesPage() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [selected, setSelected] = React.useState<Set<string>>(new Set())

  // Live data from the Go backend — falls back to mock devices when backend is offline
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

  const runScript = () => {
    const count = selected.size
    toast.success(`Script queued on ${count} endpoint${count === 1 ? "" : "s"}`, {
      description: "Collect Diagnostics - execution started in the background.",
    })
  }

  return (
    <ConsoleShell
      tenant={tenant}
      onTenantChange={setTenant}
      query={query}
      onQueryChange={setQuery}
      selectedCount={selected.size}
      onRunScript={runScript}
      title="Monitored Devices"
      subtitle={`${filtered.length} visible endpoint${filtered.length === 1 ? "" : "s"}`}
    >
      <KpiCards devices={filtered} />

      <Card className="gap-0 p-0">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold">Endpoint Inventory</h2>
          <p className="text-xs text-muted-foreground">
            Full device list with sorting, status visibility, and bulk actions.
          </p>
        </div>
        <DeviceTable
          devices={filtered}
          selected={selected}
          onToggle={toggle}
          onToggleAll={toggleAll}
        />
      </Card>
    </ConsoleShell>
  )
}
