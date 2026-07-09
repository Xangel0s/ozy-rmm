"use client"

import * as React from "react"
import { fetchNotes, createNote, type NoteItem } from "@/lib/api"
import { Card } from "@/components/ui/card"
import { Button } from "@/components/ui/button"

interface NotesTabProps {
  agentId: string
}

export function NotesTab({ agentId }: NotesTabProps) {
  const [items, setItems] = React.useState<NoteItem[]>([])
  const [content, setContent] = React.useState("")
  const [saving, setSaving] = React.useState(false)

  React.useEffect(() => {
    fetchNotes(agentId).then(setItems)
  }, [agentId])

  const handleSubmit = async () => {
    if (!content.trim()) return
    setSaving(true)
    const res = await createNote(agentId, content)
    if (res.id) {
      setContent("")
      const updated = await fetchNotes(agentId)
      setItems(updated)
    }
    setSaving(false)
  }

  return (
    <Card className="p-4">
      <h3 className="mb-3 text-sm font-semibold">Notes</h3>

      <div className="mb-3 flex gap-2">
        <textarea
          className="flex-1 rounded-lg border border-border bg-background p-2 text-sm outline-none focus:border-primary"
          rows={3}
          placeholder="Add a note..."
          value={content}
          onChange={(e) => setContent(e.target.value)}
          onKeyDown={(e) => { if (e.ctrlKey && e.key === "Enter") handleSubmit() }}
        />
        <Button size="sm" onClick={handleSubmit} disabled={saving || !content.trim()}>
          {saving ? "Saving..." : "Add"}
        </Button>
      </div>

      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">No notes yet.</p>
      ) : (
        <div className="flex flex-col gap-2">
          {items.map((n) => (
            <div key={n.id} className="rounded-lg border border-border p-3">
              <p className="whitespace-pre-wrap text-sm">{n.content}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {n.userName} &middot; {new Date(n.createdAt).toLocaleString()}
              </p>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}
