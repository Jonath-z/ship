"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Select, Textarea } from "@/components/form";
import { Panel } from "@/components/panels";
import { api, type ImportResult } from "@/lib/api";
import { ScopeSelect } from "@/features/variables/ScopeSelect";

/** Bulk .env import into either plain variables or secrets. */
export function ImportPanel({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const queryClient = useQueryClient();
  const [target, setTarget] = useState<"variables" | "secrets">("variables");
  const [scope, setScope] = useState("");
  const [content, setContent] = useState("");
  const [result, setResult] = useState<ImportResult>();

  const importEnv = useMutation({
    mutationFn: () => {
      const body = { content, serviceId: scope || null };
      return target === "variables"
        ? api.variables.import(projectId, environmentId, body)
        : api.secrets.import(projectId, environmentId, body);
    },
    onSuccess: async (imported) => {
      setResult(imported);
      setContent("");
      await queryClient.invalidateQueries({
        queryKey: [target, projectId, environmentId],
      });
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setResult(undefined);
    importEnv.mutate();
  }

  return (
    <Panel
      description="Paste a .env file (NAME=value per line). Existing entries with the same name are updated."
      title="Bulk import"
    >
      <form className="grid gap-4" onSubmit={submit}>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Import as">
            <Select
              onChange={(event) =>
                setTarget(event.target.value as "variables" | "secrets")
              }
              value={target}
            >
              <option value="variables">Environment variables</option>
              <option value="secrets">Secrets (encrypted)</option>
            </Select>
          </Field>
          <Field label="Scope">
            <ScopeSelect
              environmentId={environmentId}
              onChange={setScope}
              projectId={projectId}
              value={scope}
            />
          </Field>
        </div>
        <Textarea
          aria-label=".env content"
          className="font-mono"
          onChange={(event) => setContent(event.target.value)}
          placeholder={"APP_ENV=production\nDATABASE_URL=postgres://…"}
          required
          rows={6}
          value={content}
        />
        {importEnv.error ? <ErrorNotice error={importEnv.error} /> : null}
        <div className="flex items-center justify-end gap-3">
          {result ? (
            <span className="text-sm text-emerald-400">
              Imported: {result.created} created, {result.updated} updated.
            </span>
          ) : null}
          <Button
            disabled={importEnv.isPending}
            type="submit"
            variant="primary"
          >
            {importEnv.isPending ? "Importing…" : "Import"}
          </Button>
        </div>
      </form>
    </Panel>
  );
}
