"use client"

import * as React from "react"
import { toast } from "sonner"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { KpiCards } from "@/components/rmm/kpi-cards"
import { DeviceTable } from "@/components/rmm/device-table"
import { AlertsPanel } from "@/components/rmm/alerts-panel"
import { useAgents, useTenants, agentToDevice } from "@/lib/use-live-data"

export default function Page() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [selected, setSelected] = React.useState<Set<string>>(new Set())

  // Live data from the Go backend
  const { agents, loading: agentsLoading } = useAgents()
  const { tenants: liveTenants } = useTenants()

  const allDevices = React.useMemo(
    () => agents.map(agentToDevice),
    [agents]
  )

  const tenantName = liveTenants.find((t) => t.id === tenant)?.name

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
      description: "Collect Diagnostics — execution started in the background.",
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
      title={
        !agentsLoading && allDevices.length > 0
          ? `Fleet Dashboard · ${allDevices.length} live agent${allDevices.length > 1 ? "s" : ""}`
          : "Fleet Dashboard"
      }
    >
      <KpiCards devices={filtered} />

      <div className="grid grid-cols-1 gap-4 xl:grid-cols-[1fr_360px]">
        <DeviceTable
          devices={filtered}
          selected={selected}
          onToggle={toggle}
          onToggleAll={toggleAll}
        />
        <div className="xl:h-[calc(100svh-13rem)]">
          <AlertsPanel />
        </div>
      </div>
    </ConsoleShell>
  )
}
