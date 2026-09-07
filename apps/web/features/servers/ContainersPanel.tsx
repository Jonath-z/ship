"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Modal } from "@/components/Modal";
import { LoadingState, Panel } from "@/components/panels";
import { api } from "@/lib/api";

/** Docker containers running on a server, with a per-container log viewer. */
export function ContainersPanel({ serverId }: { serverId: string }) {
  const [logsFor, setLogsFor] = useState<string>();

  const containers = useQuery({
    queryKey: ["server-containers", serverId],
    queryFn: () => api.servers.containers(serverId),
  });

  return (
    <Panel
      actions={
        <Button
          disabled={containers.isFetching}
          onClick={() => void containers.refetch()}
          size="sm"
        >
          {containers.isFetching ? "Refreshing…" : "Refresh"}
        </Button>
      }
      description="Live view of Docker containers over SSH."
      title="Containers"
    >
      {containers.error ? <ErrorNotice error={containers.error} /> : null}
      {containers.isPending ? (
        <LoadingState label="Listing containers…" />
      ) : null}
      {containers.data && containers.data.items.length === 0 ? (
        <p className="py-3 text-sm text-zinc-500">
          No containers are running on this server.
        </p>
      ) : null}
      {containers.data && containers.data.items.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="text-zinc-500">
              <tr>
                <th className="pb-3 pr-4">Name</th>
                <th className="pb-3 pr-4">Image</th>
                <th className="pb-3 pr-4">State</th>
                <th className="pb-3 pr-4">Status</th>
                <th className="pb-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {containers.data.items.map((container) => (
                <tr key={container.name}>
                  <td className="py-3 pr-4 font-mono text-zinc-200">
                    {container.name}
                  </td>
                  <td className="py-3 pr-4 text-zinc-400">{container.image}</td>
                  <td className="py-3 pr-4 capitalize text-zinc-300">
                    {container.state}
                  </td>
                  <td className="py-3 pr-4 text-zinc-500">
                    {container.status}
                  </td>
                  <td className="py-3 text-right">
                    <Button
                      onClick={() => setLogsFor(container.name)}
                      size="sm"
                      variant="ghost"
                    >
                      Logs
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {logsFor ? (
        <ContainerLogsDialog
          containerName={logsFor}
          onClose={() => setLogsFor(undefined)}
          serverId={serverId}
        />
      ) : null}
    </Panel>
  );
}

function ContainerLogsDialog({
  serverId,
  containerName,
  onClose,
}: {
  serverId: string;
  containerName: string;
  onClose: () => void;
}) {
  const logs = useQuery({
    queryKey: ["container-logs", serverId, containerName],
    queryFn: () => api.servers.containerLogs(serverId, containerName),
  });

  return (
    <Modal onClose={onClose} title={`Logs · ${containerName}`} wide>
      {logs.isPending ? <LoadingState label="Fetching logs…" /> : null}
      {logs.error ? <ErrorNotice error={logs.error} /> : null}
      {logs.data ? (
        <pre className="max-h-[60vh] overflow-y-auto rounded-lg border border-zinc-800 bg-zinc-950 p-4 font-mono text-xs leading-relaxed text-zinc-300">
          {logs.data.log || "No log output."}
        </pre>
      ) : null}
      <div className="mt-4 flex justify-end gap-2">
        <Button
          disabled={logs.isFetching}
          onClick={() => void logs.refetch()}
        >
          {logs.isFetching ? "Refreshing…" : "Refresh"}
        </Button>
        <Button onClick={onClose} variant="primary">
          Close
        </Button>
      </div>
    </Modal>
  );
}
