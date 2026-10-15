"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, type ReactNode } from "react";
import { api } from "@/lib/api";
import { useSession } from "@/lib/hooks";
import { EnvSwitcher } from "@/features/shell/EnvSwitcher";
import { ShipLogo } from "@/features/shell/ShipLogo";
import {
  isActive,
  navigationFor,
  pageTitle,
  type NavItem,
  type Navigation,
} from "@/features/shell/navigation";

/**
 * Dashboard shell, structured like Vercel's: a left sidebar whose items depend
 * on the scope, and a top bar with the scope switcher, the page title, and
 * account actions.
 *
 * - Control-plane scope (no project/environment): Projects, Servers, System,
 *   Settings. Settings here are the account, users, and the audit log.
 * - Environment scope (/env/...?project=X&env=Y): Overview, Applications,
 *   Databases, Variables, Deployments, Logs, Settings. Settings here belong
 *   to the environment only.
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
  const scope =
    projectId && environmentId ? { projectId, environmentId } : undefined;
  const navigation = navigationFor(scope);
  const title = pageTitle(pathname);

  useEffect(() => {
    document.title = title ? `${title} · Ship` : "Ship";
  }, [title]);

  return (
    <div className="flex min-h-screen">
      <Sidebar navigation={navigation} pathname={pathname} />

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-20 border-b border-zinc-800 bg-[#0a0a0a]">
          {/* Below lg the title is hidden and the account column sizes to its content. */}
          <div className="grid h-14 grid-cols-[minmax(0,1fr)_auto] items-center gap-4 px-4 sm:px-6 lg:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
            <div className="flex min-w-0 items-center">
              <Link
                aria-label="Ship home"
                className="mr-2 shrink-0 text-zinc-50 md:hidden"
                href="/projects"
              >
                <ShipLogo size={20} />
              </Link>
              <EnvSwitcher
                environmentId={environmentId}
                projectId={projectId}
              />
            </div>
            <span className="hidden truncate text-sm font-medium text-zinc-100 lg:block">
              {title}
            </span>
            <AccountActions />
          </div>
          <MobileNav
            items={[...navigation.primary, ...navigation.secondary]}
            pathname={pathname}
          />
        </header>

        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </div>
  );
}

function Sidebar({
  navigation,
  pathname,
}: {
  navigation: Navigation;
  pathname: string;
}) {
  return (
    <aside className="sticky top-0 hidden h-screen w-60 shrink-0 flex-col border-r border-zinc-800 bg-[#0a0a0a] md:flex">
      <div className="flex h-14 shrink-0 items-center px-5">
        <Link
          aria-label="Ship home"
          className="flex items-center gap-2 text-zinc-50"
          href="/projects"
        >
          <ShipLogo size={20} />
          <span className="text-sm font-semibold tracking-tight">Ship</span>
        </Link>
      </div>
      <nav
        aria-label="Primary"
        className="flex flex-1 flex-col gap-0.5 overflow-y-auto px-3 pb-4 pt-1"
      >
        {navigation.primary.map((item) => (
          <SidebarLink
            active={isActive(item, pathname)}
            item={item}
            key={item.label}
          />
        ))}
        {navigation.secondary.length > 0 ? (
          <>
            <div className="my-3 border-t border-zinc-800" />
            {navigation.secondary.map((item) => (
              <SidebarLink
                active={isActive(item, pathname)}
                item={item}
                key={item.label}
              />
            ))}
          </>
        ) : null}
      </nav>
    </aside>
  );
}

function SidebarLink({ item, active }: { item: NavItem; active: boolean }) {
  const Icon = item.icon;
  return (
    <Link
      aria-current={active ? "page" : undefined}
      className={`flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors duration-150 ${
        active
          ? "bg-zinc-900 font-medium text-zinc-50"
          : "text-zinc-400 hover:bg-zinc-900/60 hover:text-zinc-100"
      }`}
      href={item.href}
    >
      <Icon className={active ? "text-zinc-100" : "text-zinc-500"} />
      {item.label}
    </Link>
  );
}

/** Below the md breakpoint the sidebar is replaced by a scrollable strip. */
function MobileNav({
  items,
  pathname,
}: {
  items: NavItem[];
  pathname: string;
}) {
  return (
    <nav
      aria-label="Primary"
      className="flex gap-1 overflow-x-auto border-t border-zinc-800 px-3 py-2 md:hidden"
    >
      {items.map((item) => {
        const active = isActive(item, pathname);
        return (
          <Link
            aria-current={active ? "page" : undefined}
            className={`whitespace-nowrap rounded-md px-3 py-1.5 text-sm transition-colors duration-150 ${
              active
                ? "bg-zinc-900 font-medium text-zinc-50"
                : "text-zinc-400 hover:text-zinc-100"
            }`}
            href={item.href}
            key={item.label}
          >
            {item.label}
          </Link>
        );
      })}
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
    <div className="flex shrink-0 items-center justify-end gap-3 text-sm lg:col-start-3">
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
