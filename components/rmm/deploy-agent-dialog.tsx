"use client"

import * as React from "react"
import { Check, Copy, Download, Loader2, Terminal } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { createRegistrationToken } from "@/lib/api"
import { cn } from "@/lib/utils"
import { toast } from "sonner"

type Platform = "linux" | "windows"

export function DeployAgentDialog() {
  const [platform, setPlatform] = React.useState<Platform>("linux")
  const [copied, setCopied] = React.useState(false)
  const [loading, setLoading] = React.useState(false)
  const [tokenData, setTokenData] = React.useState<{ token: string; expiresAt: string } | null>(null)

  const generateToken = async () => {
    setLoading(true)
    try {
      const data = await createRegistrationToken("Agent Deployment Token")
      setTokenData(data)
      toast.success("Registration token generated", {
        description: `Expires: ${new Date(data.expiresAt).toLocaleString()}`,
      })
    } catch (err) {
      toast.error("Failed to generate token", {
        description: err instanceof Error ? err.message : "Unknown error",
      })
    } finally {
      setLoading(false)
    }
  }

  const getCommand = (p: Platform, token: string) => {
    if (p === "linux") {
      return `curl -fsSL https://agent.apexrmm.io/install.sh | sudo bash -s -- \\\n  --token=${token} \\\n  --backend=${window.location.hostname}:8080`
    }
    return `iwr https://agent.apexrmm.io/install.ps1 -UseBasicParsing | iex; \\\nInstall-ApexAgent -Token "${token}" -Backend "${window.location.hostname}:8080"`
  }

  const copy = async () => {
    if (!tokenData) return
    try {
      await navigator.clipboard.writeText(getCommand(platform, tokenData.token))
      setCopied(true)
      setTimeout(() => setCopied(false), 1600)
    } catch {
      /* clipboard unavailable */
    }
  }

  return (
    <Dialog onOpenChange={(open) => { if (open && !tokenData) generateToken() }}>
      <DialogTrigger
        render={
          <Button size="sm">
            <Download data-icon="inline-start" />
            Deploy New Agent
          </Button>
        }
      />
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Terminal className="size-4 text-primary" />
            Deploy Monitoring Agent
          </DialogTitle>
          <DialogDescription>
            Generate a one-time enrollment token and run the command on the target endpoint.
            The token expires in 24 hours and can only be used once.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-3">
          {tokenData && (
            <>
              <div className="flex items-center gap-2 rounded-lg bg-destructive/10 p-2 text-xs text-destructive">
                <span className="font-semibold">Token:</span>
                <code className="flex-1 truncate font-mono">{tokenData.token}</code>
                <span className="shrink-0">Expires: {new Date(tokenData.expiresAt).toLocaleTimeString()}</span>
              </div>

              <div className="flex w-fit items-center gap-1 rounded-lg bg-secondary p-0.5">
                {(["linux", "windows"] as Platform[]).map((p) => (
                  <button
                    key={p}
                    type="button"
                    onClick={() => setPlatform(p)}
                    className={cn(
                      "rounded-md px-3 py-1 text-xs font-medium capitalize transition-colors",
                      platform === p
                        ? "bg-card text-foreground ring-1 ring-border"
                        : "text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {p === "linux" ? "Linux / macOS" : "Windows (PowerShell)"}
                  </button>
                ))}
              </div>

              <div className="relative rounded-lg bg-secondary/60 ring-1 ring-border">
                <pre className="overflow-hidden whitespace-pre-wrap break-words p-3 pr-11 font-mono text-xs leading-relaxed text-foreground">
                  <code>{getCommand(platform, tokenData.token)}</code>
                </pre>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={copy}
                  className="absolute top-2 right-2"
                  aria-label="Copy install command"
                >
                  {copied ? <Check className="text-success" /> : <Copy />}
                </Button>
              </div>
            </>
          )}

          {loading && (
            <div className="flex items-center justify-center gap-2 py-4 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              Generating enrollment token...
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => { setTokenData(null); generateToken() }} disabled={loading}>
            Generate New Token
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
