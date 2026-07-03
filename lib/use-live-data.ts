"use client"

/**
 * lib/use-live-data.ts
 * React hooks that poll the Go backend and expose live data to the dashboard.
 * All hooks return stable empty arrays/objects while data is loading, so the
 * UI renders correctly on both the first paint and subsequent refreshes.
 */

import * as React from "react"
import { fetchAgents, fetchAlerts, fetchBackups, type AgentInfo, type AlertRow, type BackupJob } from "@/lib/api"

export type { AlertRow } from "@/lib/api"
export type { BackupJob } from "@/lib/api"

// ─── useAgents ────────────────────────────────────────────────────────────────

/**
 * Polls /api/agents every `intervalMs` ms (default 5 s).
 * Returns { agents, loading, error }.
 */
export function useAgents(intervalMs = 5000) {
  const [agents, setAgents] = React.useState<AgentInfo[]>([])
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        const data = await fetchAgents()
        if (!cancelled) {
          setAgents(data ?? [])
          setError(null)
        }
      } catch (e) {
        if (!cancelled) setError(String(e))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    load()
    const id = setInterval(load, intervalMs)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [intervalMs])

  return { agents, loading, error }
}

// ─── useAlerts ────────────────────────────────────────────────────────────────

/**
 * Polls /api/alerts every `intervalMs` ms (default 10 s).
 * Returns { alerts, loading, error }.
 */
export function useAlerts(intervalMs = 10000) {
  const [alerts, setAlerts] = React.useState<AlertRow[]>([])
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        const data = await fetchAlerts()
        if (!cancelled) {
          setAlerts(data ?? [])
          setError(null)
        }
      } catch (e) {
        if (!cancelled) setError(String(e))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    load()
    const id = setInterval(load, intervalMs)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [intervalMs])

  return { alerts, loading, error }
}

// ─── useBackups ───────────────────────────────────────────────────────────────

/**
 * Polls /api/backups every `intervalMs` ms (default 15 s).
 * Returns { backups, loading, error }.
 */
export function useBackups(intervalMs = 15000) {
  const [backups, setBackups] = React.useState<BackupJob[]>([])
  const [loading, setLoading] = React.useState(true)
  const [error, setError] = React.useState<string | null>(null)

  React.useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        const data = await fetchBackups()
        if (!cancelled) {
          setBackups(data ?? [])
          setError(null)
        }
      } catch (e) {
        if (!cancelled) setError(String(e))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    load()
    const id = setInterval(load, intervalMs)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [intervalMs])

  return { backups, loading, error }
}

// ─── agentToDevice ────────────────────────────────────────────────────────────

import type { Device } from "@/lib/rmm-data"

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
