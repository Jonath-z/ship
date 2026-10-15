import type { ComponentType } from "react";
import { envRoute } from "@/lib/routes";
import {
  IconActivity,
  IconArrowLeft,
  IconBox,
  IconDashboard,
  IconDatabase,
  IconFileText,
  IconGrid,
  IconKey,
  IconRocket,
  IconServer,
  IconSettings,
  type IconProps,
} from "@/features/shell/icons";

export interface NavItem {
  label: string;
  href: string;
  icon: ComponentType<IconProps>;
  /** Match nested routes too. */
  prefix?: boolean;
}

export interface NavScope {
  projectId: string;
  environmentId: string;
}

export interface Navigation {
  /** The scope's own sections. */
  primary: NavItem[];
  /** Shortcuts out of the scope, rendered after a divider. */
  secondary: NavItem[];
}

/**
 * Control-plane level: everything that is not tied to one environment.
 * Settings here are the account, local users, and the audit log.
 */
const controlPlaneNav: NavItem[] = [
  { label: "Projects", href: "/projects", icon: IconGrid },
  { label: "Servers", href: "/servers", icon: IconServer, prefix: true },
  { label: "System", href: "/dashboard", icon: IconActivity },
  { label: "Settings", href: "/settings", icon: IconSettings },
];

/**
 * Environment level, ordered to match the setup flow: what runs → how it is
 * configured → deploy → observe. Settings here belong to the environment only
 * (naming, server groups, deletion).
 */
function environmentNav(scope: NavScope): NavItem[] {
  const href = (section?: string) =>
    envRoute(scope.projectId, scope.environmentId, section);
  return [
    { label: "Overview", href: href(), icon: IconDashboard },
    {
      label: "Applications",
      href: href("/applications"),
      icon: IconBox,
      prefix: true,
    },
    {
      label: "Databases",
      href: href("/databases"),
      icon: IconDatabase,
      prefix: true,
    },
    {
      label: "Variables",
      href: href("/variables"),
      icon: IconKey,
      prefix: true,
    },
    {
      label: "Deployments",
      href: href("/deployments"),
      icon: IconRocket,
      prefix: true,
    },
    { label: "Logs", href: href("/logs"), icon: IconFileText, prefix: true },
    {
      label: "Settings",
      href: href("/settings"),
      icon: IconSettings,
      prefix: true,
    },
  ];
}

export function navigationFor(scope?: NavScope): Navigation {
  if (!scope) return { primary: controlPlaneNav, secondary: [] };
  return {
    primary: environmentNav(scope),
    secondary: [
      { label: "All projects", href: "/projects", icon: IconArrowLeft },
      { label: "Servers", href: "/servers", icon: IconServer, prefix: true },
    ],
  };
}

/** hrefs may carry the ?project=…&env=… scope; match on the path only. */
export function isActive(item: NavItem, pathname: string): boolean {
  const itemPath = item.href.split("?")[0] ?? item.href;
  return item.prefix ? pathname.startsWith(itemPath) : pathname === itemPath;
}

const titles: Record<string, string> = {
  "/projects": "Projects",
  "/servers": "Servers",
  "/servers/detail": "Server",
  "/dashboard": "System",
  "/settings": "Settings",
  "/env": "Overview",
  "/env/applications": "Applications",
  "/env/applications/service": "Application",
  "/env/databases": "Databases",
  "/env/variables": "Variables",
  "/env/deployments": "Deployments",
  "/env/deployments/detail": "Deployment",
  "/env/logs": "Logs",
  "/env/settings": "Settings",
};

/** Title shown in the centre of the top bar for a route path. */
export function pageTitle(pathname: string): string | undefined {
  return titles[pathname.replace(/\/+$/, "") || "/"];
}
