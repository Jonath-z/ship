"use client";

import Link from "next/link";
import { useState } from "react";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { LoadingState, Panel } from "@/components/panels";
import type { Environment, Project } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import { useEnvironments } from "@/lib/hooks";
import { DeleteProjectDialog } from "@/features/projects/DeleteProjectDialog";
import { EnvironmentDialog } from "@/features/projects/EnvironmentDialog";

type EnvDialog =
  | { kind: "create" }
  | { kind: "clone"; environment: Environment };

/** One project with its environments and create/clone/delete actions. */
export function ProjectCard({ project }: { project: Project }) {
  const [envDialog, setEnvDialog] = useState<EnvDialog>();
  const [deleting, setDeleting] = useState(false);
  const { data, isPending, error } = useEnvironments(project.id);

  return (
    <Panel
      actions={
        <>
          <Button onClick={() => setEnvDialog({ kind: "create" })} size="sm">
            New environment
          </Button>
          <Button onClick={() => setDeleting(true)} size="sm" variant="danger">
            Delete
          </Button>
        </>
      }
      description={`${project.slug} · created ${relativeTime(project.createdAt)}`}
      title={project.name}
    >
      {error ? <ErrorNotice error={error} /> : null}
      {isPending ? <LoadingState label="Loading environments…" /> : null}
      <ul className="divide-y divide-zinc-800">
        {(data?.items ?? []).map((environment) => (
          <li
            className="flex flex-wrap items-center justify-between gap-3 py-3"
            key={environment.id}
          >
            <Link
              className="group flex items-center gap-3"
              href={`/p/${project.id}/e/${environment.id}`}
            >
              <span className="font-medium text-zinc-200 group-hover:text-emerald-300">
                {environment.name}
              </span>
              <Badge>{environment.slug}</Badge>
            </Link>
            <div className="flex items-center gap-3">
              <span className="text-xs text-zinc-500">
                updated {relativeTime(environment.updatedAt)}
              </span>
              <Button
                onClick={() => setEnvDialog({ kind: "clone", environment })}
                size="sm"
                variant="ghost"
              >
                Clone
              </Button>
            </div>
          </li>
        ))}
      </ul>
      {data && data.items.length === 0 ? (
        <p className="py-3 text-sm text-zinc-500">
          No environments yet — create one to get started.
        </p>
      ) : null}

      {envDialog ? (
        <EnvironmentDialog
          onClose={() => setEnvDialog(undefined)}
          projectId={project.id}
          source={envDialog.kind === "clone" ? envDialog.environment : undefined}
        />
      ) : null}
      {deleting ? (
        <DeleteProjectDialog onClose={() => setDeleting(false)} project={project} />
      ) : null}
    </Panel>
  );
}
