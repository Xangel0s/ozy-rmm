"use client"

import * as React from "react"
import { Terminal } from "xterm"
import { FitAddon } from "xterm-addon-fit"
import "xterm/css/xterm.css"

const BACKEND = process.env.NEXT_PUBLIC_BACKEND_URL ?? "http://localhost:8080"
const WS_BACKEND = BACKEND.replace(/^http/, "ws")

interface RemoteTerminalProps {
  agentId: string
}

type ConnectionState = "connecting" | "connected" | "error"

export function RemoteTerminal({ agentId }: RemoteTerminalProps) {
  const terminalRef = React.useRef<HTMLDivElement>(null)
  const xtermRef = React.useRef<Terminal | null>(null)
  const wsRef = React.useRef<WebSocket | null>(null)
  const lineBufRef = React.useRef("")
  const [connectionState, setConnectionState] = React.useState<ConnectionState>(
    agentId ? "connecting" : "error"
  )

  React.useEffect(() => {
    if (!terminalRef.current) return
    if (!agentId) {
      setConnectionState("error")
      return
    }

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "Menlo, Monaco, 'Courier New', monospace",
      theme: {
        background: "#0c0a09",
        foreground: "#f5f5f4",
      },
    })
    xtermRef.current = term

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)

    term.open(terminalRef.current)
    fitAddon.fit()

    term.writeln("Connecting to remote agent shell...")

    const token = typeof window !== "undefined" ? localStorage.getItem("token") : ""
    const wsUrl = `${WS_BACKEND}/terminal/ws?id=${encodeURIComponent(
      agentId
    )}&token=${encodeURIComponent(token || "")}`

    let retryTimer: ReturnType<typeof setTimeout> | null = null
    let disposed = false

    function connect() {
      if (disposed) return
      lineBufRef.current = ""

      setConnectionState("connecting")
      const ws = new WebSocket(wsUrl)
      wsRef.current = ws

      ws.onopen = () => {
        if (disposed) return
        setConnectionState("connected")
        term.writeln("Connected to agent terminal successfully.\r\n")
      }

      ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)
          if (msg.agentId && msg.agentId !== agentId) {
            return
          }
          if (msg.type === "terminal_output") {
            term.write(msg.payload)
          } else if (msg.type === "terminal_closed") {
            term.writeln(`\r\n\x1b[33mSession closed: ${msg.payload}\x1b[0m`)
            ws.close()
          }
        } catch {
          term.write(event.data)
        }
      }

      ws.onclose = () => {
        if (disposed) return
        if (connectionState === "connected") {
          term.writeln("\r\nConnection closed by host.")
        }
        setConnectionState("error")
      }

      ws.onerror = () => {
        if (disposed) return
        term.writeln("\r\n\x1b[31mWebSocket connection error.\x1b[0m")
        ws.close()
        setConnectionState("error")
        if (!retryTimer) {
          retryTimer = setTimeout(() => {
            retryTimer = null
            if (!disposed) {
              term.writeln("\r\nRetrying connection...")
              connect()
            }
          }, 3000)
        }
      }
    }

    connect()

    term.onData((data) => {
      const ws = wsRef.current
      if (!ws || ws.readyState !== WebSocket.OPEN) return

      if (data === '\r') {
        const line = lineBufRef.current
        lineBufRef.current = ""
        term.write('\r\n')
        ws.send(JSON.stringify({
          agentId,
          type: "terminal_input",
          payload: line + '\r\n',
        }))
      } else if (data === '\x7f' || data === '\b') {
        if (lineBufRef.current.length > 0) {
          lineBufRef.current = lineBufRef.current.slice(0, -1)
          term.write('\b \b')
        }
      } else if (data === '\x03') {
        lineBufRef.current = ""
        term.write('^C\r\n')
        ws.send(JSON.stringify({
          agentId,
          type: "terminal_input",
          payload: '\x03',
        }))
      } else {
        lineBufRef.current += data
        term.write(data)
      }
    })

    const handleResize = () => {
      fitAddon.fit()
    }
    window.addEventListener("resize", handleResize)

    return () => {
      disposed = true
      if (retryTimer) clearTimeout(retryTimer)
      window.removeEventListener("resize", handleResize)
      wsRef.current?.close()
      term.dispose()
    }
  }, [agentId])

  const dotColor =
    connectionState === "connected"
      ? "bg-success"
      : connectionState === "connecting"
        ? "bg-yellow-500 animate-pulse"
        : "bg-destructive"

  const stateLabel =
    connectionState === "connected"
      ? "Session Connected"
      : connectionState === "connecting"
        ? "Connecting..."
        : "Disconnected"

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border bg-stone-950 p-4">
      <div className="flex items-center justify-between border-b border-stone-800 pb-2">
        <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          Interactive Live Terminal (SYSTEM Shell)
        </span>
        <div className="flex items-center gap-1.5">
          <span className={`size-2 rounded-full ${dotColor}`} />
          <span className="text-[11px] text-muted-foreground font-medium">{stateLabel}</span>
        </div>
      </div>
      <div ref={terminalRef} className="h-96 w-full overflow-hidden rounded-md" />
    </div>
  )
}
