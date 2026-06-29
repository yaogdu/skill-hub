"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { useTheme } from "next-themes"
import { useSyncExternalStore } from "react"
import { LogIn, LogOut, Moon, Settings, Sun } from "lucide-react"
import { Button, buttonVariants } from "@/components/ui/button"
import { clearStoredSession, useStoredSession } from "@/lib/session"
import { cn } from "@/lib/utils"

export function Navigation() {
  const pathname = usePathname()
  const { theme, setTheme } = useTheme()
  const mounted = useSyncExternalStore(() => () => {}, () => true, () => false)
  const session = useStoredSession()

  const isActive = (path: string) => {
    if (path === "/") {
      return pathname === "/"
    }
    return pathname.startsWith(path)
  }

  return (
    <nav className="sticky top-0 z-50 border-b bg-background/95 backdrop-blur">
      <div className="container mx-auto px-6">
        <div className="flex h-16 items-center gap-10">
          <Link href="/" className="shrink-0 rounded-md px-2 py-1">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-2xl bg-[linear-gradient(135deg,#f97316,#0ea5e9)] text-sm font-semibold text-white shadow-lg shadow-orange-200/60">
                SH
              </div>
              <div>
                <p className="text-[11px] font-semibold uppercase tracking-[0.26em] text-muted-foreground">skill-hub</p>
                <p className="text-sm font-medium text-foreground">Registry dashboard</p>
              </div>
            </div>
          </Link>

          <div className="flex items-center gap-1">
            <Link
              href="/"
              className={`relative px-3 py-1.5 text-[15px] font-medium transition-colors ${
                isActive("/")
                  ? "text-foreground after:absolute after:bottom-[-15px] after:left-1 after:right-1 after:h-[2px] after:rounded-full after:bg-primary"
                  : "text-muted-foreground hover:text-foreground"
              }`}
            >
              Catalog
            </Link>
            {session && (
              <a
                href="/settings"
                className={`relative px-3 py-1.5 text-[15px] font-medium transition-colors ${
                  isActive("/settings")
                    ? "text-foreground after:absolute after:bottom-[-15px] after:left-1 after:right-1 after:h-[2px] after:rounded-full after:bg-primary"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                Settings
              </a>
            )}
          </div>

          <div className="ml-auto flex items-center gap-2">
            {session ? (
              <>
                <span className="hidden rounded-full border px-3 py-1 text-xs font-medium text-muted-foreground md:inline-flex">
                  {session.user.username}
                </span>
                <a
                  href="/settings"
                  className={cn(buttonVariants({ variant: "outline", size: "sm" }), "gap-2")}
                >
                  <Settings className="h-4 w-4" />
                  Manage
                </a>
                <Button
                  variant="ghost"
                  size="sm"
                  className="gap-2"
                  onClick={() => {
                    clearStoredSession()
                    window.location.assign("/login")
                  }}
                >
                  <LogOut className="h-4 w-4" />
                  Logout
                </Button>
              </>
            ) : (
              <a
                href="/login"
                className={cn(buttonVariants({ variant: "outline", size: "sm" }), "gap-2")}
              >
                <LogIn className="h-4 w-4" />
                Login
              </a>
            )}

            {mounted && (
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8"
                onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
                title={`Switch to ${theme === "dark" ? "light" : "dark"} mode`}
              >
                {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
              </Button>
            )}
          </div>
        </div>
      </div>
    </nav>
  )
}
