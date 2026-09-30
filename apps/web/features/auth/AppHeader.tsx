"use client";

import { createShipClient } from "@ship/api-client";
import type { components } from "@ship/api-client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ShipLogo } from "@/features/shell/ShipLogo";

type Session = components["schemas"]["Session"];

const ship = createShipClient({ baseUrl: "/api" });

export function AppHeader() {
  const router = useRouter();
  const [session, setSession] = useState<Session>();

  useEffect(() => {
    void ship.GET("/auth/session").then(({ data, response }) => {
      if (response.status === 401) {
        router.replace("/login?expired=1");
        return;
      }
      if (data) setSession(data);
    });
  }, [router]);

  async function logout() {
    if (!session) return;
    await ship.POST("/auth/logout", {
      params: { header: { "X-CSRF-Token": session.csrfToken } },
    });
    router.replace("/login");
    router.refresh();
  }

  return (
    <nav className="border-b border-zinc-800 bg-[#0a0a0a] px-5 py-3 sm:px-8">
      <div className="mx-auto flex max-w-7xl items-center justify-between gap-4">
        <div className="flex items-center gap-5">
          <Link
            className="flex items-center gap-2 font-semibold text-zinc-50"
            href="/projects"
          >
            <ShipLogo />
            Ship
          </Link>
          <Link
            className="text-sm text-zinc-400 transition-colors duration-150 hover:text-zinc-100"
            href="/projects"
          >
            Projects
          </Link>
          <Link
            className="text-sm text-zinc-400 transition-colors duration-150 hover:text-zinc-100"
            href="/dashboard"
          >
            System
          </Link>
          <Link
            className="text-sm text-zinc-400 transition-colors duration-150 hover:text-zinc-100"
            href="/settings"
          >
            Settings
          </Link>
        </div>
        <div className="flex items-center gap-3 text-sm">
          {session ? (
            <>
              <span className="hidden text-zinc-500 sm:inline">
                {session.user.email} · {session.user.role}
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
      </div>
    </nav>
  );
}
