"use client"

import * as React from "react"
import { StickyNote, Plus, Trash2, Save } from "lucide-react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { fetchNotes, createNote, updateNote, deleteNote, type NoteItem } from "@/lib/api"

interface NotesTabProps {
  agentId: string;
}

export function NotesTab({ agentId }: NotesTabProps) {
  const [notes, setNotes] = React.useState<NoteItem[]>([])
  const [loading, setLoading] = React.useState(true)
  const [editingId, setEditingId] = React.useState<number | null>(null)
  const [editContent, setEditContent] = React.useState("")
  const [newContent, setNewContent] = React.useState("")
  const saveTimeoutRef = React.useRef<NodeJS.Timeout | null>(null)

  const loadNotes = React.useCallback(async () => {
    setLoading(true)
    try {
      const items = await fetchNotes(agentId)
      setNotes(items)
    } finally {
      setLoading(false)
    }
  }, [agentId])

  React.useEffect(() => {
    loadNotes()
  }, [loadNotes])

  React.useEffect(() => {
    return () => {
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current)
      }
    }
  }, [])

  const handleCreate = async () => {
    if (!newContent.trim()) return
    try {
      await createNote(agentId, newContent.trim())
      setNewContent("")
      toast.success("Note created")
      loadNotes()
    } catch {
      toast.error("Failed to create note")
    }
  }

  const handleEdit = (note: NoteItem) => {
    setEditingId(note.id)
    setEditContent(note.content)
  }

  const handleSave = async (noteId: number) => {
    if (!editContent.trim()) return
    try {
      await updateNote(noteId, editContent.trim())
      setEditingId(null)
      loadNotes()
    } catch {
      toast.error("Failed to save note")
    }
  }

  const handleDelete = async (noteId: number) => {
    try {
      await deleteNote(noteId)
      toast.success("Note deleted")
      loadNotes()
    } catch {
      toast.error("Failed to delete note")
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent, noteId: number) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      handleSave(noteId)
    }
  }

  return (
    <Card className="gap-4 p-4">
      <div className="flex items-center gap-2">
        <StickyNote className="size-4 text-primary" />
        <h2 className="text-sm font-semibold">Technician Notes</h2>
        <Badge variant="secondary">{notes.length}</Badge>
      </div>

      {/* New Note */}
      <div className="space-y-2">
        <textarea
          value={newContent}
          onChange={(e) => setNewContent(e.target.value)}
          placeholder="Add a note about this device..."
          className="w-full rounded-lg border border-border bg-muted/50 p-3 text-sm resize-none focus:outline-none focus:ring-2 focus:ring-ring"
          rows={3}
        />
        <Button
          size="sm"
          onClick={handleCreate}
          disabled={!newContent.trim()}
        >
          <Plus className="size-4 mr-1" />
          Add Note
        </Button>
      </div>

      {/* Notes List */}
      {loading ? (
        <div className="flex items-center justify-center py-8">
          <div className="size-6 animate-spin rounded-full border-2 border-muted border-t-primary" />
        </div>
      ) : notes.length === 0 ? (
        <div className="py-8 text-center text-sm text-muted-foreground">
          No notes yet. Add a note above to get started.
        </div>
      ) : (
        <div className="space-y-3">
          {notes.map((note) => (
            <div
              key={note.id}
              className="rounded-lg border border-border p-3 space-y-2"
            >
              <div className="flex items-center justify-between text-xs text-muted-foreground">
                <span>{note.userName}</span>
                <span>{new Date(note.updatedAt).toLocaleString()}</span>
              </div>

              {editingId === note.id ? (
                <div className="space-y-2">
                  <textarea
                    value={editContent}
                    onChange={(e) => setEditContent(e.target.value)}
                    onKeyDown={(e) => handleKeyDown(e, note.id)}
                    className="w-full rounded-lg border border-border bg-muted/50 p-3 text-sm resize-none focus:outline-none focus:ring-2 focus:ring-ring"
                    rows={4}
                    autoFocus
                  />
                  <div className="flex gap-2">
                    <Button size="sm" onClick={() => handleSave(note.id)}>
                      <Save className="size-4 mr-1" />
                      Save
                    </Button>
                    <Button size="sm" variant="ghost" onClick={() => setEditingId(null)}>
                      Cancel
                    </Button>
                    <span className="ml-auto text-xs text-muted-foreground self-center">
                      Ctrl+Enter to save
                    </span>
                  </div>
                </div>
              ) : (
                <div
                  className="cursor-pointer rounded-md p-2 text-sm hover:bg-muted/50"
                  onClick={() => handleEdit(note)}
                >
                  {note.content}
                </div>
              )}

              {editingId !== note.id && (
                <div className="flex justify-end">
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => handleDelete(note.id)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}
