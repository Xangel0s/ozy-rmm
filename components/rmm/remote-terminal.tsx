"use client"

import * as React from "react"
import { Terminal } from "xterm"
import { FitAddon } from "xterm-addon-fit"
import "xterm/css/xterm.css"

interface RemoteTerminalProps {
  agentId: string
}

export function RemoteTerminal({ agentId }: RemoteTerminalProps) {
  const terminalRef = React.useRef<HTMLDivElement>(null)
  const xtermRef = React.useRef<Terminal | null>(null)
  const wsRef = React.useRef<WebSocket | null>(null)

  React.useEffect(() => {
    if (!terminalRef.current) return

    // Initialize Xterm Terminal
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "Menlo, Monaco, 'Courier New', monospace",
      theme: {
        background: "#0c0a09", // Sleek dark stone
        foreground: "#f5f5f4",
      },
    })
    xtermRef.current = term

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)

    term.open(terminalRef.current)
    fitAddon.fit()

    term.writeln("Connecting to remote agent shell...")

    // Connect to WebSocket with token from localStorage
    const token = typeof window !== "undefined" ? localStorage.getItem("token") : ""
    const wsUrl = `ws://localhost:8080/terminal/ws?id=${encodeURIComponent(
      agentId
    )}&token=${encodeURIComponent(token || "")}`
    
    const ws = new WebSocket(wsUrl)
    wsRef.current = ws

    ws.onopen = () => {
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
        }
      } catch (err) {
        // Fallback for raw text
        term.write(event.data)
      }
    }

    ws.onclose = () => {
      term.writeln("\r\nConnection closed by host.")
    }

    ws.onerror = () => {
      term.writeln("\r\nWebSocket connection error.")
    }

    // Input from technician keyboard
    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        const payload = JSON.stringify({
          agentId: agentId,
          type: "terminal_input",
          payload: data,
        })
        ws.send(payload)
      }
    })

    const handleResize = () => {
      fitAddon.fit()
    }
    window.addEventListener("resize", handleResize)

    return () => {
      window.removeEventListener("resize", handleResize)
      ws.close()
      term.dispose()
    }
  }, [agentId])

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border bg-stone-950 p-4">
      <div className="flex items-center justify-between border-b border-stone-800 pb-2">
        <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          Interactive Live Terminal (SYSTEM Shell)
        </span>
        <div className="flex items-center gap-1.5">
          <span className="size-2 rounded-full bg-success animate-pulse" />
          <span className="text-[11px] text-muted-foreground font-medium">Session Connected</span>
        </div>
      </div>
      <div ref={terminalRef} className="h-96 w-full overflow-hidden rounded-md" />
    </div>
  )
}
