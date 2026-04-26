import { beforeEach, describe, expect, it } from "vitest"
import { clearStoredSession, getStoredSession, setStoredSession, type SkillHubSession } from "./session"

const sampleSession: SkillHubSession = {
  token: "token-1",
  user: {
    id: "user-1",
    username: "alice",
    role: "admin",
  },
}

describe("session store", () => {
  beforeEach(() => {
    window.localStorage.clear()
    clearStoredSession()
  })

  it("returns the same object reference while storage is unchanged", () => {
    setStoredSession(sampleSession)

    const first = getStoredSession()
    const second = getStoredSession()

    expect(first).toEqual(sampleSession)
    expect(second).toBe(first)
  })

  it("returns a new snapshot after storage changes", () => {
    setStoredSession(sampleSession)
    const first = getStoredSession()

    setStoredSession({
      ...sampleSession,
      token: "token-2",
    })
    const second = getStoredSession()

    expect(second).not.toBe(first)
    expect(second?.token).toBe("token-2")
  })

  it("clears invalid JSON from storage", () => {
    window.localStorage.setItem("skill-hub-session", "{broken")

    expect(getStoredSession()).toBeNull()
    expect(window.localStorage.getItem("skill-hub-session")).toBeNull()
  })
})
