"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Input } from "@/components/form";
import { MonoValue } from "@/components/MonoValue";
import { LoadingState, Panel } from "@/components/panels";
import { api, fieldErrorOf, type EnvironmentVariable } from "@/lib/api";
import { ScopeSelect, useScopeName } from "@/features/variables/ScopeSelect";

/** Name | value | scope | actions; the value column absorbs the width. */
const rowColumns =
  "gap-x-6 gap-y-2 sm:grid-cols-[minmax(0,5fr)_minmax(0,8fr)_minmax(0,3fr)_auto]";

/** Plain environment variables with per-service scope and inline editing. */
export function VariablesPanel({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const queryClient = useQueryClient();
  const scopeName = useScopeName(projectId, environmentId);
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [scope, setScope] = useState("");
  const [editing, setEditing] = useState<{ id: string; value: string }>();
  const [deleting, setDeleting] = useState<EnvironmentVariable>();

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["variables", projectId, environmentId],
    });

  const variables = useQuery({
    queryKey: ["variables", projectId, environmentId],
    queryFn: () => api.variables.list(projectId, environmentId),
  });

  const create = useMutation({
    mutationFn: () =>
      api.variables.create(projectId, environmentId, {
        name,
        value,
        serviceId: scope || null,
      }),
    onSuccess: async () => {
      setName("");
      setValue("");
      await invalidate();
    },
  });

  const update = useMutation({
    mutationFn: (input: { id: string; value: string }) =>
      api.variables.update(projectId, environmentId, input.id, {
        value: input.value,
      }),
    onSuccess: async () => {
      setEditing(undefined);
      await invalidate();
    },
  });

  const remove = useMutation({
    mutationFn: (variableId: string) =>
      api.variables.remove(projectId, environmentId, variableId),
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
      description="Injected as plain environment variables at deploy time."
      title="Environment variables"
    >
      <form
        className="grid gap-3 sm:grid-cols-[1fr_1.5fr_180px_auto]"
        onSubmit={submit}
      >
        <Input
          aria-label="Variable name"
          className="font-mono"
          onChange={(event) => setName(event.target.value)}
          placeholder="APP_ENV"
          required
          value={name}
        />
        <Input
          aria-label="Variable value"
          className="font-mono"
          onChange={(event) => setValue(event.target.value)}
          placeholder="production"
          required
          value={value}
        />
        <ScopeSelect
          environmentId={environmentId}
          onChange={setScope}
          projectId={projectId}
          value={scope}
        />
        <Button disabled={create.isPending} type="submit" variant="primary">
          {create.isPending ? "Adding…" : "Add"}
        </Button>
      </form>
      {fieldErrorOf(create.error, "name") || create.error ? (
        <div className="mt-3">
          <ErrorNotice error={create.error} />
        </div>
      ) : null}
      {update.error ? (
        <div className="mt-3">
          <ErrorNotice error={update.error} />
        </div>
      ) : null}
      {variables.error ? (
        <div className="mt-3">
          <ErrorNotice error={variables.error} />
        </div>
      ) : null}

      {variables.isPending ? (
        <LoadingState label="Loading variables…" />
      ) : null}
      {variables.data && variables.data.items.length > 0 ? (
        <div className="mt-4">
          <div
            className={`${rowColumns} hidden border-b border-zinc-800 pb-2 text-xs font-medium text-zinc-500 sm:grid`}
          >
            <span>Name</span>
            <span>Value</span>
            <span>Scope</span>
            <span />
          </div>
          <ul className="divide-y divide-zinc-800">
            {variables.data.items.map((variable) => (
              <li className={`${rowColumns} grid py-3`} key={variable.id}>
                <p className="min-w-0 break-all font-mono text-sm text-zinc-200">
                  {variable.name}
                </p>
                <div className="min-w-0 text-zinc-400">
                  {editing?.id === variable.id ? (
                    <Input
                      aria-label={`New value for ${variable.name}`}
                      autoFocus
                      className="font-mono"
                      onChange={(event) =>
                        setEditing({
                          id: variable.id,
                          value: event.target.value,
                        })
                      }
                      value={editing.value}
                    />
                  ) : (
                    <MonoValue value={variable.value} />
                  )}
                </div>
                <p className="min-w-0 truncate text-sm text-zinc-400">
                  {scopeName(variable.serviceId)}
                </p>
                <div className="flex items-start justify-end gap-2">
                  {editing?.id === variable.id ? (
                    <>
                      <Button
                        disabled={update.isPending}
                        onClick={() => update.mutate(editing)}
                        size="sm"
                        variant="primary"
                      >
                        {update.isPending ? "Saving…" : "Save"}
                      </Button>
                      <Button
                        onClick={() => setEditing(undefined)}
                        size="sm"
                        variant="ghost"
                      >
                        Cancel
                      </Button>
                    </>
                  ) : (
                    <>
                      <Button
                        onClick={() =>
                          setEditing({
                            id: variable.id,
                            value: variable.value,
                          })
                        }
                        size="sm"
                        variant="ghost"
                      >
                        Edit
                      </Button>
                      <Button
                        onClick={() => setDeleting(variable)}
                        size="sm"
                        variant="danger"
                      >
                        Delete
                      </Button>
                    </>
                  )}
                </div>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {variables.data && variables.data.items.length === 0 ? (
        <p className="py-3 text-sm text-zinc-500">No variables yet.</p>
      ) : null}

      {deleting ? (
        <ConfirmDialog
          busy={remove.isPending}
          confirmLabel="Delete variable"
          danger
          error={remove.error}
          message={`${deleting.name} will no longer be injected on the next deployment.`}
          onCancel={() => setDeleting(undefined)}
          onConfirm={() => remove.mutate(deleting.id)}
          title={`Delete ${deleting.name}?`}
        />
      ) : null}
    </Panel>
  );
}
