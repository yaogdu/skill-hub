import type { ReactNode } from "react"
import { render, screen } from "@testing-library/react"
import { describe, expect, it, vi, beforeEach } from "vitest"
import { Navigation } from "../navigation"

const useStoredSessionMock = vi.fn()
const clearStoredSessionMock = vi.fn()
const setThemeMock = vi.fn()

vi.mock("next/link", () => ({
  default: ({ href, children, ...props }: { href: string; children: ReactNode }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

vi.mock("next/navigation", () => ({
  usePathname: () => "/",
}))

vi.mock("next-themes", () => ({
  useTheme: () => ({
    theme: "light",
    setTheme: setThemeMock,
  }),
}))

vi.mock("@/lib/session", () => ({
  clearStoredSession: () => clearStoredSessionMock(),
  useStoredSession: () => useStoredSessionMock(),
}))

describe("Navigation", () => {
  beforeEach(() => {
    useStoredSessionMock.mockReset()
    clearStoredSessionMock.mockReset()
    setThemeMock.mockReset()
  })

  it("renders a plain login link when no session exists", () => {
    useStoredSessionMock.mockReturnValue(null)

    render(<Navigation />)

    const loginLink = screen.getByRole("link", { name: /login/i })
    expect(loginLink).toHaveAttribute("href", "/login")
    expect(screen.queryByRole("link", { name: /manage/i })).not.toBeInTheDocument()
  })

  it("renders a plain settings link when a session exists", () => {
    useStoredSessionMock.mockReturnValue({
      token: "token",
      user: {
        id: "user-1",
        username: "alice",
        role: "user",
      },
    })

    render(<Navigation />)

    const manageLink = screen.getByRole("link", { name: /manage/i })
    expect(manageLink).toHaveAttribute("href", "/settings")
    expect(screen.getByText("alice")).toBeInTheDocument()
  })
})
