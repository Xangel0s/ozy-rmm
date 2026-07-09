"use client"

import * as React from "react"
import { useParams, useSearchParams } from "next/navigation"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { fetchAgent, type AgentInfo } from "@/lib/api"
import { SummaryTab } from "./summary-tab"
import { SoftwareTab } from "./software-tab"
import { PatchesTab } from "./patches-tab"
import { ChecksTab } from "./checks-tab"
import { NotesTab } from "./notes-tab"
import { AssetsTab } from "./assets-tab"
import { AuditTab } from "./audit-tab"
import { DebugTab } from "./debug-tab"
import { RemoteTerminal } from "@/components/rmm/remote-terminal"
import { RemoteScreen } from "@/components/rmm/remote-screen"
import { AgentBackupsTab } from "@/components/rmm/agent-backups-tab"

const tabs = [
  "summary", "software", "patches", "checks", "notes",
  "assets", "audit", "debug", "backups", "terminal", "screen",
] as const

export default function DeviceDetailPage() {
  const params = useParams<{ id: string }>()
  const searchParams = useSearchParams()
  const initialTab = tabs.includes((searchParams.get("tab") || "summary") as typeof tabs[number])
    ? (searchParams.get("tab") || "summary") as typeof tabs[number]
    : "summary"
  const [activeTab, setActiveTab] = React.useState<string>(initialTab)
  const [agent, setAgent] = React.useState<AgentInfo | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")

  React.useEffect(() => {
    if (!params?.id) return
    setLoading(true)
    fetchAgent(params.id).then((a) => {
      setAgent(a)
      setLoading(false)
    })
  }, [params?.id])

  return (
    <ConsoleShell
      tenant={tenant}
      onTenantChange={setTenant}
      query={query}
      onQueryChange={setQuery}
      showSearch={false}
      title={agent ? (agent.hostname || agent.id.slice(0, 8)) : "Device"}
      subtitle={agent ? `${agent.os} · ${agent.status}` : ""}
    >
      {loading && (
        <div className="flex h-48 items-center justify-center text-sm text-muted-foreground">
          Loading device...
        </div>
      )}

      {!loading && !agent && (
        <div className="flex h-48 items-center justify-center text-sm text-muted-foreground">
          Device not found
        </div>
      )}

      {!loading && agent && (
        <Tabs value={activeTab} onValueChange={(v) => setActiveTab(v)}>
          <TabsList>
            {tabs.map((tab) => (
              <TabsTrigger key={tab} value={tab} className="capitalize">
                {tab}
              </TabsTrigger>
            ))}
          </TabsList>

          <TabsContent value="summary">
            <SummaryTab agent={agent} />
          </TabsContent>

          <TabsContent value="software">
            <SoftwareTab agentId={agent.id} />
          </TabsContent>

          <TabsContent value="patches">
            <PatchesTab agentId={agent.id} />
          </TabsContent>

          <TabsContent value="checks">
            <ChecksTab agentId={agent.id} />
          </TabsContent>

          <TabsContent value="notes">
            <NotesTab agentId={agent.id} />
          </TabsContent>

          <TabsContent value="assets">
            <AssetsTab agent={agent} />
          </TabsContent>

          <TabsContent value="audit">
            <AuditTab agentId={agent.id} />
          </TabsContent>

          <TabsContent value="debug">
            <DebugTab agentId={agent.id} />
          </TabsContent>

          <TabsContent value="backups">
            <AgentBackupsTab agentId={agent.id} />
          </TabsContent>

          <TabsContent value="terminal">
            <RemoteTerminal agentId={agent.id} />
          </TabsContent>

          <TabsContent value="screen">
            <RemoteScreen agentId={agent.id} />
          </TabsContent>
        </Tabs>
      )}
    </ConsoleShell>
  )
}
