"use client"

import * as React from "react"
import { Bug, RefreshCw, Filter } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { fetchLogs, type LogEntry } from "@/lib/api"

interface DebugTabProps {
  agentId: string;
}

const levelColors: Record<string, string> = {
  info: "bg-blue-500/10 text-blue-500",
  warning: "bg-yellow-500/10 text-yellow-500",
  error: "bg-red-500/10 text-red-500",
  debug: "bg-gray-500/10 text-gray-500",
}

export function DebugTab({ agentId }: DebugTabProps) {
  const [logs, setLogs] = React.useState<LogEntry[]>([])
  const [loading, setLoading] = React.useState(true)
  const [level, setLevel] = React.useState<string>("")
  const [cursor, setCursor] = React.useState("")
  const [hasMore, setHasMore] = React.useState(false)

  const loadLogs = React.useCallback(async (reset = false) => {
    setLoading(true)
    try {
      const res = await fetchLogs(agentId, level || undefined, reset ? undefined : cursor)
      if (reset) {
        setLogs(res.items)
      } else {
        setLogs((prev) => [...prev, ...res.items])
      }
      setCursor(res.cursor)
      setHasMore(res.hasMore)
    } finally {
      setLoading(false)
    }
  }, [agentId, level, cursor])

  React.useEffect(() => {
    loadLogs(true)
  }, [level])

  const handleRefresh = () => {
    setCursor("")
    loadLogs(true)
  }

  const handleLoadMore = () => {
    loadLogs(false)
  }

  return (
    <Card className="gap-4 p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Bug className="size-4 text-primary" />
          <h2 className="text-sm font-semibold">Agent Logs</h2>
          <Badge variant="secondary">{logs.length} entries</Badge>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex items-center gap-1">
            <Filter className="size-4 text-muted-foreground" />
            <select
              value={level}
              onChange={(e) => setLevel(e.target.value)}
              className="rounded-md border border-border bg-background px-2 py-1 text-xs"
            >
              <option value="">All levels</option>
              <option value="info">Info</option>
              <option value="warning">Warning</option>
              <option value="error">Error</option>
              <option value="debug">Debug</option>
            </select>
          </div>
          <Button variant="outline" size="sm" onClick={handleRefresh}>
            <RefreshCw className="size-4" />
          </Button>
        </div>
      </div>

      {loading && logs.length === 0 ? (
        <div className="flex items-center justify-center py-8">
          <RefreshCw className="size-6 animate-spin text-muted-foreground" />
        </div>
      ) : logs.length === 0 ? (
        <div className="py-8 text-center text-sm text-muted-foreground">
          No logs found. The agent will send logs automatically during operation.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs font-medium text-muted-foreground">
                <th className="pb-2 pr-4 w-20">Level</th>
                <th className="pb-2 pr-4 w-24">Type</th>
                <th className="pb-2 pr-4">Message</th>
                <th className="pb-2 w-36">Time</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log) => (
                <tr key={log.id} className="border-b border-border/50">
                  <td className="py-2 pr-4">
                    <span className={`inline-block rounded px-1.5 py-0.5 text-[10px] font-medium uppercase ${levelColors[log.level] || "bg-gray-500/10 text-gray-500"}`}>
                      {log.level}
                    </span>
                  </td>
                  <td className="py-2 pr-4 text-muted-foreground">{log.logType}</td>
                  <td className="py-2 pr-4 font-mono text-xs max-w-[400px] truncate" title={log.message}>
                    {log.message}
                  </td>
                  <td className="py-2 text-muted-foreground text-xs">
                    {new Date(log.createdAt).toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {hasMore && (
        <div className="flex justify-center pt-2">
          <Button variant="outline" size="sm" onClick={handleLoadMore} disabled={loading}>
            {loading ? "Loading..." : "Load More"}
          </Button>
        </div>
      )}
    </Card>
  )
}
