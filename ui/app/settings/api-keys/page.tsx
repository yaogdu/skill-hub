"use client"

import { FormEvent, useEffect, useMemo, useState } from "react"
import { KeyRound, Plus, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { createAPIKey, deleteAPIKey, listAPIKeysForCurrentUser, type APIKey, type CreateAPIKeyResponse } from "@/lib/admin-api"
import { clearStoredSession } from "@/lib/session"

function isAuthenticationError(message: string) {
  return message.toLowerCase().includes("authentication")
}

export default function APIKeysSettingsPage() {
  const [loading, setLoading] = useState(true)
  const [creatingKey, setCreatingKey] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [apiKeys, setAPIKeys] = useState<APIKey[]>([])
  const [createdKey, setCreatedKey] = useState<CreateAPIKeyResponse | null>(null)
  const [newKeyName, setNewKeyName] = useState("default-cli")

  const shellSnippet = useMemo(() => {
    if (!createdKey) {
      return ""
    }
    return `# ~/.zshrc or ~/.bashrc
export SHUB_API_TOKEN=${createdKey.secret}
export ARCTL_API_TOKEN=${createdKey.secret}

# ~/.config/fish/config.fish
set -gx SHUB_API_TOKEN ${createdKey.secret}
set -gx ARCTL_API_TOKEN ${createdKey.secret}`
  }, [createdKey])

  async function refresh() {
    try {
      setLoading(true)
      setError(null)
      setAPIKeys(await listAPIKeysForCurrentUser())
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to load API keys"
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

  async function handleCreateKey(event: FormEvent) {
    event.preventDefault()
    try {
      setCreatingKey(true)
      setError(null)
      const response = await createAPIKey(newKeyName)
      setCreatedKey(response)
      setNewKeyName("default-cli")
      setAPIKeys(await listAPIKeysForCurrentUser())
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create API key")
    } finally {
      setCreatingKey(false)
    }
  }

  async function handleDeleteKey(id: string) {
    try {
      setError(null)
      await deleteAPIKey(id)
      setAPIKeys(await listAPIKeysForCurrentUser())
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to delete API key")
    }
  }

  return (
    <section className="rounded-3xl border bg-background p-6 shadow-sm">
      <div className="mb-4 flex items-center gap-2">
        <KeyRound className="h-4 w-4" />
        <h2 className="text-lg font-semibold">API keys</h2>
      </div>
      <p className="mb-5 text-sm text-muted-foreground">
        Create per-device or per-user CLI tokens here. The secret is shown once, so save it immediately into your shell profile after generation.
      </p>
      <form className="flex flex-col gap-3 md:flex-row" onSubmit={handleCreateKey}>
        <Input value={newKeyName} onChange={(event) => setNewKeyName(event.target.value)} placeholder="Key name" />
        <Button type="submit" disabled={creatingKey} className="gap-2">
          <Plus className="h-4 w-4" />
          {creatingKey ? "Creating..." : "Create API key"}
        </Button>
      </form>

      {createdKey && (
        <div className="mt-5 rounded-2xl border border-emerald-500/30 bg-background p-4 text-sm shadow-sm">
          <p className="font-medium text-foreground">Save this secret now</p>
          <p className="mt-2 break-all rounded-xl bg-emerald-500/10 px-3 py-2 font-mono text-emerald-700 dark:text-emerald-300">
            {createdKey.secret}
          </p>
          <p className="mt-3 text-xs leading-5 text-muted-foreground">
            This token is only shown once. The commands below are examples you can paste into your shell config; nothing is written automatically.
          </p>
          <Textarea
            className="mt-3 min-h-[116px] border-border bg-muted/30 font-mono text-xs text-foreground"
            readOnly
            value={shellSnippet}
          />
        </div>
      )}

      {error && <p className="mt-4 text-sm text-destructive">{error}</p>}

      <div className="mt-6 space-y-3">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading API keys...</p>
        ) : apiKeys.length === 0 ? (
          <p className="text-sm text-muted-foreground">No API keys yet.</p>
        ) : apiKeys.map((key) => (
          <div key={key.id} className="flex items-center justify-between rounded-2xl border px-4 py-3">
            <div>
              <p className="font-medium">{key.name}</p>
              <p className="text-xs text-muted-foreground">
                {key.prefix} · created {new Date(key.createdAt).toLocaleString()}
                {key.lastUsedAt ? ` · last used ${new Date(key.lastUsedAt).toLocaleString()}` : ""}
              </p>
            </div>
            <Button variant="ghost" size="icon" onClick={() => void handleDeleteKey(key.id)}>
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </div>
    </section>
  )
}
