"use client"

import { FormEvent, useEffect, useState } from "react"
import { AlertCircle, LogIn } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { login } from "@/lib/admin-api"
import { getStoredSession, setStoredSession } from "@/lib/session"

export default function LoginPage() {
  const [username, setUsername] = useState("admin")
  const [password, setPassword] = useState("admin")
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (getStoredSession()) {
      window.location.replace("/settings")
    }
  }, [])

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setLoading(true)
    setError(null)
    try {
      const response = await login(username, password)
      setStoredSession({ token: response.token, user: response.user })
      window.location.assign("/settings")
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed")
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="min-h-[calc(100vh-8rem)] bg-[radial-gradient(circle_at_top_left,#d0f2ff,transparent_35%),radial-gradient(circle_at_bottom_right,#f4d5a8,transparent_30%)]">
      <div className="container mx-auto flex min-h-[calc(100vh-8rem)] items-center justify-center px-6 py-16">
        <div className="w-full max-w-md rounded-3xl border bg-background/95 p-8 shadow-[0_24px_80px_rgba(15,23,42,0.12)] backdrop-blur">
          <div className="mb-8 space-y-3">
            <p className="text-xs font-semibold uppercase tracking-[0.28em] text-muted-foreground">skill-hub</p>
            <h1 className="text-3xl font-semibold tracking-tight">Sign in</h1>
            <p className="text-sm text-muted-foreground">
              Use the built-in account or a user created by an administrator to manage skills, sources, and API keys.
            </p>
          </div>

          <form className="space-y-5" onSubmit={handleSubmit}>
            <div className="space-y-2">
              <Label htmlFor="username">Username</Label>
              <Input id="username" value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input id="password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" />
            </div>

            {error && (
              <div className="flex items-start gap-2 rounded-xl border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
                <AlertCircle className="mt-0.5 h-4 w-4" />
                <span>{error}</span>
              </div>
            )}

            <Button className="w-full gap-2" disabled={loading} type="submit">
              <LogIn className="h-4 w-4" />
              {loading ? "Signing in..." : "Sign in"}
            </Button>
          </form>
        </div>
      </div>
    </main>
  )
}
