"use client"

import * as React from "react"
import { Check, Copy, Download, Terminal } from "lucide-react"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { tenants } from "@/lib/rmm-data"
import { cn } from "@/lib/utils"

type Platform = "linux" | "windows"

const commands: Record<Platform, string> = {
  linux:
    'curl -fsSL https://agent.apexrmm.io/install.sh | sudo bash -s -- \\\n  --token=AX-8f2c91e4-northwind \\\n  --site="Production"',
  windows:
    'iwr https://agent.apexrmm.io/install.ps1 -UseBasicParsing | iex; \\\nInstall-ApexAgent -Token "AX-8f2c91e4-northwind" -Site "Production"',
}

export function DeployAgentDialog() {
  const [platform, setPlatform] = React.useState<Platform>("linux")
  const [copied, setCopied] = React.useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(commands[platform])
      setCopied(true)
      setTimeout(() => setCopied(false), 1600)
    } catch {
      /* clipboard unavailable */
    }
  }

  return (
    <Dialog>
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
            Run the command below on the endpoint to enroll it into ApexRMM. The
            token is scoped to the selected client.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">Assign to client</span>
            <Select defaultValue="northwind">
              <SelectTrigger className="w-full">
                <SelectValue>
                  {(value: string) => tenants.find((t) => t.id === value)?.name}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {tenants
                  .filter((t) => t.id !== "all")
                  .map((t) => (
                    <SelectItem key={t.id} value={t.id}>
                      {t.name}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
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
              <code>{commands[platform]}</code>
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
        </div>

        <DialogFooter>
          <Button>
            <Download data-icon="inline-start" />
            Download Installer
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
