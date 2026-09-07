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
import { EnvSwitcher } from "@/features/shell/EnvSwitcher";

interface NavItem {
  label: string;
  href: string;
  /** Match nested routes too. */
  prefix?: boolean;
}

/**
 * Dashboard shell: left sidebar navigation plus a top bar with the
 * project/environment switcher. Environment scope comes from the URL
 * (/p/[projectId]/e/[environmentId]/...); globally scoped screens such as
 * /servers fall back to the last visited environment for their nav links.
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
  const envBase = scope
    ? `/p/${scope.projectId}/e/${scope.environmentId}`
    : undefined;

  const items: NavItem[] = [
    { label: "Overview", href: envBase ?? "/projects" },
    {
      label: "Applications",
      href: envBase ? `${envBase}/applications` : "/projects",
      prefix: true,
    },
    { label: "Servers", href: "/servers", prefix: true },
    {
      label: "Databases",
      href: envBase ? `${envBase}/databases` : "/projects",
      prefix: true,
    },
    {
      label: "Deployments",
      href: envBase ? `${envBase}/deployments` : "/projects",
      prefix: true,
    },
    {
      label: "Settings",
      href: envBase ? `${envBase}/settings` : "/settings",
      prefix: true,
    },
  ];

  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-56 shrink-0 flex-col border-r border-zinc-800 bg-zinc-950 px-3 py-5 md:flex">
        <Link
          className="px-3 text-lg font-semibold text-emerald-400"
          href={envBase ?? "/projects"}
        >
          Ship
        </Link>
        <nav className="mt-6 flex flex-col gap-1">
          {items.map((item) => {
            const active = item.prefix
              ? pathname.startsWith(item.href)
              : pathname === item.href;
            return (
              <Link
                className={`rounded-lg px-3 py-2 text-sm ${
                  active
                    ? "bg-zinc-900 font-medium text-zinc-100"
                    : "text-zinc-400 hover:bg-zinc-900/60 hover:text-zinc-200"
                }`}
                href={item.href}
                key={item.label}
              >
                {item.label}
              </Link>
            );
          })}
        </nav>
        <div className="mt-auto px-3 pt-6 text-xs text-zinc-600">
          Ship control plane
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar environmentId={environmentId} projectId={projectId} />
        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </div>
  );
}

function TopBar({
  projectId,
  environmentId,
}: {
  projectId?: string;
  environmentId?: string;
}) {
  const router = useRouter();
  const { data: session } = useSession();

  async function logout() {
    await api.session.logout();
    router.replace("/login");
    router.refresh();
  }

  return (
    <header className="sticky top-0 z-40 border-b border-zinc-800 bg-zinc-950/90 px-4 py-3 backdrop-blur sm:px-6">
      <div className="flex items-center justify-between gap-4">
        <EnvSwitcher environmentId={environmentId} projectId={projectId} />
        <div className="flex items-center gap-3 text-sm">
          {session ? (
            <>
              <span className="hidden text-zinc-500 lg:inline">
                {session.user.email} · {session.user.role}
              </span>
              <button
                className="rounded-md border border-zinc-700 px-3 py-1.5 text-zinc-300 hover:bg-zinc-900"
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
      </div>
    </header>
  );
}
