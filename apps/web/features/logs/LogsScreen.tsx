"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useEffect, useState } from "react";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Select } from "@/components/form";
import {
  EmptyState,
  LoadingState,
  PageHeader,
  Panel,
} from "@/components/panels";
import { api } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import { useServers, useServices } from "@/lib/hooks";
import { LogTerminal } from "@/features/deployments/LogTerminal";
import { StatusBadge } from "@/features/deployments/StatusBadge";
import { ContainerLogViewer } from "@/features/logs/ContainerLogViewer";

type LogSource = "deployment" | "container";

/**
 * Unified logs screen (SH-148): deployment output (persisted + live SSE) or
 * container logs fetched over SSH. Server-journal logs stay V1.1 (SH-092).
 */
export function LogsScreen({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const [source, setSource] = useState<LogSource>("deployment");

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        description="Deployment output and container logs for this environment."
        eyebrow="Environment"
        title="Logs"
      />

      <div className="mt-8">
        <Panel
          actions={
            <Select
              aria-label="Log source"
              className="w-44"
              onChange={(event) => setSource(event.target.value as LogSource)}
              value={source}
            >
              <option value="deployment">Deployment logs</option>
              <option value="container">Container logs</option>
            </Select>
          }
          title="Source"
        >
          {source === "deployment" ? (
            <DeploymentLogs
              environmentId={environmentId}
              projectId={projectId}
            />
          ) : (
            <ContainerLogs />
          )}
        </Panel>
      </div>
    </main>
  );
}

function DeploymentLogs({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const [serviceFilter, setServiceFilter] = useState("");
  const [deploymentId, setDeploymentId] = useState("");

  const services = useServices(projectId, environmentId);
  const deployments = useQuery({
    queryKey: ["deployments", projectId, environmentId, serviceFilter, "logs"],
    queryFn: () =>
      api.deployments.list(projectId, environmentId, {
        serviceId: serviceFilter || undefined,
        limit: 25,
      }),
  });

  const items = deployments.data?.items ?? [];
  useEffect(() => {
    const first = items[0];
    if (!deploymentId && first) setDeploymentId(first.id);
  }, [deploymentId, items]);
  const selected = items.find((deployment) => deployment.id === deploymentId);

  return (
    <div className="grid gap-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Service">
          <Select
            onChange={(event) => {
              setServiceFilter(event.target.value);
              setDeploymentId("");
            }}
            value={serviceFilter}
          >
            <option value="">All services</option>
            {(services.data?.items ?? []).map((service) => (
              <option key={service.id} value={service.id}>
                {service.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Deployment">
          <Select
            disabled={items.length === 0}
            onChange={(event) => setDeploymentId(event.target.value)}
            value={deploymentId}
          >
            {items.map((deployment) => (
              <option key={deployment.id} value={deployment.id}>
                {(deployment.serviceName ??
                  services.data?.items.find(
                    (service) => service.id === deployment.serviceId,
                  )?.name ??
                  deployment.serviceId) +
                  ` · ${deployment.status} · ${relativeTime(deployment.createdAt)}`}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      {deployments.isPending ? (
        <LoadingState label="Loading deployments…" />
      ) : null}
      {deployments.error ? <ErrorNotice error={deployments.error} /> : null}
      {deployments.data && items.length === 0 ? (
        <EmptyState
          description="Deploy something to see its output here."
          title="No deployments"
        />
      ) : null}

      {selected ? (
        <>
          <div className="flex items-center gap-3 text-sm">
            <StatusBadge status={selected.status} />
            <Link
              className="text-emerald-400 hover:text-emerald-300"
              href={`/p/${projectId}/e/${environmentId}/deployments/${selected.id}`}
            >
              Open deployment detail
            </Link>
          </div>
          <LogTerminal
            deploymentId={selected.id}
            environmentId={environmentId}
            projectId={projectId}
          />
        </>
      ) : null}
    </div>
  );
}

function ContainerLogs() {
  const [serverId, setServerId] = useState("");
  const [containerName, setContainerName] = useState("");

  const servers = useServers();
  const members = servers.data?.items ?? [];

  useEffect(() => {
    const first = members[0];
    if (!serverId && first) setServerId(first.id);
  }, [serverId, members]);

  const containers = useQuery({
    queryKey: ["server-containers", serverId],
    queryFn: () => api.servers.containers(serverId),
    enabled: Boolean(serverId),
  });

  useEffect(() => {
    if (containerName || !containers.data) return;
    const first = containers.data.items[0];
    if (first) setContainerName(first.name);
  }, [containerName, containers.data]);

  if (servers.isPending) return <LoadingState label="Loading servers…" />;
  if (servers.error) return <ErrorNotice error={servers.error} />;
  if (members.length === 0) {
    return (
      <EmptyState
        description="Register a server to read container logs."
        title="No servers"
      />
    );
  }

  return (
    <div className="grid gap-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Server">
          <Select
            onChange={(event) => {
              setServerId(event.target.value);
              setContainerName("");
            }}
            value={serverId}
          >
            {members.map((server) => (
              <option key={server.id} value={server.id}>
                {server.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Container">
          <Select
            disabled={!containers.data || containers.data.items.length === 0}
            onChange={(event) => setContainerName(event.target.value)}
            value={containerName}
          >
            {(containers.data?.items ?? []).map((container) => (
              <option key={container.name} value={container.name}>
                {container.name}
              </option>
            ))}
          </Select>
        </Field>
      </div>

      {containers.error ? <ErrorNotice error={containers.error} /> : null}
      {containers.isFetching && !containers.data ? (
        <LoadingState label="Listing containers…" />
      ) : null}
      {containers.data && containers.data.items.length === 0 ? (
        <p className="py-3 text-sm text-zinc-500">
          No containers are running on this server.
        </p>
      ) : null}
      {serverId && containerName ? (
        <ContainerLogViewer
          containerName={containerName}
          serverId={serverId}
        />
      ) : null}
    </div>
  );
}
