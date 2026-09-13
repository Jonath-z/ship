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
  useServers,
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
  const servers = useServers();
  const variables = useQuery({
    queryKey: ["variables", projectId, environmentId],
    queryFn: () => api.variables.list(projectId, environmentId),
  });
  const secrets = useQuery({
    queryKey: ["secrets", projectId, environmentId],
    queryFn: () => api.secrets.list(projectId, environmentId),
  });

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

      <SetupChecklist
        base={base}
        hasDatabase={(accessories.data?.items.length ?? 0) > 0}
        hasDeployment={Boolean(lastDeployment)}
        hasGroupedServer={(serverGroups.data?.items ?? []).some(
          (group) => group.members.length > 0,
        )}
        hasRegistry={(secrets.data?.items ?? []).some(
          (secret) =>
            secret.name === "KAMAL_REGISTRY_PASSWORD" && !secret.serviceId,
        )}
        hasServer={(servers.data?.items.length ?? 0) > 0}
        hasService={(services.data?.items.length ?? 0) > 0}
        hasVariables={(variables.data?.items.length ?? 0) > 0}
        onDeploy={() => setDeploying(true)}
        ready={
          Boolean(servers.data) &&
          Boolean(serverGroups.data) &&
          Boolean(services.data) &&
          Boolean(deployments.data)
        }
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

/**
 * Guided setup for a fresh environment: machines → placement group →
 * application → optional database/variables → deploy. Hidden once the
 * essential steps are complete.
 */
function SetupChecklist({
  base,
  hasServer,
  hasGroupedServer,
  hasRegistry,
  hasService,
  hasDatabase,
  hasVariables,
  hasDeployment,
  ready,
  onDeploy,
}: {
  base: string;
  hasServer: boolean;
  hasGroupedServer: boolean;
  hasRegistry: boolean;
  hasService: boolean;
  hasDatabase: boolean;
  hasVariables: boolean;
  hasDeployment: boolean;
  ready: boolean;
  onDeploy: () => void;
}) {
  // Wait for the queries so the checklist doesn't flash on every visit.
  if (!ready) return null;
  if (hasServer && hasGroupedServer && hasService && hasDeployment) {
    return null;
  }

  const steps: Array<{
    title: string;
    description: string;
    done: boolean;
    optional?: boolean;
    href?: string;
    action?: ReactNode;
  }> = [
    {
      title: "Register a server",
      description:
        "Add an SSH key, then connect the VPS Ship will deploy to. The wizard checks SSH, Docker, and resources.",
      done: hasServer,
      href: "/servers",
    },
    {
      title: "Group your servers",
      description:
        "Create a server group (Kamal role) such as web and add servers to it — applications target groups, not machines.",
      done: hasGroupedServer,
      href: `${base}/settings`,
    },
    {
      title: "Connect your image registry",
      description:
        "Save the registry server and credentials so Ship can list your images and Kamal can pull them on the servers.",
      done: hasRegistry,
      href: `${base}/variables`,
    },
    {
      title: "Create an application",
      description:
        "Pick an image straight from your registry, choose the group and port, then add domains and volumes on its tabs.",
      done: hasService,
      href: `${base}/applications`,
    },
    {
      title: "Add a database",
      description:
        "Postgres or Redis as an accessory; Ship suggests a data volume and generates the connection secret.",
      done: hasDatabase,
      optional: true,
      href: `${base}/databases`,
    },
    {
      title: "Set variables and secrets",
      description:
        "Plaintext variables and encrypted secrets, environment-wide or per application. Bulk-paste a .env block to import.",
      done: hasVariables,
      optional: true,
      href: `${base}/variables`,
    },
    {
      title: "Deploy",
      description:
        "Review validation and the configuration diff, then queue the first deployment and follow its live log.",
      done: hasDeployment,
      action: (
        <Button onClick={onDeploy} size="sm" variant="primary">
          Deploy
        </Button>
      ),
    },
  ];

  return (
    <section className="mt-8">
      <Panel
        description="Work through these in order — validation will block a deploy until the essentials are in place."
        title="Set up this environment"
      >
        <ol className="divide-y divide-zinc-800">
          {steps.map((step, index) => (
            <li
              className="flex flex-wrap items-center justify-between gap-3 py-3"
              key={step.title}
            >
              <div className="flex min-w-0 items-start gap-3">
                <span
                  className={`mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-xs font-semibold ${
                    step.done
                      ? "border-emerald-900 bg-emerald-950 text-emerald-300"
                      : "border-zinc-700 bg-zinc-900 text-zinc-400"
                  }`}
                >
                  {step.done ? "✓" : index + 1}
                </span>
                <div className="min-w-0">
                  <p className="flex items-center gap-2 font-medium text-zinc-200">
                    {step.title}
                    {step.optional ? (
                      <span className="text-xs font-normal text-zinc-500">
                        optional
                      </span>
                    ) : null}
                  </p>
                  <p className="mt-0.5 text-sm text-zinc-500">
                    {step.description}
                  </p>
                </div>
              </div>
              {step.done ? null : (step.action ?? (
                <Link
                  className="text-sm text-emerald-400 hover:text-emerald-300"
                  href={step.href ?? base}
                >
                  Go →
                </Link>
              ))}
            </li>
          ))}
        </ol>
      </Panel>
    </section>
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
