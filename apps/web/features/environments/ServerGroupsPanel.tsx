"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Select } from "@/components/form";
import { LoadingState, Panel } from "@/components/panels";
import { StatusDot } from "@/components/StatusDot";
import { api, type ServerGroup } from "@/lib/api";
import { useServerGroups, useServers } from "@/lib/hooks";

/** Server groups (placement roles) with member add/remove. */
export function ServerGroupsPanel({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const groups = useServerGroups(projectId, environmentId);

  return (
    <Panel
      description="Groups map roles (e.g. web, worker) to the servers that run them. Applications and databases are placed via these groups."
      title="Server groups"
    >
      {groups.error ? <ErrorNotice error={groups.error} /> : null}
      {groups.isPending ? <LoadingState label="Loading server groups…" /> : null}
      {groups.data && groups.data.items.length === 0 ? (
        <p className="py-3 text-sm text-zinc-500">
          No server groups yet — they are created automatically from service
          roles.
        </p>
      ) : null}
      <div className="grid gap-4">
        {(groups.data?.items ?? []).map((group) => (
          <GroupRow
            environmentId={environmentId}
            group={group}
            key={group.id}
            projectId={projectId}
          />
        ))}
      </div>
    </Panel>
  );
}

function GroupRow({
  projectId,
  environmentId,
  group,
}: {
  projectId: string;
  environmentId: string;
  group: ServerGroup;
}) {
  const queryClient = useQueryClient();
  const [selected, setSelected] = useState("");
  const servers = useServers();

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["server-groups", projectId, environmentId],
    });

  const addMember = useMutation({
    mutationFn: (serverId: string) =>
      api.serverGroups.addMember(projectId, environmentId, group.id, serverId),
    onSuccess: async () => {
      setSelected("");
      await invalidate();
    },
  });

  const removeMember = useMutation({
    mutationFn: (serverId: string) =>
      api.serverGroups.removeMember(
        projectId,
        environmentId,
        group.id,
        serverId,
      ),
    onSuccess: invalidate,
  });

  const memberIds = new Set(group.members.map((member) => member.id));
  const candidates = (servers.data?.items ?? []).filter(
    (server) => !memberIds.has(server.id),
  );

  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="font-medium text-zinc-200">{group.name}</p>
        <div className="flex items-center gap-2">
          <Select
            aria-label={`Add server to ${group.name}`}
            className="w-48"
            onChange={(event) => setSelected(event.target.value)}
            value={selected}
          >
            <option disabled value="">
              {candidates.length === 0 ? "No servers to add" : "Add a server…"}
            </option>
            {candidates.map((server) => (
              <option key={server.id} value={server.id}>
                {server.name}
              </option>
            ))}
          </Select>
          <Button
            disabled={!selected || addMember.isPending}
            onClick={() => addMember.mutate(selected)}
            size="sm"
            variant="primary"
          >
            {addMember.isPending ? "Adding…" : "Add"}
          </Button>
        </div>
      </div>

      {addMember.error ? (
        <div className="mt-3">
          <ErrorNotice error={addMember.error} />
        </div>
      ) : null}
      {removeMember.error ? (
        <div className="mt-3">
          <ErrorNotice error={removeMember.error} />
        </div>
      ) : null}

      <ul className="mt-3 divide-y divide-zinc-800/60">
        {group.members.map((member) => (
          <li
            className="flex items-center justify-between gap-3 py-2"
            key={member.id}
          >
            <div className="flex items-center gap-3 text-sm">
              <span className="text-zinc-200">{member.name}</span>
              <StatusDot status={member.status} />
            </div>
            <Button
              disabled={removeMember.isPending}
              onClick={() => removeMember.mutate(member.id)}
              size="sm"
              variant="danger"
            >
              Remove
            </Button>
          </li>
        ))}
        {group.members.length === 0 ? (
          <li className="py-2 text-sm text-amber-400">
            No servers in this group — deployments that target it will fail.
          </li>
        ) : null}
      </ul>
    </div>
  );
}
