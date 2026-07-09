"use client"

import * as React from "react"
import { Card } from "@/components/ui/card"
import type { AgentInfo } from "@/lib/api"

interface AssetsTabProps {
  agent: AgentInfo
}

export function AssetsTab({ agent }: AssetsTabProps) {
  const pairs: [string, string][] = [
    ["Hostname", agent.hostname],
    ["OS", agent.os],
    ["CPU Model", agent.cpuModel],
    ["CPU Cores", String(agent.numCPU)],
    ["Total RAM", `${(agent.totalRam / 1073741824).toFixed(1)} GB`],
    ["Vendor", agent.vendor],
    ["Model", agent.model],
    ["Serial Number", agent.serialNumber],
    ["Kernel", agent.kernelVersion],
    ["Agent Version", agent.agentVersion],
    ["IP", agent.localIP],
    ["MAC", agent.macAddress],
    ["Gateway", agent.gateway],
    ["GPU", agent.gpuName || "—"],
    ["GPU Driver", agent.gpuDriver || "—"],
  ]

  const copy = (val: string) => {
    navigator.clipboard.writeText(val)
  }

  return (
    <Card className="p-4">
      <h3 className="mb-3 text-sm font-semibold">Assets</h3>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {pairs.map(([key, val]) => (
          <div
            key={key}
            className="cursor-pointer rounded-lg border border-border p-3 hover:bg-muted/50"
            onClick={() => copy(val)}
            title="Click to copy"
          >
            <span className="text-xs text-muted-foreground">{key}</span>
            <p className="truncate text-sm font-medium">{val || "—"}</p>
          </div>
        ))}
      </div>
    </Card>
  )
}
