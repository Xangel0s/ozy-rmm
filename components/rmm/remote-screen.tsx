"use client"

import * as React from "react"

const BACKEND = process.env.NEXT_PUBLIC_BACKEND_URL ?? "http://localhost:8080"
const WS_BACKEND = BACKEND.replace(/^http/, "ws")

interface RemoteScreenProps {
  agentId: string
}

type ConnectionState = "connecting" | "connected" | "error"

export function RemoteScreen({ agentId }: RemoteScreenProps) {
  const canvasRef = React.useRef<HTMLCanvasElement>(null)
  const wsRef = React.useRef<WebSocket | null>(null)
  const [connectionState, setConnectionState] = React.useState<ConnectionState>(
    agentId ? "connecting" : "error"
  )

  React.useEffect(() => {
    if (!agentId) {
      setConnectionState("error")
      return
    }

    const token = typeof window !== "undefined" ? localStorage.getItem("token") : ""
    const wsUrl = `${WS_BACKEND}/screen/ws?id=${encodeURIComponent(agentId)}&token=${encodeURIComponent(token || "")}`

    let retryTimer: ReturnType<typeof setTimeout> | null = null
    let disposed = false

    function connect() {
      if (disposed) return
      setConnectionState("connecting")
      const ws = new WebSocket(wsUrl)
      wsRef.current = ws

      ws.onopen = () => {
        if (disposed) return
        setConnectionState("connected")
      }

      ws.onmessage = (event) => {
        if (disposed) return
        try {
          const msg = JSON.parse(event.data)
          if (msg.agentId && msg.agentId !== agentId) return

          if (msg.type === "screen_frame" && msg.payload) {
            const img = new Image()
            img.onload = () => {
              const canvas = canvasRef.current
              if (!canvas) return
              const ctx = canvas.getContext("2d")
              if (!ctx) return
              canvas.width = img.width
              canvas.height = img.height
              ctx.drawImage(img, 0, 0)
              img.src = ""
            }
            img.src = `data:image/jpeg;base64,${msg.payload}`
          } else if (msg.type === "screen_error") {
            setConnectionState("error")
          }
        } catch {
          // ignore malformed messages
        }
      }

      ws.onclose = () => {
        if (disposed) return
        setConnectionState("error")
        if (!retryTimer) {
          retryTimer = setTimeout(() => {
            retryTimer = null
            if (!disposed) connect()
          }, 3000)
        }
      }

      ws.onerror = () => {
        ws.close()
        setConnectionState("error")
      }
    }

    connect()

    return () => {
      disposed = true
      if (retryTimer) clearTimeout(retryTimer)
      wsRef.current?.close()
    }
  }, [agentId])

  const canvasRefForEvents = canvasRef

  function sendInput(type: string, payload: Record<string, unknown>) {
    const ws = wsRef.current
    if (!ws || ws.readyState !== WebSocket.OPEN) return
    ws.send(JSON.stringify({
      agentId,
      type: "screen_input",
      payload: JSON.stringify({ type, ...payload }),
    }))
  }

  function getCanvasCoords(e: React.MouseEvent<HTMLCanvasElement>) {
    const canvas = canvasRef.current
    if (!canvas) return { x: 0, y: 0 }
    const rect = canvas.getBoundingClientRect()
    const scaleX = canvas.width / rect.width
    const scaleY = canvas.height / rect.height
    return {
      x: Math.round((e.clientX - rect.left) * scaleX),
      y: Math.round((e.clientY - rect.top) * scaleY),
    }
  }

  const handleMouseMove = React.useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const { x, y } = getCanvasCoords(e)
    sendInput("mouse_move", { x, y })
  }, [agentId])

  const handleMouseDown = React.useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const { x, y } = getCanvasCoords(e)
    const button = ["left", "middle", "right"][e.button] || "left"
    sendInput("mouse_down", { x, y, button })
  }, [agentId])

  const handleMouseUp = React.useCallback((e: React.MouseEvent<HTMLCanvasElement>) => {
    const { x, y } = getCanvasCoords(e)
    const button = ["left", "middle", "right"][e.button] || "left"
    sendInput("mouse_up", { x, y, button })
  }, [agentId])

  const handleWheel = React.useCallback((e: React.WheelEvent<HTMLCanvasElement>) => {
    sendInput("mouse_scroll", { delta: e.deltaY })
  }, [agentId])

  const handleKeyDown = React.useCallback((e: React.KeyboardEvent<HTMLCanvasElement>) => {
    e.preventDefault()
    sendInput("key_down", { key: e.key, code: e.code, alt: e.altKey, ctrl: e.ctrlKey, shift: e.shiftKey, meta: e.metaKey })
  }, [agentId])

  const handleKeyUp = React.useCallback((e: React.KeyboardEvent<HTMLCanvasElement>) => {
    e.preventDefault()
    sendInput("key_up", { key: e.key, code: e.code, alt: e.altKey, ctrl: e.ctrlKey, shift: e.shiftKey, meta: e.metaKey })
  }, [agentId])

  const dotColor =
    connectionState === "connected"
      ? "bg-success"
      : connectionState === "connecting"
        ? "bg-yellow-500 animate-pulse"
        : "bg-destructive"

  const stateLabel =
    connectionState === "connected"
      ? "Connected"
      : connectionState === "connecting"
        ? "Connecting..."
        : "Disconnected"

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border bg-stone-950 p-4">
      <div className="flex items-center justify-between border-b border-stone-800 pb-2">
        <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          Remote Screen
        </span>
        <div className="flex items-center gap-1.5">
          <span className={`size-2 rounded-full ${dotColor}`} />
          <span className="text-[11px] text-muted-foreground font-medium">{stateLabel}</span>
        </div>
      </div>
      <div className="relative flex items-center justify-center overflow-hidden rounded-md bg-stone-950">
        <canvas
          ref={canvasRef}
          className="max-h-[70vh] w-full cursor-crosshair object-contain"
          tabIndex={0}
          onMouseMove={connectionState === "connected" ? handleMouseMove : undefined}
          onMouseDown={connectionState === "connected" ? handleMouseDown : undefined}
          onMouseUp={connectionState === "connected" ? handleMouseUp : undefined}
          onWheel={connectionState === "connected" ? handleWheel : undefined}
          onKeyDown={connectionState === "connected" ? handleKeyDown : undefined}
          onKeyUp={connectionState === "connected" ? handleKeyUp : undefined}
        />
        {connectionState !== "connected" && (
          <div className="absolute inset-0 flex items-center justify-center bg-stone-950/80 text-sm text-muted-foreground">
            {connectionState === "connecting" ? "Connecting to remote screen..." : "Disconnected"}
          </div>
        )}
      </div>
      <p className="text-[11px] text-muted-foreground">
        {connectionState === "connected"
          ? "Click the screen to send mouse and keyboard input."
          : "Reconnecting automatically..."}
      </p>
    </div>
  )
}
