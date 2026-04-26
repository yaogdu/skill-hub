"use client"

import { useEffect, useState } from "react"
import { KeyRound, Shield } from "lucide-react"
import { useSettingsContext } from "@/components/settings-shell"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { getRegistryAuthSettings, updateRegistryAuthSettings, type RegistryAuthSettings } from "@/lib/admin-api"
import { clearStoredSession } from "@/lib/session"

function isAuthenticationError(message: string) {
  return message.toLowerCase().includes("authentication")
}

export default function SettingsOverviewPage() {
  const { isAdmin } = useSettingsContext()
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [settings, setSettings] = useState<RegistryAuthSettings | null>(null)
  const [apiKeyToggle, setAPIKeyToggle] = useState(true)

  async function refresh() {
    try {
      setLoading(true)
      setError(null)
      const authSettings = await getRegistryAuthSettings()
      setSettings(authSettings)
      setAPIKeyToggle(authSettings.apiKeyValidationEnabled)
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to load registry settings"
      setError(message)
      if (isAuthenticationError(message)) {
        clearStoredSession()
        window.location.replace("/login")
      }
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
  }, [])

  async function handleUpdateSettings() {
    try {
      setSaving(true)
      setError(null)
      const updated = await updateRegistryAuthSettings(apiKeyToggle)
      setSettings(updated)
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to update registry settings"
      setError(message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <section className="rounded-3xl border bg-background p-6 shadow-sm">
        <p className="text-sm leading-6 text-muted-foreground">
          Use the left navigation to switch between API keys, users, and fallback sources. The right panel only shows the active settings area.
        </p>
      </section>

      <section className="grid gap-6 lg:grid-cols-[1.2fr_0.8fr]">
        <div className="rounded-3xl border bg-background p-6 shadow-sm">
          <div className="mb-4 flex items-center gap-2">
            <Shield className="h-4 w-4" />
            <h2 className="text-lg font-semibold">SHUB auth mode</h2>
          </div>
          <p className="text-sm text-muted-foreground">
            When enabled, `npx @yaogdu-skill-hub/shub` asks for an API key for registry-backed read flows. When disabled, anonymous users can still search, add, and inspect existing assets, but publish flows still require authentication.
          </p>
          <div className="mt-5 rounded-2xl border p-4">
            <div className="flex items-start gap-3">
              <Checkbox checked={apiKeyToggle} onCheckedChange={(checked) => setAPIKeyToggle(Boolean(checked))} id="api-key-toggle" />
              <div className="space-y-1">
                <Label htmlFor="api-key-toggle">Enable API key validation for SHUB read flows</Label>
                <p className="text-xs text-muted-foreground">Default: enabled</p>
              </div>
            </div>
            <Button className="mt-4 w-full" disabled={!isAdmin || saving || loading} onClick={handleUpdateSettings}>
              {saving ? "Saving..." : "Save setting"}
            </Button>
          </div>
          {!isAdmin && <p className="mt-3 text-xs text-muted-foreground">Only administrators can change this switch.</p>}
          {settings?.updatedAt && (
            <p className="mt-3 text-xs text-muted-foreground">Last updated {new Date(settings.updatedAt).toLocaleString()}</p>
          )}
          {error && <p className="mt-3 text-sm text-destructive">{error}</p>}
        </div>

        <div className="rounded-3xl border bg-background p-6 shadow-sm">
          <div className="flex items-center gap-2">
            <KeyRound className="h-4 w-4" />
            <h2 className="text-lg font-semibold">CLI setup</h2>
          </div>
          <p className="mt-4 text-sm leading-6 text-muted-foreground">
            Generate a key from the API keys page, then place it in your shell profile or user-level config. `@yaogdu-skill-hub/shub` reads `SHUB_API_TOKEN`; `arctl` also accepts `ARCTL_API_TOKEN`.
          </p>
          <div className="mt-5 rounded-2xl border border-dashed bg-muted/30 p-4 text-sm text-muted-foreground">
            Fish:
            <pre className="mt-2 overflow-x-auto rounded-xl bg-background p-3 font-mono text-xs text-foreground">{`set -gx SHUB_API_TOKEN <your-token>
set -gx ARCTL_API_TOKEN <your-token>`}</pre>
            Zsh / Bash:
            <pre className="mt-2 overflow-x-auto rounded-xl bg-background p-3 font-mono text-xs text-foreground">{`export SHUB_API_TOKEN=<your-token>
export ARCTL_API_TOKEN=<your-token>`}</pre>
          </div>
        </div>
      </section>
    </>
  )
}
