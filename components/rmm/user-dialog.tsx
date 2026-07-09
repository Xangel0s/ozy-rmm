"use client"

import * as React from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { UserInfo } from "@/lib/api"

const userSchema = z.object({
  email: z.string().email("Invalid email"),
  username: z.string().min(2, "Username must be at least 2 characters"),
  fullName: z.string().optional(),
  password: z.string().min(8, "Password must be at least 8 characters").optional().or(z.literal("")),
  role: z.enum(["admin", "technician", "viewer"]),
})

type UserFormData = z.infer<typeof userSchema>

interface UserDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user?: UserInfo | null
  onSave: (data: UserFormData) => Promise<void>
}

export function UserDialog({ open, onOpenChange, user, onSave }: UserDialogProps) {
  const isEdit = !!user

  const {
    register,
    handleSubmit,
    reset,
    setValue,
    watch,
    formState: { errors, isSubmitting },
  } = useForm<UserFormData>({
    resolver: zodResolver(userSchema),
    defaultValues: {
      email: "",
      username: "",
      fullName: "",
      password: "",
      role: "technician",
    },
  })

  React.useEffect(() => {
    if (open) {
      if (user) {
        reset({
          email: user.email,
          username: user.username,
          fullName: user.fullName || "",
          password: "",
          role: user.role,
        })
      } else {
        reset({
          email: "",
          username: "",
          fullName: "",
          password: "",
          role: "technician",
        })
      }
    }
  }, [open, user, reset])

  const roleValue = watch("role")

  const onSubmit = async (data: UserFormData) => {
    const payload = { ...data }
    if (isEdit && !payload.password) {
      delete payload.password
    }
    await onSave(payload)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit User" : "Add User"}</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">Email</span>
            <Input {...register("email")} placeholder="user@example.com" />
            {errors.email && (
              <span className="text-xs text-destructive">{errors.email.message}</span>
            )}
          </label>

          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">Username</span>
            <Input {...register("username")} placeholder="jdoe" />
            {errors.username && (
              <span className="text-xs text-destructive">{errors.username.message}</span>
            )}
          </label>

          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">Full Name</span>
            <Input {...register("fullName")} placeholder="John Doe" />
          </label>

          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">
              {isEdit ? "New Password (leave blank to keep current)" : "Password"}
            </span>
            <Input
              type="password"
              {...register("password")}
              placeholder={isEdit ? "Leave blank to keep" : "Min. 8 characters"}
            />
            {errors.password && (
              <span className="text-xs text-destructive">{errors.password.message}</span>
            )}
          </label>

          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">Role</span>
            <Select
              value={roleValue}
              onValueChange={(v) => setValue("role", v as "admin" | "technician" | "viewer")}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select role" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="admin">Admin</SelectItem>
                <SelectItem value="technician">Technician</SelectItem>
                <SelectItem value="viewer">Viewer</SelectItem>
              </SelectContent>
            </Select>
            {errors.role && (
              <span className="text-xs text-destructive">{errors.role.message}</span>
            )}
          </label>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "Saving..." : isEdit ? "Save Changes" : "Create User"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
