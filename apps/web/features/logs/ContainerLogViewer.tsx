"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Select } from "@/components/form";
import { LoadingState } from "@/components/panels";
import { api } from "@/lib/api";

const tailSizes = [200, 500, 1000, 2000];

/**
 * Container log tail fetched over the SSH allowlist. "Follow" polls the tail
 * endpoint until the backend grows a streaming follow mode (SH-091).
 */
export function ContainerLogViewer({
  serverId,
  containerName,
}: {
  serverId: string;
  containerName: string;
}) {
  const [lines, setLines] = useState(200);
  const [follow, setFollow] = useState(false);
  const containerRef = useRef<HTMLPreElement>(null);

  const logs = useQuery({
    queryKey: ["container-logs", serverId, containerName, lines],
    queryFn: () => api.servers.containerLogs(serverId, containerName, lines),
    refetchInterval: follow ? 5_000 : false,
  });

  useEffect(() => {
    const element = containerRef.current;
    if (follow && element) element.scrollTop = element.scrollHeight;
  }, [logs.data, follow]);

  return (
    <div className="overflow-hidden rounded-xl border border-zinc-800 bg-zinc-950">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-800 px-4 py-2 text-xs text-zinc-500">
        <span className="font-mono text-zinc-400">{containerName}</span>
        <span className="flex items-center gap-2">
          <Select
            aria-label="Tail size"
            className="w-32 py-1 text-xs"
            onChange={(event) => setLines(Number(event.target.value))}
            value={lines}
          >
            {tailSizes.map((size) => (
              <option key={size} value={size}>
                Last {size} lines
              </option>
            ))}
          </Select>
          <Button
            onClick={() => setFollow((value) => !value)}
            size="sm"
            variant={follow ? "primary" : "secondary"}
          >
            {follow ? "Following" : "Follow"}
          </Button>
          <Button
            disabled={logs.isFetching}
            onClick={() => void logs.refetch()}
            size="sm"
          >
            {logs.isFetching ? "Refreshing…" : "Refresh"}
          </Button>
        </span>
      </div>
      {logs.error ? (
        <div className="p-4">
          <ErrorNotice error={logs.error} />
        </div>
      ) : null}
      {logs.isPending ? <LoadingState label="Fetching logs…" /> : null}
      {logs.data ? (
        <pre
          className="h-96 overflow-y-auto p-4 font-mono text-xs leading-relaxed text-zinc-300"
          ref={containerRef}
        >
          {logs.data.log || "No log output."}
        </pre>
      ) : null}
    </div>
  );
}
