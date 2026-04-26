// Auto-generated API client configuration.
// Types and SDK functions are generated from the OpenAPI spec.
// Regenerate with: make gen-client

import { client } from './api/client.gen'
import { getStoredSession } from './session'

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || (typeof window !== 'undefined' && window.location.origin) || ''

client.setConfig({ baseUrl: API_BASE_URL })

let requestInterceptorConfigured = false
if (!requestInterceptorConfigured) {
  client.interceptors.request.use(async (request) => {
    const session = getStoredSession()
    if (!session?.token) {
      return request
    }
    const headers = new Headers(request.headers)
    headers.set('Authorization', `Bearer ${session.token}`)
    return new Request(request, { headers })
  })
  requestInterceptorConfigured = true
}

function apiUrl(path: string) {
  const trimmedBase = API_BASE_URL.replace(/\/$/, '')
  return `${trimmedBase}${path}`
}

function extractErrorMessage(raw: string) {
  try {
    const parsed = JSON.parse(raw) as {
      detail?: string
      errors?: Array<{ message?: string }>
    }
    const messages = parsed.errors?.map((entry) => entry.message).filter(Boolean)
    if (messages && messages.length > 0) {
      return messages.join("; ")
    }
    if (parsed.detail) {
      return parsed.detail
    }
  } catch {
    // Fall back to the raw response text.
  }
  return raw
}

async function requestJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set('Content-Type', 'application/json')
  const session = getStoredSession()
  if (session?.token) {
    headers.set('Authorization', `Bearer ${session.token}`)
  }
  const response = await fetch(apiUrl(path), { ...init, headers })
  if (!response.ok) {
    const text = await response.text()
    throw new Error(extractErrorMessage(text) || `request failed with status ${response.status}`)
  }
  if (response.status === 204) {
    return {} as T
  }
  return await response.json() as T
}

export type SessionUser = {
  id: string
  username: string
  role: string
}

export type LoginResponse = {
  token: string
  expiresAt: number
  user: SessionUser
}

export type RegistryUser = SessionUser & {
  createdAt?: string
  updatedAt?: string
}

export type APIKey = {
  id: string
  name: string
  prefix: string
  createdAt: string
  lastUsedAt?: string
  revokedAt?: string
}

export type CreateAPIKeyResponse = {
  apiKey: APIKey
  secret: string
}

export type SHUBSource = {
  name: string
  address: string
  description?: string
  provider?: string
  builtIn?: boolean
  createdAt?: string
  updatedAt?: string
}

export type RegistryAuthSettings = {
  apiKeyValidationEnabled: boolean
  updatedAt?: string
}

export async function login(username: string, password: string) {
  return requestJSON<LoginResponse>('/v0/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })
}

export async function getCurrentUser() {
  return requestJSON<RegistryUser>('/v0/auth/me', { method: 'GET' })
}

export async function listRegistryUsers() {
  const response = await requestJSON<{ users: RegistryUser[] }>('/v0/auth/users', { method: 'GET' })
  return response.users
}

export async function createRegistryUser(username: string, password: string, role = 'user') {
  return requestJSON<RegistryUser>('/v0/auth/users', {
    method: 'POST',
    body: JSON.stringify({ username, password, role }),
  })
}

export async function listAPIKeysForCurrentUser() {
  const response = await requestJSON<{ apiKeys: APIKey[] }>('/v0/auth/api-keys', { method: 'GET' })
  return response.apiKeys
}

export async function createAPIKey(name: string) {
  return requestJSON<CreateAPIKeyResponse>('/v0/auth/api-keys', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export async function deleteAPIKey(id: string) {
  return requestJSON('/v0/auth/api-keys/' + encodeURIComponent(id), { method: 'DELETE' })
}

export async function getRegistryAuthSettings() {
  return requestJSON<RegistryAuthSettings>('/v0/auth/settings', { method: 'GET' })
}

export async function updateRegistryAuthSettings(apiKeyValidationEnabled: boolean) {
  return requestJSON<RegistryAuthSettings>('/v0/auth/settings', {
    method: 'PUT',
    body: JSON.stringify({ apiKeyValidationEnabled }),
  })
}

export async function listSHUBSources() {
  const response = await requestJSON<{ sources: SHUBSource[] }>('/v0/shub/sources', { method: 'GET' })
  return response.sources
}

export async function setSHUBSource(name: string, address: string) {
  return requestJSON<SHUBSource>('/v0/shub/sources/' + encodeURIComponent(name), {
    method: 'PUT',
    body: JSON.stringify({ address }),
  })
}

export async function deleteSHUBSource(name: string) {
  return requestJSON('/v0/shub/sources/' + encodeURIComponent(name), { method: 'DELETE' })
}

export { client }
export * from './api/sdk.gen'
export * from './api/types.gen'
