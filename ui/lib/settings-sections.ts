export type SettingsRoute = {
  href: string
  label: string
  description: string
  adminOnly?: boolean
}

const settingsRoutes: SettingsRoute[] = [
  {
    href: "/settings",
    label: "Overview",
    description: "Review access controls and jump into each management area.",
  },
  {
    href: "/settings/api-keys",
    label: "API keys",
    description: "Create CLI tokens, review active keys, and revoke old ones.",
  },
  {
    href: "/settings/users",
    label: "Users",
    description: "Create standard users and review who can access this registry.",
    adminOnly: true,
  },
  {
    href: "/settings/fallback-sources",
    label: "Fallback sources",
    description: "Review mirrored upstream sources and manage external lookups.",
  },
]

export function getSettingsRoutes(isAdmin: boolean) {
  return settingsRoutes.filter((route) => !route.adminOnly || isAdmin)
}

export function getSettingsManagementRoutes(isAdmin: boolean) {
  return getSettingsRoutes(isAdmin).filter((route) => route.href !== "/settings")
}

export function getSettingsPageCopy(pathname: string) {
  if (pathname.startsWith("/settings/api-keys")) {
    return {
      title: "API keys",
      description: "Generate CLI credentials, rotate them safely, and keep read and publish flows aligned with your local shell setup.",
    }
  }
  if (pathname.startsWith("/settings/users")) {
    return {
      title: "Users",
      description: "Manage who can sign in to this registry and provision standard users for CLI and dashboard access.",
    }
  }
  if (pathname.startsWith("/settings/fallback-sources")) {
    return {
      title: "Fallback sources",
      description: "Track the upstream sources that the registry can query and mirror when content is missing locally.",
    }
  }
  return {
    title: "Workspace settings",
    description: "Review auth mode, jump into separate management pages, and keep CLI and registry access consistent.",
  }
}
