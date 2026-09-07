"use client";

import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { LoadingState, Panel } from "@/components/panels";
import { api } from "@/lib/api";
import { ValidationList } from "@/features/services/ValidationList";

/** Rendered configuration preview plus validation results. */
export function ConfigTab({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const preview = useQuery({
    queryKey: ["configuration-preview", projectId, environmentId],
    queryFn: () => api.configuration.preview(projectId, environmentId),
  });

  return (
    <div className="grid gap-6">
      <Panel
        actions={
          <Button
            disabled={preview.isFetching}
            onClick={() => void preview.refetch()}
            size="sm"
          >
            {preview.isFetching ? "Refreshing…" : "Refresh"}
          </Button>
        }
        description="What the deploy engine will validate before every deployment."
        title="Validation"
      >
        {preview.isPending ? (
          <LoadingState label="Rendering configuration…" />
        ) : null}
        {preview.error ? <ErrorNotice error={preview.error} /> : null}
        {preview.data ? (
          <ValidationList violations={preview.data.validation} />
        ) : null}
      </Panel>

      {preview.data ? (
        <Panel
          description="Generated from the environment's services, databases, domains, volumes, and variables."
          title="Rendered configuration"
        >
          {Object.entries(preview.data.rendered).map(([file, content]) => (
            <div className="mb-6 last:mb-0" key={file}>
              <p className="mb-2 font-mono text-xs font-medium text-emerald-400">
                {file}
              </p>
              <pre className="overflow-x-auto rounded-lg border border-zinc-800 bg-zinc-950 p-4 font-mono text-xs leading-relaxed text-zinc-300">
                {content}
              </pre>
            </div>
          ))}
          {Object.keys(preview.data.rendered).length === 0 ? (
            <p className="text-sm text-zinc-500">Nothing to render yet.</p>
          ) : null}
        </Panel>
      ) : null}
    </div>
  );
}
