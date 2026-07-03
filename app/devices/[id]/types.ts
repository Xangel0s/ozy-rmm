// ─── Shared Types ─────────────────────────────────────────────────────────────

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  limit: number;
  offset?: number;
  cursor?: string;
  hasMore: boolean;
}

// ─── Summary Tab ──────────────────────────────────────────────────────────────

export interface DeviceSummary {
  id: string;
  hostname: string;
  status: "online" | "offline";
  lastSeen: string;
  os: string;
  kernelVersion: string;
  agentVersion: string;
  uptime: string;
  cpuModel: string;
  numCPU: number;
  cpuLoad: number;
  totalRAM: number;
  freeRam: number;
  gpuName: string;
  gpuDriver: string;
  vendor: string;
  model: string;
  serialNumber: string;
  localIP: string;
  macAddress: string;
  gateway: string;
  disks: DiskPartition[];
  recentAlerts: AlertSummary[];
  recentBackups: BackupSummary[];
}

export interface DiskPartition {
  deviceID: string;
  size: number;
  freeSpace: number;
  label: string;
  filesystem: string;
}

export interface AlertSummary {
  id: string;
  message: string;
  severity: "critical" | "warning" | "info";
  time: string;
}

export interface BackupSummary {
  id: string;
  name: string;
  status: "completed" | "running" | "failed";
  size: string;
  lastRun: string;
}

// ─── Software Tab ─────────────────────────────────────────────────────────────

export interface SoftwareItem {
  id: number;
  name: string;
  publisher: string;
  version: string;
  installDate: string;
  estimatedSizeKB: number | null;
  quietUninstallString: string;
  scannedAt: string;
}

// ─── Notes Tab ────────────────────────────────────────────────────────────────

export interface NoteItem {
  id: number;
  content: string;
  userId: string;
  userName: string;
  createdAt: string;
  updatedAt: string;
}

// ─── Assets Tab ───────────────────────────────────────────────────────────────

export interface AssetKV {
  key: string;
  value: string;
  source: string;
}

// ─── Debug Tab ────────────────────────────────────────────────────────────────

export interface LogEntry {
  id: number;
  level: "info" | "warning" | "error" | "debug";
  logType: string;
  message: string;
  createdAt: string;
}

// ─── Patches Tab ──────────────────────────────────────────────────────────────

export interface PatchItem {
  id: number;
  kbId: string;
  name: string;
  severity: string;
  description: string;
  installed: boolean;
  installedAt: string;
  scannedAt: string;
}

// ─── Checks Tab ───────────────────────────────────────────────────────────────

export interface CheckResult {
  id: number;
  checkType: string;
  description: string;
  status: "pass" | "fail" | "error" | "pending";
  lastOutput: string;
  lastRun: string;
  enabled: boolean;
  config: Record<string, unknown>;
}

// ─── Audit Tab ────────────────────────────────────────────────────────────────

export interface AuditEntry {
  id: number;
  action: string;
  resourceType: string;
  resourceId: string;
  details: Record<string, unknown>;
  userName: string;
  ipAddress: string;
  createdAt: string;
}
