"use client"

import { useSyncExternalStore } from "react"

export type SessionUser = {
  id: string
  username: string
  role: string
}

export type SkillHubSession = {
  token: string
  user: SessionUser
}

const SESSION_STORAGE_KEY = "skill-hub-session"
const listeners = new Set<() => void>()
let cachedRawSession: string | null | undefined
let cachedParsedSession: SkillHubSession | null

function readSession(): SkillHubSession | null {
  if (typeof window === "undefined") {
    return null
  }
  const raw = window.localStorage.getItem(SESSION_STORAGE_KEY)
  if (raw === cachedRawSession) {
    return cachedParsedSession
  }
  if (!raw) {
    cachedRawSession = null
    cachedParsedSession = null
    return null
  }
  try {
    cachedParsedSession = JSON.parse(raw) as SkillHubSession
    cachedRawSession = raw
    return cachedParsedSession
  } catch {
    window.localStorage.removeItem(SESSION_STORAGE_KEY)
    cachedRawSession = null
    cachedParsedSession = null
    return null
  }
}

function emit() {
  for (const listener of listeners) {
    listener()
  }
}

export function getStoredSession(): SkillHubSession | null {
  return readSession()
}

export function setStoredSession(session: SkillHubSession) {
  if (typeof window === "undefined") {
    return
  }
  window.localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(session))
  emit()
}

export function clearStoredSession() {
  if (typeof window === "undefined") {
    return
  }
  window.localStorage.removeItem(SESSION_STORAGE_KEY)
  emit()
}

export function useStoredSession() {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    () => readSession(),
    () => null,
  )
}
