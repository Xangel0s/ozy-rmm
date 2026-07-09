/**
 * lib/types.ts
 * UI-only types used across the dashboard. These types describe the SHAPE
 * the frontend works with; the source of truth lives in the Go backend and
 * is fetched via lib/api.ts and lib/use-live-data.ts.
 *
 * History: these types used to live in lib/rmm-data.ts alongside mock data
 * arrays. After Fase 1.4 (no mocks) the mock arrays are gone and the types
 * are extracted here for cleaner imports.
 */

export type DeviceStatus = "online" | "offline" | "warning"

export type OS =
  | "windows-server"
  | "ubuntu-server"
  | "debian"
  | "windows"
  | "macos"

export type Severity = "critical" | "warning" | "info"

export type Device = {
  id: string
  name: string
  os: OS
  tenant: string
  status: DeviceStatus
  cpu: number
  ram: number
  disk: number
  cpuTrend: number[]
  ramTrend: number[]
  diskTrend: number[]
  lastSync: string
  lastSeen?: string
  ip: string
  vendor?: string
  model?: string
  serialNumber?: string
  uptime?: string
  kernelVersion?: string
  agentVersion?: string
  localIP?: string
  macAddress?: string
  gateway?: string
  numCPU?: number
  cpuModel?: string
  totalRam?: number
  freeRam?: number
  diskTotal?: number
  diskFree?: number
}

export type AlertEvent = {
  id: string
  device: string
  message: string
  severity: Severity
  time: string
}

export type DeviceBackup = {
  id: string
  deviceId: string
  name: string
  type: "full" | "incremental"
  status: "completed" | "running" | "failed"
  size: string
  location: string
  cron: string
  createdAt: string
}

export const osLabels: Record<OS, string> = {
  "windows-server": "Windows Server",
  "ubuntu-server": "Ubuntu Server",
  debian: "Debian",
  windows: "Windows 11",
  macos: "macOS",
}
