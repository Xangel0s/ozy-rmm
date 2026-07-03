"use client"

import * as React from "react"
import { Shield, RefreshCw } from "lucide-react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { fetchPatches, scanPatches, type PatchItem } from "@/lib/api"

interface PatchesTabProps {
  agentId: string;
}

const severityColors: Record<string, string> = {
  Critical: "bg-red-500/10 text-red-500",
  Important: "bg-orange-500/10 text-orange-500",
  Moderate: "bg-yellow-500/10 text-yellow-500",
  Low: "bg-blue-500/10 text-blue-500",
  Unspecified: "bg-gray-500/10 text-gray-500",
}

export function PatchesTab({ agentId }: PatchesTabProps) {
  const [patches, setPatches] = React.useState<PatchItem[]>([])
  const [loading, setLoading] = React.useState(true)
  const [scanning, setScanning] = React.useState(false)
  const [page, setPage] = React.useState(0)
  const [total, setTotal] = React.useState(0)
  const [hasMore, setHasMore] = React.useState(false)
  const limit = 50

  const loadPatches = React.useCallback(async (offset: number) => {
    setLoading(true)
    try {
      const res = await fetchPatches(agentId, limit, offset)
      setPatches(res.items)
      setTotal(res.total)
      setHasMore(res.hasMore)
    } finally {
      setLoading(false)
    }
  }, [agentId])

  React.useEffect(() => {
    loadPatches(0)
  }, [loadPatches])

  const handleScan = async () => {
    setScanning(true)
    try {
      await scanPatches(agentId)
      toast.success("Patch scan initiated", {
        description: "The agent will report back with results shortly.",
      })
      setTimeout(() => loadPatches(0), 5000)
    } catch {
      toast.error("Failed to initiate scan")
    } finally {
      setScanning(false)
    }
  }

  const installed = patches.filter((p) => p.installed).length
  const missing = patches.filter((p) => !p.installed).length

  return (
    <Card className="gap-4 p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Shield className="size-4 text-primary" />
          <h2 className="text-sm font-semibold">Windows Patches</h2>
          <Badge variant="secondary">{total} total</Badge>
          {installed > 0 && <Badge className="bg-green-500/10 text-green-500">{installed} installed</Badge>}
          {missing > 0 && <Badge className="bg-yellow-500/10 text-yellow-500">{missing} missing</Badge>}
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={handleScan}
          disabled={scanning}
        >
          <RefreshCw className={`size-4 ${scanning ? "animate-spin" : ""}`} />
          {scanning ? "Scanning..." : "Scan Now"}
        </Button>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-8">
          <RefreshCw className="size-6 animate-spin text-muted-foreground" />
        </div>
      ) : patches.length === 0 ? (
        <div className="py-8 text-center text-sm text-muted-foreground">
          No patch data. Click &apos;Scan Now&apos; to query installed updates.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs font-medium text-muted-foreground">
                <th className="pb-2 pr-4">KB ID</th>
                <th className="pb-2 pr-4">Name</th>
                <th className="pb-2 pr-4">Severity</th>
                <th className="pb-2 pr-4">Status</th>
                <th className="pb-2">Installed</th>
              </tr>
            </thead>
            <tbody>
              {patches.map((patch) => (
                <tr key={patch.id} className="border-b border-border/50">
                  <td className="py-2 pr-4 font-mono text-xs">{patch.kbId || "—"}</td>
                  <td className="py-2 pr-4 max-w-[300px] truncate">{patch.name || "—"}</td>
                  <td className="py-2 pr-4">
                    <span className={`inline-block rounded px-1.5 py-0.5 text-[10px] font-medium ${severityColors[patch.severity] || severityColors.Unspecified}`}>
                      {patch.severity || "Unknown"}
                    </span>
                  </td>
                  <td className="py-2 pr-4">
                    <Badge variant={patch.installed ? "default" : "secondary"}>
                      {patch.installed ? "Installed" : "Missing"}
                    </Badge>
                  </td>
                  <td className="py-2 text-muted-foreground text-xs">
                    {patch.installedAt || "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {hasMore && (
        <div className="flex justify-center gap-2 pt-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              const newPage = page - 1
              setPage(newPage)
              loadPatches(newPage * limit)
            }}
            disabled={page === 0}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              const newPage = page + 1
              setPage(newPage)
              loadPatches(newPage * limit)
            }}
          >
            Next
          </Button>
        </div>
      )}
    </Card>
  )
}
