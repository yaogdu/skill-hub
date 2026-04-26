"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react"
import { ChevronRight, LogOut, RefreshCw, Shield } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { getCurrentUser, type RegistryUser } from "@/lib/admin-api"
import { clearStoredSession, getStoredSession } from "@/lib/session"
import { getSettingsPageCopy, getSettingsRoutes } from "@/lib/settings-sections"
import { cn } from "@/lib/utils"

type SettingsContextValue = {
  isAdmin: boolean
  logout: () => void
  user: RegistryUser
}

const SettingsContext = createContext<SettingsContextValue | null>(null)

function isAuthenticationError(message: string) {
  return message.toLowerCase().includes("authentication")
}

function redirectToLogin() {
  window.location.replace("/login")
}

export function useSettingsContext() {
  const value = useContext(SettingsContext)
  if (!value) {
    throw new Error("useSettingsContext must be used inside SettingsShell")
  }
  return value
}

export function SettingsShell({ children }: { children: ReactNode }) {
  const pathname = usePathname()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [user, setUser] = useState<RegistryUser | null>(null)

  const pageCopy = useMemo(() => getSettingsPageCopy(pathname), [pathname])
  const isAdmin = user?.role === "admin"
  const routes = useMemo(() => getSettingsRoutes(Boolean(isAdmin)), [isAdmin])

  function logout() {
    clearStoredSession()
    redirectToLogin()
  }

  async function loadCurrentUser() {
    const session = getStoredSession()
    if (!session?.token) {
      redirectToLogin()
      return
    }

    try {
      setLoading(true)
      setError(null)
      setUser(await getCurrentUser())
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to load settings"
      setError(message)
      if (isAuthenticationError(message)) {
        clearStoredSession()
        redirectToLogin()
        return
      }
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadCurrentUser()
  }, [])

  if (loading) {
    return (
      <main className="container mx-auto px-6 py-16">
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <RefreshCw className="h-4 w-4 animate-spin" />
          Loading settings...
        </div>
      </main>
    )
  }

  if (!user) {
    return (
      <main className="container mx-auto px-6 py-16">
        <section className="rounded-3xl border bg-background p-8 shadow-sm">
          <h1 className="text-2xl font-semibold tracking-tight">Settings unavailable</h1>
          <p className="mt-3 text-sm text-muted-foreground">
            {error || "The registry dashboard could not load your session."}
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            <Button onClick={() => void loadCurrentUser()} className="gap-2">
              <RefreshCw className="h-4 w-4" />
              Retry
            </Button>
            <Button variant="outline" onClick={logout}>
              Sign out
            </Button>
          </div>
        </section>
      </main>
    )
  }

  return (
    <SettingsContext.Provider value={{ user, isAdmin: Boolean(isAdmin), logout }}>
      <main className="bg-[linear-gradient(135deg,#fff7ed_0%,#f8fafc_42%,#e0f2fe_100%)]">
        <div className="container mx-auto space-y-8 px-6 py-10">
          <section className="rounded-3xl border bg-background/95 p-8 shadow-[0_28px_90px_rgba(15,23,42,0.12)] backdrop-blur">
            <div className="flex flex-col gap-6 md:flex-row md:items-start md:justify-between">
              <div className="space-y-3">
                <p className="text-xs font-semibold uppercase tracking-[0.3em] text-muted-foreground">skill-hub</p>
                <div className="flex items-center gap-3">
                  <h1 className="text-3xl font-semibold tracking-tight">{pageCopy.title}</h1>
                  <Badge variant="outline">{user.role}</Badge>
                </div>
                <p className="max-w-2xl text-sm text-muted-foreground">
                  Logged in as <span className="font-medium text-foreground">{user.username}</span>. {pageCopy.description}
                </p>
              </div>
              <Button variant="outline" className="gap-2" onClick={logout}>
                <LogOut className="h-4 w-4" />
                Sign out
              </Button>
            </div>
            {error && <p className="mt-4 text-sm text-destructive">{error}</p>}
          </section>

          <div className="grid gap-6 xl:grid-cols-[280px_minmax(0,1fr)]">
            <aside className="rounded-3xl border bg-background p-5 shadow-sm">
              <div className="mb-4 flex items-center gap-2">
                <Shield className="h-4 w-4" />
                <h2 className="text-sm font-semibold uppercase tracking-[0.25em] text-muted-foreground">Settings</h2>
              </div>
              <div className="space-y-2">
                {routes.map((route) => {
                  const active = route.href === "/settings" ? pathname === route.href : pathname.startsWith(route.href)
                  return (
                    <Link
                      key={route.href}
                      href={route.href}
                      className={cn(
                        "flex items-start justify-between rounded-2xl border px-4 py-3 transition-colors",
                        active
                          ? "border-primary/25 bg-primary/5 text-foreground"
                          : "border-transparent bg-muted/40 text-muted-foreground hover:border-border hover:bg-background hover:text-foreground",
                      )}
                    >
                      <div>
                        <p className="font-medium">{route.label}</p>
                        <p className="mt-1 text-xs leading-5">{route.description}</p>
                      </div>
                      <ChevronRight className="mt-0.5 h-4 w-4 shrink-0" />
                    </Link>
                  )
                })}
              </div>
            </aside>

            <div className="space-y-6">{children}</div>
          </div>
        </div>
      </main>
    </SettingsContext.Provider>
  )
}
