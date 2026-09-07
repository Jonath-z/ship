"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { LoadingState, PageHeader, Panel } from "@/components/panels";
import { api, fieldErrorOf } from "@/lib/api";
import { DeleteEnvironmentDialog } from "@/features/environments/DeleteEnvironmentDialog";
import { ServerGroupsPanel } from "@/features/environments/ServerGroupsPanel";
import { EnvironmentDialog } from "@/features/projects/EnvironmentDialog";

/** Environment lifecycle: rename, clone, server groups, deletion. */
export function EnvironmentSettingsScreen({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [saved, setSaved] = useState(false);
  const [cloning, setCloning] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const environment = useQuery({
    queryKey: ["environment", projectId, environmentId],
    queryFn: () => api.environments.get(projectId, environmentId),
  });

  useEffect(() => {
    if (environment.data) {
      setName(environment.data.name);
      setSlug(environment.data.slug);
    }
  }, [environment.data]);

  const save = useMutation({
    mutationFn: () =>
      api.environments.update(projectId, environmentId, { name, slug }),
    onSuccess: async () => {
      setSaved(true);
      await queryClient.invalidateQueries({
        queryKey: ["environment", projectId, environmentId],
      });
      await queryClient.invalidateQueries({
        queryKey: ["environments", projectId],
      });
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaved(false);
    save.mutate();
  }

  if (environment.isPending) {
    return (
      <main className="mx-auto max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
        <LoadingState label="Loading environment…" />
      </main>
    );
  }
  if (environment.error || !environment.data) {
    return (
      <main className="mx-auto max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
        <ErrorNotice
          error={environment.error ?? new Error("Environment not found")}
        />
      </main>
    );
  }

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        actions={
          <Button onClick={() => setCloning(true)}>Clone environment</Button>
        }
        description="Naming, server groups, and destructive actions for this environment."
        eyebrow={environment.data.name}
        title="Environment settings"
      />

      <div className="mt-8 grid gap-6">
        <Panel title="Name & slug">
          <form className="grid gap-4 sm:grid-cols-2" onSubmit={submit}>
            <Field error={fieldErrorOf(save.error, "name")} label="Name">
              <Input
                onChange={(event) => setName(event.target.value)}
                required
                value={name}
              />
            </Field>
            <Field
              error={fieldErrorOf(save.error, "slug")}
              hint="Used in configuration and CLI commands."
              label="Slug"
            >
              <Input
                onChange={(event) => setSlug(event.target.value)}
                required
                value={slug}
              />
            </Field>
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

        <ServerGroupsPanel
          environmentId={environmentId}
          projectId={projectId}
        />

        <Panel
          description="Deleting an environment removes its services, databases, volumes, domains, variables, secrets, and deployment history."
          title="Danger zone"
        >
          <Button onClick={() => setDeleting(true)} variant="danger">
            Delete environment
          </Button>
        </Panel>
      </div>

      {cloning ? (
        <EnvironmentDialog
          onClose={() => setCloning(false)}
          projectId={projectId}
          source={environment.data}
        />
      ) : null}
      {deleting ? (
        <DeleteEnvironmentDialog
          environment={environment.data}
          onClose={() => setDeleting(false)}
          projectId={projectId}
        />
      ) : null}
    </main>
  );
}
