"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useEffect, useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { Panel } from "@/components/panels";
import { api, fieldErrorOf, type Service } from "@/lib/api";
import {
  SourcePicker,
  type ServiceSource,
} from "@/features/services/SourcePicker";

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
  const [source, setSource] = useState<ServiceSource>({
    repository: service.repository ?? "",
    branch: service.branch ?? "",
    image: service.image ?? "",
  });
  const [port, setPort] = useState(service.port ? String(service.port) : "");
  const [command, setCommand] = useState(service.command ?? "");
  const [role, setRole] = useState(service.role);
  const [saved, setSaved] = useState(false);

  const save = useMutation({
    mutationFn: () =>
      api.services.update(projectId, environmentId, service.id, {
        name,
        type,
        repository: source.repository,
        branch: source.branch,
        image: source.image,
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
    <div className="grid gap-6">
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
            <SourcePicker
              environmentId={environmentId}
              errors={{
                repository: fieldErrorOf(save.error, "repository"),
                branch: fieldErrorOf(save.error, "branch"),
                image: fieldErrorOf(save.error, "image"),
              }}
              onChange={setSource}
              projectId={projectId}
              value={source}
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

      {service.repository && !service.image ? (
        <WebhookPanel
          branch={service.branch || "main"}
          environmentId={environmentId}
          projectId={projectId}
        />
      ) : null}
    </div>
  );
}

/**
 * Setup instructions for push-to-deploy: the operator adds the webhook to
 * their GitHub repository by hand and shares an HMAC secret with Ship as the
 * GITHUB_WEBHOOK_SECRET environment secret — no OAuth or GitHub App in V1.
 */
function WebhookPanel({
  projectId,
  environmentId,
  branch,
}: {
  projectId: string;
  environmentId: string;
  branch: string;
}) {
  // The dashboard origin is only known in the browser; render after mount to
  // keep server and client markup identical.
  const [webhookUrl, setWebhookUrl] = useState("");
  useEffect(() => {
    setWebhookUrl(`${window.location.origin}/api/webhooks/github`);
  }, []);

  return (
    <Panel
      description={`Pushes to ${branch} build and deploy this application automatically.`}
      title="Auto-deploy from GitHub"
    >
      <ol className="grid list-decimal gap-2 pl-5 text-sm text-zinc-400">
        <li>
          In your repository, open{" "}
          <span className="text-zinc-300">
            Settings → Webhooks → Add webhook
          </span>
          .
        </li>
        <li>
          Set the payload URL to{" "}
          <code className="rounded bg-zinc-900 px-1.5 py-0.5 font-mono text-xs text-emerald-400">
            {webhookUrl || "…"}
          </code>{" "}
          with content type{" "}
          <span className="font-mono text-xs text-zinc-300">
            application/json
          </span>
          , and choose a random webhook secret.
        </li>
        <li>
          Save the same value in Ship as the{" "}
          <span className="font-mono text-xs text-zinc-300">
            GITHUB_WEBHOOK_SECRET
          </span>{" "}
          secret under{" "}
          <Link
            className="underline hover:text-zinc-300"
            href={`/p/${projectId}/e/${environmentId}/variables`}
          >
            Variables
          </Link>
          .
        </li>
      </ol>
    </Panel>
  );
}
