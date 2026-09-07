"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { Modal } from "@/components/Modal";
import { LoadingState, Panel } from "@/components/panels";
import { api, fieldErrorOf, type Volume } from "@/lib/api";

export interface VolumeOwner {
  serviceId?: string;
  accessoryId?: string;
}

/** Persistent volumes attached to a service or accessory. */
export function VolumesTab({
  projectId,
  environmentId,
  owner,
}: {
  projectId: string;
  environmentId: string;
  owner: VolumeOwner;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [source, setSource] = useState("");
  const [destination, setDestination] = useState("");
  const [editing, setEditing] = useState<Volume>();
  const [deleting, setDeleting] = useState<Volume>();

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["volumes", projectId, environmentId],
    });

  const volumes = useQuery({
    queryKey: ["volumes", projectId, environmentId],
    queryFn: () => api.volumes.list(projectId, environmentId),
  });
  const mine = (volumes.data?.items ?? []).filter((volume) =>
    owner.serviceId
      ? volume.serviceId === owner.serviceId
      : volume.accessoryId === owner.accessoryId,
  );

  const create = useMutation({
    mutationFn: () =>
      api.volumes.create(projectId, environmentId, {
        name,
        source,
        destination,
        serviceId: owner.serviceId ?? null,
        accessoryId: owner.accessoryId ?? null,
      }),
    onSuccess: async () => {
      setName("");
      setSource("");
      setDestination("");
      await invalidate();
    },
  });

  const remove = useMutation({
    mutationFn: (volumeId: string) =>
      api.volumes.remove(projectId, environmentId, volumeId),
    onSuccess: async () => {
      setDeleting(undefined);
      await invalidate();
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    create.mutate();
  }

  return (
    <Panel
      description="Host paths or named volumes mounted into the container."
      title="Volumes"
    >
      <form className="grid gap-3 sm:grid-cols-[1fr_1fr_1fr_auto]" onSubmit={submit}>
        <Field error={fieldErrorOf(create.error, "name")} label="Name">
          <Input
            onChange={(event) => setName(event.target.value)}
            placeholder="data"
            required
            value={name}
          />
        </Field>
        <Field error={fieldErrorOf(create.error, "source")} label="Source">
          <Input
            className="font-mono"
            onChange={(event) => setSource(event.target.value)}
            placeholder="app-data"
            required
            value={source}
          />
        </Field>
        <Field
          error={fieldErrorOf(create.error, "destination")}
          label="Destination"
        >
          <Input
            className="font-mono"
            onChange={(event) => setDestination(event.target.value)}
            placeholder="/var/lib/app"
            required
            value={destination}
          />
        </Field>
        <div className="pt-6">
          <Button disabled={create.isPending} type="submit" variant="primary">
            {create.isPending ? "Adding…" : "Add volume"}
          </Button>
        </div>
      </form>
      {create.error ? (
        <div className="mt-3">
          <ErrorNotice error={create.error} />
        </div>
      ) : null}
      {volumes.error ? (
        <div className="mt-3">
          <ErrorNotice error={volumes.error} />
        </div>
      ) : null}

      {volumes.isPending ? <LoadingState label="Loading volumes…" /> : null}
      <ul className="mt-4 divide-y divide-zinc-800">
        {mine.map((volume) => (
          <li
            className="flex flex-wrap items-center justify-between gap-3 py-3"
            key={volume.id}
          >
            <div className="min-w-0">
              <p className="font-medium text-zinc-200">{volume.name}</p>
              <p className="mt-0.5 font-mono text-xs text-zinc-500">
                {volume.source} → {volume.destination}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Button
                onClick={() => setEditing(volume)}
                size="sm"
                variant="ghost"
              >
                Edit
              </Button>
              <Button
                onClick={() => setDeleting(volume)}
                size="sm"
                variant="danger"
              >
                Remove
              </Button>
            </div>
          </li>
        ))}
      </ul>
      {volumes.data && mine.length === 0 ? (
        <p className="py-3 text-sm text-zinc-500">No volumes attached.</p>
      ) : null}

      {editing ? (
        <EditVolumeDialog
          environmentId={environmentId}
          onClose={() => setEditing(undefined)}
          projectId={projectId}
          volume={editing}
        />
      ) : null}
      {deleting ? (
        <ConfirmDialog
          busy={remove.isPending}
          confirmLabel="Remove volume"
          danger
          error={remove.error}
          message={`Data in ${deleting.source} stays on the server but is no longer mounted.`}
          onCancel={() => setDeleting(undefined)}
          onConfirm={() => remove.mutate(deleting.id)}
          title={`Remove ${deleting.name}?`}
        />
      ) : null}
    </Panel>
  );
}

function EditVolumeDialog({
  projectId,
  environmentId,
  volume,
  onClose,
}: {
  projectId: string;
  environmentId: string;
  volume: Volume;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(volume.name);
  const [source, setSource] = useState(volume.source);
  const [destination, setDestination] = useState(volume.destination);

  const save = useMutation({
    mutationFn: () =>
      api.volumes.update(projectId, environmentId, volume.id, {
        name,
        source,
        destination,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ["volumes", projectId, environmentId],
      });
      onClose();
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    save.mutate();
  }

  return (
    <Modal onClose={onClose} title={`Edit ${volume.name}`}>
      <form className="grid gap-4" onSubmit={submit}>
        <Field error={fieldErrorOf(save.error, "name")} label="Name">
          <Input
            onChange={(event) => setName(event.target.value)}
            required
            value={name}
          />
        </Field>
        <Field error={fieldErrorOf(save.error, "source")} label="Source">
          <Input
            className="font-mono"
            onChange={(event) => setSource(event.target.value)}
            required
            value={source}
          />
        </Field>
        <Field
          error={fieldErrorOf(save.error, "destination")}
          label="Destination"
        >
          <Input
            className="font-mono"
            onChange={(event) => setDestination(event.target.value)}
            required
            value={destination}
          />
        </Field>
        {save.error ? <ErrorNotice error={save.error} /> : null}
        <div className="flex justify-end gap-2">
          <Button onClick={onClose} variant="secondary">
            Cancel
          </Button>
          <Button disabled={save.isPending} type="submit" variant="primary">
            {save.isPending ? "Saving…" : "Save volume"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
