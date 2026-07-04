/**
 * lib/api.ts
 * Client-side helpers to consume the Go backend REST API.
 * All functions are async and return typed data.
 */

const BACKEND = process.env.NEXT_PUBLIC_BACKEND_URL ?? "http://localhost:8080"

// ─── Types ────────────────────────────────────────────────────────────────────

export type AgentStatus = "online" | "offline"

export type AgentInfo = {
  id: string
  tenantId: string
  hostname: string
  os: string
  cpuModel: string
  cpuLoad: number
  totalRam: number
  freeRam: number
  diskTotal: number
  diskFree: number
  disks: Array<{ deviceID: string; size: number; freeSpace: number; label: string; filesystem: string }>
  status: AgentStatus
  lastSeen: string
  vendor: string
  model: string
  serialNumber: string
  uptime: string
  kernelVersion: string
  agentVersion: string
  localIP: string
  macAddress: string
  gateway: string
  numCPU: number
  gpuName: string
  gpuDriver: string
}

export type AlertRow = {
  id: number
  agentId: string
  severity: "critical" | "warning" | "info"
  message: string
  time: string
}

export type TelemetryRow = {
  cpuLoad: number
  totalRam: number
  freeRam: number
  diskTotal: number
  diskFree: number
  recordedAt: string
}

export type BackupJob = {
  id: number
  agentId: string
  name: string
  location: string
  type: "full" | "incremental"
  status: "completed" | "running" | "failed" | "pending"
  sizeBytes: number
  cron: string
  executedAt: string
  createdAt: string
}

export type UserInfo = {
  id: string
  email: string
  username: string
  fullName: string
  role: "admin" | "technician" | "viewer"
  isActive: boolean
  lastLogin: string
  createdAt: string
}

export type RegistrationToken = {
  token: string
  tokenId: string
  expiresAt: string
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

async function get<T>(path: string): Promise<T> {
  const token = typeof window !== "undefined" ? localStorage.getItem("token") : ""
  const headers: Record<string, string> = {
    Accept: "application/json",
  }
  if (token) {
    headers["Authorization"] = `Bearer ${token}`
  }

  const res = await fetch(`${BACKEND}${path}`, {
    cache: "no-store",
    headers,
  })
  if (!res.ok) throw new Error(`API ${path} responded with ${res.status}`)
  return res.json() as Promise<T>
}

async function post<T>(path: string, body?: unknown): Promise<T> {
  const token = typeof window !== "undefined" ? localStorage.getItem("token") : ""
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Accept: "application/json",
  }
  if (token) {
    headers["Authorization"] = `Bearer ${token}`
  }

  const res = await fetch(`${BACKEND}${path}`, {
    method: "POST",
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "Unknown error" }))
    throw new Error(err.error || `API ${path} responded with ${res.status}`)
  }
  return res.json() as Promise<T>
}

// ─── Auth ─────────────────────────────────────────────────────────────────────

export async function authenticate(email: string, password: string): Promise<boolean> {
  try {
    const res = await fetch(`${BACKEND}/api/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    })
    if (!res.ok) return false
    const data = await res.json()
    if (data.token) {
      localStorage.setItem("token", data.token)
      localStorage.setItem("userId", data.userId)
      localStorage.setItem("tenantId", data.tenantId)
      localStorage.setItem("role", data.role)
      document.cookie = `token=${data.token}; path=/; max-age=86400; SameSite=Lax;`
      return true
    }
    return false
  } catch {
    return false
  }
}

export function logout() {
  localStorage.removeItem("token")
  localStorage.removeItem("userId")
  localStorage.removeItem("tenantId")
  localStorage.removeItem("role")
  localStorage.removeItem("fullName")
  document.cookie = "token=; path=/; expires=Thu, 01 Jan 1970 00:00:01 GMT;"
}

export function getCurrentUser() {
  if (typeof window === "undefined") return null
  return {
    userId: localStorage.getItem("userId"),
    tenantId: localStorage.getItem("tenantId"),
    role: localStorage.getItem("role") as "admin" | "technician" | "viewer" | null,
    fullName: localStorage.getItem("fullName"),
  }
}

// ─── API Functions ────────────────────────────────────────────────────────────

export async function fetchAgents(): Promise<AgentInfo[]> {
  try {
    return await get<AgentInfo[]>("/api/agents")
  } catch {
    return []
  }
}

export async function fetchAgentTelemetry(id: string): Promise<TelemetryRow[]> {
  try {
    return await get<TelemetryRow[]>(`/api/agents/telemetry?id=${encodeURIComponent(id)}`)
  } catch {
    return []
  }
}

export async function fetchAlerts(): Promise<AlertRow[]> {
  try {
    return await get<AlertRow[]>("/api/alerts")
  } catch {
    return []
  }
}

export async function fetchAlertsByAgent(agentId: string, limit = 10): Promise<AlertRow[]> {
  try {
    return await get<AlertRow[]>(`/api/alerts?agent_id=${encodeURIComponent(agentId)}&limit=${limit}`)
  } catch {
    return []
  }
}

export async function fetchAlert(id: string): Promise<AlertRow | null> {
  try {
    return await get<AlertRow>(`/api/alerts/detail?id=${encodeURIComponent(id)}`)
  } catch {
    return null
  }
}

export async function fetchAgent(id: string): Promise<AgentInfo | null> {
  try {
    return await get<AgentInfo>(`/api/agents/detail?id=${encodeURIComponent(id)}`)
  } catch {
    return null
  }
}

export async function acknowledgeAlert(alertId: number): Promise<boolean> {
  try {
    await post("/api/alerts/acknowledge", { alertId })
    return true
  } catch {
    return false
  }
}

export async function fetchBackups(): Promise<BackupJob[]> {
  try {
    return await get<BackupJob[]>("/api/backups")
  } catch {
    return []
  }
}

export async function runBackup(agentId: string): Promise<void> {
  try {
    await post("/api/backups/run", { agentId })
  } catch {
    // silently ignore
  }
}

// ─── Enrollment ───────────────────────────────────────────────────────────────

export async function createRegistrationToken(label?: string): Promise<RegistrationToken> {
  return await post<RegistrationToken>("/api/agents/register-token", {
    label: label || "Agent Enrollment Token",
    maxUses: 1,
    expiryHours: 24,
  })
}

export async function fetchUsers(): Promise<UserInfo[]> {
  try {
    return await get<UserInfo[]>("/api/users")
  } catch {
    return []
  }
}

// ─── Software ────────────────────────────────────────────────────────────────

export type SoftwareItem = {
  id: number
  name: string
  publisher: string
  version: string
  installDate: string
  estimatedSizeKB: number
  quietUninstallString: string
  scannedAt: string
}

export type SoftwareResponse = {
  items: SoftwareItem[]
  total: number
  limit: number
  offset: number
  hasMore: boolean
}

export async function fetchSoftware(agentId: string, limit = 50, offset = 0): Promise<SoftwareResponse> {
  try {
    return await get<SoftwareResponse>(`/api/agents/${agentId}/software?limit=${limit}&offset=${offset}`)
  } catch {
    return { items: [], total: 0, limit, offset, hasMore: false }
  }
}

export async function scanSoftware(agentId: string): Promise<void> {
  try {
    await post(`/api/agents/${agentId}/software/scan`)
  } catch {
    // silently ignore
  }
}

export async function uninstallSoftware(agentId: string, softwareId: string): Promise<{ status: string }> {
  return await post<{ status: string }>(`/api/agents/${agentId}/software/${softwareId}/uninstall`)
}

// ─── Notes ───────────────────────────────────────────────────────────────────

export type NoteItem = {
  id: number
  content: string
  userName: string
  createdAt: string
  updatedAt: string
}

export async function fetchNotes(agentId: string): Promise<NoteItem[]> {
  try {
    return await get<NoteItem[]>(`/api/agents/${agentId}/notes`)
  } catch {
    return []
  }
}

export async function createNote(agentId: string, content: string): Promise<{ id: number }> {
  return await post<{ id: number }>(`/api/agents/${agentId}/notes`, { content })
}

export async function updateNote(noteId: number, content: string): Promise<void> {
  try {
    await fetch(`${BACKEND}/api/notes/${noteId}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${typeof window !== "undefined" ? localStorage.getItem("token") : ""}`,
      },
      body: JSON.stringify({ content }),
    })
  } catch {
    // silently ignore
  }
}

export async function deleteNote(noteId: number): Promise<void> {
  try {
    await fetch(`${BACKEND}/api/notes/${noteId}`, {
      method: "DELETE",
      headers: {
        Authorization: `Bearer ${typeof window !== "undefined" ? localStorage.getItem("token") : ""}`,
      },
    })
  } catch {
    // silently ignore
  }
}

// ─── Logs ────────────────────────────────────────────────────────────────────

export type LogEntry = {
  id: number
  level: string
  logType: string
  message: string
  createdAt: string
}

export type LogsResponse = {
  items: LogEntry[]
  cursor: string
  hasMore: boolean
}

export async function fetchLogs(agentId: string, level?: string, cursor?: string, limit = 100): Promise<LogsResponse> {
  try {
    let path = `/api/agents/${agentId}/logs?limit=${limit}`
    if (level) path += `&level=${level}`
    if (cursor) path += `&cursor=${encodeURIComponent(cursor)}`
    return await get<LogsResponse>(path)
  } catch {
    return { items: [], cursor: "", hasMore: false }
  }
}

// ─── Audit ───────────────────────────────────────────────────────────────────

export type AuditEntry = {
  id: number
  action: string
  resourceType: string
  resourceId: string
  details: Record<string, unknown>
  ipAddress: string
  createdAt: string
}

export type AuditResponse = {
  items: AuditEntry[]
  total: number
  limit: number
  offset: number
  hasMore: boolean
}

export async function fetchAudit(agentId: string, limit = 50, offset = 0): Promise<AuditResponse> {
  try {
    return await get<AuditResponse>(`/api/agents/${agentId}/audit?limit=${limit}&offset=${offset}`)
  } catch {
    return { items: [], total: 0, limit, offset, hasMore: false }
  }
}

// ─── Patches ─────────────────────────────────────────────────────────────────

export type PatchItem = {
  id: number
  kbId: string
  name: string
  severity: string
  description: string
  installed: boolean
  installedAt: string
  scannedAt: string
}

export type PatchesResponse = {
  items: PatchItem[]
  total: number
  limit: number
  offset: number
  hasMore: boolean
}

export async function fetchPatches(agentId: string, limit = 50, offset = 0): Promise<PatchesResponse> {
  try {
    return await get<PatchesResponse>(`/api/agents/${agentId}/patches?limit=${limit}&offset=${offset}`)
  } catch {
    return { items: [], total: 0, limit, offset, hasMore: false }
  }
}

export async function scanPatches(agentId: string): Promise<void> {
  try {
    await post(`/api/agents/${agentId}/patches/scan`)
  } catch {
    // silently ignore
  }
}

// ─── Checks ──────────────────────────────────────────────────────────────────

export type CheckResult = {
  id: number
  checkType: string
  description: string
  config: Record<string, unknown>
  status: string
  lastOutput: string
  lastRun: string
  enabled: boolean
}

export async function fetchChecks(agentId: string): Promise<CheckResult[]> {
  try {
    return await get<CheckResult[]>(`/api/agents/${agentId}/checks`)
  } catch {
    return []
  }
}

export async function createCheck(agentId: string, checkType: string, description: string, config: Record<string, unknown>): Promise<{ id: number }> {
  return await post<{ id: number }>(`/api/agents/${agentId}/checks`, { checkType, description, config })
}

export async function updateCheck(checkId: number, data: { description?: string; config?: Record<string, unknown>; enabled?: boolean }): Promise<void> {
  try {
    await fetch(`${BACKEND}/api/checks/${checkId}`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${typeof window !== "undefined" ? localStorage.getItem("token") : ""}`,
      },
      body: JSON.stringify(data),
    })
  } catch {
    // silently ignore
  }
}

export async function deleteCheck(checkId: number): Promise<void> {
  try {
    await fetch(`${BACKEND}/api/checks/${checkId}`, {
      method: "DELETE",
      headers: {
        Authorization: `Bearer ${typeof window !== "undefined" ? localStorage.getItem("token") : ""}`,
      },
    })
  } catch {
    // silently ignore
  }
}

export async function runCheck(checkId: number): Promise<void> {
  try {
    await fetch(`${BACKEND}/api/checks/${checkId}/run`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${typeof window !== "undefined" ? localStorage.getItem("token") : ""}`,
      },
    })
  } catch {
    // silently ignore
  }
}
