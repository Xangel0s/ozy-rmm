"use client"

import * as React from "react"
import { fetchAudit, type AuditEntry } from "@/lib/api"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

interface AuditTabProps {
  agentId: string
}

export function AuditTab({ agentId }: AuditTabProps) {
  const [items, setItems] = React.useState<AuditEntry[]>([])
  const [page, setPage] = React.useState(0)
  const [total, setTotal] = React.useState(0)
  const limit = 50

  const load = React.useCallback(async () => {
    const res = await fetchAudit(agentId, limit, page * limit)
    setItems(res.items)
    setTotal(res.total)
  }, [agentId, page])

  React.useEffect(() => { load() }, [load])

  return (
    <Card className="p-4">
      <h3 className="mb-3 text-sm font-semibold">Audit Log</h3>
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">No audit entries.</p>
      ) : (
        <div className="flex flex-col gap-2">
          {items.map((a) => (
            <div key={a.id} className="rounded-lg border border-border p-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Badge variant="outline">{a.action}</Badge>
                  <span className="text-sm">{a.resourceType}</span>
                </div>
                <span className="text-xs text-muted-foreground">
                  {new Date(a.createdAt).toLocaleString()}
                </span>
              </div>
              {a.details && Object.keys(a.details).length > 0 && (
                <p className="mt-1 text-xs text-muted-foreground">
                  {JSON.stringify(a.details)}
                </p>
              )}
            </div>
          ))}
        </div>
      )}

      {total > limit && (
        <div className="mt-3 flex items-center justify-center gap-2">
          <Button size="sm" variant="outline" disabled={page === 0} onClick={() => setPage(p => p - 1)}>
            Previous
          </Button>
          <span className="text-xs text-muted-foreground">
            Page {page + 1} of {Math.ceil(total / limit)}
          </span>
          <Button size="sm" variant="outline" disabled={(page + 1) * limit >= total} onClick={() => setPage(p => p + 1)}>
            Next
          </Button>
        </div>
      )}
    </Card>
  )
}
