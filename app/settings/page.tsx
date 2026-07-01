"use client"

import * as React from "react"
import { toast } from "sonner"
import { Save } from "lucide-react"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export default function SettingsPage() {
  const [tenant, setTenant] = React.useState("all")
  const [query, setQuery] = React.useState("")

  const [agentInterval, setAgentInterval] = React.useState("60")
  const [retentionDays, setRetentionDays] = React.useState("30")
  const [timezone, setTimezone] = React.useState("America/Bogota")
  const [autoDeploy, setAutoDeploy] = React.useState(true)
  const [maintenanceMode, setMaintenanceMode] = React.useState(false)

  const saveSettings = () => {
    toast.success("Settings saved", {
      description: "Operational preferences were updated successfully.",
    })
  }

  return (
    <ConsoleShell
      tenant={tenant}
      onTenantChange={setTenant}
      query={query}
      onQueryChange={setQuery}
      title="Settings"
      subtitle="Platform configuration"
      showSearch={false}
    >
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <Card className="gap-4 p-4">
          <div>
            <h2 className="text-sm font-semibold">Agent Policies</h2>
            <p className="text-xs text-muted-foreground">
              Define polling cadence and automated onboarding behavior.
            </p>
          </div>

          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">Heartbeat interval (seconds)</span>
            <Input value={agentInterval} onChange={(e) => setAgentInterval(e.target.value)} />
          </label>

          <div className="flex items-center justify-between rounded-lg border border-border p-3">
            <div>
              <p className="text-sm font-medium">Auto deploy agent on discovery</p>
              <p className="text-xs text-muted-foreground">Enroll new endpoints automatically.</p>
            </div>
            <Checkbox checked={autoDeploy} onCheckedChange={(checked) => setAutoDeploy(checked === true)} />
          </div>
        </Card>

        <Card className="gap-4 p-4">
          <div>
            <h2 className="text-sm font-semibold">Platform Defaults</h2>
            <p className="text-xs text-muted-foreground">
              Audit retention and regional settings used by the console.
            </p>
          </div>

          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">Log retention (days)</span>
            <Input value={retentionDays} onChange={(e) => setRetentionDays(e.target.value)} />
          </label>

          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">Timezone</span>
            <Select value={timezone} onValueChange={setTimezone}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder="Select timezone" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="America/Bogota">America/Bogota (UTC-5)</SelectItem>
                <SelectItem value="America/Mexico_City">America/Mexico_City (UTC-6)</SelectItem>
                <SelectItem value="America/Santiago">America/Santiago (UTC-4)</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex items-center justify-between rounded-lg border border-border p-3">
            <div>
              <p className="text-sm font-medium">Maintenance mode</p>
              <p className="text-xs text-muted-foreground">Pause automation and alert escalations.</p>
            </div>
            <Checkbox
              checked={maintenanceMode}
              onCheckedChange={(checked) => setMaintenanceMode(checked === true)}
            />
          </div>
        </Card>
      </div>

      <div className="flex justify-end">
        <Button onClick={saveSettings}>
          <Save data-icon="inline-start" />
          Save Changes
        </Button>
      </div>
    </ConsoleShell>
  )
}
