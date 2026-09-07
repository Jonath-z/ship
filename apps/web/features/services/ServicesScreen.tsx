"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useState, type FormEvent } from "react";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { Modal } from "@/components/Modal";
import {
  EmptyState,
  LoadingState,
  PageHeader,
  Panel,
} from "@/components/panels";
import { api, fieldErrorOf, type Service } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import { useServices } from "@/lib/hooks";

/** Human-readable source column: image, or repository@branch. */
export function serviceSource(service: Service): string {
  if (service.image) return service.image;
  if (service.repository) {
    return service.branch
      ? `${service.repository} @ ${service.branch}`
      : service.repository;
  }
  return "—";
}

/** Applications (services) in the current environment. */
export function ServicesScreen({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const [creating, setCreating] = useState(false);
  const { data, isPending, error } = useServices(projectId, environmentId);
  const base = `/p/${projectId}/e/${environmentId}/applications`;

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        actions={
          <Button onClick={() => setCreating(true)} variant="primary">
            New application
          </Button>
        }
        description="Deployable services in this environment — from a prebuilt image or a git repository."
        eyebrow="Environment"
        title="Applications"
      />

      <div className="mt-8">
        <Panel>
          {error ? <ErrorNotice error={error} /> : null}
          {isPending ? <LoadingState label="Loading applications…" /> : null}
          {data && data.items.length === 0 ? (
            <EmptyState
              action={
                <Button onClick={() => setCreating(true)} variant="primary">
                  Create an application
                </Button>
              }
              description="Add a service that Ship can deploy to your servers."
              title="No applications yet"
            />
          ) : null}
          {data && data.items.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="text-zinc-500">
                  <tr>
                    <th className="pb-3 pr-4">Name</th>
                    <th className="pb-3 pr-4">Type</th>
                    <th className="pb-3 pr-4">Source</th>
                    <th className="pb-3 pr-4">Port</th>
                    <th className="pb-3 pr-4">Role</th>
                    <th className="pb-3">Updated</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-800">
                  {data.items.map((service) => (
                    <tr key={service.id}>
                      <td className="py-3 pr-4">
                        <Link
                          className="font-medium text-zinc-200 hover:text-emerald-300"
                          href={`${base}/${service.id}`}
                        >
                          {service.name}
                        </Link>
                      </td>
                      <td className="py-3 pr-4">
                        <Badge>{service.type}</Badge>
                      </td>
                      <td className="py-3 pr-4 font-mono text-xs text-zinc-400">
                        {serviceSource(service)}
                      </td>
                      <td className="py-3 pr-4 text-zinc-400">
                        {service.port ?? "—"}
                      </td>
                      <td className="py-3 pr-4 text-zinc-400">{service.role}</td>
                      <td className="py-3 text-zinc-500">
                        {relativeTime(service.updatedAt)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
        </Panel>
      </div>

      {creating ? (
        <CreateServiceDialog
          environmentId={environmentId}
          onClose={() => setCreating(false)}
          projectId={projectId}
        />
      ) : null}
    </main>
  );
}

function CreateServiceDialog({
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
  const [type, setType] = useState("app");
  const [image, setImage] = useState("");
  const [repository, setRepository] = useState("");
  const [branch, setBranch] = useState("");
  const [port, setPort] = useState("");
  const [command, setCommand] = useState("");
  const [role, setRole] = useState("web");

  const create = useMutation({
    mutationFn: () =>
      api.services.create(projectId, environmentId, {
        name,
        type,
        image: image || undefined,
        repository: repository || undefined,
        branch: branch || undefined,
        port: port ? Number(port) : undefined,
        command: command || undefined,
        role: role || undefined,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ["services", projectId, environmentId],
      });
      onClose();
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    create.mutate();
  }

  return (
    <Modal onClose={onClose} title="New application" wide>
      <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
        <Field error={fieldErrorOf(create.error, "name")} label="Name">
          <Input
            autoFocus
            onChange={(event) => setName(event.target.value)}
            placeholder="web"
            required
            value={name}
          />
        </Field>
        <Field
          error={fieldErrorOf(create.error, "type")}
          hint='Usually "app" or "worker".'
          label="Type"
        >
          <Input
            onChange={(event) => setType(event.target.value)}
            required
            value={type}
          />
        </Field>
        <Field
          error={fieldErrorOf(create.error, "image")}
          hint="Prebuilt image; leave empty when building from a repository."
          label="Image"
        >
          <Input
            className="font-mono"
            onChange={(event) => setImage(event.target.value)}
            placeholder="ghcr.io/acme/web:latest"
            value={image}
          />
        </Field>
        <div className="grid grid-cols-[1fr_120px] gap-4">
          <Field
            error={fieldErrorOf(create.error, "repository")}
            label="Repository"
          >
            <Input
              className="font-mono"
              onChange={(event) => setRepository(event.target.value)}
              placeholder="github.com/acme/web"
              value={repository}
            />
          </Field>
          <Field error={fieldErrorOf(create.error, "branch")} label="Branch">
            <Input
              onChange={(event) => setBranch(event.target.value)}
              placeholder="main"
              value={branch}
            />
          </Field>
        </div>
        <Field error={fieldErrorOf(create.error, "port")} label="Port">
          <Input
            min={1}
            onChange={(event) => setPort(event.target.value)}
            placeholder="3000"
            type="number"
            value={port}
          />
        </Field>
        <Field
          error={fieldErrorOf(create.error, "role")}
          hint="Servers with this role run the service."
          label="Role"
        >
          <Input
            onChange={(event) => setRole(event.target.value)}
            value={role}
          />
        </Field>
        <div className="sm:col-span-2">
          <Field
            error={fieldErrorOf(create.error, "command")}
            hint="Optional container start command override."
            label="Command"
          >
            <Input
              className="font-mono"
              onChange={(event) => setCommand(event.target.value)}
              placeholder="bundle exec puma"
              value={command}
            />
          </Field>
        </div>
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
            {create.isPending ? "Creating…" : "Create application"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
