"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useState } from "react";
import { useEnvironments, useProjects } from "@/lib/hooks";
import { envRoute } from "@/lib/routes";

const crumbSelectClasses =
  "max-w-40 cursor-pointer appearance-none truncate rounded-md bg-transparent px-2 py-1 text-sm font-medium text-zinc-100 outline-none transition-colors duration-150 hover:bg-zinc-900 focus:bg-zinc-900 disabled:cursor-default disabled:opacity-50";

/** Slanted divider between breadcrumb segments, as on vercel.com. */
function CrumbSlash() {
  return (
    <svg
      aria-hidden
      className="h-6 w-6 shrink-0 text-zinc-800"
      fill="none"
      stroke="currentColor"
      strokeWidth="1"
      viewBox="0 0 24 24"
    >
      <path d="M16.88 3.549L7.12 20.451" />
    </svg>
  );
}

/**
 * Project/environment breadcrumb switcher in the top bar. The selection is
 * persisted in the URL (/env/...?project=X&env=Y); switching keeps the
 * current section when possible.
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
    projectId && environmentId ? pathname.replace(/^\/env/, "") : "";

  function navigate(nextProject: string, nextEnvironment: string) {
    router.push(envRoute(nextProject, nextEnvironment, section));
  }

  function onProjectChange(nextProject: string) {
    setSelectedProject(nextProject);
  }

  function onEnvironmentChange(nextEnvironment: string) {
    if (activeProject) navigate(activeProject, nextEnvironment);
  }

  return (
    <div className="flex min-w-0 items-center">
      <CrumbSlash />
      <select
        aria-label="Project"
        className={crumbSelectClasses}
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
      </select>
      <CrumbSlash />
      <select
        aria-label="Environment"
        className={crumbSelectClasses}
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
      </select>
      <Link
        className="ml-3 hidden whitespace-nowrap text-sm text-zinc-500 transition-colors duration-150 hover:text-zinc-100 sm:inline"
        href="/projects"
      >
        Manage
      </Link>
    </div>
  );
}
