"use client";

import { createShipClient } from "@ship/api-client";
import type { components } from "@ship/api-client";
import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useState, type ReactNode } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import {
  EmptyState,
  LoadingState,
  PageHeader,
  Panel,
} from "@/components/panels";
import { api } from "@/lib/api";
import {
  useAccessories,
  useServerGroups,
  useServices,
} from "@/lib/hooks";
import { DeployModal } from "@/features/deployments/DeployModal";
import { DeploymentTable } from "@/features/deployments/DeploymentTable";
import { DiffView } from "@/features/deployments/DiffView";
import { StatusBadge } from "@/features/deployments/StatusBadge";
import { ValidationList } from "@/features/services/ValidationList";

type SystemStatus = components["schemas"]["SystemStatus"];

const ship = createShipClient({ baseUrl: "/api" });
const componentOrder = ["api", "worker", "postgres", "redis"] as const;

/**
 * Environment overview (SH-140, form-based V1): aggregate counts, validation
 * state, pending configuration changes, and recent deployments. Topology and
 * drift arrive with the V1.1 canvas and monitoring epics.
 */
export function EnvironmentOverview({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const [deploying, setDeploying] = useState(false);
  const base = `/p/${projectId}/e/${environmentId}`;

  const environment = useQuery({
    queryKey: ["environment", projectId, environmentId],
    queryFn: () => api.environments.get(projectId, environmentId),
  });
  const services = useServices(projectId, environmentId);
  const accessories = useAccessories(projectId, environmentId);
  const serverGroups = useServerGroups(projectId, environmentId);

  const preview = useQuery({
    queryKey: ["configuration-preview", projectId, environmentId],
    queryFn: () => api.configuration.preview(projectId, environmentId),
  });
  const pendingDiff = useQuery({
    queryKey: ["configuration-pending-diff", projectId, environmentId],
    queryFn: () => api.configuration.pendingDiff(projectId, environmentId),
  });
  const deployments = useQuery({
    queryKey: ["deployments", projectId, environmentId, "recent"],
    queryFn: () => api.deployments.list(projectId, environmentId, { limit: 5 }),
    refetchInterval: 10_000,
  });

  const serverIds = new Set(
    (serverGroups.data?.items ?? []).flatMap((group) =>
      group.members.map((member) => member.id),
    ),
  );
  const pendingChanges = (pendingDiff.data?.entities ?? []).filter(
    (entity) => entity.change !== "unchanged",
  ).length;
  const lastDeployment = deployments.data?.items[0];

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        actions={
          <Button onClick={() => setDeploying(true)} variant="primary">
            Deploy
          </Button>
        }
        description="Desired state, pending changes, and recent activity for this environment."
        eyebrow={environment.data ? environment.data.name : "Environment"}
        title="Overview"
      />

      <section className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          href={`${base}/applications`}
          label="Applications"
          value={services.data ? String(services.data.items.length) : "…"}
        />
        <StatCard
          href={`${base}/databases`}
          label="Databases"
          value={accessories.data ? String(accessories.data.items.length) : "…"}
        />
        <StatCard
          href="/servers"
          label="Servers"
          value={serverGroups.data ? String(serverIds.size) : "…"}
        />
        <StatCard
          href={
            lastDeployment
              ? `${base}/deployments/${lastDeployment.id}`
              : `${base}/deployments`
          }
          label="Last deployment"
          value={
            deployments.isPending ? (
              "…"
            ) : lastDeployment ? (
              <StatusBadge status={lastDeployment.status} />
            ) : (
              "None yet"
            )
          }
        />
      </section>

      <section className="mt-6 grid gap-6 lg:grid-cols-2">
        <Panel
          description={
            pendingChanges > 0
              ? `${pendingChanges} ${pendingChanges === 1 ? "entity" : "entities"} differ from the last deployed snapshot.`
              : "The desired configuration matches the last snapshot."
          }
          title="Pending configuration changes"
        >
          {pendingDiff.isPending ? (
            <LoadingState label="Computing diff…" />
          ) : null}
          {pendingDiff.error ? <ErrorNotice error={pendingDiff.error} /> : null}
          {pendingDiff.data ? <DiffView diff={pendingDiff.data} /> : null}
        </Panel>

        <Panel
          description="Validation runs against the current desired state; blocking issues disable deploys."
          title="Configuration validation"
        >
          {preview.isPending ? (
            <LoadingState label="Validating configuration…" />
          ) : null}
          {preview.error ? <ErrorNotice error={preview.error} /> : null}
          {preview.data ? (
            <ValidationList violations={preview.data.validation} />
          ) : null}
        </Panel>
      </section>

      <section className="mt-6">
        <Panel
          actions={
            <Link
              className="text-sm text-emerald-400 hover:text-emerald-300"
              href={`${base}/deployments`}
            >
              View all
            </Link>
          }
          title="Recent deployments"
        >
          {deployments.isPending ? (
            <LoadingState label="Loading deployments…" />
          ) : null}
          {deployments.error ? <ErrorNotice error={deployments.error} /> : null}
          {deployments.data && deployments.data.items.length === 0 ? (
            <EmptyState
              action={
                <Button onClick={() => setDeploying(true)} variant="primary">
                  Deploy something
                </Button>
              }
              description="Nothing has been deployed to this environment yet."
              title="No deployments"
            />
          ) : null}
          {deployments.data && deployments.data.items.length > 0 ? (
            <DeploymentTable
              deployments={deployments.data.items}
              detailBase={`${base}/deployments`}
              resolveServiceName={(serviceId) =>
                services.data?.items.find(
                  (service) => service.id === serviceId,
                )?.name
              }
            />
          ) : null}
        </Panel>
      </section>

      <section className="mt-6">
        <ControlPlaneStrip />
      </section>

      {deploying ? (
        <DeployModal
          environmentId={environmentId}
          onClose={() => setDeploying(false)}
          projectId={projectId}
        />
      ) : null}
    </main>
  );
}

function StatCard({
  label,
  value,
  href,
}: {
  label: string;
  value: ReactNode;
  href: string;
}) {
  return (
    <Link
      className="rounded-xl border border-zinc-800 bg-zinc-900/60 p-5 transition-colors hover:border-zinc-700"
      href={href}
    >
      <p className="text-xs font-medium uppercase tracking-wider text-zinc-500">
        {label}
      </p>
      <div className="mt-2 text-2xl font-semibold text-zinc-100">{value}</div>
    </Link>
  );
}

/** Compact control-plane health row; the full view lives at /dashboard. */
function ControlPlaneStrip() {
  const system = useQuery({
    queryKey: ["system-status"],
    queryFn: async () => {
      const { data, error } = await ship.GET("/system", {
        headers: { "Cache-Control": "no-store" },
      });
      if (!data || error) throw new Error("System status is unavailable");
      return data as SystemStatus;
    },
    refetchInterval: 10_000,
  });

  return (
    <Panel
      actions={
        <Link
          className="text-sm text-emerald-400 hover:text-emerald-300"
          href="/dashboard"
        >
          Full system status
        </Link>
      }
      title="Control plane"
    >
      {system.isPending ? <LoadingState label="Checking components…" /> : null}
      {system.error ? <ErrorNotice error={system.error} /> : null}
      {system.data ? (
        <div className="flex flex-wrap gap-4">
          {componentOrder.flatMap((name) => {
            const component = system.data.components[name];
            if (!component) return [];
            const healthy = component.status === "ok";
            return [
              <span
                className="flex items-center gap-2 text-sm text-zinc-300"
                key={name}
              >
                <span
                  className={`h-2 w-2 rounded-full ${
                    healthy ? "bg-emerald-400" : "bg-amber-400"
                  }`}
                />
                <span className="capitalize">{name}</span>
                <span
                  className={healthy ? "text-emerald-400" : "text-amber-400"}
                >
                  {component.status}
                </span>
              </span>,
            ];
          })}
        </div>
      ) : null}
    </Panel>
  );
}
