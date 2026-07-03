"use client"

import * as React from "react"
import {
  Cpu,
  HardDrive,
  MemoryStick,
  Network,
  Server,
  Monitor,
} from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Card } from "@/components/ui/card"
import type { DeviceSummary } from "./types"

interface SummaryTabProps {
  device: DeviceSummary;
  alertsNotImplemented?: boolean;
  backupsNotImplemented?: boolean;
}

export function SummaryTab({ device, alertsNotImplemented, backupsNotImplemented }: SummaryTabProps) {
  const ramUsed = device.totalRAM > 0
    ? Math.round(((device.totalRAM - device.freeRam) / device.totalRAM) * 100)
    : 0

  return (
    <div className="space-y-4">
      {/* System Identity - High Density Profile */}
      <Card className="gap-0 p-0">
        <div className="flex items-center justify-between border-b border-border px-4 py-3">
          <div className="flex items-center gap-2">
            <Server className="size-4 text-primary" />
            <h2 className="text-sm font-semibold">System Identity</h2>
          </div>
          <Badge variant={device.status === "online" ? "default" : "secondary"} className="capitalize">
            {device.status}
          </Badge>
        </div>

        <div className="grid grid-cols-1 gap-6 p-4 md:grid-cols-2 lg:grid-cols-3">
          {/* Column 1: OS & Software */}
          <div className="space-y-3">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Operating System</h3>
            <div className="space-y-2">
              <div className="flex justify-between">
                <span className="text-xs text-muted-foreground">Hostname</span>
                <span className="font-mono text-xs font-medium">{device.hostname}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-xs text-muted-foreground">OS</span>
                <span className="text-xs font-medium">{device.os}</span>
              </div>
              {device.kernelVersion && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">Kernel</span>
                  <span className="font-mono text-xs">{device.kernelVersion}</span>
                </div>
              )}
              {device.agentVersion && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">Agent</span>
                  <span className="font-mono text-xs text-muted-foreground">v{device.agentVersion}</span>
                </div>
              )}
            </div>
          </div>

          {/* Column 2: Hardware */}
          <div className="space-y-3">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Hardware</h3>
            <div className="space-y-2">
              {device.vendor && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">Vendor</span>
                  <span className="text-xs font-medium">{device.vendor}</span>
                </div>
              )}
              {device.model && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">Model</span>
                  <span className="text-xs font-medium">{device.model}</span>
                </div>
              )}
              {device.serialNumber && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">Serial</span>
                  <span className="font-mono text-xs">{device.serialNumber}</span>
                </div>
              )}
              {device.cpuModel && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">CPU</span>
                  <span className="text-xs font-medium truncate max-w-[200px]" title={device.cpuModel}>{device.cpuModel}</span>
                </div>
              )}
              {device.numCPU > 0 && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">Cores</span>
                  <span className="text-xs font-medium">{device.numCPU}</span>
                </div>
              )}
              {device.gpuName && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">GPU</span>
                  <span className="text-xs font-medium truncate max-w-[200px]" title={device.gpuName}>{device.gpuName}</span>
                </div>
              )}
              {device.gpuDriver && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">GPU Driver</span>
                  <span className="font-mono text-xs">{device.gpuDriver}</span>
                </div>
              )}
            </div>
          </div>

          {/* Column 3: Network */}
          <div className="space-y-3 rounded-lg border border-border bg-muted/20 p-3">
            <div className="flex items-center gap-2">
              <Network className="size-4 text-purple-400" />
              <h3 className="text-sm font-medium">Network</h3>
            </div>
            <div className="space-y-2">
              {device.localIP && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">Local IP</span>
                  <span className="font-mono text-xs font-medium">{device.localIP}</span>
                </div>
              )}
              {device.macAddress && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">MAC</span>
                  <span className="font-mono text-[10px]">{device.macAddress}</span>
                </div>
              )}
              {device.gateway && (
                <div className="flex justify-between">
                  <span className="text-xs text-muted-foreground">Gateway</span>
                  <span className="font-mono text-xs">{device.gateway}</span>
                </div>
              )}
              {!device.localIP && !device.macAddress && !device.gateway && (
                <p className="text-xs text-muted-foreground">No network data available</p>
              )}
            </div>
          </div>
        </div>
      </Card>

      {/* Disk Partitions */}
      {device.disks && device.disks.length > 0 && (
        <Card className="gap-3 p-4">
          <div className="flex items-center gap-2">
            <HardDrive className="size-4 text-blue-400" />
            <h2 className="text-sm font-semibold">Disk Partitions</h2>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {device.disks.map((disk) => {
              const usedPercent = disk.size > 0
                ? Math.round(((disk.size - disk.freeSpace) / disk.size) * 100)
                : 0
              const usedGB = ((disk.size - disk.freeSpace) / 1073741824).toFixed(1)
              const totalGB = (disk.size / 1073741824).toFixed(1)

              return (
                <div key={disk.deviceID} className="rounded-lg border border-border p-3">
                  <div className="flex items-center justify-between mb-2">
                    <span className="font-mono text-sm font-medium">{disk.deviceID}</span>
                    <Badge variant={usedPercent > 90 ? "destructive" : "secondary"}>
                      {usedPercent}%
                    </Badge>
                  </div>
                  <div className="h-2 w-full rounded-full bg-muted overflow-hidden">
                    <div
                      className={`h-full rounded-full transition-all ${
                        usedPercent > 90 ? "bg-red-500" : usedPercent > 70 ? "bg-yellow-500" : "bg-green-500"
                      }`}
                      style={{ width: `${usedPercent}%` }}
                    />
                  </div>
                  <div className="mt-2 flex justify-between text-xs text-muted-foreground">
                    <span>{usedGB} GB used</span>
                    <span>{totalGB} GB total</span>
                  </div>
                </div>
              )
            })}
          </div>
        </Card>
      )}

      {/* Alerts & Backups - Not Implemented */}
      {(alertsNotImplemented || backupsNotImplemented) && (
        <Card className="gap-3 p-4">
          <div className="flex items-center gap-2 text-muted-foreground">
            <span className="text-xs">Note:</span>
            {alertsNotImplemented && (
              <Badge variant="outline" className="text-[10px]">Recent Alerts — not implemented</Badge>
            )}
            {backupsNotImplemented && (
              <Badge variant="outline" className="text-[10px]">Recent Backups — not implemented</Badge>
            )}
          </div>
          <p className="text-xs text-muted-foreground">
            These sections will be connected to real data in a future update.
          </p>
        </Card>
      )}
    </div>
  )
}
