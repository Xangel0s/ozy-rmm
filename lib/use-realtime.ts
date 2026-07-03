"use client"

import * as React from "react"
import { toast } from "sonner"

const BACKEND = process.env.NEXT_PUBLIC_BACKEND_URL ?? "http://localhost:8080"
const WS_BACKEND = BACKEND.replace(/^http/, "ws")

export function useRealTimeNotifications() {
  React.useEffect(() => {
    let ws: WebSocket | null = null
    let reconnectTimeout: NodeJS.Timeout | null = null

    function connect() {
      const token = typeof window !== "undefined" ? localStorage.getItem("token") : ""
      if (!token) return

      const wsUrl = `${WS_BACKEND}/api/events/ws?token=${encodeURIComponent(token)}`
      ws = new WebSocket(wsUrl)

      ws.onopen = () => {
        console.log("WebSocket Event Hub connected.")
      }

      ws.onmessage = (event) => {
        try {
          const alert = JSON.parse(event.data)
          if (alert.type === "critical") {
            toast.error(`CRITICAL: ${alert.message}`, {
              description: `Agent: ${alert.agentId}`,
              duration: 8000,
            })
          } else if (alert.type === "warning") {
            toast.warning(`WARNING: ${alert.message}`, {
              description: `Agent: ${alert.agentId}`,
              duration: 6000,
            })
          } else {
            toast.info(alert.message, {
              description: `Agent: ${alert.agentId}`,
              duration: 4000,
            })
          }
        } catch (err) {
          console.error("Failed to parse Event Hub message:", err)
        }
      }

      ws.onclose = () => {
        console.log("WebSocket Event Hub disconnected. Retrying in 5s...")
        reconnectTimeout = setTimeout(connect, 5000)
      }

      ws.onerror = () => {
        ws?.close()
      }
    }

    connect()

    return () => {
      if (ws) ws.close()
      if (reconnectTimeout) clearTimeout(reconnectTimeout)
    }
  }, [])
}
