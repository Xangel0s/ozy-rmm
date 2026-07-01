"use client"

import { PlayCircle, RefreshCw, Search } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { DeployAgentDialog } from "@/components/rmm/deploy-agent-dialog"
import { tenants } from "@/lib/rmm-data"

export function Topbar({
  tenant,
  query,
  onQueryChange,
  selectedCount,
  onRunScript,
  title = "Fleet Dashboard",
  subtitle,
  showSearch = true,
}: {
  tenant: string
  query: string
  onQueryChange: (value: string) => void
  selectedCount: number
  onRunScript?: () => void
  title?: string
  subtitle?: string
  showSearch?: boolean
}) {
  const tenantName = tenants.find((t) => t.id === tenant)?.name ?? "All Clients"
  const headerSubtitle = subtitle ?? tenantName

  return (
    <header className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b border-border bg-background/80 px-4 backdrop-blur-md">
      <div className="flex min-w-0 flex-col leading-tight">
        <h1 className="truncate text-sm font-semibold tracking-tight">{title}</h1>
        <span className="truncate text-xs text-muted-foreground">{headerSubtitle}</span>
      </div>

      {showSearch && (
        <div className="relative ml-auto hidden w-64 md:block">
          <Search className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="Search devices, IPs, clients…"
            className="h-8 pl-8"
          />
        </div>
      )}

      {onRunScript && (
        <Button
          variant="outline"
          size="sm"
          onClick={onRunScript}
          disabled={selectedCount === 0}
          className="ml-auto md:ml-0"
        >
          <PlayCircle data-icon="inline-start" />
          Run Script
          {selectedCount > 0 && (
            <span className="ml-1 rounded bg-primary/15 px-1.5 text-xs font-semibold text-primary tabular-nums">
              {selectedCount}
            </span>
          )}
        </Button>
      )}

      <Button variant="outline" size="icon-sm" aria-label="Refresh fleet">
        <RefreshCw />
      </Button>

      <DeployAgentDialog />
    </header>
  )
}
