"use client"

import * as React from "react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { fetchGlobalAudit, type GlobalAuditEntry } from "@/lib/api"

const ACTION_COLORS: Record<string, string> = {
  login: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200",
  "backup.run": "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
  "software.uninstall": "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
  "agent.enroll": "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200",
  "token.create": "bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200",
  "alert.acknowledge": "bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200",
}

export function AuditLog() {
  const [items, setItems] = React.useState<GlobalAuditEntry[]>([])
  const [total, setTotal] = React.useState(0)
  const [page, setPage] = React.useState(0)
  const [actionFilter, setActionFilter] = React.useState("")
  const limit = 50

  const load = React.useCallback(async () => {
    const res = await fetchGlobalAudit({
      action: actionFilter || undefined,
      limit,
      offset: page * limit,
    })
    setItems(res.items)
    setTotal(res.total)
  }, [actionFilter, page])

  React.useEffect(() => { load() }, [load])

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Input
          placeholder="Filter by action..."
          value={actionFilter}
          onChange={(e) => { setActionFilter(e.target.value); setPage(0) }}
          className="max-w-xs"
        />
        <span className="text-sm text-muted-foreground">{total} entries</span>
      </div>

      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">No audit entries.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Action</TableHead>
              <TableHead>Resource</TableHead>
              <TableHead>User</TableHead>
              <TableHead>IP</TableHead>
              <TableHead>Details</TableHead>
              <TableHead>Time</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((a) => (
              <TableRow key={a.id}>
                <TableCell>
                  <Badge className={ACTION_COLORS[a.action] ?? ""} variant="outline">
                    {a.action}
                  </Badge>
                </TableCell>
                <TableCell className="text-sm">
                  {a.resourceType}/{a.resourceId?.substring(0, 8)}
                </TableCell>
                <TableCell className="text-sm font-mono">{a.userId?.substring(0, 12) || "-"}</TableCell>
                <TableCell className="text-sm font-mono">{a.ipAddress || "-"}</TableCell>
                <TableCell className="text-xs text-muted-foreground max-w-[200px] truncate">
                  {a.details && Object.keys(a.details).length > 0
                    ? JSON.stringify(a.details)
                    : "-"}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground whitespace-nowrap">
                  {new Date(a.createdAt).toLocaleString()}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      {total > limit && (
        <div className="flex items-center justify-center gap-2">
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
    </div>
  )
}
