"use client";

import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";
import { useEnvironments, useProjects } from "@/lib/hooks";
import { envRoute } from "@/lib/routes";
import { IconChevronsUpDown } from "@/features/shell/icons";

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

function CrumbSelect({
  ariaLabel,
  value,
  onChange,
  disabled,
  children,
}: {
  ariaLabel: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <span className="relative inline-flex min-w-0 items-center">
      <select
        aria-label={ariaLabel}
        className="min-w-0 max-w-[11rem] cursor-pointer appearance-none truncate rounded-md bg-transparent py-1 pl-2 pr-7 text-sm font-medium text-zinc-100 outline-none transition-colors duration-150 hover:bg-zinc-900 focus-visible:bg-zinc-900 disabled:cursor-default disabled:opacity-50"
        disabled={disabled}
        onChange={(event) => onChange(event.target.value)}
        value={value}
      >
        {children}
      </select>
      <IconChevronsUpDown
        className="pointer-events-none absolute right-2 text-zinc-500"
        size={14}
      />
    </span>
  );
}

/**
 * Scope switcher in the top bar. At the control-plane level it reads
 * "All projects"; picking a project reveals its environments, and picking an
 * environment enters that scope (/env/...?project=X&env=Y). Inside a scope
 * the current section is kept when switching, and "All projects" leaves it.
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
  const [selectedProject, setSelectedProject] = useState<string>();
  const activeProject = selectedProject ?? projectId;

  const { data: projects } = useProjects();
  const { data: environments } = useEnvironments(activeProject);

  // A pending project choice only lives until the next navigation.
  useEffect(() => {
    setSelectedProject(undefined);
  }, [pathname, projectId, environmentId]);

  // Preserve the section suffix (e.g. /applications) when switching scope.
  const section =
    projectId && environmentId ? pathname.replace(/^\/env/, "") : "";

  function onProjectChange(nextProject: string) {
    if (!nextProject) {
      setSelectedProject(undefined);
      router.push("/projects");
      return;
    }
    setSelectedProject(nextProject);
  }

  function onEnvironmentChange(nextEnvironment: string) {
    if (activeProject && nextEnvironment) {
      router.push(envRoute(activeProject, nextEnvironment, section));
    }
  }

  return (
    <div className="flex min-w-0 items-center">
      <CrumbSelect
        ariaLabel="Project"
        onChange={onProjectChange}
        value={activeProject ?? ""}
      >
        <option value="">All projects</option>
        {(projects?.items ?? []).map((project) => (
          <option key={project.id} value={project.id}>
            {project.name}
          </option>
        ))}
      </CrumbSelect>
      {activeProject ? (
        <>
          <CrumbSlash />
          <CrumbSelect
            ariaLabel="Environment"
            onChange={onEnvironmentChange}
            value={
              selectedProject && selectedProject !== projectId
                ? ""
                : (environmentId ?? "")
            }
          >
            <option disabled value="">
              {environments && environments.items.length === 0
                ? "No environments"
                : "Environment…"}
            </option>
            {(environments?.items ?? []).map((environment) => (
              <option key={environment.id} value={environment.id}>
                {environment.name}
              </option>
            ))}
          </CrumbSelect>
        </>
      ) : null}
    </div>
  );
}
