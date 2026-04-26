import { describe, expect, it } from "vitest"
import { getSettingsManagementRoutes, getSettingsPageCopy, getSettingsRoutes } from "./settings-sections"

describe("settings routes", () => {
  it("hides admin-only routes for standard users", () => {
    expect(getSettingsRoutes(false).map((route) => route.href)).toEqual([
      "/settings",
      "/settings/api-keys",
      "/settings/fallback-sources",
    ])
  })

  it("keeps overview out of the management route list", () => {
    expect(getSettingsManagementRoutes(true).map((route) => route.href)).toEqual([
      "/settings/api-keys",
      "/settings/users",
      "/settings/fallback-sources",
    ])
  })

  it("returns per-page copy for nested settings routes", () => {
    expect(getSettingsPageCopy("/settings/users")).toEqual({
      title: "Users",
      description: "Manage who can sign in to this registry and provision standard users for CLI and dashboard access.",
    })
  })
})
