"use client"

import * as React from "react"
import { Package, Search, RefreshCw } from "lucide-react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { fetchSoftware, scanSoftware, type SoftwareItem } from "@/lib/api"

interface SoftwareTabProps {
  agentId: string;
}

export function SoftwareTab({ agentId }: SoftwareTabProps) {
  const [software, setSoftware] = React.useState<SoftwareItem[]>([])
  const [loading, setLoading] = React.useState(true)
  const [scanning, setScanning] = React.useState(false)
  const [search, setSearch] = React.useState("")
  const [page, setPage] = React.useState(0)
  const [total, setTotal] = React.useState(0)
  const [hasMore, setHasMore] = React.useState(false)
  const limit = 50

  const loadSoftware = React.useCallback(async (offset: number) => {
    setLoading(true)
    try {
      const res = await fetchSoftware(agentId, limit, offset)
      setSoftware(res.items)
      setTotal(res.total)
      setHasMore(res.hasMore)
    } finally {
      setLoading(false)
    }
  }, [agentId])

  React.useEffect(() => {
    loadSoftware(0)
  }, [loadSoftware])

  const handleScan = async () => {
    setScanning(true)
    try {
      await scanSoftware(agentId)
      toast.success("Software scan initiated", {
        description: "The agent will report back with results shortly.",
      })
      setTimeout(() => loadSoftware(0), 3000)
    } catch {
      toast.error("Failed to initiate scan")
    } finally {
      setScanning(false)
    }
  }

  const filtered = software.filter((s) =>
    s.name.toLowerCase().includes(search.toLowerCase()) ||
    s.publisher.toLowerCase().includes(search.toLowerCase())
  )

  const formatSize = (kb: number) => {
    if (kb <= 0) return "N/A"
    if (kb < 1024) return `${kb} KB`
    if (kb < 1048576) return `${(kb / 1024).toFixed(1)} MB`
    return `${(kb / 1048576).toFixed(1)} GB`
  }

  return (
    <Card className="gap-4 p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Package className="size-4 text-primary" />
          <h2 className="text-sm font-semibold">Installed Software</h2>
          <Badge variant="secondary">{total} items</Badge>
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

      <div className="relative">
        <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          placeholder="Search software..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="pl-9"
        />
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-8">
          <RefreshCw className="size-6 animate-spin text-muted-foreground" />
        </div>
      ) : filtered.length === 0 ? (
        <div className="py-8 text-center text-sm text-muted-foreground">
          {software.length === 0
            ? "No software data. Click 'Scan Now' to inventory installed applications."
            : "No matching software found."
          }
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs font-medium text-muted-foreground">
                <th className="pb-2 pr-4">Name</th>
                <th className="pb-2 pr-4">Publisher</th>
                <th className="pb-2 pr-4">Version</th>
                <th className="pb-2 pr-4">Size</th>
                <th className="pb-2">Installed</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((item) => (
                <tr key={item.id} className="border-b border-border/50">
                  <td className="py-2 pr-4 font-medium">{item.name}</td>
                  <td className="py-2 pr-4 text-muted-foreground">{item.publisher || "—"}</td>
                  <td className="py-2 pr-4 font-mono text-xs">{item.version || "—"}</td>
                  <td className="py-2 pr-4 text-muted-foreground">{formatSize(item.estimatedSizeKB)}</td>
                  <td className="py-2 text-muted-foreground">
                    {item.installDate
                      ? `${item.installDate.slice(0, 4)}-${item.installDate.slice(4, 6)}-${item.installDate.slice(6, 8)}`
                      : "—"
                    }
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {hasMore && !search && (
        <div className="flex justify-center gap-2 pt-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              const newPage = page - 1
              setPage(newPage)
              loadSoftware(newPage * limit)
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
              loadSoftware(newPage * limit)
            }}
          >
            Next
          </Button>
        </div>
      )}
    </Card>
  )
}
