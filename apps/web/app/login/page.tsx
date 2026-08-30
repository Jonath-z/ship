"use client";

import { createShipClient } from "@ship/api-client";
import { useRouter, useSearchParams } from "next/navigation";
import { FormEvent, Suspense, useEffect, useState } from "react";

const ship = createShipClient({ baseUrl: "/api" });

function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string>();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    void ship.GET("/setup").then(({ data }) => {
      if (data?.required) router.replace("/setup");
    });
  }, [router]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError(undefined);
    try {
      const result = await ship.POST("/auth/login", {
        body: { email, password },
      });
      if (!result.data) {
        setError(result.error?.error.message ?? "Sign in failed.");
        return;
      }
      router.replace("/dashboard");
      router.refresh();
    } finally {
      setSaving(false);
    }
  }

  return (
    <main className="grid min-h-screen place-items-center px-6 py-12">
      <section className="w-full max-w-md rounded-2xl border border-zinc-800 bg-zinc-900/60 p-8">
        <p className="text-sm font-medium text-emerald-400">
          Ship control plane
        </p>
        <h1 className="mt-3 text-3xl font-semibold">Sign in</h1>
        <p className="mt-3 text-sm leading-6 text-zinc-400">
          Use a local account created by the installation owner.
        </p>
        {searchParams.get("expired") ? (
          <p className="mt-5 rounded-lg border border-amber-900 bg-amber-950/40 p-3 text-sm text-amber-200">
            Your session expired. Sign in again to continue.
          </p>
        ) : null}
        <form className="mt-7" onSubmit={submit}>
          <label className="block">
            <span className="text-sm font-medium text-zinc-300">Email</span>
            <input
              autoComplete="email"
              className="mt-2 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-4 py-3 outline-none focus:border-emerald-500"
              onChange={(event) => setEmail(event.target.value)}
              required
              type="email"
              value={email}
            />
          </label>
          <label className="mt-5 block">
            <span className="text-sm font-medium text-zinc-300">Password</span>
            <input
              autoComplete="current-password"
              className="mt-2 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-4 py-3 outline-none focus:border-emerald-500"
              onChange={(event) => setPassword(event.target.value)}
              required
              type="password"
              value={password}
            />
          </label>
          {error ? (
            <p className="mt-5 rounded-lg border border-red-900 bg-red-950/40 p-3 text-sm text-red-200">
              {error}
            </p>
          ) : null}
          <button
            className="mt-7 w-full rounded-lg bg-emerald-400 px-5 py-3 font-semibold text-zinc-950 hover:bg-emerald-300 disabled:opacity-50"
            disabled={saving}
            type="submit"
          >
            {saving ? "Signing in…" : "Sign in"}
          </button>
        </form>
      </section>
    </main>
  );
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}
