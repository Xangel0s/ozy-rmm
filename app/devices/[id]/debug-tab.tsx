"use client"

import * as React from "react"
import { fetchLogs, type LogEntry } from "@/lib/api"
import { Card } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

interface DebugTabProps {
  agentId: string
}

const levels = ["", "info", "warning", "error"] as const

export function DebugTab({ agentId }: DebugTabProps) {
  const [items, setItems] = React.useState<LogEntry[]>([])
  const [level, setLevel] = React.useState("")
  const [cursor, setCursor] = React.useState<string | undefined>()
  const [hasMore, setHasMore] = React.useState(false)

  const load = React.useCallback(async (newCursor?: string) => {
    const res = await fetchLogs(agentId, level || undefined, newCursor, 100)
    if (newCursor) {
      setItems(prev => [...prev, ...res.items])
    } else {
      setItems(res.items)
    }
    setCursor(res.cursor)
    setHasMore(res.hasMore)
  }, [agentId, level])

  React.useEffect(() => {
    setCursor(undefined)
    load()
  }, [load])

  return (
    <Card className="p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-semibold">Agent Logs</h3>
        <div className="flex items-center rounded-lg bg-secondary p-0.5">
          {levels.map((l) => (
            <button
              key={l}
              type="button"
              onClick={() => setLevel(l)}
              className={
                level === l
                  ? "rounded-md bg-card px-2.5 py-1 text-xs font-medium text-foreground ring-1 ring-border"
                  : "rounded-md px-2.5 py-1 text-xs font-medium capitalize text-muted-foreground hover:text-foreground"
              }
            >
              {l || "All"}
            </button>
          ))}
        </div>
      </div>

      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">No log entries.</p>
      ) : (
        <div className="flex flex-col gap-1">
          {items.map((l) => (
            <div key={l.id} className="flex items-start gap-2 rounded-md px-2 py-1 font-mono text-xs hover:bg-muted/50">
              <Badge
                variant="outline"
                className={
                  l.level === "error"
                    ? "shrink-0 text-destructive"
                    : l.level === "warning"
                      ? "shrink-0 text-warning"
                      : "shrink-0 text-muted-foreground"
                }
              >
                {l.level}
              </Badge>
              <span className="shrink-0 text-muted-foreground">
                {new Date(l.createdAt).toLocaleTimeString()}
              </span>
              <span className="text-foreground">{l.message}</span>
            </div>
          ))}

          {hasMore && (
            <Button
              size="sm"
              variant="ghost"
              className="mt-2"
              onClick={() => load(cursor)}
            >
              Load more
            </Button>
          )}
        </div>
      )}
    </Card>
  )
}
