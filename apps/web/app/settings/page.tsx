"use client";

import { AppHeader } from "@/features/auth/AppHeader";
import { createShipClient } from "@ship/api-client";
import type { components } from "@ship/types";
import { useRouter } from "next/navigation";
import { FormEvent, useCallback, useEffect, useState } from "react";

type Session = components["schemas"]["Session"];
type ManagedUser = components["schemas"]["ManagedUser"];
type AuditEntry = components["schemas"]["AuditEntry"];
type Role = components["schemas"]["Role"];
type AuditFilters = {
  action?: string;
  resourceType?: string;
  actorUserId?: string;
  from?: string;
  to?: string;
};

const ship = createShipClient({ baseUrl: "/api" });
const roles: Role[] = ["owner", "admin", "deployer", "viewer"];

export default function SettingsPage() {
  const router = useRouter();
  const [session, setSession] = useState<Session>();
  const [users, setUsers] = useState<ManagedUser[]>([]);
  const [audit, setAudit] = useState<AuditEntry[]>([]);
  const [error, setError] = useState<string>();
  const canManage =
    session?.user.role === "owner" || session?.user.role === "admin";

  const loadAudit = useCallback(async (filters: AuditFilters = {}) => {
    const result = await ship.GET("/audit", {
      params: {
        query: {
          limit: 50,
          action: filters.action || undefined,
          resourceType: filters.resourceType || undefined,
          actorUserId: filters.actorUserId || undefined,
          from: filters.from || undefined,
          to: filters.to || undefined,
        },
      },
    });
    if (result.data) setAudit(result.data.items);
    return result;
  }, []);

  const load = useCallback(async () => {
    const current = await ship.GET("/auth/session");
    if (!current.data) {
      router.replace("/login?expired=1");
      return;
    }
    setSession(current.data);
    if (
      current.data.user.role === "owner" ||
      current.data.user.role === "admin"
    ) {
      const [userResult, auditResult] = await Promise.all([
        ship.GET("/users"),
        loadAudit(),
      ]);
      if (userResult.data) setUsers(userResult.data.items);
      if (!auditResult.data && auditResult.error) {
        setError(auditResult.error.error.message);
      }
    }
  }, [loadAudit, router]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <>
      <AppHeader />
      <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
        <header className="border-b border-zinc-800 pb-7">
          <p className="text-sm font-medium text-emerald-400">Control plane</p>
          <h1 className="mt-2 text-3xl font-semibold">Settings</h1>
          <p className="mt-3 text-zinc-400">
            Account security, local users, roles, and the immutable audit trail.
          </p>
        </header>

        {error ? (
          <p className="mt-6 rounded-lg border border-red-900 bg-red-950/40 p-3 text-sm text-red-200">
            {error}
          </p>
        ) : null}

        <div className="mt-8 grid gap-8">
          {session ? (
            <PasswordPanel
              onError={setError}
              onSession={setSession}
              session={session}
            />
          ) : (
            <p className="text-zinc-500">Loading account settings…</p>
          )}

          {canManage && session ? (
            <UsersPanel
              currentUserID={session.user.id}
              onChanged={load}
              onError={setError}
              session={session}
              users={users}
            />
          ) : null}

          {canManage ? (
            <AuditPanel entries={audit} onFilter={loadAudit} />
          ) : null}
        </div>
      </main>
    </>
  );
}

function PasswordPanel({
  session,
  onSession,
  onError,
}: {
  session: Session;
  onSession: (session: Session) => void;
  onError: (message?: string) => void;
}) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [saved, setSaved] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaved(false);
    onError(undefined);
    const result = await ship.POST("/auth/password", {
      body: { currentPassword, newPassword },
      params: { header: { "X-CSRF-Token": session.csrfToken } },
    });
    if (!result.data) {
      onError(result.error?.error.message ?? "Password could not be changed.");
      return;
    }
    onSession(result.data);
    setCurrentPassword("");
    setNewPassword("");
    setSaved(true);
  }

  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-6">
      <h2 className="text-xl font-semibold">Password</h2>
      <p className="mt-2 text-sm text-zinc-400">
        Changing your password ends every existing session and rotates this one.
      </p>
      <form className="mt-6 grid gap-4 sm:grid-cols-2" onSubmit={submit}>
        <input
          autoComplete="current-password"
          className="rounded-lg border border-zinc-700 bg-zinc-950 px-4 py-3"
          onChange={(event) => setCurrentPassword(event.target.value)}
          placeholder="Current password"
          required
          type="password"
          value={currentPassword}
        />
        <input
          autoComplete="new-password"
          className="rounded-lg border border-zinc-700 bg-zinc-950 px-4 py-3"
          minLength={12}
          onChange={(event) => setNewPassword(event.target.value)}
          placeholder="New password"
          required
          type="password"
          value={newPassword}
        />
        <div className="flex items-center gap-4 sm:col-span-2">
          <button
            className="rounded-lg bg-emerald-400 px-4 py-2 font-semibold text-zinc-950"
            type="submit"
          >
            Change password
          </button>
          {saved ? (
            <span className="text-sm text-emerald-400">Password changed.</span>
          ) : null}
        </div>
      </form>
    </section>
  );
}

function UsersPanel({
  session,
  users,
  currentUserID,
  onChanged,
  onError,
}: {
  session: Session;
  users: ManagedUser[];
  currentUserID: string;
  onChanged: () => Promise<void>;
  onError: (message?: string) => void;
}) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<Role>("viewer");

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    onError(undefined);
    const result = await ship.POST("/users", {
      body: { email, password, role },
      params: { header: { "X-CSRF-Token": session.csrfToken } },
    });
    if (!result.data) {
      onError(result.error?.error.message ?? "User could not be created.");
      return;
    }
    setEmail("");
    setPassword("");
    setRole("viewer");
    await onChanged();
  }

  async function changeRole(id: string, nextRole: Role) {
    onError(undefined);
    const result = await ship.PATCH("/users/{id}/role", {
      params: {
        path: { id },
        header: { "X-CSRF-Token": session.csrfToken },
      },
      body: { role: nextRole },
    });
    if (!result.data) {
      onError(result.error?.error.message ?? "Role could not be changed.");
      return;
    }
    await onChanged();
  }

  async function disable(id: string) {
    onError(undefined);
    const result = await ship.DELETE("/users/{id}", {
      params: {
        path: { id },
        header: { "X-CSRF-Token": session.csrfToken },
      },
    });
    if (result.response.status !== 204) {
      onError(result.error?.error.message ?? "User could not be disabled.");
      return;
    }
    await onChanged();
  }

  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-6">
      <h2 className="text-xl font-semibold">Users and roles</h2>
      <form
        className="mt-6 grid gap-3 md:grid-cols-[1fr_1fr_160px_auto]"
        onSubmit={create}
      >
        <input
          className="rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2"
          onChange={(event) => setEmail(event.target.value)}
          placeholder="Email"
          required
          type="email"
          value={email}
        />
        <input
          className="rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2"
          minLength={12}
          onChange={(event) => setPassword(event.target.value)}
          placeholder="Temporary password"
          required
          type="password"
          value={password}
        />
        <select
          className="rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2"
          onChange={(event) => setRole(event.target.value as Role)}
          value={role}
        >
          {roles
            .filter(
              (candidate) =>
                session.user.role === "owner" || candidate !== "owner",
            )
            .map((candidate) => (
              <option key={candidate}>{candidate}</option>
            ))}
        </select>
        <button
          className="rounded-lg bg-emerald-400 px-4 py-2 font-semibold text-zinc-950"
          type="submit"
        >
          Add user
        </button>
      </form>

      <div className="mt-6 overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead className="text-zinc-500">
            <tr>
              <th className="pb-3">Account</th>
              <th className="pb-3">Role</th>
              <th className="pb-3">Status</th>
              <th />
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800">
            {users.map((user) => (
              <tr key={user.id}>
                <td className="py-3 pr-4">{user.email}</td>
                <td className="py-3 pr-4">
                  <select
                    className="rounded border border-zinc-700 bg-zinc-950 px-2 py-1 disabled:opacity-50"
                    disabled={
                      user.disabled ||
                      (session.user.role !== "owner" && user.role === "owner")
                    }
                    onChange={(event) =>
                      void changeRole(user.id, event.target.value as Role)
                    }
                    value={user.role}
                  >
                    {roles.map((candidate) => (
                      <option key={candidate}>{candidate}</option>
                    ))}
                  </select>
                </td>
                <td className="py-3 pr-4 text-zinc-400">
                  {user.disabled ? "Disabled" : "Active"}
                </td>
                <td className="py-3 text-right">
                  {!user.disabled && user.id !== currentUserID ? (
                    <button
                      className="text-red-400 hover:text-red-300"
                      onClick={() => void disable(user.id)}
                      type="button"
                    >
                      Disable
                    </button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function AuditPanel({
  entries,
  onFilter,
}: {
  entries: AuditEntry[];
  onFilter: (filters: AuditFilters) => Promise<unknown>;
}) {
  const [action, setAction] = useState("");
  const [resourceType, setResourceType] = useState("");
  const [actorUserId, setActorUserId] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");

  function filter(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void onFilter({
      action,
      resourceType,
      actorUserId,
      from: from ? new Date(from).toISOString() : undefined,
      to: to ? new Date(to).toISOString() : undefined,
    });
  }

  return (
    <section className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-6">
      <div>
        <div>
          <h2 className="text-xl font-semibold">Audit log</h2>
          <p className="mt-2 text-sm text-zinc-400">
            Append-only security and control-plane events.
          </p>
        </div>
        <form
          className="mt-5 grid gap-3 sm:grid-cols-2 lg:grid-cols-6"
          onSubmit={filter}
        >
          <input
            className="rounded border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm"
            onChange={(event) => setAction(event.target.value)}
            placeholder="Action"
            value={action}
          />
          <input
            className="rounded border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm"
            onChange={(event) => setResourceType(event.target.value)}
            placeholder="Resource type"
            value={resourceType}
          />
          <input
            className="rounded border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm"
            onChange={(event) => setActorUserId(event.target.value)}
            placeholder="Actor user ID"
            value={actorUserId}
          />
          <input
            className="rounded border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-zinc-400"
            onChange={(event) => setFrom(event.target.value)}
            type="datetime-local"
            value={from}
          />
          <input
            className="rounded border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-zinc-400"
            onChange={(event) => setTo(event.target.value)}
            type="datetime-local"
            value={to}
          />
          <button
            className="rounded border border-zinc-700 px-3 py-2 text-sm hover:bg-zinc-800"
            type="submit"
          >
            Apply filters
          </button>
        </form>
      </div>
      <div className="mt-6 space-y-3">
        {entries.map((entry) => (
          <article
            className="grid gap-2 rounded-lg border border-zinc-800 p-4 text-sm sm:grid-cols-[190px_1fr_auto]"
            key={entry.id}
          >
            <time className="text-zinc-500">
              {new Date(entry.createdAt).toLocaleString()}
            </time>
            <div>
              <span className="font-medium text-zinc-200">{entry.action}</span>
              <span className="ml-2 text-zinc-500">
                {entry.resourceType}
                {entry.resourceId ? ` · ${entry.resourceId}` : ""}
              </span>
              <p className="mt-1 text-xs text-zinc-500">
                {entry.actorEmail || "Ship system"}
              </p>
            </div>
            <span
              className={
                entry.outcome === "success"
                  ? "text-emerald-400"
                  : "text-red-400"
              }
            >
              {entry.outcome}
            </span>
          </article>
        ))}
        {entries.length === 0 ? (
          <p className="text-sm text-zinc-500">No matching events.</p>
        ) : null}
      </div>
    </section>
  );
}
