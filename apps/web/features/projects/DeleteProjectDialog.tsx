"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { Modal } from "@/components/Modal";
import { LoadingState } from "@/components/panels";
import { api, type Project, type ProjectDeletionImpact } from "@/lib/api";

const impactLabels: Array<{
  key: keyof ProjectDeletionImpact;
  label: string;
}> = [
  { key: "environments", label: "environments" },
  { key: "serverGroups", label: "server groups" },
  { key: "services", label: "services" },
  { key: "accessories", label: "databases" },
  { key: "volumes", label: "volumes" },
  { key: "domains", label: "domains" },
  { key: "environmentVariables", label: "variables" },
  { key: "secrets", label: "secrets" },
  { key: "dependencies", label: "dependencies" },
  { key: "configurations", label: "configurations" },
  { key: "deployments", label: "deployments" },
  { key: "backups", label: "backups" },
];

/** Deletes a project after showing its blast radius and a typed-slug confirm. */
export function DeleteProjectDialog({
  project,
  onClose,
}: {
  project: Project;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [confirmSlug, setConfirmSlug] = useState("");

  const impact = useQuery({
    queryKey: ["project-deletion-impact", project.id],
    queryFn: () => api.projects.deletionImpact(project.id),
  });

  const remove = useMutation({
    mutationFn: () => api.projects.remove(project.id, confirmSlug),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["projects"] });
      onClose();
    },
  });

  const affected = impactLabels.filter(
    ({ key }) => typeof impact.data?.[key] === "number" && impact.data[key] !== 0,
  );

  return (
    <Modal onClose={onClose} title={`Delete ${project.name}`}>
      <p className="text-sm text-zinc-400">
        This permanently deletes the project and everything inside it. This
        cannot be undone.
      </p>
      {impact.isPending ? <LoadingState label="Calculating impact…" /> : null}
      {impact.error ? (
        <div className="mt-4">
          <ErrorNotice error={impact.error} />
        </div>
      ) : null}
      {impact.data ? (
        <div className="mt-4 rounded-lg border border-red-900 bg-red-950/30 p-4 text-sm text-red-200">
          {affected.length === 0 ? (
            <p>The project is empty; nothing else will be deleted.</p>
          ) : (
            <ul className="grid grid-cols-2 gap-x-6 gap-y-1">
              {affected.map(({ key, label }) => (
                <li key={key}>
                  <span className="font-semibold">{impact.data[key]}</span>{" "}
                  {label}
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : null}
      <div className="mt-5">
        <Field
          hint={`Type "${project.slug}" to confirm.`}
          label="Confirm project slug"
        >
          <Input
            autoFocus
            onChange={(event) => setConfirmSlug(event.target.value)}
            placeholder={project.slug}
            value={confirmSlug}
          />
        </Field>
      </div>
      {remove.error ? (
        <div className="mt-4">
          <ErrorNotice error={remove.error} />
        </div>
      ) : null}
      <div className="mt-6 flex justify-end gap-2">
        <Button onClick={onClose} variant="secondary">
          Cancel
        </Button>
        <Button
          disabled={confirmSlug !== project.slug || remove.isPending}
          onClick={() => remove.mutate()}
          variant="danger"
        >
          {remove.isPending ? "Deleting…" : "Delete project"}
        </Button>
      </div>
    </Modal>
  );
}
