"use client"

import * as React from "react"
import { toast } from "sonner"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { ConsoleShell } from "@/components/rmm/console-shell"
import { UsersTable } from "@/components/rmm/users-table"
import { UserDialog } from "@/components/rmm/user-dialog"
import { AuditLog } from "@/components/rmm/audit-log"
import { useUsers } from "@/lib/use-live-data"
import { createUser, updateUser, deleteUser } from "@/lib/api"
import type { UserInfo } from "@/lib/api"

export default function SettingsPage() {
  const { users, loading } = useUsers()

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editingUser, setEditingUser] = React.useState<UserInfo | null>(null)

  const handleCreate = () => {
    setEditingUser(null)
    setDialogOpen(true)
  }

  const handleEdit = (user: UserInfo) => {
    setEditingUser(user)
    setDialogOpen(true)
  }

  const handleSave = async (data: any) => {
    try {
      if (editingUser) {
        await updateUser(editingUser.id, data)
        toast.success("User updated", {
          description: `${data.email} has been updated.`,
        })
      } else {
        const res = await createUser(data)
        toast.success("User created", {
          description: `${data.email} (ID: ${res.id.substring(0, 8)}...)`,
        })
      }
    } catch (e: any) {
      toast.error("Error", {
        description: String(e?.message ?? e),
      })
    }
  }

  const handleDelete = async (user: UserInfo) => {
    if (!confirm(`Delete user "${user.email}"? This cannot be undone.`)) return

    try {
      await deleteUser(user.id)
      toast.success("User deleted", {
        description: `${user.email} has been removed.`,
      })
    } catch (e: any) {
      toast.error("Error", {
        description: String(e?.message ?? e),
      })
    }
  }

  return (
    <ConsoleShell title="Settings" subtitle="Platform configuration" showSearch={false}>
      <Tabs defaultValue="users">
        <TabsList>
          <TabsTrigger value="users">Users</TabsTrigger>
          <TabsTrigger value="audit">Audit</TabsTrigger>
          <TabsTrigger value="general">General</TabsTrigger>
        </TabsList>

        <TabsContent value="users">
          {loading ? (
            <p className="text-sm text-muted-foreground">Loading users...</p>
          ) : (
            <UsersTable
              users={users}
              onEdit={handleEdit}
              onDelete={handleDelete}
              onCreate={handleCreate}
            />
          )}

          <UserDialog
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            user={editingUser}
            onSave={handleSave}
          />
        </TabsContent>

        <TabsContent value="audit">
          <AuditLog />
        </TabsContent>

        <TabsContent value="general">
          <p className="text-sm text-muted-foreground">
            General settings coming soon.
          </p>
        </TabsContent>
      </Tabs>
    </ConsoleShell>
  )
}
