"use client"

import Link from "next/link"
import { usePathname, useRouter } from "next/navigation"
import {
  Activity,
  ArchiveRestore,
  Bell,
  ChevronsUpDown,
  LayoutDashboard,
  LogOut,
  MonitorSmartphone,
  Settings,
  TerminalSquare,
} from "lucide-react"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { tenants } from "@/lib/rmm-data"
import { useAgents, useAlerts, useBackups } from "@/lib/use-live-data"
import { cn } from "@/lib/utils"
import { logout, getCurrentUser } from "@/lib/api"
import { Button } from "@/components/ui/button"

const nav = [
  { label: "Dashboard", icon: LayoutDashboard, href: "/", badge: null },
  { label: "Monitored Devices", icon: MonitorSmartphone, href: "/devices", badge: "devices" },
  { label: "Backups", icon: ArchiveRestore, href: "/backups", badge: "backups" },
  { label: "Automation / Scripts", icon: TerminalSquare, href: "/automation", badge: null },
  { label: "Alerts", icon: Bell, href: "/alerts", badge: "alerts" },
  { label: "Settings", icon: Settings, href: "/settings", badge: null },
]

export function Sidebar({
  tenant,
  onTenantChange,
}: {
  tenant: string
  onTenantChange: (value: string) => void
}) {
  const pathname = usePathname()
  const router = useRouter()

  // Live counts from the backend — these replace the static mock values
  const { agents } = useAgents(5000)
  const { alerts: liveAlerts } = useAlerts(10000)
  const { backups: liveBackups } = useBackups(15000)

  const criticalAlerts = liveAlerts.filter((a) => a.severity === "critical").length
  const runningBackups = liveBackups.filter((b) => b.status === "running").length
  const deviceCount = agents.length

  const resolveBadge = (badge: string | null): string | null => {
    if (!badge) return null
    if (badge === "devices") return deviceCount > 0 ? String(deviceCount) : null
    if (badge === "alerts") return criticalAlerts > 0 ? String(criticalAlerts) : null
    if (badge === "backups") return runningBackups > 0 ? String(runningBackups) : null
    return badge
  }

  const isActiveRoute = (href: string) => {
    if (href === "/") return pathname === "/"
    return pathname === href || pathname.startsWith(`${href}/`)
  }

  return (
    <aside className="flex h-full w-64 shrink-0 flex-col border-r border-sidebar-border bg-sidebar">
      <div className="flex h-14 items-center gap-2 border-b border-sidebar-border px-4">
        <div className="flex size-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <Activity className="size-4" />
        </div>
        <div className="flex flex-col leading-none">
          <span className="text-sm font-semibold tracking-tight text-sidebar-foreground">
            ApexRMM
          </span>
          <span className="text-[11px] text-muted-foreground">Operations Console</span>
        </div>
      </div>

      <div className="border-b border-sidebar-border p-3">
        <label className="mb-1.5 block px-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
          Active Client
        </label>
        <Select value={tenant} onValueChange={(v) => v && onTenantChange(v)}>
          <SelectTrigger className="w-full bg-secondary/60">
            <SelectValue>
              {(value: string) => tenants.find((t) => t.id === value)?.name}
            </SelectValue>
            <ChevronsUpDown className="ml-auto size-3.5 text-muted-foreground" />
          </SelectTrigger>
          <SelectContent>
            {tenants.map((t) => (
              <SelectItem key={t.id} value={t.id}>
                <span className="flex w-full items-center justify-between gap-3">
                  <span>{t.name}</span>
                  <span className="text-xs text-muted-foreground">{t.devices}</span>
                </span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <nav className="flex-1 space-y-1 p-3">
        {nav.map((item) => (
          <Link
            key={item.label}
            href={item.href}
            aria-current={isActiveRoute(item.href) ? "page" : undefined}
            className={cn(
              "flex items-center gap-3 rounded-md px-2.5 py-2 text-sm font-medium transition-colors",
              isActiveRoute(item.href)
                ? "bg-sidebar-accent text-sidebar-accent-foreground"
                : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-foreground",
            )}
          >
            <item.icon className="size-4 shrink-0" />
            <span className="flex-1 truncate">{item.label}</span>
            {resolveBadge(item.badge) && (
              <span
                className={cn(
                  "rounded-full px-1.5 py-0.5 text-[10px] font-semibold",
                  item.label === "Alerts"
                    ? "bg-destructive/15 text-destructive"
                    : "bg-secondary text-muted-foreground",
                )}
              >
                {resolveBadge(item.badge)}
              </span>
            )}
          </Link>
        ))}
      </nav>

      <div className="border-t border-sidebar-border p-3">
        <div className="flex items-center gap-3 rounded-md px-1 py-1">
          {(() => {
            const user = getCurrentUser()
            const name = user?.fullName || user?.userId?.slice(0, 8) || "User"
            const role = user?.role || "technician"
            const initials = name.split(" ").map((w: string) => w[0]).join("").slice(0, 2).toUpperCase()
            return (
              <>
                <div className="flex size-8 items-center justify-center rounded-full bg-secondary text-xs font-semibold text-foreground ring-1 ring-border">
                  {initials}
                </div>
                <div className="flex min-w-0 flex-1 flex-col leading-tight">
                  <span className="truncate text-sm font-medium">{name}</span>
                  <span className="truncate text-xs text-muted-foreground capitalize">{role}</span>
                </div>
              </>
            )
          })()}
          <Button
            variant="ghost"
            size="icon"
            className="size-7 text-muted-foreground hover:text-destructive"
            title="Sign out"
            onClick={() => { logout(); router.push("/login") }}
          >
            <LogOut className="size-4" />
          </Button>
        </div>
      </div>
    </aside>
  )
}
