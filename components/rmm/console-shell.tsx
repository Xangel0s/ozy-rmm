"use client"

import type * as React from "react"
import { Sidebar } from "@/components/rmm/sidebar"
import { Topbar } from "@/components/rmm/topbar"

type ConsoleShellProps = {
  tenant: string
  onTenantChange: (value: string) => void
  query: string
  onQueryChange: (value: string) => void
  title?: string
  subtitle?: string
  showSearch?: boolean
  selectedCount?: number
  onRunScript?: () => void
  children: React.ReactNode
}

export function ConsoleShell({
  tenant,
  onTenantChange,
  query,
  onQueryChange,
  title,
  subtitle,
  showSearch = true,
  selectedCount = 0,
  onRunScript,
  children,
}: ConsoleShellProps) {
  return (
    <div className="flex h-svh overflow-hidden bg-background text-foreground">
      <div className="hidden md:block">
        <Sidebar tenant={tenant} onTenantChange={onTenantChange} />
      </div>

      <div className="flex min-w-0 flex-1 flex-col">
        <Topbar
          tenant={tenant}
          query={query}
          onQueryChange={onQueryChange}
          selectedCount={selectedCount}
          onRunScript={onRunScript}
          title={title}
          subtitle={subtitle}
          showSearch={showSearch}
        />

        <main className="flex-1 overflow-y-auto p-4">
          <div className="mx-auto flex max-w-[1600px] flex-col gap-4">{children}</div>
        </main>
      </div>
    </div>
  )
}
