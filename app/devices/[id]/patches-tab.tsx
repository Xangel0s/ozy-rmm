"use client"

import * as React from "react"
import { fetchPatches, scanPatches, type PatchItem } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"

interface PatchesTabProps {
  agentId: string
}

export function PatchesTab({ agentId }: PatchesTabProps) {
  const [items, setItems] = React.useState<PatchItem[]>([])
  const [total, setTotal] = React.useState(0)
  const [page, setPage] = React.useState(0)
  const limit = 50
  const [scanning, setScanning] = React.useState(false)

  const load = React.useCallback(async () => {
    const res = await fetchPatches(agentId, limit, page * limit)
    setItems(res.items)
    setTotal(res.total)
  }, [agentId, page])

  React.useEffect(() => { load() }, [load])

  const handleScan = async () => {
    setScanning(true)
    await scanPatches(agentId)
    setTimeout(() => { setScanning(false); load() }, 3000)
  }

  return (
    <Card className="p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold">Patches ({total})</h3>
        <Button size="sm" variant="outline" onClick={handleScan} disabled={scanning}>
          {scanning ? "Scanning..." : "Scan"}
        </Button>
      </div>

      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">No patches found.</p>
      ) : (
        <div className="flex flex-col gap-2">
          {items.map((p) => (
            <div key={p.id} className="flex items-center justify-between rounded-lg border border-border p-3">
              <div>
                <p className="text-sm font-medium">{p.name}</p>
                <p className="text-xs text-muted-foreground">{p.kbId}</p>
              </div>
              <div className="flex items-center gap-2">
                <Badge variant="outline" className={p.severity === "Critical" ? "text-destructive" : ""}>
                  {p.severity}
                </Badge>
                <Badge variant={p.installed ? "secondary" : "outline"}>
                  {p.installed ? "Installed" : "Missing"}
                </Badge>
              </div>
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
