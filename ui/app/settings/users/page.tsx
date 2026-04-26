"use client"

import { FormEvent, useCallback, useEffect, useState } from "react"
import { ShieldAlert, UserPlus } from "lucide-react"
import { useSettingsContext } from "@/components/settings-shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { createRegistryUser, listRegistryUsers, type RegistryUser } from "@/lib/admin-api"
import { clearStoredSession } from "@/lib/session"

function isAuthenticationError(message: string) {
  return message.toLowerCase().includes("authentication")
}

function formatCreatedAt(createdAt?: string) {
  if (!createdAt || createdAt.startsWith("0001-01-01")) {
    return null
  }
  return `created ${new Date(createdAt).toLocaleString()}`
}

export default function UsersSettingsPage() {
  const { isAdmin } = useSettingsContext()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [users, setUsers] = useState<RegistryUser[]>([])
  const [newUsername, setNewUsername] = useState("")
  const [newPassword, setNewPassword] = useState("")

  const refresh = useCallback(async () => {
    if (!isAdmin) {
      setLoading(false)
      return
    }

    try {
      setLoading(true)
      setError(null)
      setUsers(await listRegistryUsers())
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to load users"
      setError(message)
      if (isAuthenticationError(message)) {
        clearStoredSession()
        window.location.replace("/login")
      }
    } finally {
      setLoading(false)
    }
  }, [isAdmin])

  useEffect(() => {
    void refresh()
  }, [refresh])

  async function handleCreateUser(event: FormEvent) {
    event.preventDefault()
    try {
      setError(null)
      await createRegistryUser(newUsername, newPassword, "user")
      setNewUsername("")
      setNewPassword("")
      setUsers(await listRegistryUsers())
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create user")
    }
  }

  if (!isAdmin) {
    return (
      <section className="rounded-3xl border bg-background p-6 shadow-sm">
        <div className="flex items-center gap-2">
          <ShieldAlert className="h-4 w-4" />
          <h2 className="text-lg font-semibold">Admin only</h2>
        </div>
        <p className="mt-4 text-sm text-muted-foreground">
          User creation and role management are restricted to administrators.
        </p>
      </section>
    )
  }

  return (
    <section className="rounded-3xl border bg-background p-6 shadow-sm">
      <div className="mb-4 flex items-center gap-2">
        <UserPlus className="h-4 w-4" />
        <h2 className="text-lg font-semibold">Users</h2>
      </div>
      <form className="grid gap-4 md:grid-cols-[1fr_1fr_auto]" onSubmit={handleCreateUser}>
        <div className="space-y-2">
          <Label htmlFor="new-username">Username</Label>
          <Input id="new-username" value={newUsername} onChange={(event) => setNewUsername(event.target.value)} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="new-password">Temporary password</Label>
          <Input id="new-password" type="password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} />
        </div>
        <Button type="submit" className="mt-auto gap-2">
          <UserPlus className="h-4 w-4" />
          Add user
        </Button>
      </form>

      {error && <p className="mt-4 text-sm text-destructive">{error}</p>}

      <div className="mt-6 space-y-3">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading users...</p>
        ) : users.length === 0 ? (
          <p className="text-sm text-muted-foreground">No users found.</p>
        ) : users.map((user) => (
          <div key={user.id} className="flex items-center justify-between rounded-2xl border px-4 py-3">
            <div>
              <p className="font-medium">{user.username}</p>
              <p className="text-xs text-muted-foreground">
                {[user.role, formatCreatedAt(user.createdAt)].filter(Boolean).join(" · ")}
              </p>
            </div>
            <Badge variant={user.role === "admin" ? "default" : "outline"}>{user.role}</Badge>
          </div>
        ))}
      </div>
    </section>
  )
}
