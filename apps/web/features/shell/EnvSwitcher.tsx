"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";
import { Select } from "@/components/form";
import { useEnvironments, useProjects } from "@/lib/hooks";

/**
 * Project/environment switcher in the top bar. The selection is persisted in
 * the URL (/p/[projectId]/e/[environmentId]/...); switching keeps the current
 * section when possible.
 */
export function EnvSwitcher({
  projectId,
  environmentId,
}: {
  projectId?: string;
  environmentId?: string;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const [selectedProject, setSelectedProject] = useState<string | undefined>();
  const activeProject = selectedProject ?? projectId;

  const { data: projects } = useProjects();
  const { data: environments } = useEnvironments(activeProject);

  // Preserve the section suffix (e.g. /applications) when switching scope.
  const section =
    projectId && environmentId
      ? pathname.replace(`/p/${projectId}/e/${environmentId}`, "")
      : "";

  function navigate(nextProject: string, nextEnvironment: string) {
    router.push(`/p/${nextProject}/e/${nextEnvironment}${section}`);
  }

  function onProjectChange(nextProject: string) {
    setSelectedProject(nextProject);
  }

  function onEnvironmentChange(nextEnvironment: string) {
    if (activeProject) navigate(activeProject, nextEnvironment);
  }

  return (
    <div className="flex items-center gap-2">
      <Select
        aria-label="Project"
        className="w-40"
        onChange={(event) => onProjectChange(event.target.value)}
        value={activeProject ?? ""}
      >
        <option disabled value="">
          Project…
        </option>
        {(projects?.items ?? []).map((project) => (
          <option key={project.id} value={project.id}>
            {project.name}
          </option>
        ))}
      </Select>
      <Select
        aria-label="Environment"
        className="w-36"
        disabled={!activeProject}
        onChange={(event) => onEnvironmentChange(event.target.value)}
        value={
          selectedProject && selectedProject !== projectId
            ? ""
            : (environmentId ?? "")
        }
      >
        <option disabled value="">
          Environment…
        </option>
        {(environments?.items ?? []).map((environment) => (
          <option key={environment.id} value={environment.id}>
            {environment.name}
          </option>
        ))}
      </Select>
      <Link
        className="text-sm text-zinc-500 hover:text-zinc-200"
        href="/projects"
      >
        Manage
      </Link>
    </div>
  );
}
