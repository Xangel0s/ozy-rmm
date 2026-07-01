/**
 * lib/api.ts
 * Client-side helpers to consume the Go backend REST API.
 * All functions are async and return typed data.
 */

const BACKEND = process.env.NEXT_PUBLIC_BACKEND_URL ?? "http://localhost:8080"

// ─── Types returned by the backend ───────────────────────────────────────────

export type AgentStatus = "online" | "offline"

export type AgentInfo = {
  id: string
  hostname: string
  os: string
  cpuModel: string
  cpuLoad: number
  totalRam: number
  freeRam: number
  diskTotal: number
  diskFree: number
  status: AgentStatus
  lastSeen: string
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

// ─── Helpers ─────────────────────────────────────────────────────────────────

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${BACKEND}${path}`, {
    cache: "no-store",
    headers: { Accept: "application/json" },
  })
  if (!res.ok) throw new Error(`API ${path} responded with ${res.status}`)
  return res.json() as Promise<T>
}

// ─── API Functions ────────────────────────────────────────────────────────────

/** Returns all agents with their latest telemetry snapshot. */
export async function fetchAgents(): Promise<AgentInfo[]> {
  try {
    return await get<AgentInfo[]>("/api/agents")
  } catch {
    return []
  }
}

/** Returns the last 100 telemetry rows for a given agent. */
export async function fetchAgentTelemetry(id: string): Promise<TelemetryRow[]> {
  try {
    return await get<TelemetryRow[]>(`/api/agents/telemetry?id=${encodeURIComponent(id)}`)
  } catch {
    return []
  }
}

/** Returns the 50 most recent alerts across all agents. */
export async function fetchAlerts(): Promise<AlertRow[]> {
  try {
    return await get<AlertRow[]>("/api/alerts")
  } catch {
    return []
  }
}

/** Returns all backup jobs from the DB. */
export async function fetchBackups(): Promise<BackupJob[]> {
  try {
    return await get<BackupJob[]>("/api/backups")
  } catch {
    return []
  }
}

/** Triggers a manual backup for the given agentId. */
export async function runBackup(agentId: string): Promise<void> {
  try {
    await fetch(`${BACKEND}/api/backups/run`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ agentId }),
    })
  } catch {
    // silently ignore
  }
}
