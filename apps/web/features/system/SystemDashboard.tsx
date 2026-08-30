"use client";

import { createShipClient } from "@ship/api-client";
import type { components } from "@ship/types";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";

type SystemStatus = components["schemas"]["SystemStatus"];
type ComponentStatus = components["schemas"]["ComponentStatus"];

const ship = createShipClient({ baseUrl: "/api" });
const componentOrder = ["api", "worker", "postgres", "redis"] as const;

export function SystemDashboard() {
  const router = useRouter();
  const [system, setSystem] = useState<SystemStatus>();
  const [error, setError] = useState<string>();
  const [refreshing, setRefreshing] = useState(true);

  const refresh = useCallback(async () => {
    setRefreshing(true);
    try {
      const {
        data,
        error: requestError,
        response,
      } = await ship.GET("/system", {
        headers: { "Cache-Control": "no-store" },
      });
      if (response.status === 401) {
        router.replace("/login?expired=1");
        return;
      }
      if (!data || requestError) {
        throw new Error("System status is unavailable");
      }
      setSystem(data);
      setError(undefined);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "System status is unavailable",
      );
    } finally {
      setRefreshing(false);
    }
  }, [router]);

  useEffect(() => {
    void refresh();
    const interval = window.setInterval(() => void refresh(), 10_000);
    return () => window.clearInterval(interval);
  }, [refresh]);

  if (!system) {
    return (
      <main className="grid min-h-screen place-items-center px-6">
        <div className="text-center">
          <p className="text-sm text-zinc-500">Ship control plane</p>
          <p className="mt-3 text-lg text-zinc-200">
            {error ?? "Loading live system status…"}
          </p>
          {error ? (
            <button
              className="mt-6 rounded-lg border border-zinc-700 px-4 py-2 text-sm hover:bg-zinc-900"
              onClick={() => void refresh()}
              type="button"
            >
              Try again
            </button>
          ) : null}
        </div>
      </main>
    );
  }

  const components = componentOrder.flatMap((name) => {
    const component = system.components[name];
    return component ? [[name, component] as const] : [];
  });

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <header className="flex flex-col gap-5 border-b border-zinc-800 pb-8 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-sm font-medium text-emerald-400">
            {system.configuration.environment}
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-tight sm:text-4xl">
            Ship system
          </h1>
          <p className="mt-3 text-sm text-zinc-500">
            Live status for {system.configuration.hostname} · refreshed every 10
            seconds
          </p>
        </div>
        <div className="flex items-center gap-3">
          {error ? (
            <span className="text-sm text-amber-400">Last refresh failed</span>
          ) : null}
          <StatusPill status={system.status} />
          <button
            className="rounded-lg border border-zinc-700 px-3 py-1.5 text-sm text-zinc-300 hover:bg-zinc-900 disabled:opacity-50"
            disabled={refreshing}
            onClick={() => void refresh()}
            type="button"
          >
            {refreshing ? "Refreshing…" : "Refresh"}
          </button>
        </div>
      </header>

      <section
        className="grid gap-4 py-8 sm:grid-cols-2 lg:grid-cols-4"
        aria-label="Ship components"
      >
        {components.map(([name, component]) => (
          <ComponentCard component={component} key={name} name={name} />
        ))}
      </section>

      <section className="grid gap-6 lg:grid-cols-[1.35fr_1fr]">
        <article className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-6">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-xs font-medium uppercase tracking-wider text-zinc-500">
                Machine
              </p>
              <h2 className="mt-2 text-xl font-semibold">
                {system.machine.hostname || "Unknown host"}
              </h2>
            </div>
            <span className="rounded-md bg-zinc-800 px-2 py-1 text-xs text-zinc-400">
              {system.machine.scope}
            </span>
          </div>

          <div className="mt-7 space-y-6">
            <ResourceBar
              detail={`${system.machine.cpu.logicalCores} logical cores`}
              label="CPU"
              percent={system.machine.cpu.usedPercent}
            />
            <ResourceBar
              detail={`${formatBytes(system.machine.memory.usedBytes)} of ${formatBytes(system.machine.memory.totalBytes)}`}
              label="Memory"
              percent={system.machine.memory.usedPercent}
            />
            <ResourceBar
              detail={`${formatBytes(system.machine.disk.usedBytes)} of ${formatBytes(system.machine.disk.totalBytes)}`}
              label={`Disk ${system.machine.disk.path}`}
              percent={system.machine.disk.usedPercent}
            />
          </div>

          <dl className="mt-8 grid gap-x-6 gap-y-5 border-t border-zinc-800 pt-6 sm:grid-cols-2">
            <Detail
              label="Operating system"
              value={machineOS(system.machine)}
            />
            <Detail
              label="Kernel"
              value={system.machine.kernelVersion || "Unavailable"}
            />
            <Detail label="Architecture" value={system.machine.architecture} />
            <Detail
              label="CPU"
              value={`${system.machine.cpu.logicalCores} logical / ${system.machine.cpu.physicalCores} physical`}
            />
            <Detail
              label="Load average"
              value={`${system.machine.cpu.load1.toFixed(2)} · ${system.machine.cpu.load5.toFixed(2)} · ${system.machine.cpu.load15.toFixed(2)}`}
            />
            <Detail
              label="Machine uptime"
              value={formatDuration(system.machine.uptimeSeconds)}
            />
            <Detail
              label="Virtualization"
              value={
                [
                  system.machine.virtualizationSystem,
                  system.machine.virtualizationRole,
                ]
                  .filter(Boolean)
                  .join(" · ") || "None reported"
              }
            />
            <Detail
              label="Available memory"
              value={formatBytes(system.machine.memory.availableBytes)}
            />
          </dl>
        </article>

        <div className="grid gap-6">
          <article className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-6">
            <p className="text-xs font-medium uppercase tracking-wider text-zinc-500">
              Ship configuration
            </p>
            <dl className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">
              <Detail label="Hostname" value={system.configuration.hostname} />
              <Detail
                label="Public URL"
                value={system.configuration.publicUrl}
              />
              <Detail
                label="Transport security"
                value={
                  system.configuration.secureTransport
                    ? "HTTPS"
                    : "Insecure HTTP bootstrap"
                }
              />
              <Detail
                label="Environment"
                value={system.configuration.environment}
              />
              <Detail
                label="API address"
                value={system.configuration.apiAddress}
              />
              <Detail
                label="Worker address"
                value={system.configuration.workerAddress}
              />
              <Detail
                label="Data directory"
                value={system.configuration.dataDirectory}
              />
              <Detail label="Log level" value={system.configuration.logLevel} />
              <Detail
                label="Migrations on start"
                value={
                  system.configuration.migrationsOnStart
                    ? "Enabled"
                    : "Disabled"
                }
              />
              <Detail
                label="Sensitive values"
                value={
                  system.configuration.sensitiveValuesHidden
                    ? "Hidden"
                    : "Visible"
                }
              />
            </dl>
          </article>

          <article className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-6">
            <p className="text-xs font-medium uppercase tracking-wider text-zinc-500">
              API runtime
            </p>
            <dl className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2">
              <Detail label="Version" value={system.runtime.version} />
              <Detail label="Build" value={system.runtime.buildSha} />
              <Detail label="Go" value={system.runtime.goVersion} />
              <Detail
                label="Process"
                value={`PID ${system.runtime.processId}`}
              />
              <Detail
                label="Process uptime"
                value={formatDuration(system.runtime.uptimeSeconds)}
              />
              <Detail
                label="Goroutines"
                value={String(system.runtime.goroutines)}
              />
              <Detail
                label="Heap allocated"
                value={formatBytes(system.runtime.heapAllocatedBytes)}
              />
              <Detail
                label="Runtime memory"
                value={formatBytes(system.runtime.systemMemoryBytes)}
              />
            </dl>
          </article>
        </div>
      </section>

      {system.warnings.length ? (
        <aside className="mt-6 rounded-xl border border-amber-900/60 bg-amber-950/30 p-5 text-sm text-amber-200">
          {system.warnings.join(" · ")}
        </aside>
      ) : null}

      <footer className="py-8 text-xs text-zinc-600">
        Collected {new Date(system.collectedAt).toLocaleString()} · machine
        metrics reflect what the API container can see
      </footer>
    </main>
  );
}

function ComponentCard({
  name,
  component,
}: {
  name: string;
  component: ComponentStatus;
}) {
  const healthy = component.status === "ok";
  return (
    <article className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-5">
      <div className="flex items-center justify-between gap-3">
        <h2 className="font-semibold capitalize">{name}</h2>
        <span
          className={`text-sm ${healthy ? "text-emerald-400" : "text-amber-400"}`}
        >
          {component.status}
        </span>
      </div>
      <p className="mt-4 text-sm text-zinc-500">{component.detail}</p>
      {component.lastSeenAt ? (
        <p className="mt-2 text-xs text-zinc-600">
          Last seen {new Date(component.lastSeenAt).toLocaleTimeString()}
        </p>
      ) : null}
    </article>
  );
}

function StatusPill({ status }: { status: SystemStatus["status"] }) {
  const healthy = status === "ok";
  return (
    <span
      className={`rounded-full border px-3 py-1 text-sm ${
        healthy
          ? "border-emerald-900 bg-emerald-950 text-emerald-300"
          : "border-amber-900 bg-amber-950 text-amber-300"
      }`}
    >
      {healthy ? "All systems operational" : "System degraded"}
    </span>
  );
}

function ResourceBar({
  label,
  detail,
  percent,
}: {
  label: string;
  detail: string;
  percent: number;
}) {
  const bounded = Math.max(0, Math.min(percent, 100));
  return (
    <div>
      <div className="mb-2 flex items-center justify-between gap-4 text-sm">
        <span className="font-medium text-zinc-200">{label}</span>
        <span className="text-zinc-500">
          {detail} · {bounded.toFixed(1)}%
        </span>
      </div>
      <div className="h-2 overflow-hidden rounded-full bg-zinc-800">
        <div
          className={
            bounded >= 90
              ? "h-full rounded-full bg-amber-500"
              : "h-full rounded-full bg-emerald-500"
          }
          style={{ width: `${bounded}%` }}
        />
      </div>
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-zinc-500">{label}</dt>
      <dd className="mt-1 truncate text-sm text-zinc-200" title={value}>
        {value || "Unavailable"}
      </dd>
    </div>
  );
}

function formatBytes(bytes: number) {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  );
  return `${(bytes / 1024 ** index).toFixed(index > 2 ? 1 : 0)} ${units[index]}`;
}

function formatDuration(seconds: number) {
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  return [days ? `${days}d` : "", hours ? `${hours}h` : "", `${minutes}m`]
    .filter(Boolean)
    .join(" ");
}

function machineOS(machine: SystemStatus["machine"]) {
  return [machine.platform, machine.platformVersion, machine.operatingSystem]
    .filter(Boolean)
    .join(" ");
}
