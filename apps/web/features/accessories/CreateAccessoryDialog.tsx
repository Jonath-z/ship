"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input, Select } from "@/components/form";
import { Modal } from "@/components/Modal";
import { api, fieldErrorOf, type Accessory } from "@/lib/api";
import { useServerGroups, useServers } from "@/lib/hooks";

type DbType = "postgres" | "redis";

const defaults: Record<DbType, { image: string; port: string }> = {
  postgres: { image: "postgres:16", port: "5432" },
  redis: { image: "redis:7", port: "6379" },
};

/** Create a database, then surface the suggested volume from the response. */
export function CreateAccessoryDialog({
  projectId,
  environmentId,
  onClose,
}: {
  projectId: string;
  environmentId: string;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [type, setType] = useState<DbType>("postgres");
  const [image, setImage] = useState(defaults.postgres.image);
  const [port, setPort] = useState(defaults.postgres.port);
  const [placement, setPlacement] = useState<"server" | "group">("server");
  const [serverId, setServerId] = useState("");
  const [groupId, setGroupId] = useState("");
  const [created, setCreated] = useState<Accessory>();

  const servers = useServers();
  const groups = useServerGroups(projectId, environmentId);

  const create = useMutation({
    mutationFn: () =>
      api.accessories.create(projectId, environmentId, {
        name,
        type,
        image,
        port: port ? Number(port) : undefined,
        serverId: placement === "server" ? serverId : null,
        serverGroupId: placement === "group" ? groupId : null,
      }),
    onSuccess: async (accessory) => {
      await queryClient.invalidateQueries({
        queryKey: ["accessories", projectId, environmentId],
      });
      setCreated(accessory);
    },
  });

  const attachVolume = useMutation({
    mutationFn: (accessory: Accessory) => {
      const suggested = accessory.suggestedVolume;
      if (!suggested) throw new Error("No suggested volume available");
      return api.volumes.create(projectId, environmentId, {
        name: suggested.name,
        source: suggested.source,
        destination: suggested.destination,
        accessoryId: accessory.id,
      });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ["volumes", projectId, environmentId],
      });
      onClose();
    },
  });

  function onTypeChange(next: DbType) {
    setType(next);
    if (image === defaults[type].image) setImage(defaults[next].image);
    if (port === defaults[type].port) setPort(defaults[next].port);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    create.mutate();
  }

  if (created) {
    return (
      <Modal onClose={onClose} title={`${created.name} created`}>
        {created.suggestedConnectionSecret ? (
          <p className="text-sm text-zinc-400">
            Connection details will be injected via the{" "}
            <Badge tone="emerald">{created.suggestedConnectionSecret}</Badge>{" "}
            secret.
          </p>
        ) : null}
        {created.suggestedVolume ? (
          <div className="mt-4 rounded-lg border border-zinc-800 bg-zinc-950 p-4">
            <p className="text-sm font-medium text-zinc-200">
              Suggested volume
            </p>
            <p className="mt-1 font-mono text-xs text-zinc-400">
              {created.suggestedVolume.name}: {created.suggestedVolume.source} →{" "}
              {created.suggestedVolume.destination}
            </p>
            <p className="mt-2 text-xs text-zinc-500">
              Attach it so the data survives container replacement.
            </p>
          </div>
        ) : (
          <p className="mt-4 text-sm text-zinc-400">
            The database was created without a persistent volume.
          </p>
        )}
        {attachVolume.error ? (
          <div className="mt-4">
            <ErrorNotice error={attachVolume.error} />
          </div>
        ) : null}
        <div className="mt-6 flex justify-end gap-2">
          <Button onClick={onClose} variant="secondary">
            {created.suggestedVolume ? "Skip" : "Close"}
          </Button>
          {created.suggestedVolume ? (
            <Button
              disabled={attachVolume.isPending}
              onClick={() => attachVolume.mutate(created)}
              variant="primary"
            >
              {attachVolume.isPending ? "Attaching…" : "Attach volume"}
            </Button>
          ) : null}
        </div>
      </Modal>
    );
  }

  return (
    <Modal onClose={onClose} title="New database" wide>
      <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
        <Field error={fieldErrorOf(create.error, "name")} label="Name">
          <Input
            autoFocus
            onChange={(event) => setName(event.target.value)}
            placeholder="primary-db"
            required
            value={name}
          />
        </Field>
        <Field error={fieldErrorOf(create.error, "type")} label="Type">
          <Select
            onChange={(event) => onTypeChange(event.target.value as DbType)}
            value={type}
          >
            <option value="postgres">Postgres</option>
            <option value="redis">Redis</option>
          </Select>
        </Field>
        <Field error={fieldErrorOf(create.error, "image")} label="Image">
          <Input
            className="font-mono"
            onChange={(event) => setImage(event.target.value)}
            required
            value={image}
          />
        </Field>
        <Field error={fieldErrorOf(create.error, "port")} label="Port">
          <Input
            min={1}
            onChange={(event) => setPort(event.target.value)}
            required
            type="number"
            value={port}
          />
        </Field>
        <div className="sm:col-span-2">
          <span className="mb-1.5 block text-sm font-medium text-zinc-300">
            Placement
          </span>
          <div className="flex gap-4 text-sm text-zinc-300">
            <label className="flex items-center gap-2">
              <input
                checked={placement === "server"}
                className="accent-emerald-400"
                onChange={() => setPlacement("server")}
                type="radio"
              />
              Specific server
            </label>
            <label className="flex items-center gap-2">
              <input
                checked={placement === "group"}
                className="accent-emerald-400"
                onChange={() => setPlacement("group")}
                type="radio"
              />
              Server group
            </label>
          </div>
        </div>
        {placement === "server" ? (
          <Field error={fieldErrorOf(create.error, "serverId")} label="Server">
            <Select
              onChange={(event) => setServerId(event.target.value)}
              required
              value={serverId}
            >
              <option disabled value="">
                Choose a server…
              </option>
              {(servers.data?.items ?? []).map((server) => (
                <option key={server.id} value={server.id}>
                  {server.name}
                </option>
              ))}
            </Select>
          </Field>
        ) : (
          <Field
            error={fieldErrorOf(create.error, "serverGroupId")}
            label="Server group"
          >
            <Select
              onChange={(event) => setGroupId(event.target.value)}
              required
              value={groupId}
            >
              <option disabled value="">
                Choose a group…
              </option>
              {(groups.data?.items ?? []).map((group) => (
                <option key={group.id} value={group.id}>
                  {group.name}
                </option>
              ))}
            </Select>
          </Field>
        )}
        {create.error ? (
          <div className="sm:col-span-2">
            <ErrorNotice error={create.error} />
          </div>
        ) : null}
        <div className="flex justify-end gap-2 sm:col-span-2">
          <Button onClick={onClose} variant="secondary">
            Cancel
          </Button>
          <Button disabled={create.isPending} type="submit" variant="primary">
            {create.isPending ? "Creating…" : "Create database"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
