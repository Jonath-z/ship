"use client";

import { createShipClient } from "@ship/api-client";
import type { components } from "@ship/api-client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

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
    <nav className="border-b border-zinc-800 bg-zinc-950/90 px-5 py-3 sm:px-8">
      <div className="mx-auto flex max-w-7xl items-center justify-between gap-4">
        <div className="flex items-center gap-5">
          <Link className="font-semibold text-emerald-400" href="/dashboard">
            Ship
          </Link>
          <Link
            className="text-sm text-zinc-400 hover:text-zinc-100"
            href="/dashboard"
          >
            System
          </Link>
          <Link
            className="text-sm text-zinc-400 hover:text-zinc-100"
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
    </nav>
  );
}
