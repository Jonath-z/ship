"use client";

import { createShipClient } from "@ship/api-client";
import type { components } from "@ship/api-client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useEffect, useState } from "react";

type SetupStatus = components["schemas"]["SetupStatus"];

const ship = createShipClient({ baseUrl: "/api" });

export default function SetupPage() {
  const router = useRouter();
  const [status, setStatus] = useState<SetupStatus>();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [token, setToken] = useState("");
  const [error, setError] = useState<string>();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    void loadStatus();
  }, []);

  async function loadStatus() {
    const result = await ship.GET("/setup", {
      headers: { "Cache-Control": "no-store" },
    });
    if (result.data) {
      setStatus(result.data);
      setError(undefined);
    } else {
      setError(result.error?.error.message ?? "Setup status is unavailable.");
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError(undefined);
    try {
      const result = await ship.POST("/setup", {
        body: { email, password, token },
      });
      if (!result.data) {
        setError(
          result.error?.error.message ?? "Owner account could not be created.",
        );
        return;
      }
      setStatus((current) =>
        current ? { ...current, required: false } : current,
      );
      setPassword("");
      setToken("");
      router.replace("/dashboard");
      router.refresh();
    } finally {
      setSaving(false);
    }
  }

  if (!status) {
    return (
      <main className="grid min-h-screen place-items-center px-6">
        <div className="text-center">
          <p className="text-sm font-medium text-emerald-400">Ship setup</p>
          <p className="mt-3 text-zinc-300">
            {error ?? "Checking this installation…"}
          </p>
          {error ? (
            <button
              className="mt-5 rounded-lg border border-zinc-700 px-4 py-2 text-sm hover:bg-zinc-900"
              onClick={() => void loadStatus()}
              type="button"
            >
              Try again
            </button>
          ) : null}
        </div>
      </main>
    );
  }

  if (!status.required) {
    return (
      <main className="grid min-h-screen place-items-center px-6">
        <section className="w-full max-w-lg rounded-2xl border border-zinc-800 bg-zinc-900/60 p-8 text-center">
          <p className="text-sm font-medium text-emerald-400">Setup complete</p>
          <h1 className="mt-3 text-3xl font-semibold">Ship is ready.</h1>
          <p className="mt-4 text-zinc-400">
            The owner exists and this setup endpoint is now permanently
            disabled.
          </p>
          <Link
            className="mt-7 inline-block rounded-lg bg-emerald-400 px-5 py-3 font-semibold text-zinc-950 hover:bg-emerald-300"
            href="/dashboard"
          >
            Open dashboard
          </Link>
        </section>
      </main>
    );
  }

  return (
    <main className="mx-auto grid min-h-screen max-w-5xl items-center gap-10 px-6 py-14 lg:grid-cols-[1fr_1.1fr]">
      <section>
        <p className="text-sm font-semibold uppercase tracking-[0.24em] text-emerald-400">
          First run
        </p>
        <h1 className="mt-4 text-4xl font-semibold tracking-tight sm:text-5xl">
          Create the Ship owner.
        </h1>
        <p className="mt-5 max-w-md leading-7 text-zinc-400">
          This is the only account that can be created with the installer token.
          The setup closes as soon as the owner is saved.
        </p>

        <dl className="mt-8 space-y-4 text-sm">
          <div>
            <dt className="text-zinc-500">Control-plane hostname</dt>
            <dd className="mt-1 text-zinc-200">{status.hostname}</dd>
          </div>
          <div>
            <dt className="text-zinc-500">Next steps</dt>
            <dd className="mt-1 text-zinc-300">
              Add your first server and connect GitHub from Settings when those
              features are configured.
            </dd>
          </div>
        </dl>
      </section>

      <form
        className="rounded-2xl border border-zinc-800 bg-zinc-900/60 p-7 sm:p-9"
        onSubmit={submit}
      >
        <label className="block">
          <span className="text-sm font-medium text-zinc-300">Owner email</span>
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
            autoComplete="new-password"
            className="mt-2 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-4 py-3 outline-none focus:border-emerald-500"
            minLength={12}
            onChange={(event) => setPassword(event.target.value)}
            required
            type="password"
            value={password}
          />
          <span className="mt-2 block text-xs text-zinc-500">
            At least 12 characters. Ship stores an Argon2id hash.
          </span>
        </label>

        <label className="mt-5 block">
          <span className="text-sm font-medium text-zinc-300">
            First-run token
          </span>
          <input
            autoComplete="off"
            className="mt-2 w-full rounded-lg border border-zinc-700 bg-zinc-950 px-4 py-3 font-mono text-sm outline-none focus:border-emerald-500"
            minLength={32}
            onChange={(event) => setToken(event.target.value)}
            required
            value={token}
          />
          <span className="mt-2 block text-xs text-zinc-500">
            Paste the token printed after installation.
          </span>
        </label>

        {!status.tokenConfigured ? (
          <p className="mt-5 rounded-lg border border-amber-900 bg-amber-950/40 p-3 text-sm text-amber-200">
            No token is configured. Run <code>ship setup-token</code> on the
            VPS.
          </p>
        ) : null}
        {error ? (
          <p className="mt-5 rounded-lg border border-red-900 bg-red-950/40 p-3 text-sm text-red-200">
            {error}
          </p>
        ) : null}

        <button
          className="mt-7 w-full rounded-lg bg-emerald-400 px-5 py-3 font-semibold text-zinc-950 hover:bg-emerald-300 disabled:cursor-not-allowed disabled:opacity-50"
          disabled={saving || !status.tokenConfigured}
          type="submit"
        >
          {saving ? "Creating owner…" : "Create owner"}
        </button>
      </form>
    </main>
  );
}
