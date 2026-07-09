"use client"

import * as React from "react"
import {
  fetchAgentBackupConfig, updateAgentBackupConfig,
  fetchAgentBackups, deleteBackupJob, runAgentBackup,
  listSnapshots, restoreSnapshot,
  type BackupConfig, type BackupJob, type SnapshotItem,
} from "@/lib/api"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"

interface AgentBackupsTabProps {
  agentId: string
}

export function AgentBackupsTab({ agentId }: AgentBackupsTabProps) {
  const [config, setConfig] = React.useState<BackupConfig>({
    repoUrl: "", sourcePaths: [], cron: "0 2 * * *", retentionDays: 30, enabled: true,
  })
  const [repoPassword, setRepoPassword] = React.useState("")
  const [jobs, setJobs] = React.useState<BackupJob[]>([])
  const [saving, setSaving] = React.useState(false)
  const [running, setRunning] = React.useState(false)
  const [newPath, setNewPath] = React.useState("")
  const [snapshots, setSnapshots] = React.useState<SnapshotItem[]>([])
  const [loadingSnapshots, setLoadingSnapshots] = React.useState(false)
  const [restoring, setRestoring] = React.useState<string | null>(null)
  const [restoreDest, setRestoreDest] = React.useState("")

  React.useEffect(() => {
    fetchAgentBackupConfig(agentId).then(setConfig)
    fetchAgentBackups(agentId).then(setJobs)
  }, [agentId])

  const handleSaveConfig = async () => {
    setSaving(true)
    await updateAgentBackupConfig(agentId, { ...config, repoPassword })
    setRepoPassword("")
    setSaving(false)
    const updated = await fetchAgentBackupConfig(agentId)
    setConfig(updated)
  }

  const handleRunBackup = async () => {
    setRunning(true)
    await runAgentBackup(agentId)
    setTimeout(async () => {
      const updated = await fetchAgentBackups(agentId)
      setJobs(updated)
      setRunning(false)
    }, 3000)
  }

  const handleDeleteJob = async (jobId: number) => {
    await deleteBackupJob(jobId)
    const updated = await fetchAgentBackups(agentId)
    setJobs(updated)
  }

  const handleLoadSnapshots = async () => {
    setLoadingSnapshots(true)
    const items = await listSnapshots(agentId)
    setSnapshots(items)
    setLoadingSnapshots(false)
  }

  const handleRestore = async (snapshotId: string) => {
    if (!restoreDest.trim()) return
    setRestoring(snapshotId)
    await restoreSnapshot(agentId, snapshotId, restoreDest.trim())
    setRestoreDest("")
    setRestoring(null)
  }

  const addPath = () => {
    if (newPath.trim() && !config.sourcePaths.includes(newPath.trim())) {
      setConfig({ ...config, sourcePaths: [...config.sourcePaths, newPath.trim()] })
      setNewPath("")
    }
  }

  const removePath = (path: string) => {
    setConfig({ ...config, sourcePaths: config.sourcePaths.filter((p) => p !== path) })
  }

  const statusBadge = (status: string) => {
    const colors: Record<string, string> = {
      completed: "bg-green-500/20 text-green-400",
      running: "bg-blue-500/20 text-blue-400 animate-pulse",
      failed: "bg-red-500/20 text-red-400",
      pending: "bg-yellow-500/20 text-yellow-400",
    }
    return (
      <Badge className={`${colors[status] || "bg-stone-500/20 text-stone-400"} border-0`}>
        {status}
      </Badge>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <Card className="border border-border bg-stone-950 p-4">
        <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider mb-4">
          Backup Repository Configuration
        </h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">Repository URL (Kopia)</Label>
            <Input
              value={config.repoUrl}
              onChange={(e) => setConfig({ ...config, repoUrl: e.target.value })}
              placeholder="s3://bucket/path or cifs://server/share"
              className="border-stone-700 bg-stone-900 text-sm h-9"
            />
          </div>
          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">Repository Password</Label>
            <Input
              type="password"
              value={repoPassword}
              onChange={(e) => setRepoPassword(e.target.value)}
              placeholder="Leave empty to keep current"
              className="border-stone-700 bg-stone-900 text-sm h-9"
            />
          </div>
          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">Schedule (cron)</Label>
            <Input
              value={config.cron}
              onChange={(e) => setConfig({ ...config, cron: e.target.value })}
              placeholder="0 2 * * *"
              className="border-stone-700 bg-stone-900 text-sm h-9"
            />
          </div>
          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">Retention (days)</Label>
            <Input
              type="number"
              value={config.retentionDays}
              onChange={(e) => setConfig({ ...config, retentionDays: parseInt(e.target.value) || 30 })}
              className="border-stone-700 bg-stone-900 text-sm h-9"
            />
          </div>
        </div>

        <div className="mt-4 space-y-2">
          <Label className="text-xs text-muted-foreground">Source Paths</Label>
          <div className="flex flex-wrap gap-2">
            {config.sourcePaths.map((path) => (
              <Badge
                key={path}
                className="bg-stone-800 text-stone-300 border border-stone-700 cursor-pointer hover:bg-red-900/30"
                onClick={() => removePath(path)}
              >
                {path} &times;
              </Badge>
            ))}
          </div>
          <div className="flex gap-2">
            <Input
              value={newPath}
              onChange={(e) => setNewPath(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && addPath()}
              placeholder="Add path (e.g. C:\Data)"
              className="border-stone-700 bg-stone-900 text-sm h-9 flex-1"
            />
            <Button variant="outline" size="sm" onClick={addPath} className="h-9">
              Add
            </Button>
          </div>
        </div>

        <div className="mt-4 flex items-center gap-2">
          <label className="flex items-center gap-2 text-sm text-muted-foreground cursor-pointer">
            <input
              type="checkbox"
              checked={config.enabled}
              onChange={(e) => setConfig({ ...config, enabled: e.target.checked })}
              className="accent-stone-500"
            />
            Scheduled backups enabled
          </label>
        </div>

        <div className="mt-4 flex gap-2">
          <Button onClick={handleSaveConfig} disabled={saving} size="sm">
            {saving ? "Saving..." : "Save Config"}
          </Button>
          <Button onClick={handleRunBackup} disabled={running} size="sm" variant="secondary">
            {running ? "Running..." : "Run Backup Now"}
          </Button>
        </div>
      </Card>

      <Card className="border border-border bg-stone-950 p-4">
        <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider mb-4">
          Backup History
        </h3>
        {jobs.length === 0 ? (
          <p className="text-sm text-muted-foreground">No backup jobs yet.</p>
        ) : (
          <div className="space-y-2">
            {jobs.map((job) => (
              <div key={job.id} className="flex items-center justify-between rounded-md border border-stone-800 bg-stone-900/50 px-3 py-2">
                <div className="flex items-center gap-3 min-w-0">
                  <span className="text-sm font-medium text-stone-200 truncate max-w-[200px]">
                    {job.name}
                  </span>
                  {statusBadge(job.status)}
                  <span className="text-xs text-muted-foreground truncate max-w-[180px]">
                    {job.location}
                  </span>
                  {job.sizeBytes > 0 && (
                    <span className="text-xs text-muted-foreground whitespace-nowrap">
                      {(job.sizeBytes / 1024 / 1024).toFixed(1)} MB
                    </span>
                  )}
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <span className="text-[11px] text-muted-foreground">
                    {job.executedAt ? new Date(job.executedAt).toLocaleString() : "—"}
                  </span>
                  {job.status !== "running" && (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 text-xs text-red-400 hover:text-red-300"
                      onClick={() => handleDeleteJob(job.id)}
                    >
                      Delete
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card className="border border-border bg-stone-950 p-4">
        <div className="flex items-center justify-between mb-4">
          <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
            Snapshots
          </h3>
          <Button onClick={handleLoadSnapshots} disabled={loadingSnapshots} size="sm" variant="outline" className="h-8">
            {loadingSnapshots ? "Loading..." : "Refresh"}
          </Button>
        </div>
        {snapshots.length === 0 && !loadingSnapshots && (
          <p className="text-sm text-muted-foreground">
            No snapshots loaded. Click "Refresh" to fetch from the agent.
          </p>
        )}
        {loadingSnapshots && (
          <p className="text-sm text-muted-foreground animate-pulse">Fetching snapshots from agent...</p>
        )}
        {snapshots.length > 0 && (
          <div className="space-y-2">
            {snapshots.slice(0, 20).map((snap) => (
              <div key={snap.id} className="rounded-md border border-stone-800 bg-stone-900/50 px-3 py-2">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="text-xs font-mono text-stone-300 truncate max-w-[160px]">
                      {snap.id?.slice(0, 16)}...
                    </span>
                    <span className="text-xs text-muted-foreground truncate max-w-[200px]">
                      {snap.source || "—"}
                    </span>
                    {snap.endTime && (
                      <span className="text-[11px] text-muted-foreground whitespace-nowrap">
                        {new Date(snap.endTime || snap.startTime).toLocaleDateString()}
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-2 shrink-0">
                    {restoring === snap.id ? (
                      <span className="text-xs text-blue-400 animate-pulse">Restoring...</span>
                    ) : (
                      <div className="flex items-center gap-1">
                        <Input
                          value={restoreDest}
                          onChange={(e) => setRestoreDest(e.target.value)}
                          placeholder="Dest path e.g. C:\Restore"
                          className="w-44 border-stone-700 bg-stone-900 text-xs h-7"
                        />
                        <Button
                          variant="secondary"
                          size="sm"
                          className="h-7 text-xs"
                          onClick={() => handleRestore(snap.id!)}
                          disabled={!restoreDest.trim()}
                        >
                          Restore
                        </Button>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            ))}
            {snapshots.length > 20 && (
              <p className="text-xs text-muted-foreground">
                Showing 20 of {snapshots.length} snapshots
              </p>
            )}
          </div>
        )}
      </Card>
    </div>
  )
}