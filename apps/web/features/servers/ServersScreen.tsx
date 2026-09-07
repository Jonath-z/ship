"use client";

import Link from "next/link";
import { useState } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { StatusDot } from "@/components/StatusDot";
import {
  EmptyState,
  LoadingState,
  PageHeader,
  Panel,
} from "@/components/panels";
import type { ServerResources } from "@/lib/api";
import { formatBytes, relativeTime } from "@/lib/format";
import { useServers } from "@/lib/hooks";
import { AddServerWizard } from "@/features/servers/AddServerWizard";
import { SshKeysPanel } from "@/features/servers/SshKeysPanel";

/** Compact resource summary such as "4 CPU · 8 GB". */
export function formatResources(resources: ServerResources): string {
  const parts: string[] = [];
  if (resources.cpuCores) parts.push(`${resources.cpuCores} CPU`);
  if (resources.memoryBytes) parts.push(formatBytes(resources.memoryBytes));
  if (resources.diskBytes) parts.push(`${formatBytes(resources.diskBytes)} disk`);
  return parts.length > 0 ? parts.join(" · ") : "—";
}

/** Fleet overview: registered servers plus SSH key management. */
export function ServersScreen() {
  const [adding, setAdding] = useState(false);
  const { data, isPending, error } = useServers();

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        actions={
          <Button onClick={() => setAdding(true)} variant="primary">
            Add server
          </Button>
        }
        description="Servers run your applications and databases over SSH. Register a server, verify it with checks, and prepare it for deployments."
        eyebrow="Infrastructure"
        title="Servers"
      />

      <div className="mt-8 grid gap-6">
        <Panel title="Registered servers">
          {error ? <ErrorNotice error={error} /> : null}
          {isPending ? <LoadingState label="Loading servers…" /> : null}
          {data && data.items.length === 0 ? (
            <EmptyState
              action={
                <Button onClick={() => setAdding(true)} variant="primary">
                  Add your first server
                </Button>
              }
              description="Register an SSH-reachable host to deploy applications onto it."
              title="No servers yet"
            />
          ) : null}
          {data && data.items.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="text-zinc-500">
                  <tr>
                    <th className="pb-3 pr-4">Name</th>
                    <th className="pb-3 pr-4">Address</th>
                    <th className="pb-3 pr-4">Status</th>
                    <th className="pb-3 pr-4">Resources</th>
                    <th className="pb-3">Added</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-800">
                  {data.items.map((server) => (
                    <tr key={server.id}>
                      <td className="py-3 pr-4">
                        <Link
                          className="font-medium text-zinc-200 hover:text-emerald-300"
                          href={`/servers/${server.id}`}
                        >
                          {server.name}
                        </Link>
                      </td>
                      <td className="py-3 pr-4 text-zinc-400">
                        {server.hostname || server.ipAddress || "—"}
                        <span className="text-zinc-600">
                          {" "}
                          · {server.sshUser}@{server.sshPort}
                        </span>
                      </td>
                      <td className="py-3 pr-4">
                        <StatusDot status={server.status} />
                      </td>
                      <td className="py-3 pr-4 text-zinc-400">
                        {formatResources(server.resources)}
                      </td>
                      <td className="py-3 text-zinc-500">
                        {relativeTime(server.createdAt)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
        </Panel>

        <SshKeysPanel />
      </div>

      {adding ? <AddServerWizard onClose={() => setAdding(false)} /> : null}
    </main>
  );
}
