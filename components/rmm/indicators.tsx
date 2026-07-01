import { AppWindow, Server, Terminal } from "lucide-react"
import { cn } from "@/lib/utils"
import { osLabels, type DeviceStatus, type OS, type Severity } from "@/lib/rmm-data"

export function StatusDot({ status }: { status: DeviceStatus }) {
  const map = {
    online: "bg-success",
    offline: "bg-muted-foreground",
    warning: "bg-warning",
  } as const
  const label = { online: "Online", offline: "Offline", warning: "Warning" } as const
  return (
    <span className="inline-flex items-center gap-2 text-xs font-medium">
      <span className="relative flex size-2">
        {status === "online" && (
          <span className="absolute inline-flex size-full animate-ping rounded-full bg-success opacity-60" />
        )}
        <span className={cn("relative inline-flex size-2 rounded-full", map[status])} />
      </span>
      <span
        className={cn(
          status === "online" && "text-success",
          status === "warning" && "text-warning",
          status === "offline" && "text-muted-foreground",
        )}
      >
        {label[status]}
      </span>
    </span>
  )
}

export function OsIcon({ os }: { os: OS }) {
  const isWindows = os === "windows-server" || os === "windows"
  const isMac = os === "macos"
  const Icon = isWindows ? AppWindow : isMac ? Server : Terminal
  return (
    <span
      className="flex size-7 shrink-0 items-center justify-center rounded-md bg-secondary text-muted-foreground ring-1 ring-border"
      title={osLabels[os]}
    >
      <Icon className="size-4" />
    </span>
  )
}

export function UsageBar({
  value,
  tone,
}: {
  value: number
  tone: "info" | "success" | "warning" | "destructive"
}) {
  const bg = {
    info: "bg-info",
    success: "bg-success",
    warning: "bg-warning",
    destructive: "bg-destructive",
  }[tone]
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-secondary">
      <div className={cn("h-full rounded-full", bg)} style={{ width: `${Math.min(value, 100)}%` }} />
    </div>
  )
}

export function usageTone(value: number): "info" | "success" | "warning" | "destructive" {
  if (value >= 90) return "destructive"
  if (value >= 75) return "warning"
  if (value >= 50) return "info"
  return "success"
}

export function severityToneClasses(severity: Severity): string {
  return {
    critical: "bg-destructive/15 text-destructive ring-destructive/25",
    warning: "bg-warning/15 text-warning ring-warning/25",
    info: "bg-info/15 text-info ring-info/25",
  }[severity]
}
