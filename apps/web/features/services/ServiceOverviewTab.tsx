"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { Panel } from "@/components/panels";
import { api, fieldErrorOf, type Service } from "@/lib/api";
import { ImagePicker } from "@/features/services/ImagePicker";

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
        <div className="sm:col-span-2">
          <ImagePicker
            environmentId={environmentId}
            error={fieldErrorOf(save.error, "image")}
            onChange={setImage}
            projectId={projectId}
            value={image}
          />
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
