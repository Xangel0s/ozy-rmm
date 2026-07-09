import { AlertTriangle, Cpu, MonitorSmartphone, ShieldCheck } from "lucide-react"
import { Card } from "@/components/ui/card"
import { cn } from "@/lib/utils"
import type { Device } from "@/lib/types"

export function KpiCards({ devices }: { devices: Device[] }) {
  const total = devices.length
  const online = devices.filter((d) => d.status === "online").length
  const offline = devices.filter((d) => d.status === "offline").length
  const warning = devices.filter((d) => d.status === "warning").length
  const critical = devices.filter((d) => d.status === "warning" || d.status === "offline").length
  const highCpu = devices.filter((d) => d.cpu >= 90).length
  const patched = Math.round((online / Math.max(total, 1)) * 100)

  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <Card className="gap-0 p-4 transition-all duration-300 hover:scale-[1.02] hover:-translate-y-1 hover:shadow-lg hover:ring-foreground/25 cursor-pointer">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">Total Devices</span>
          <MonitorSmartphone className="size-4 text-muted-foreground" />
        </div>
        <div className="mt-2 text-2xl font-semibold tabular-nums">{total}</div>
        <div className="mt-3 flex items-center gap-3 text-xs">
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-full bg-success" /> {online} online
          </span>
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-full bg-warning" /> {warning} warn
          </span>
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-full bg-muted-foreground" /> {offline} down
          </span>
        </div>
      </Card>

      <Card className="gap-0 border-destructive/30 bg-destructive/5 p-4 transition-all duration-300 hover:scale-[1.02] hover:-translate-y-1 hover:shadow-lg hover:shadow-destructive/5 hover:ring-destructive/30 hover:border-destructive/50 cursor-pointer">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">Critical Alerts</span>
          <span className="flex size-6 items-center justify-center rounded-md bg-destructive/15">
            <AlertTriangle className="size-4 text-destructive" />
          </span>
        </div>
        <div className="mt-2 text-2xl font-semibold tabular-nums text-destructive">{critical}</div>
        <p className="mt-3 text-xs text-muted-foreground">
          Requires immediate attention across active endpoints
        </p>
      </Card>

      <Card className="gap-0 p-4 transition-all duration-300 hover:scale-[1.02] hover:-translate-y-1 hover:shadow-lg hover:ring-foreground/25 cursor-pointer">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">Patch Status</span>
          <ShieldCheck className="size-4 text-success" />
        </div>
        <div className="mt-2 flex items-baseline gap-1">
          <span className="text-2xl font-semibold tabular-nums">{patched}%</span>
          <span className="text-xs text-muted-foreground">up to date</span>
        </div>
        <div className="mt-3 h-1.5 w-full overflow-hidden rounded-full bg-secondary">
          <div className="h-full rounded-full bg-success" style={{ width: `${patched}%` }} />
        </div>
      </Card>

      <Card className={cn("gap-0 p-4 transition-all duration-300 hover:scale-[1.02] hover:-translate-y-1 hover:shadow-lg cursor-pointer", highCpu > 0 ? "border-warning/30 bg-warning/5 hover:ring-warning/30 hover:border-warning/50 hover:shadow-warning/5" : "hover:ring-foreground/25")}>
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">Resource Alerts</span>
          <Cpu className={cn("size-4", highCpu > 0 ? "text-warning" : "text-muted-foreground")} />
        </div>
        <div className="mt-2 text-2xl font-semibold tabular-nums">{highCpu}</div>
        <p className="mt-3 text-xs text-muted-foreground">
          {highCpu > 0 ? `servers with CPU > 90%` : "all endpoints within thresholds"}
        </p>
      </Card>
    </div>
  )
}
