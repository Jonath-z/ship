"use client";

import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Select } from "@/components/form";
import { EmptyState, LoadingState, Panel } from "@/components/panels";
import { api, type Service } from "@/lib/api";
import { useServerGroups } from "@/lib/hooks";
import { ContainerLogViewer } from "@/features/logs/ContainerLogViewer";

/**
 * Container logs for one application: pick a host from the service's server
 * group, then a container on it. Containers whose name includes the service
 * name are preselected.
 */
export function ServiceLogsTab({
  projectId,
  environmentId,
  service,
}: {
  projectId: string;
  environmentId: string;
  service: Service;
}) {
  const [serverId, setServerId] = useState("");
  const [containerName, setContainerName] = useState("");

  const groups = useServerGroups(projectId, environmentId);
  const group = groups.data?.items.find(
    (candidate) => candidate.id === service.serverGroupId,
  );
  const members = group?.members ?? [];

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
    const items = containers.data.items;
    const match =
      items.find((container) => container.name.includes(service.name)) ??
      items[0];
    if (match) setContainerName(match.name);
  }, [containerName, containers.data, service.name]);

  if (groups.isPending) {
    return <LoadingState label="Resolving servers…" />;
  }
  if (groups.error) {
    return <ErrorNotice error={groups.error} />;
  }
  if (members.length === 0) {
    return (
      <EmptyState
        description={`Assign servers to the "${service.role}" group to read container logs for this application.`}
        title="No servers in this application's group"
      />
    );
  }

  return (
    <Panel
      description="Docker logs fetched over SSH from the selected host."
      title="Container logs"
    >
      <div className="mb-4 grid gap-4 sm:grid-cols-2">
        <Field label="Server">
          <Select
            onChange={(event) => {
              setServerId(event.target.value);
              setContainerName("");
            }}
            value={serverId}
          >
            {members.map((member) => (
              <option key={member.id} value={member.id}>
                {member.name}
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
    </Panel>
  );
}
