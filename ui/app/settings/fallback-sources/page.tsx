"use client"

import { FormEvent, useEffect, useState } from "react"
import { Plus, Sparkles, Trash2 } from "lucide-react"
import { useSettingsContext } from "@/components/settings-shell"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { deleteSHUBSource, listSHUBSources, setSHUBSource, type SHUBSource } from "@/lib/admin-api"
import { clearStoredSession } from "@/lib/session"

function isAuthenticationError(message: string) {
  return message.toLowerCase().includes("authentication")
}

export default function FallbackSourcesSettingsPage() {
  const { isAdmin } = useSettingsContext()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [sources, setSources] = useState<SHUBSource[]>([])
  const [sourceName, setSourceName] = useState("")
  const [sourceAddress, setSourceAddress] = useState("")

  async function refresh() {
    try {
      setLoading(true)
      setError(null)
      setSources(await listSHUBSources())
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to load fallback sources"
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

  async function handleSaveSource(event: FormEvent) {
    event.preventDefault()
    try {
      setError(null)
      await setSHUBSource(sourceName, sourceAddress)
      setSourceName("")
      setSourceAddress("")
      setSources(await listSHUBSources())
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save source")
    }
  }

  async function handleDeleteSource(name: string) {
    try {
      setError(null)
      await deleteSHUBSource(name)
      setSources(await listSHUBSources())
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete source")
    }
  }

  return (
    <section className="rounded-3xl border bg-background p-6 shadow-sm">
      <div className="mb-4 flex items-center gap-2">
        <Sparkles className="h-4 w-4" />
        <h2 className="text-lg font-semibold">Fallback sources</h2>
      </div>
      <p className="text-sm text-muted-foreground">
        The registry can query these configured upstream sources when content is missing locally, then mirror resolved content back into the registry.
      </p>
      <div className="mt-4 rounded-2xl border border-dashed bg-muted/30 p-4 text-sm text-muted-foreground">
        Built-in defaults now cover common flows: <code>github-direct</code> for repo-per-skill installs, <code>github-skills-main</code> for repositories that keep skills under <code>skills/&lt;name&gt;</code>, <code>github-plugin-skills-main</code> for plugin repos that keep skills under <code>plugins/&lt;name&gt;/skills/&lt;name&gt;</code>, plus <code>openai-skills</code> and <code>anthropic-skills</code> for the official OpenAI and Claude Code catalogs. <code>shub add</code> will try these automatically on a registry miss before moving on to any custom sources you add here.
      </div>

      {isAdmin ? (
        <form className="mt-5 space-y-3" onSubmit={handleSaveSource}>
          <div className="space-y-2">
            <Label htmlFor="source-name">Source name</Label>
            <Input id="source-name" value={sourceName} onChange={(event) => setSourceName(event.target.value)} placeholder="github-main" />
          </div>
          <div className="space-y-2">
            <Label htmlFor="source-address">Source address</Label>
            <Textarea
              id="source-address"
              value={sourceAddress}
              onChange={(event) => setSourceAddress(event.target.value)}
              placeholder="https://github.com/acme/skills/tree/main/skills/{name}"
            />
          </div>
          <Button type="submit" className="gap-2">
            <Plus className="h-4 w-4" />
            Save source
          </Button>
        </form>
      ) : (
        <div className="mt-5 rounded-2xl border border-dashed bg-muted/30 p-4 text-sm text-muted-foreground">
          Only administrators can add, change, or delete fallback sources. This page stays readable for everyone so operators can verify what upstreams are configured.
        </div>
      )}

      {error && <p className="mt-4 text-sm text-destructive">{error}</p>}

      <div className="mt-6 space-y-3">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading fallback sources...</p>
        ) : sources.length === 0 ? (
          <p className="text-sm text-muted-foreground">No fallback sources configured.</p>
        ) : sources.map((source) => (
          <div key={source.name} className="rounded-2xl border px-4 py-3">
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <p className="font-medium">{source.name}</p>
                  {source.builtIn && <Badge variant="outline">Built in</Badge>}
                  {source.provider && <Badge variant="secondary">{source.provider}</Badge>}
                </div>
                {source.description && (
                  <p className="mt-1 text-xs text-muted-foreground">{source.description}</p>
                )}
                <p className="mt-1 break-all text-xs text-muted-foreground">{source.address}</p>
              </div>
              {isAdmin && !source.builtIn && (
                <Button variant="ghost" size="icon" onClick={() => void handleDeleteSource(source.name)}>
                  <Trash2 className="h-4 w-4" />
                </Button>
              )}
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}
