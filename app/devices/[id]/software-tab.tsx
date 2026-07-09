"use client"

import * as React from "react"
import { fetchSoftware, scanSoftware, type SoftwareItem } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"

interface SoftwareTabProps {
  agentId: string
}

export function SoftwareTab({ agentId }: SoftwareTabProps) {
  const [items, setItems] = React.useState<SoftwareItem[]>([])
  const [total, setTotal] = React.useState(0)
  const [page, setPage] = React.useState(0)
  const limit = 50
  const [scanning, setScanning] = React.useState(false)

  const load = React.useCallback(async () => {
    const res = await fetchSoftware(agentId, limit, page * limit)
    setItems(res.items)
    setTotal(res.total)
  }, [agentId, page])

  React.useEffect(() => { load() }, [load])

  const handleScan = async () => {
    setScanning(true)
    await scanSoftware(agentId)
    setTimeout(() => { setScanning(false); load() }, 3000)
  }

  return (
    <Card className="p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold">Software ({total})</h3>
        <Button size="sm" variant="outline" onClick={handleScan} disabled={scanning}>
          {scanning ? "Scanning..." : "Scan"}
        </Button>
      </div>

      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">No software found.</p>
      ) : (
        <div className="flex flex-col gap-2">
          {items.map((s) => (
            <div key={s.id} className="flex items-center justify-between rounded-lg border border-border p-3">
              <div>
                <p className="text-sm font-medium">{s.name}</p>
                <p className="text-xs text-muted-foreground">
                  {s.publisher} &middot; {s.version}
                </p>
              </div>
              <div className="flex items-center gap-2">
                {s.estimatedSizeKB > 0 && (
                  <span className="text-xs text-muted-foreground">
                    {(s.estimatedSizeKB / 1024).toFixed(1)} MB
                  </span>
                )}
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
