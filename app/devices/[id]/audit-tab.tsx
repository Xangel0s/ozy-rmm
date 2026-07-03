"use client"

import * as React from "react"
import { Layers, RefreshCw } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { fetchAudit, type AuditEntry } from "@/lib/api"

interface AuditTabProps {
  agentId: string;
}

const actionColors: Record<string, string> = {
  create: "bg-green-500/10 text-green-500",
  update: "bg-blue-500/10 text-blue-500",
  delete: "bg-red-500/10 text-red-500",
  login: "bg-purple-500/10 text-purple-500",
}

export function AuditTab({ agentId }: AuditTabProps) {
  const [entries, setEntries] = React.useState<AuditEntry[]>([])
  const [loading, setLoading] = React.useState(true)
  const [total, setTotal] = React.useState(0)
  const [page, setPage] = React.useState(0)
  const [hasMore, setHasMore] = React.useState(false)
  const limit = 50

  const loadAudit = React.useCallback(async (offset: number) => {
    setLoading(true)
    try {
      const res = await fetchAudit(agentId, limit, offset)
      setEntries(res.items)
      setTotal(res.total)
      setHasMore(res.hasMore)
    } finally {
      setLoading(false)
    }
  }, [agentId])

  React.useEffect(() => {
    loadAudit(0)
  }, [loadAudit])

  return (
    <Card className="gap-4 p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Layers className="size-4 text-primary" />
          <h2 className="text-sm font-semibold">Audit Trail</h2>
          <Badge variant="secondary">{total} entries</Badge>
        </div>
        <Button variant="outline" size="sm" onClick={() => loadAudit(0)}>
          <RefreshCw className="size-4" />
        </Button>
      </div>

      {loading && entries.length === 0 ? (
        <div className="flex items-center justify-center py-8">
          <RefreshCw className="size-6 animate-spin text-muted-foreground" />
        </div>
      ) : entries.length === 0 ? (
        <div className="py-8 text-center text-sm text-muted-foreground">
          No audit entries found for this device.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs font-medium text-muted-foreground">
                <th className="pb-2 pr-4 w-24">Action</th>
                <th className="pb-2 pr-4 w-28">Resource</th>
                <th className="pb-2 pr-4">Details</th>
                <th className="pb-2 pr-4 w-28">IP</th>
                <th className="pb-2 w-36">Time</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((entry) => (
                <tr key={entry.id} className="border-b border-border/50">
                  <td className="py-2 pr-4">
                    <span className={`inline-block rounded px-1.5 py-0.5 text-[10px] font-medium uppercase ${actionColors[entry.action] || "bg-gray-500/10 text-gray-500"}`}>
                      {entry.action}
                    </span>
                  </td>
                  <td className="py-2 pr-4 text-muted-foreground">{entry.resourceType}</td>
                  <td className="py-2 pr-4 font-mono text-xs max-w-[300px] truncate">
                    {entry.details && Object.keys(entry.details).length > 0
                      ? JSON.stringify(entry.details)
                      : "—"
                    }
                  </td>
                  <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">
                    {entry.ipAddress || "—"}
                  </td>
                  <td className="py-2 text-muted-foreground text-xs">
                    {new Date(entry.createdAt).toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {hasMore && (
        <div className="flex justify-center gap-2 pt-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              const newPage = page - 1
              setPage(newPage)
              loadAudit(newPage * limit)
            }}
            disabled={page === 0}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              const newPage = page + 1
              setPage(newPage)
              loadAudit(newPage * limit)
            }}
          >
            Next
          </Button>
        </div>
      )}
    </Card>
  )
}
