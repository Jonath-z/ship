"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";
import { api } from "@/lib/api";
import { useSession } from "@/lib/hooks";
import {
  loadLastEnvironment,
  saveLastEnvironment,
  type LastEnvironment,
} from "@/lib/last-env";
import { envRoute } from "@/lib/routes";
import { EnvSwitcher } from "@/features/shell/EnvSwitcher";
import { ShipLogo } from "@/features/shell/ShipLogo";

interface NavItem {
  label: string;
  href: string;
  /** Match nested routes too. */
  prefix?: boolean;
}

/**
 * Dashboard shell: Vercel-style top header (logo, project/environment
 * breadcrumb, account actions) with a horizontal tab bar underneath.
 * Environment scope comes from the URL (/env/...?project=X&env=Y);
 * globally scoped screens such as /servers fall back to the last visited
 * environment for their nav links.
 */
export function AppShell({
  projectId,
  environmentId,
  children,
}: {
  projectId?: string;
  environmentId?: string;
  children: ReactNode;
}) {
  const pathname = usePathname();
  const [lastEnv, setLastEnv] = useState<LastEnvironment>();

  useEffect(() => {
    if (projectId && environmentId) {
      saveLastEnvironment({ projectId, environmentId });
      setLastEnv({ projectId, environmentId });
    } else {
      setLastEnv(loadLastEnvironment());
    }
  }, [projectId, environmentId]);

  const scope =
    projectId && environmentId ? { projectId, environmentId } : lastEnv;
  const envHref = (section?: string) =>
    scope ? envRoute(scope.projectId, scope.environmentId, section) : undefined;

  // Ordered to match the setup flow: machines → placement → what runs →
  // how it's configured → deploy → observe.
  const items: NavItem[] = [
    { label: "Overview", href: envHref() ?? "/projects" },
    { label: "Servers", href: "/servers", prefix: true },
    {
      label: "Applications",
      href: envHref("/applications") ?? "/projects",
      prefix: true,
    },
    {
      label: "Databases",
      href: envHref("/databases") ?? "/projects",
      prefix: true,
    },
    {
      label: "Variables",
      href: envHref("/variables") ?? "/projects",
      prefix: true,
    },
    {
      label: "Deployments",
      href: envHref("/deployments") ?? "/projects",
      prefix: true,
    },
    {
      label: "Logs",
      href: envHref("/logs") ?? "/projects",
      prefix: true,
    },
    {
      label: "Settings",
      href: envHref("/settings") ?? "/settings",
      prefix: true,
    },
  ];

  return (
    <div className="flex min-h-screen flex-col">
      <header className="border-b border-zinc-800 bg-[#0a0a0a]">
        <div className="mx-auto flex max-w-7xl items-center justify-between gap-4 px-5 pt-3 sm:px-8">
          <div className="flex min-w-0 items-center gap-2">
            <Link
              aria-label="Ship home"
              className="shrink-0 text-zinc-50"
              href={envHref() ?? "/projects"}
            >
              <ShipLogo />
            </Link>
            <EnvSwitcher environmentId={environmentId} projectId={projectId} />
          </div>
          <AccountActions />
        </div>
        <TabNav activePathname={pathname} items={items} />
      </header>

      <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}

function TabNav({
  items,
  activePathname,
}: {
  items: NavItem[];
  activePathname: string;
}) {
  return (
    <nav className="mx-auto max-w-7xl overflow-x-auto px-5 sm:px-8">
      <div className="-mb-px flex gap-1">
        {items.map((item) => {
          // hrefs may carry the ?project=…&env=… scope; match on path only.
          // Overview is an exact match on /env so section tabs can
          // prefix-match /env/<section>.
          const itemPath = item.href.split("?")[0] ?? item.href;
          const active = item.prefix
            ? activePathname.startsWith(itemPath)
            : activePathname === itemPath;
          return (
            <Link
              className={`whitespace-nowrap border-b-2 px-3 py-3 text-sm transition-colors duration-150 ${
                active
                  ? "border-zinc-50 font-medium text-zinc-50"
                  : "border-transparent text-zinc-400 hover:text-zinc-100"
              }`}
              href={item.href}
              key={item.label}
            >
              {item.label}
            </Link>
          );
        })}
      </div>
    </nav>
  );
}

function AccountActions() {
  const router = useRouter();
  const { data: session } = useSession();

  async function logout() {
    await api.session.logout();
    router.replace("/login");
    router.refresh();
  }

  return (
    <div className="flex shrink-0 items-center gap-3 text-sm">
      {session ? (
        <>
          <span className="hidden text-zinc-500 lg:inline">
            {session.user.email}
          </span>
          <button
            className="h-8 rounded-md border border-zinc-700 px-3 text-sm text-zinc-300 transition-colors duration-150 hover:border-zinc-500 hover:bg-zinc-900 hover:text-zinc-100"
            onClick={() => void logout()}
            type="button"
          >
            Sign out
          </button>
        </>
      ) : (
        <span className="text-zinc-600">Checking session…</span>
      )}
    </div>
  );
}
