"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { Panel } from "@/components/panels";
import { api, fieldErrorOf, type Service } from "@/lib/api";

/** Editable service fields; saves via PATCH with inline field errors. */
export function ServiceOverviewTab({
  projectId,
  environmentId,
  service,
}: {
  projectId: string;
  environmentId: string;
  service: Service;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(service.name);
  const [type, setType] = useState(service.type);
  const [image, setImage] = useState(service.image ?? "");
  const [repository, setRepository] = useState(service.repository ?? "");
  const [branch, setBranch] = useState(service.branch ?? "");
  const [port, setPort] = useState(service.port ? String(service.port) : "");
  const [command, setCommand] = useState(service.command ?? "");
  const [role, setRole] = useState(service.role);
  const [saved, setSaved] = useState(false);

  const save = useMutation({
    mutationFn: () =>
      api.services.update(projectId, environmentId, service.id, {
        name,
        type,
        image,
        repository,
        branch,
        port: port ? Number(port) : null,
        command,
        role,
      }),
    onSuccess: async () => {
      setSaved(true);
      await queryClient.invalidateQueries({
        queryKey: ["service", projectId, environmentId, service.id],
      });
      await queryClient.invalidateQueries({
        queryKey: ["services", projectId, environmentId],
      });
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaved(false);
    save.mutate();
  }

  return (
    <Panel title="Settings">
      <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
        <Field error={fieldErrorOf(save.error, "name")} label="Name">
          <Input
            onChange={(event) => setName(event.target.value)}
            required
            value={name}
          />
        </Field>
        <Field error={fieldErrorOf(save.error, "type")} label="Type">
          <Input
            onChange={(event) => setType(event.target.value)}
            required
            value={type}
          />
        </Field>
        <Field
          error={fieldErrorOf(save.error, "image")}
          hint="Prebuilt image; leave empty when building from a repository."
          label="Image"
        >
          <Input
            className="font-mono"
            onChange={(event) => setImage(event.target.value)}
            value={image}
          />
        </Field>
        <div className="grid grid-cols-[1fr_120px] gap-4">
          <Field
            error={fieldErrorOf(save.error, "repository")}
            label="Repository"
          >
            <Input
              className="font-mono"
              onChange={(event) => setRepository(event.target.value)}
              value={repository}
            />
          </Field>
          <Field error={fieldErrorOf(save.error, "branch")} label="Branch">
            <Input
              onChange={(event) => setBranch(event.target.value)}
              value={branch}
            />
          </Field>
        </div>
        <Field error={fieldErrorOf(save.error, "port")} label="Port">
          <Input
            min={1}
            onChange={(event) => setPort(event.target.value)}
            type="number"
            value={port}
          />
        </Field>
        <Field
          error={fieldErrorOf(save.error, "role")}
          hint="Servers with this role run the service."
          label="Role"
        >
          <Input
            onChange={(event) => setRole(event.target.value)}
            value={role}
          />
        </Field>
        <div className="sm:col-span-2">
          <Field error={fieldErrorOf(save.error, "command")} label="Command">
            <Input
              className="font-mono"
              onChange={(event) => setCommand(event.target.value)}
              value={command}
            />
          </Field>
        </div>
        {save.error ? (
          <div className="sm:col-span-2">
            <ErrorNotice error={save.error} />
          </div>
        ) : null}
        <div className="flex items-center justify-end gap-3 sm:col-span-2">
          {saved && !save.error ? (
            <span className="text-sm text-emerald-400">Saved.</span>
          ) : null}
          <Button disabled={save.isPending} type="submit" variant="primary">
            {save.isPending ? "Saving…" : "Save changes"}
          </Button>
        </div>
      </form>
    </Panel>
  );
}
