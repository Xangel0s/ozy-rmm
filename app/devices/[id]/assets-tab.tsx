"use client"

import * as React from "react"
import { Boxes, Copy, Check } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"

interface AssetKV {
  key: string;
  value: string;
  source: string;
}

interface AssetsTabProps {
  agent: {
    hostname: string;
    os: string;
    cpuModel: string;
    numCPU: number;
    vendor: string;
    model: string;
    serialNumber: string;
    gpuName: string;
    gpuDriver: string;
    localIP: string;
    macAddress: string;
    gateway: string;
    kernelVersion: string;
    agentVersion: string;
    uptime: string;
    disks: Array<{ deviceID: string; size: number; freeSpace: number; label: string }>;
  };
}

export function AssetsTab({ agent }: AssetsTabProps) {
  const [copiedKey, setCopiedKey] = React.useState<string | null>(null)

  const assets: AssetKV[] = [
    { key: "Hostname", value: agent.hostname, source: "Agent" },
    { key: "Operating System", value: agent.os, source: "WMI" },
    { key: "Kernel Version", value: agent.kernelVersion, source: "Agent" },
    { key: "Agent Version", value: agent.agentVersion, source: "Agent" },
    { key: "Uptime", value: agent.uptime, source: "Agent" },
    { key: "CPU Model", value: agent.cpuModel, source: "WMI" },
    { key: "CPU Cores", value: String(agent.numCPU), source: "WMI" },
    { key: "Vendor", value: agent.vendor, source: "WMI" },
    { key: "Model", value: agent.model, source: "WMI" },
    { key: "Serial Number", value: agent.serialNumber, source: "WMI" },
    { key: "GPU", value: agent.gpuName, source: "WMI" },
    { key: "GPU Driver", value: agent.gpuDriver, source: "WMI" },
    { key: "Local IP", value: agent.localIP, source: "WMI" },
    { key: "MAC Address", value: agent.macAddress, source: "WMI" },
    { key: "Gateway", value: agent.gateway, source: "WMI" },
    ...agent.disks.flatMap((d) => [
      { key: `${d.deviceID} Total`, value: `${(d.size / 1073741824).toFixed(1)} GB`, source: "WMI" },
      { key: `${d.deviceID} Free`, value: `${(d.freeSpace / 1073741824).toFixed(1)} GB`, source: "WMI" },
    ]),
  ].filter((a) => a.value && a.value !== "0" && a.value !== "")

  const handleCopy = async (key: string, value: string) => {
    try {
      await navigator.clipboard.writeText(value)
      setCopiedKey(key)
      setTimeout(() => setCopiedKey(null), 2000)
    } catch {
      // clipboard API not available
    }
  }

  return (
    <Card className="gap-4 p-4">
      <div className="flex items-center gap-2">
        <Boxes className="size-4 text-primary" />
        <h2 className="text-sm font-semibold">System Assets</h2>
        <Badge variant="secondary">{assets.length} properties</Badge>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left text-xs font-medium text-muted-foreground">
              <th className="pb-2 pr-4">Property</th>
              <th className="pb-2 pr-4">Value</th>
              <th className="pb-2 pr-4">Source</th>
              <th className="pb-2 w-10"></th>
            </tr>
          </thead>
          <tbody>
            {assets.map((asset) => (
              <tr key={asset.key} className="border-b border-border/50">
                <td className="py-2 pr-4 font-medium">{asset.key}</td>
                <td className="py-2 pr-4 font-mono text-xs">{asset.value}</td>
                <td className="py-2 pr-4">
                  <Badge variant="outline" className="text-[10px]">
                    {asset.source}
                  </Badge>
                </td>
                <td className="py-2">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-6 w-6 p-0"
                    onClick={() => handleCopy(asset.key, asset.value)}
                  >
                    {copiedKey === asset.key ? (
                      <Check className="size-3 text-green-500" />
                    ) : (
                      <Copy className="size-3" />
                    )}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}
