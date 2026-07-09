"use client"

import * as React from "react"
import { FileText, PlayCircle, Plus, TerminalSquare, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { DeviceTable } from "@/components/rmm/device-table"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { useAgents, useTenants, agentToDevice } from "@/lib/use-live-data"
import {
  fetchScripts,
  createScript,
  updateScript,
  deleteScript,
  runScript,
  fetchScriptExecutions,
  type ScriptInfo,
  type ScriptExecution,
} from "@/lib/api"
import { cn } from "@/lib/utils"

export default function AutomationPage() {
  const [tab, setTab] = React.useState("library")
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")
  const [selected, setSelected] = React.useState<Set<string>>(new Set())
  const [scripts, setScripts] = React.useState<ScriptInfo[]>([])
  const [loading, setLoading] = React.useState(true)
  const [selectedScript, setSelectedScript] = React.useState<ScriptInfo | null>(null)
  const [showCreateDialog, setShowCreateDialog] = React.useState(false)
  const [showEditDialog, setShowEditDialog] = React.useState(false)
  const [showDeleteDialog, setShowDeleteDialog] = React.useState(false)
  const [executions, setExecutions] = React.useState<ScriptExecution[]>([])
  const [showOutputId, setShowOutputId] = React.useState<number | null>(null)
  const role = typeof window !== "undefined" ? localStorage.getItem("role") : null
  const isAdmin = role === "admin"

  const { agents } = useAgents()
  const { tenants: liveTenants } = useTenants()
  const allDevices = React.useMemo(() => agents.map(agentToDevice), [agents])

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

  const loadScripts = React.useCallback(async () => {
    setLoading(true)
    const data = await fetchScripts()
    setScripts(data)
    setLoading(false)
  }, [])

  const loadExecutions = React.useCallback(async () => {
    const data = await fetchScriptExecutions({ limit: 100 })
    setExecutions(data)
  }, [])

  React.useEffect(() => {
    loadScripts()
    loadExecutions()
  }, [loadScripts, loadExecutions])

  const handleCreate = async (data: { name: string; description: string; command: string; language: string }) => {
    try {
      await createScript(data)
      toast.success("Script created")
      setShowCreateDialog(false)
      loadScripts()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to create script"
      toast.error(msg)
    }
  }

  const handleUpdate = async (id: number, data: { name: string; description: string; command: string; language: string }) => {
    try {
      await updateScript(id, data)
      toast.success("Script updated")
      setShowEditDialog(false)
      setSelectedScript(null)
      loadScripts()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to update script"
      toast.error(msg)
    }
  }

  const handleDelete = async () => {
    if (!selectedScript) return
    try {
      await deleteScript(selectedScript.id)
      toast.success("Script deleted")
      setShowDeleteDialog(false)
      setSelectedScript(null)
      loadScripts()
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to delete script"
      toast.error(msg)
    }
  }

  const handleRun = async () => {
    if (!selectedScript) {
      toast.error("Select a script first", { description: "Pick a script from the library." })
      return
    }
    if (selected.size === 0) {
      toast.error("Select at least one agent", {
        description: "Pick one or more devices before running a script.",
      })
      return
    }
    let success = 0
    let fail = 0
    for (const agentId of selected) {
      try {
        await runScript(selectedScript.id, agentId)
        success++
      } catch {
        fail++
      }
    }
    if (success > 0) {
      toast.success(`${selectedScript.name} queued`, {
        description: `Started on ${success} agent(s)${fail > 0 ? `, ${fail} failed` : ""}.`,
      })
    }
    if (fail > 0) {
      toast.error(`${fail} execution(s) failed`)
    }
    loadExecutions()
  }

  const statusColor = (status: string) => {
    switch (status) {
      case "completed": return "text-green-500"
      case "running": return "text-blue-500"
      case "failed": return "text-red-500"
      case "timeout": return "text-yellow-500"
      default: return "text-muted-foreground"
    }
  }

  return (
    <>
      <ConsoleShell
        tenant={tenant}
        onTenantChange={setTenant}
        query={query}
        onQueryChange={setQuery}
        selectedCount={selected.size}
        onRunScript={handleRun}
        title="Automation / Scripts"
        subtitle={selectedScript?.name ?? ""}
      >
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="library">Script Library</TabsTrigger>
            <TabsTrigger value="history">Execution History</TabsTrigger>
          </TabsList>

          <TabsContent value="library" className="space-y-4 pt-4">
            <div className="flex items-center justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold">Script Library</h2>
                <p className="text-xs text-muted-foreground">
                  Select a script and queue execution against agents below.
                </p>
              </div>
              <Badge variant="secondary">{scripts.length} scripts</Badge>
              {isAdmin && (
                <Button size="sm" onClick={() => setShowCreateDialog(true)}>
                  <Plus data-icon="inline-start" />
                  New Script
                </Button>
              )}
            </div>

            {loading ? (
              <div className="space-y-2">
                {[1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-16 w-full" />
                ))}
              </div>
            ) : scripts.length === 0 ? (
              <Card className="p-8 text-center text-muted-foreground">
                No scripts yet. {isAdmin ? "Create one to get started." : "Ask an admin to create scripts."}
              </Card>
            ) : (
              <div className="space-y-2">
                {scripts.map((s) => (
                  <div
                    key={s.id}
                    className={cn(
                      "flex items-center justify-between rounded-lg border px-4 py-3 text-sm transition-colors cursor-pointer",
                      selectedScript?.id === s.id
                        ? "border-primary/50 bg-primary/10"
                        : "border-border hover:bg-muted/50",
                    )}
                    onClick={() => setSelectedScript(s)}
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <FileText className="size-4 shrink-0 text-muted-foreground" />
                      <span className="font-medium truncate">{s.name}</span>
                      <Badge variant="outline" className="text-[10px]">{s.language}</Badge>
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      {s.description && (
                        <span className="text-xs text-muted-foreground hidden md:block max-w-[200px] truncate">
                          {s.description}
                        </span>
                      )}
                      {isAdmin && (
                        <>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-7"
                            onClick={(e) => { e.stopPropagation(); setSelectedScript(s); setShowEditDialog(true) }}
                          >
                            <TerminalSquare className="size-3.5" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="size-7 text-destructive hover:text-destructive"
                            onClick={(e) => { e.stopPropagation(); setSelectedScript(s); setShowDeleteDialog(true) }}
                          >
                            <Trash2 className="size-3.5" />
                          </Button>
                        </>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}

            <div className="flex items-center justify-end gap-2 pt-2">
              <Button
                size="sm"
                onClick={handleRun}
                disabled={!selectedScript || selected.size === 0}
              >
                <PlayCircle data-icon="inline-start" />
                Run {selectedScript?.name ?? "Script"}
              </Button>
            </div>

            <DeviceTable
              devices={filtered}
              selected={selected}
              onToggle={toggle}
              onToggleAll={toggleAll}
            />
          </TabsContent>

          <TabsContent value="history" className="space-y-4 pt-4">
            <h2 className="text-sm font-semibold">Execution History</h2>
            {executions.length === 0 ? (
              <Card className="p-8 text-center text-muted-foreground">
                No executions yet. Run a script to see results here.
              </Card>
            ) : (
              <div className="rounded-lg border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Script</TableHead>
                      <TableHead>Agent</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Exit Code</TableHead>
                      <TableHead>Duration</TableHead>
                      <TableHead>Started</TableHead>
                      <TableHead>Output</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {executions.map((e) => (
                      <TableRow key={e.id}>
                        <TableCell className="font-medium">{e.scriptName}</TableCell>
                        <TableCell className="text-muted-foreground">{e.agentHostname || e.agentId.slice(0, 8)}</TableCell>
                        <TableCell>
                          <span className={cn("font-medium", statusColor(e.status))}>{e.status}</span>
                        </TableCell>
                        <TableCell>{e.exitCode ?? "-"}</TableCell>
                        <TableCell>{e.durationMs ? `${(e.durationMs / 1000).toFixed(1)}s` : "-"}</TableCell>
                        <TableCell className="text-xs text-muted-foreground">
                          {e.startedAt ? new Date(e.startedAt).toLocaleString() : "-"}
                        </TableCell>
                        <TableCell>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setShowOutputId(showOutputId === e.id ? null : e.id)}
                          >
                            {showOutputId === e.id ? "Hide" : "View"}
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}

            {showOutputId && (
              <Card className="p-4">
                <pre className="max-h-96 overflow-auto rounded bg-muted p-3 text-xs font-mono whitespace-pre-wrap">
                  {executions.find((e) => e.id === showOutputId)?.output || "(no output)"}
                </pre>
              </Card>
            )}
          </TabsContent>
        </Tabs>
      </ConsoleShell>

      {/* Create Dialog */}
      <ScriptDialog
        open={showCreateDialog}
        onOpenChange={setShowCreateDialog}
        title="Create Script"
        onSubmit={handleCreate}
      />

      {/* Edit Dialog */}
      {selectedScript && (
        <ScriptDialog
          open={showEditDialog}
          onOpenChange={(v) => { setShowEditDialog(v); if (!v) setSelectedScript(null) }}
          title="Edit Script"
          initial={selectedScript}
          onSubmit={(data) => handleUpdate(selectedScript.id, data)}
        />
      )}

      {/* Delete Confirmation */}
      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Script</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{selectedScript?.name}&quot;? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDeleteDialog(false)}>Cancel</Button>
            <Button variant="destructive" onClick={handleDelete}>Delete</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function ScriptDialog({
  open,
  onOpenChange,
  title,
  initial,
  onSubmit,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  title: string
  initial?: ScriptInfo
  onSubmit: (data: { name: string; description: string; command: string; language: string }) => void
}) {
  const [name, setName] = React.useState(initial?.name ?? "")
  const [description, setDescription] = React.useState(initial?.description ?? "")
  const [command, setCommand] = React.useState(initial?.command ?? "")
  const [language, setLanguage] = React.useState(initial?.language ?? "powershell")
  const [submitting, setSubmitting] = React.useState(false)

  React.useEffect(() => {
    if (open) {
      setName(initial?.name ?? "")
      setDescription(initial?.description ?? "")
      setCommand(initial?.command ?? "")
      setLanguage(initial?.language ?? "powershell")
    }
  }, [open, initial])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !command.trim()) return
    setSubmitting(true)
    try {
      await onSubmit({ name: name.trim(), description: description.trim(), command: command.trim(), language })
      onOpenChange(false)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>
              Scripts are curated by administrators and run with full system privileges on the target agent.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="name">Name</Label>
              <Input id="name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Restart Service" required />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="description">Description</Label>
              <Input id="description" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Restarts the Windows Update service" />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="language">Language</Label>
              <Select value={language} onValueChange={setLanguage}>
                <SelectTrigger id="language">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="powershell">PowerShell</SelectItem>
                  <SelectItem value="batch">Batch (CMD)</SelectItem>
                  <SelectItem value="sh">Shell (Unix)</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="command">Command</Label>
              <Textarea
                id="command"
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                placeholder="Restart-Service -Name wuauserv -Force"
                className="font-mono text-sm"
                rows={6}
                required
              />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
              Cancel
            </Button>
            <Button type="submit" disabled={submitting || !name.trim() || !command.trim()}>
              {submitting ? "Saving..." : "Save"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
