import Link from "next/link"
import { AlertOctagon, AlertTriangle, Info } from "lucide-react"
import { Card } from "@/components/ui/card"
import { ScrollArea } from "@/components/ui/scroll-area"
import { severityToneClasses } from "@/components/rmm/indicators"
import { alerts, type Severity } from "@/lib/rmm-data"
import { cn } from "@/lib/utils"

const icons: Record<Severity, typeof Info> = {
  critical: AlertOctagon,
  warning: AlertTriangle,
  info: Info,
}

const dotColor: Record<Severity, string> = {
  critical: "bg-destructive",
  warning: "bg-warning",
  info: "bg-info",
}

export function AlertsPanel() {
  const criticalCount = alerts.filter((a) => a.severity === "critical").length

  return (
    <Card className="flex h-full min-h-0 flex-col gap-0 p-0">
      <div className="flex items-center justify-between border-b border-border px-4 py-3">
        <h2 className="text-sm font-semibold">Active Alerts &amp; Logs</h2>
        <span className="flex items-center gap-1.5 rounded-full bg-destructive/15 px-2 py-0.5 text-xs font-semibold text-destructive tabular-nums">
          <span className="size-1.5 rounded-full bg-destructive" />
          {criticalCount} critical
        </span>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <ul className="divide-y divide-border">
          {alerts.map((a) => {
            const Icon = icons[a.severity]
            return (
              <li key={a.id}>
                <Link
                  href={`/alerts/${a.id}`}
                  className="flex gap-3 px-4 py-3 transition-colors hover:bg-muted/40"
                >
                <span className="relative mt-1 flex size-2 shrink-0">
                  {a.severity === "critical" && (
                    <span className="absolute inline-flex size-full animate-ping rounded-full bg-destructive opacity-60" />
                  )}
                  <span className={cn("relative inline-flex size-2 rounded-full", dotColor[a.severity])} />
                </span>
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <div className="flex items-center gap-2">
                    <span
                      className={cn(
                        "inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase ring-1",
                        severityToneClasses(a.severity),
                      )}
                    >
                      <Icon className="size-3" />
                      {a.severity}
                    </span>
                    <span className="truncate font-mono text-xs font-medium text-foreground">
                      {a.device}
                    </span>
                    <span className="ml-auto shrink-0 text-[11px] text-muted-foreground">{a.time}</span>
                  </div>
                  <p className="text-xs leading-relaxed text-muted-foreground">{a.message}</p>
                </div>
                </Link>
              </li>
            )
          })}
        </ul>
      </ScrollArea>
    </Card>
  )
}
