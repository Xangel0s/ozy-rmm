"use client"

/**
 * lib/use-live-data.ts
 * React hooks that poll the Go backend and expose live data to the dashboard.
 *
 * Implementation: TanStack Query. Each hook declares a stable queryKey and a
 * refetchInterval. The QueryClientProvider lives in app/providers.tsx.
 *
 * Returns a stable shape { data, loading, error } so existing components that
 * previously used manual useEffect/setInterval polling keep working unchanged.
 */

import {
  useQuery,
  type UseQueryResult,
} from "@tanstack/react-query"
import {
  fetchAgents,
  fetchAlerts,
  fetchBackups,
  fetchTenants,
  fetchUsers,
  fetchAggregatedTelemetry,
  type AgentInfo,
  type AlertRow,
  type BackupJob,
  type Tenant,
  type UserInfo,
  type TelemetryBucket,
} from "@/lib/api"

export type { AlertRow } from "@/lib/api"
export type { BackupJob } from "@/lib/api"

// ─── useAgents ────────────────────────────────────────────────────────────────

/**
 * Polls /api/agents every 5 s.
 * Returns { agents, loading, error }.
 */
export function useAgents(): {
  agents: AgentInfo[]
  loading: boolean
  error: string | null
} {
  const q: UseQueryResult<AgentInfo[], Error> = useQuery({
    queryKey: ["agents"],
    queryFn: fetchAgents,
    refetchInterval: 5_000,
  })
  return {
    agents: q.data ?? [],
    loading: q.isLoading,
    error: q.error ? String(q.error.message ?? q.error) : null,
  }
}

// ─── useAlerts ────────────────────────────────────────────────────────────────

/**
 * Polls /api/alerts every 10 s.
 * Returns { alerts, loading, error }.
 */
export function useAlerts(): {
  alerts: AlertRow[]
  loading: boolean
  error: string | null
} {
  const q: UseQueryResult<AlertRow[], Error> = useQuery({
    queryKey: ["alerts"],
    queryFn: fetchAlerts,
    refetchInterval: 10_000,
  })
  return {
    alerts: q.data ?? [],
    loading: q.isLoading,
    error: q.error ? String(q.error.message ?? q.error) : null,
  }
}

// ─── useBackups ───────────────────────────────────────────────────────────────

/**
 * Polls /api/backups every 15 s.
 * Returns { backups, loading, error }.
 */
export function useBackups(): {
  backups: BackupJob[]
  loading: boolean
  error: string | null
} {
  const q: UseQueryResult<BackupJob[], Error> = useQuery({
    queryKey: ["backups"],
    queryFn: fetchBackups,
    refetchInterval: 15_000,
  })
  return {
    backups: q.data ?? [],
    loading: q.isLoading,
    error: q.error ? String(q.error.message ?? q.error) : null,
  }
}

// ─── useAgentTelemetry ───────────────────────────────────────────────────────────

/**
 * Fetches aggregated telemetry for a specific agent over a time range.
 * Polls every 30s. Interval can be 'hour', 'minute', or 'day'.
 */
export function useAgentTelemetry(
  agentId: string | null,
  from: string,
  to: string,
  interval: string,
): {
  buckets: TelemetryBucket[]
  loading: boolean
  error: string | null
} {
  const q: UseQueryResult<TelemetryBucket[], Error> = useQuery({
    queryKey: ["agentTelemetry", agentId, from, to, interval],
    queryFn: () => {
      if (!agentId) return Promise.resolve([])
      return fetchAggregatedTelemetry(agentId, from, to, interval)
    },
    enabled: !!agentId,
    refetchInterval: 30_000,
  })
  return {
    buckets: q.data ?? [],
    loading: q.isLoading,
    error: q.error ? String(q.error.message ?? q.error) : null,
  }
}

// ─── useTenants ───────────────────────────────────────────────────────────────

/**
 * Loads /api/tenants once (rarely changes). refetchInterval = 60 s.
 * Returns { tenants, loading, error }.
 */
export function useTenants(): {
  tenants: Tenant[]
  loading: boolean
  error: string | null
} {
  const q: UseQueryResult<Tenant[], Error> = useQuery({
    queryKey: ["tenants"],
    queryFn: fetchTenants,
    refetchInterval: 60_000,
  })
  return {
    tenants: q.data ?? [],
    loading: q.isLoading,
    error: q.error ? String(q.error.message ?? q.error) : null,
  }
}

// ─── useUsers ─────────────────────────────────────────────────────────────────

/**
 * Loads /api/users every 30 s.
 * Returns { users, loading, error }.
 */
export function useUsers(): {
  users: UserInfo[]
  loading: boolean
  error: string | null
} {
  const q: UseQueryResult<UserInfo[], Error> = useQuery({
    queryKey: ["users"],
    queryFn: fetchUsers,
    refetchInterval: 30_000,
  })
  return {
    users: q.data ?? [],
    loading: q.isLoading,
    error: q.error ? String(q.error.message ?? q.error) : null,
  }
}

// ─── agentToDevice ────────────────────────────────────────────────────────────

import type { Device } from "@/lib/types"

/**
 * Converts an AgentInfo from the backend into the Device shape used throughout
 * the existing Next.js components.  We keep the UI components unchanged — only
 * the data source switches from the static mock to the live API.
 */
export function agentToDevice(a: AgentInfo): Device {
  const ramPct = a.totalRam > 0 ? Math.round(((a.totalRam - a.freeRam) / a.totalRam) * 100) : 0
  const diskPct = a.diskTotal > 0 ? Math.round(((a.diskTotal - a.diskFree) / a.diskTotal) * 100) : 0

  // Determine the OS label that maps to the existing OS type enum
  function guessOs(raw: string): Device["os"] {
    const lower = raw.toLowerCase()
    if (lower.includes("ubuntu")) return "ubuntu-server"
    if (lower.includes("debian")) return "debian"
    if (lower.includes("mac")) return "macos"
    if (lower.includes("server")) return "windows-server"
    return "windows"
  }

  return {
    id: a.id,
    name: a.hostname || a.id,
    os: guessOs(a.os),
    tenant: a.tenantId || "Unknown",
    status: a.status === "online" ? "online" : "offline",
    cpu: Math.round(a.cpuLoad),
    ram: ramPct,
    disk: diskPct,
    cpuTrend: [],
    ramTrend: [],
    diskTrend: [],
    ip: a.localIP || a.id,
    lastSeen: a.lastSeen
      ? new Date(a.lastSeen).toLocaleTimeString()
      : "unknown",
    lastSync: a.lastSeen
      ? relativeTime(new Date(a.lastSeen))
      : "unknown",
    vendor: a.vendor,
    model: a.model,
    serialNumber: a.serialNumber,
    uptime: a.uptime,
    kernelVersion: a.kernelVersion,
    agentVersion: a.agentVersion,
    localIP: a.localIP,
    macAddress: a.macAddress,
    gateway: a.gateway,
    numCPU: a.numCPU,
    cpuModel: a.cpuModel,
    totalRam: a.totalRam,
    freeRam: a.freeRam,
    diskTotal: a.diskTotal,
    diskFree: a.diskFree,
  }
}

function relativeTime(d: Date): string {
  const secs = Math.round((Date.now() - d.getTime()) / 1000)
  if (secs < 60) return `${secs}s ago`
  if (secs < 3600) return `${Math.round(secs / 60)}m ago`
  return `${Math.round(secs / 3600)}h ago`
}
