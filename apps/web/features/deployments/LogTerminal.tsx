"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  api,
  type DeploymentLogEntry,
  type DeploymentStatus,
  type DeploymentStreamEvent,
} from "@/lib/api";

type StreamState = "connecting" | "live" | "done" | "error";

const streamLabels: Record<StreamState, string> = {
  connecting: "connecting…",
  live: "live",
  done: "finished",
  error: "reconnecting…",
};

/**
 * Live deployment log: loads the persisted log, then follows the SSE stream
 * (plain EventSource — the fetch wrapper cannot stream). Lines are deduped by
 * sequence; auto-scroll pauses while the user scrolls up.
 */
export function LogTerminal({
  projectId,
  environmentId,
  deploymentId,
  onStatus,
}: {
  projectId: string;
  environmentId: string;
  deploymentId: string;
  onStatus?: (status: DeploymentStatus) => void;
}) {
  const [lines, setLines] = useState<DeploymentLogEntry[]>([]);
  const [streamState, setStreamState] = useState<StreamState>("connecting");
  const [stickToBottom, setStickToBottom] = useState(true);
  const containerRef = useRef<HTMLDivElement>(null);
  const seenRef = useRef(new Set<number>());
  const lastSeqRef = useRef(0);
  const statusRef = useRef(onStatus);
  statusRef.current = onStatus;

  const append = useCallback((incoming: DeploymentLogEntry[]) => {
    const fresh = incoming.filter(
      (entry) => !seenRef.current.has(entry.sequence),
    );
    if (fresh.length === 0) return;
    for (const entry of fresh) {
      seenRef.current.add(entry.sequence);
      if (entry.sequence > lastSeqRef.current) {
        lastSeqRef.current = entry.sequence;
      }
    }
    setLines((previous) =>
      [...previous, ...fresh].sort((a, b) => a.sequence - b.sequence),
    );
  }, []);

  useEffect(() => {
    let cancelled = false;
    let source: EventSource | undefined;

    async function start() {
      try {
        const initial = await api.deployments.logs(
          projectId,
          environmentId,
          deploymentId,
        );
        if (cancelled) return;
        append(initial.items);
      } catch {
        // The stream below still delivers everything after sequence 0.
      }
      if (cancelled) return;

      // Deliberately NOT the fetch wrapper: SSE needs a native EventSource.
      source = new EventSource(
        api.deployments.streamUrl(
          projectId,
          environmentId,
          deploymentId,
          lastSeqRef.current,
        ),
        { withCredentials: true },
      );
      source.onopen = () => setStreamState("live");
      source.onerror = () => setStreamState("error");
      source.onmessage = (event: MessageEvent<string>) => {
        try {
          const parsed = JSON.parse(event.data) as DeploymentStreamEvent;
          if (parsed.type === "status" && parsed.status) {
            statusRef.current?.(parsed.status);
            return;
          }
          append([
            {
              sequence: parsed.sequence,
              stream: parsed.stream ?? "out",
              message: parsed.message ?? "",
              at: new Date().toISOString(),
            },
          ]);
        } catch {
          // Ignore malformed frames; the next one resyncs by sequence.
        }
      };
      source.addEventListener("done", () => {
        setStreamState("done");
        source?.close();
      });
    }

    void start();
    return () => {
      cancelled = true;
      source?.close();
    };
  }, [projectId, environmentId, deploymentId, append]);

  useEffect(() => {
    const container = containerRef.current;
    if (stickToBottom && container) {
      container.scrollTop = container.scrollHeight;
    }
  }, [lines, stickToBottom]);

  function onScroll() {
    const container = containerRef.current;
    if (!container) return;
    const atBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight <
      32;
    setStickToBottom(atBottom);
  }

  return (
    <div className="overflow-hidden rounded-xl border border-zinc-800 bg-zinc-950">
      <div className="flex items-center justify-between border-b border-zinc-800 px-4 py-2 text-xs text-zinc-500">
        <span>Deployment log</span>
        <span className="flex items-center gap-3">
          {!stickToBottom ? (
            <button
              className="text-emerald-400 hover:text-emerald-300"
              onClick={() => setStickToBottom(true)}
              type="button"
            >
              Resume auto-scroll
            </button>
          ) : null}
          <span
            className={
              streamState === "live"
                ? "text-emerald-400"
                : streamState === "error"
                  ? "text-amber-400"
                  : undefined
            }
          >
            {streamLabels[streamState]}
          </span>
        </span>
      </div>
      <div
        className="h-96 overflow-y-auto p-4 font-mono text-xs leading-relaxed"
        onScroll={onScroll}
        ref={containerRef}
      >
        {lines.map((line) => (
          <p
            className={
              line.stream === "err" || line.stream === "stderr"
                ? "text-red-300"
                : "text-zinc-300"
            }
            key={line.sequence}
          >
            <span className="mr-3 select-none text-zinc-600">
              {String(line.sequence).padStart(4, " ")}
            </span>
            {line.message}
          </p>
        ))}
        {lines.length === 0 ? (
          <p className="text-zinc-600">Waiting for output…</p>
        ) : null}
      </div>
    </div>
  );
}
