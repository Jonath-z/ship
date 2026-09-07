"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { LoadingState, Panel } from "@/components/panels";
import { api, fieldErrorOf, type Domain } from "@/lib/api";

/** Hostnames routed to one service, with SSL toggles. */
export function DomainsTab({
  projectId,
  environmentId,
  serviceId,
}: {
  projectId: string;
  environmentId: string;
  serviceId: string;
}) {
  const queryClient = useQueryClient();
  const [hostname, setHostname] = useState("");
  const [sslEnabled, setSslEnabled] = useState(true);
  const [deleting, setDeleting] = useState<Domain>();

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["domains", projectId, environmentId],
    });

  const domains = useQuery({
    queryKey: ["domains", projectId, environmentId],
    queryFn: () => api.domains.list(projectId, environmentId),
  });
  const mine = (domains.data?.items ?? []).filter(
    (domain) => domain.serviceId === serviceId,
  );

  const create = useMutation({
    mutationFn: () =>
      api.domains.create(projectId, environmentId, {
        serviceId,
        hostname,
        sslEnabled,
      }),
    onSuccess: async () => {
      setHostname("");
      await invalidate();
    },
  });

  const toggleSsl = useMutation({
    mutationFn: (domain: Domain) =>
      api.domains.update(projectId, environmentId, domain.id, {
        sslEnabled: !domain.sslEnabled,
      }),
    onSuccess: invalidate,
  });

  const remove = useMutation({
    mutationFn: (domainId: string) =>
      api.domains.remove(projectId, environmentId, domainId),
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
      description="Traffic for these hostnames is routed to this application."
      title="Domains"
    >
      <form
        className="grid gap-3 sm:grid-cols-[1fr_auto_auto]"
        onSubmit={submit}
      >
        <Field error={fieldErrorOf(create.error, "hostname")} label="Hostname">
          <Input
            onChange={(event) => setHostname(event.target.value)}
            placeholder="app.example.com"
            required
            value={hostname}
          />
        </Field>
        <label className="flex items-center gap-2 pt-7 text-sm text-zinc-300">
          <input
            checked={sslEnabled}
            className="h-4 w-4 accent-emerald-400"
            onChange={(event) => setSslEnabled(event.target.checked)}
            type="checkbox"
          />
          SSL
        </label>
        <div className="pt-6">
          <Button disabled={create.isPending} type="submit" variant="primary">
            {create.isPending ? "Adding…" : "Add domain"}
          </Button>
        </div>
      </form>
      {create.error ? (
        <div className="mt-3">
          <ErrorNotice error={create.error} />
        </div>
      ) : null}
      {toggleSsl.error ? (
        <div className="mt-3">
          <ErrorNotice error={toggleSsl.error} />
        </div>
      ) : null}
      {domains.error ? (
        <div className="mt-3">
          <ErrorNotice error={domains.error} />
        </div>
      ) : null}

      {domains.isPending ? <LoadingState label="Loading domains…" /> : null}
      <ul className="mt-4 divide-y divide-zinc-800">
        {mine.map((domain) => (
          <li
            className="flex flex-wrap items-center justify-between gap-3 py-3"
            key={domain.id}
          >
            <div className="flex items-center gap-3">
              <span className="font-mono text-sm text-zinc-200">
                {domain.hostname}
              </span>
              <Badge tone={domain.sslEnabled ? "emerald" : "zinc"}>
                {domain.sslEnabled ? "SSL" : "no SSL"}
              </Badge>
            </div>
            <div className="flex items-center gap-2">
              <Button
                disabled={toggleSsl.isPending}
                onClick={() => toggleSsl.mutate(domain)}
                size="sm"
                variant="ghost"
              >
                {domain.sslEnabled ? "Disable SSL" : "Enable SSL"}
              </Button>
              <Button
                onClick={() => setDeleting(domain)}
                size="sm"
                variant="danger"
              >
                Remove
              </Button>
            </div>
          </li>
        ))}
      </ul>
      {domains.data && mine.length === 0 ? (
        <p className="py-3 text-sm text-zinc-500">
          No domains point at this application yet.
        </p>
      ) : null}

      {deleting ? (
        <ConfirmDialog
          busy={remove.isPending}
          confirmLabel="Remove domain"
          danger
          error={remove.error}
          message={`Traffic for ${deleting.hostname} will no longer reach this application.`}
          onCancel={() => setDeleting(undefined)}
          onConfirm={() => remove.mutate(deleting.id)}
          title={`Remove ${deleting.hostname}?`}
        />
      ) : null}
    </Panel>
  );
}
