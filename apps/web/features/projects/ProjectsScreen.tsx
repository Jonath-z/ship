"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { Modal } from "@/components/Modal";
import { EmptyState, LoadingState, PageHeader } from "@/components/panels";
import { api, fieldErrorOf } from "@/lib/api";
import { slugify } from "@/lib/format";
import { useProjects } from "@/lib/hooks";
import { ProjectCard } from "@/features/projects/ProjectCard";

/** Projects index: every project with its environments plus lifecycle actions. */
export function ProjectsScreen() {
  const [creating, setCreating] = useState(false);
  const { data, isPending, error } = useProjects();

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        actions={
          <Button onClick={() => setCreating(true)} variant="primary">
            New project
          </Button>
        }
        description="Projects group environments; each environment has its own services, databases, and deployments."
        eyebrow="Control plane"
        title="Projects"
      />

      <div className="mt-8 grid gap-6">
        {error ? <ErrorNotice error={error} /> : null}
        {isPending ? <LoadingState label="Loading projects…" /> : null}
        {data?.items.map((project) => (
          <ProjectCard key={project.id} project={project} />
        ))}
        {data && data.items.length === 0 ? (
          <EmptyState
            action={
              <Button onClick={() => setCreating(true)} variant="primary">
                Create your first project
              </Button>
            }
            description="Create a project to start defining environments, applications, and databases."
            title="No projects yet"
          />
        ) : null}
      </div>

      {creating ? <CreateProjectDialog onClose={() => setCreating(false)} /> : null}
    </main>
  );
}

function CreateProjectDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);

  const create = useMutation({
    mutationFn: () => api.projects.create({ name, slug }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      onClose();
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    create.mutate();
  }

  return (
    <Modal onClose={onClose} title="New project">
      <form className="grid gap-4" onSubmit={submit}>
        <Field error={fieldErrorOf(create.error, "name")} label="Name">
          <Input
            autoFocus
            onChange={(event) => {
              setName(event.target.value);
              if (!slugTouched) setSlug(slugify(event.target.value));
            }}
            placeholder="Acme"
            required
            value={name}
          />
        </Field>
        <Field
          error={fieldErrorOf(create.error, "slug")}
          hint="Lowercase identifier used in configuration and CLI commands."
          label="Slug"
        >
          <Input
            onChange={(event) => {
              setSlugTouched(true);
              setSlug(event.target.value);
            }}
            placeholder="acme"
            required
            value={slug}
          />
        </Field>
        {create.error ? <ErrorNotice error={create.error} /> : null}
        <div className="flex justify-end gap-2">
          <Button onClick={onClose} variant="secondary">
            Cancel
          </Button>
          <Button disabled={create.isPending} type="submit" variant="primary">
            {create.isPending ? "Creating…" : "Create project"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
