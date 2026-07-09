"use client"

import * as React from "react"
import { fetchChecks, type CheckResult } from "@/lib/api"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"

interface ChecksTabProps {
  agentId: string
}

export function ChecksTab({ agentId }: ChecksTabProps) {
  const [items, setItems] = React.useState<CheckResult[]>([])

  React.useEffect(() => {
    fetchChecks(agentId).then(setItems)
  }, [agentId])

  return (
    <Card className="p-4">
      <h3 className="mb-3 text-sm font-semibold">Checks ({items.length})</h3>
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">No checks configured.</p>
      ) : (
        <div className="flex flex-col gap-2">
          {items.map((c) => (
            <div key={c.id} className="flex items-center justify-between rounded-lg border border-border p-3">
              <div>
                <p className="text-sm font-medium">{c.description}</p>
                <p className="text-xs text-muted-foreground">{c.checkType}</p>
              </div>
              <div className="flex items-center gap-2">
                <Badge
                  variant="outline"
                  className={
                    c.status === "pass"
                      ? "text-success"
                      : c.status === "fail"
                        ? "text-destructive"
                        : "text-muted-foreground"
                  }
                >
                  {c.status}
                </Badge>
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}
