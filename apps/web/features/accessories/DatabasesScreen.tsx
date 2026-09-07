"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorNotice } from "@/components/ErrorNotice";
import {
  EmptyState,
  LoadingState,
  PageHeader,
  Panel,
} from "@/components/panels";
import { api, type Accessory } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import { useAccessories, useServerGroups, useServers } from "@/lib/hooks";
import { CreateAccessoryDialog } from "@/features/accessories/CreateAccessoryDialog";

/** Databases (accessories) in the environment: postgres and redis instances. */
export function DatabasesScreen({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const queryClient = useQueryClient();
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<Accessory>();

  const accessories = useAccessories(projectId, environmentId);
  const servers = useServers();
  const groups = useServerGroups(projectId, environmentId);
  const volumes = useQuery({
    queryKey: ["volumes", projectId, environmentId],
    queryFn: () => api.volumes.list(projectId, environmentId),
  });

  const remove = useMutation({
    mutationFn: (accessoryId: string) =>
      api.accessories.remove(projectId, environmentId, accessoryId),
    onSuccess: async () => {
      setDeleting(undefined);
      await queryClient.invalidateQueries({
        queryKey: ["accessories", projectId, environmentId],
      });
    },
  });

  function placement(accessory: Accessory): string {
    if (accessory.serverId) {
      const server = servers.data?.items.find(
        (candidate) => candidate.id === accessory.serverId,
      );
      return `server · ${server?.name ?? accessory.serverId}`;
    }
    if (accessory.serverGroupId) {
      const group = groups.data?.items.find(
        (candidate) => candidate.id === accessory.serverGroupId,
      );
      return `group · ${group?.name ?? accessory.serverGroupId}`;
    }
    return "unplaced";
  }

  const items = accessories.data?.items ?? [];

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        actions={
          <Button onClick={() => setCreating(true)} variant="primary">
            New database
          </Button>
        }
        description="Managed Postgres and Redis instances placed on your servers, with connection secrets injected automatically."
        eyebrow="Environment"
        title="Databases"
      />

      <div className="mt-8 grid gap-6">
        {accessories.error ? <ErrorNotice error={accessories.error} /> : null}
        {accessories.isPending ? (
          <LoadingState label="Loading databases…" />
        ) : null}
        {accessories.data && items.length === 0 ? (
          <EmptyState
            action={
              <Button onClick={() => setCreating(true)} variant="primary">
                Create a database
              </Button>
            }
            description="Add a Postgres or Redis instance for your applications to use."
            title="No databases yet"
          />
        ) : null}

        {items.map((accessory) => {
          const attached = (volumes.data?.items ?? []).filter(
            (volume) => volume.accessoryId === accessory.id,
          );
          return (
            <Panel
              actions={
                <Button
                  onClick={() => setDeleting(accessory)}
                  size="sm"
                  variant="danger"
                >
                  Delete
                </Button>
              }
              description={`created ${relativeTime(accessory.createdAt)}`}
              key={accessory.id}
              title={accessory.name}
            >
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone="sky">{accessory.type}</Badge>
                <Badge>{accessory.image}</Badge>
                {accessory.port ? <Badge>port {accessory.port}</Badge> : null}
                <Badge tone="zinc">{placement(accessory)}</Badge>
                {accessory.connectionSecret ? (
                  <Badge tone="emerald">
                    secret · {accessory.connectionSecret}
                  </Badge>
                ) : null}
              </div>

              <div className="mt-4">
                <p className="text-sm font-medium text-zinc-300">Volumes</p>
                {attached.length > 0 ? (
                  <ul className="mt-2 space-y-1">
                    {attached.map((volume) => (
                      <li
                        className="font-mono text-xs text-zinc-400"
                        key={volume.id}
                      >
                        {volume.name}: {volume.source} → {volume.destination}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="mt-2 text-xs text-amber-400">
                    No volume attached — data will not survive container
                    replacement.
                  </p>
                )}
              </div>
            </Panel>
          );
        })}
      </div>

      {creating ? (
        <CreateAccessoryDialog
          environmentId={environmentId}
          onClose={() => setCreating(false)}
          projectId={projectId}
        />
      ) : null}
      {deleting ? (
        <ConfirmDialog
          busy={remove.isPending}
          confirmLabel="Delete database"
          danger
          error={remove.error}
          message={`Deletes ${deleting.name}. Applications that depend on it must remove the dependency first.`}
          onCancel={() => setDeleting(undefined)}
          onConfirm={() => remove.mutate(deleting.id)}
          title={`Delete ${deleting.name}?`}
        />
      ) : null}
    </main>
  );
}
