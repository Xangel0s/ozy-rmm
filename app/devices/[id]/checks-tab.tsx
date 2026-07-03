"use client"

import * as React from "react"
import { Activity, Plus, Trash2, Play, RefreshCw } from "lucide-react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { fetchChecks, createCheck, deleteCheck, runCheck, type CheckResult } from "@/lib/api"

interface ChecksTabProps {
  agentId: string;
}

const statusColors: Record<string, string> = {
  pass: "bg-green-500/10 text-green-500",
  fail: "bg-red-500/10 text-red-500",
  error: "bg-yellow-500/10 text-yellow-500",
  pending: "bg-gray-500/10 text-gray-500",
}

export function ChecksTab({ agentId }: ChecksTabProps) {
  const [checks, setChecks] = React.useState<CheckResult[]>([])
  const [loading, setLoading] = React.useState(true)
  const [showCreate, setShowCreate] = React.useState(false)
  const [newType, setNewType] = React.useState("ping")
  const [newDesc, setNewDesc] = React.useState("")
  const [newHost, setNewHost] = React.useState("8.8.8.8")

  const loadChecks = React.useCallback(async () => {
    setLoading(true)
    try {
      const items = await fetchChecks(agentId)
      setChecks(items)
    } finally {
      setLoading(false)
    }
  }, [agentId])

  React.useEffect(() => {
    loadChecks()
  }, [loadChecks])

  const handleCreate = async () => {
    const config = newType === "ping" ? { host: newHost } : { threshold: 80 }
    try {
      await createCheck(agentId, newType, newDesc || `${newType} check`, config)
      toast.success("Check created")
      setShowCreate(false)
      setNewDesc("")
      loadChecks()
    } catch {
      toast.error("Failed to create check")
    }
  }

  const handleDelete = async (checkId: number) => {
    try {
      await deleteCheck(checkId)
      toast.success("Check deleted")
      loadChecks()
    } catch {
      toast.error("Failed to delete check")
    }
  }

  const handleRun = async (checkId: number) => {
    try {
      await runCheck(checkId)
      toast.success("Check running")
      setTimeout(loadChecks, 3000)
    } catch {
      toast.error("Failed to run check")
    }
  }

  return (
    <Card className="gap-4 p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Activity className="size-4 text-primary" />
          <h2 className="text-sm font-semibold">Health Checks</h2>
          <Badge variant="secondary">{checks.length} checks</Badge>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={loadChecks}>
            <RefreshCw className="size-4" />
          </Button>
          <Button size="sm" onClick={() => setShowCreate(!showCreate)}>
            <Plus className="size-4 mr-1" />
            Add Check
          </Button>
        </div>
      </div>

      {showCreate && (
        <div className="rounded-lg border border-border p-3 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-muted-foreground">Type</label>
              <select
                value={newType}
                onChange={(e) => setNewType(e.target.value)}
                className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
              >
                <option value="ping">Ping</option>
                <option value="disk_space">Disk Space</option>
                <option value="cpu_load">CPU Load</option>
              </select>
            </div>
            <div>
              <label className="text-xs text-muted-foreground">Description</label>
              <Input
                value={newDesc}
                onChange={(e) => setNewDesc(e.target.value)}
                placeholder="Check description"
              />
            </div>
          </div>
          {newType === "ping" && (
            <div>
              <label className="text-xs text-muted-foreground">Host</label>
              <Input
                value={newHost}
                onChange={(e) => setNewHost(e.target.value)}
                placeholder="8.8.8.8"
              />
            </div>
          )}
          <div className="flex gap-2">
            <Button size="sm" onClick={handleCreate}>Create</Button>
            <Button size="sm" variant="ghost" onClick={() => setShowCreate(false)}>Cancel</Button>
          </div>
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center py-8">
          <RefreshCw className="size-6 animate-spin text-muted-foreground" />
        </div>
      ) : checks.length === 0 ? (
        <div className="py-8 text-center text-sm text-muted-foreground">
          No health checks configured. Click &apos;Add Check&apos; to create one.
        </div>
      ) : (
        <div className="space-y-2">
          {checks.map((check) => (
            <div key={check.id} className="flex items-center justify-between rounded-lg border border-border p-3">
              <div className="flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm">{check.description || check.checkType}</span>
                  <Badge variant="outline" className="text-[10px]">{check.checkType}</Badge>
                  <span className={`inline-block rounded px-1.5 py-0.5 text-[10px] font-medium uppercase ${statusColors[check.status] || statusColors.pending}`}>
                    {check.status}
                  </span>
                </div>
                {check.lastOutput && (
                  <p className="mt-1 text-xs text-muted-foreground font-mono truncate max-w-[500px]">
                    {check.lastOutput}
                  </p>
                )}
                {check.lastRun && (
                  <p className="mt-1 text-[10px] text-muted-foreground">
                    Last run: {new Date(check.lastRun).toLocaleString()}
                  </p>
                )}
              </div>
              <div className="flex items-center gap-1 ml-4">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-8 w-8 p-0"
                  onClick={() => handleRun(check.id)}
                >
                  <Play className="size-3" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-8 w-8 p-0 text-destructive"
                  onClick={() => handleDelete(check.id)}
                >
                  <Trash2 className="size-3" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}
