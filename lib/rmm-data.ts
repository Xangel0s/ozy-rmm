export type DeviceStatus = "online" | "offline" | "warning"
export type OS = "windows-server" | "ubuntu-server" | "debian" | "windows" | "macos"
export type Severity = "critical" | "warning" | "info"

export type Tenant = {
  id: string
  name: string
  devices: number
}

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

export const tenants: Tenant[] = [
  { id: "all", name: "All Clients", devices: 148 },
  { id: "northwind", name: "Northwind Traders", devices: 42 },
  { id: "acme", name: "Acme Industrial", devices: 36 },
  { id: "globex", name: "Globex Corporation", devices: 28 },
  { id: "initech", name: "Initech Systems", devices: 24 },
  { id: "umbrella", name: "Umbrella Labs", devices: 18 },
]

export const osLabels: Record<OS, string> = {
  "windows-server": "Windows Server",
  "ubuntu-server": "Ubuntu Server",
  debian: "Debian",
  windows: "Windows 11",
  macos: "macOS",
}

// Deterministic pseudo-random trend generator so SSR and client match.
function trend(seed: number, base: number): number[] {
  const out: number[] = []
  let v = base
  for (let i = 0; i < 24; i++) {
    const n = Math.sin(seed * 12.9898 + i * 78.233) * 43758.5453
    const frac = n - Math.floor(n)
    v = Math.max(2, Math.min(99, v + (frac - 0.5) * 22))
    out.push(Math.round(v))
  }
  return out
}

export const devices: Device[] = [
  {
    id: "dev-01", name: "WEB-PROD-01", os: "ubuntu-server", tenant: "Northwind Traders",
    status: "online", cpu: 34, ram: 58, disk: 62, ip: "10.4.1.21", lastSync: "12s ago",
    cpuTrend: trend(1, 34), ramTrend: trend(2, 58), diskTrend: trend(3, 62),
  },
  {
    id: "dev-02", name: "DB-PROD-01", os: "debian", tenant: "Northwind Traders",
    status: "warning", cpu: 91, ram: 84, disk: 77, ip: "10.4.1.22", lastSync: "8s ago",
    cpuTrend: trend(4, 88), ramTrend: trend(5, 82), diskTrend: trend(6, 77),
  },
  {
    id: "dev-03", name: "DC-01", os: "windows-server", tenant: "Acme Industrial",
    status: "online", cpu: 22, ram: 41, disk: 38, ip: "10.8.0.10", lastSync: "3s ago",
    cpuTrend: trend(7, 22), ramTrend: trend(8, 41), diskTrend: trend(9, 38),
  },
  {
    id: "dev-04", name: "FILE-SRV-02", os: "windows-server", tenant: "Acme Industrial",
    status: "offline", cpu: 0, ram: 0, disk: 91, ip: "10.8.0.14", lastSync: "6m ago",
    cpuTrend: trend(10, 4), ramTrend: trend(11, 5), diskTrend: trend(12, 91),
  },
  {
    id: "dev-05", name: "APP-K8S-03", os: "ubuntu-server", tenant: "Globex Corporation",
    status: "online", cpu: 47, ram: 63, disk: 44, ip: "10.12.3.5", lastSync: "18s ago",
    cpuTrend: trend(13, 47), ramTrend: trend(14, 63), diskTrend: trend(15, 44),
  },
  {
    id: "dev-06", name: "CACHE-REDIS-01", os: "debian", tenant: "Globex Corporation",
    status: "online", cpu: 12, ram: 71, disk: 28, ip: "10.12.3.9", lastSync: "5s ago",
    cpuTrend: trend(16, 12), ramTrend: trend(17, 71), diskTrend: trend(18, 28),
  },
  {
    id: "dev-07", name: "BUILD-AGENT-04", os: "ubuntu-server", tenant: "Initech Systems",
    status: "warning", cpu: 96, ram: 55, disk: 49, ip: "10.20.1.44", lastSync: "22s ago",
    cpuTrend: trend(19, 94), ramTrend: trend(20, 55), diskTrend: trend(21, 49),
  },
  {
    id: "dev-08", name: "RDP-GATEWAY", os: "windows-server", tenant: "Initech Systems",
    status: "online", cpu: 29, ram: 48, disk: 55, ip: "10.20.1.2", lastSync: "9s ago",
    cpuTrend: trend(22, 29), ramTrend: trend(23, 48), diskTrend: trend(24, 55),
  },
  {
    id: "dev-09", name: "MAIL-RELAY-01", os: "debian", tenant: "Umbrella Labs",
    status: "online", cpu: 18, ram: 37, disk: 41, ip: "10.30.0.7", lastSync: "14s ago",
    cpuTrend: trend(25, 18), ramTrend: trend(26, 37), diskTrend: trend(27, 41),
  },
  {
    id: "dev-10", name: "BACKUP-NAS-02", os: "ubuntu-server", tenant: "Umbrella Labs",
    status: "warning", cpu: 41, ram: 52, disk: 93, ip: "10.30.0.20", lastSync: "31s ago",
    cpuTrend: trend(28, 41), ramTrend: trend(29, 52), diskTrend: trend(30, 92),
  },
  {
    id: "dev-11", name: "VPN-EDGE-01", os: "ubuntu-server", tenant: "Northwind Traders",
    status: "online", cpu: 26, ram: 44, disk: 33, ip: "10.4.1.30", lastSync: "7s ago",
    cpuTrend: trend(31, 26), ramTrend: trend(32, 44), diskTrend: trend(33, 33),
  },
  {
    id: "dev-12", name: "SQL-REPORTING", os: "windows-server", tenant: "Globex Corporation",
    status: "offline", cpu: 0, ram: 0, disk: 68, ip: "10.12.3.40", lastSync: "12m ago",
    cpuTrend: trend(34, 3), ramTrend: trend(35, 4), diskTrend: trend(36, 68),
  },
]

export const alerts: AlertEvent[] = [
  { id: "a1", device: "DB-PROD-01", message: "CPU utilization sustained above 90% for 10m", severity: "critical", time: "2m ago" },
  { id: "a2", device: "BACKUP-NAS-02", message: "Disk space low — 7% free on /volume1", severity: "critical", time: "5m ago" },
  { id: "a3", device: "SQL-REPORTING", message: "Agent disconnected — last heartbeat 12m ago", severity: "critical", time: "12m ago" },
  { id: "a4", device: "BUILD-AGENT-04", message: "CPU utilization spike detected (96%)", severity: "warning", time: "18m ago" },
  { id: "a5", device: "FILE-SRV-02", message: "Windows Update pending reboot", severity: "warning", time: "34m ago" },
  { id: "a6", device: "WEB-PROD-01", message: "Patch KB5034123 deployed successfully", severity: "info", time: "1h ago" },
  { id: "a7", device: "DC-01", message: "Scheduled script 'AD-Cleanup' completed", severity: "info", time: "2h ago" },
  { id: "a8", device: "CACHE-REDIS-01", message: "Memory usage returned to normal range", severity: "info", time: "3h ago" },
]

export const scripts = [
  "Restart Service",
  "Clear Temp Files",
  "Run Windows Update",
  "Flush DNS Cache",
  "Collect Diagnostics",
  "Reboot Endpoint",
]

export const deviceBackups: DeviceBackup[] = [
  {
    id: "bkp-001",
    deviceId: "dev-10",
    name: "Nightly-System-Image",
    type: "full",
    status: "completed",
    size: "412 GB",
    location: "s3://umbrella-backups/nas02/",
    cron: "0 2 * * *",
    createdAt: "2026-07-01 02:10 UTC",
  },
  {
    id: "bkp-002",
    deviceId: "dev-10",
    name: "Hourly-Delta",
    type: "incremental",
    status: "running",
    size: "18 GB",
    location: "s3://umbrella-backups/nas02/",
    cron: "0 * * * *",
    createdAt: "2026-07-01 10:00 UTC",
  },
  {
    id: "bkp-003",
    deviceId: "dev-02",
    name: "DB-Full-Snapshot",
    type: "full",
    status: "completed",
    size: "96 GB",
    location: "nfs://northwind-backup/db-prod-01",
    cron: "0 1 * * *",
    createdAt: "2026-07-01 01:00 UTC",
  },
  {
    id: "bkp-004",
    deviceId: "dev-02",
    name: "DB-Log-Chain",
    type: "incremental",
    status: "completed",
    size: "5.4 GB",
    location: "nfs://northwind-backup/db-prod-01",
    cron: "*/15 * * * *",
    createdAt: "2026-07-01 09:45 UTC",
  },
  {
    id: "bkp-005",
    deviceId: "dev-04",
    name: "FileSrv-Weekly",
    type: "full",
    status: "failed",
    size: "0 GB",
    location: "\\backup-core\\acme-filesrv",
    cron: "20 23 * * 1",
    createdAt: "2026-06-30 23:20 UTC",
  },
]
