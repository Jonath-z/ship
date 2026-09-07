"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Input } from "@/components/form";
import { Modal } from "@/components/Modal";
import { LoadingState, Panel } from "@/components/panels";
import { api, fieldErrorOf, type Secret } from "@/lib/api";
import { ScopeSelect, useScopeName } from "@/features/variables/ScopeSelect";

const revealDurationMs = 15_000;

/** Encrypted secrets: masked values, audited reveal, set-value dialog. */
export function SecretsPanel({
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
  const [settingValue, setSettingValue] = useState<Secret>();
  const [deleting, setDeleting] = useState<Secret>();
  const [revealed, setRevealed] = useState<{ id: string; value: string }>();
  const revealTimer = useRef<number>();

  useEffect(
    () => () => window.clearTimeout(revealTimer.current),
    [],
  );

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["secrets", projectId, environmentId],
    });

  const secrets = useQuery({
    queryKey: ["secrets", projectId, environmentId],
    queryFn: () => api.secrets.list(projectId, environmentId),
  });

  const create = useMutation({
    mutationFn: () =>
      api.secrets.create(projectId, environmentId, {
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

  const remove = useMutation({
    mutationFn: (secretId: string) =>
      api.secrets.remove(projectId, environmentId, secretId),
    onSuccess: async () => {
      setDeleting(undefined);
      await invalidate();
    },
  });

  const reveal = useMutation({
    mutationFn: (secretId: string) =>
      api.secrets.reveal(projectId, environmentId, secretId),
    onSuccess: (result, secretId) => {
      setRevealed({ id: secretId, value: result.value });
      window.clearTimeout(revealTimer.current);
      revealTimer.current = window.setTimeout(
        () => setRevealed(undefined),
        revealDurationMs,
      );
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    create.mutate();
  }

  return (
    <Panel
      description="Stored encrypted; values are only decrypted at deploy time. Revealing a value is recorded in the audit log."
      title="Secrets"
    >
      <form
        className="grid gap-3 sm:grid-cols-[1fr_1.5fr_180px_auto]"
        onSubmit={submit}
      >
        <Input
          aria-label="Secret name"
          className="font-mono"
          onChange={(event) => setName(event.target.value)}
          placeholder="DATABASE_URL"
          required
          value={name}
        />
        <Input
          aria-label="Secret value"
          autoComplete="off"
          className="font-mono"
          onChange={(event) => setValue(event.target.value)}
          placeholder="value"
          required
          type="password"
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
      {create.error ? (
        <div className="mt-3">
          <ErrorNotice error={create.error} />
        </div>
      ) : null}
      {reveal.error ? (
        <div className="mt-3">
          <ErrorNotice error={reveal.error} />
        </div>
      ) : null}
      {secrets.error ? (
        <div className="mt-3">
          <ErrorNotice error={secrets.error} />
        </div>
      ) : null}

      {secrets.isPending ? <LoadingState label="Loading secrets…" /> : null}
      {secrets.data && secrets.data.items.length > 0 ? (
        <div className="mt-4 overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="text-zinc-500">
              <tr>
                <th className="pb-3 pr-4">Name</th>
                <th className="pb-3 pr-4">Value</th>
                <th className="pb-3 pr-4">Scope</th>
                <th className="pb-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {secrets.data.items.map((secret) => (
                <tr key={secret.id}>
                  <td className="py-3 pr-4 font-mono text-zinc-200">
                    {secret.name}
                  </td>
                  <td className="py-3 pr-4 font-mono text-zinc-400">
                    {revealed?.id === secret.id ? (
                      <span className="break-all text-emerald-300">
                        {revealed.value}
                      </span>
                    ) : secret.hasValue ? (
                      "••••••••"
                    ) : (
                      <span className="text-amber-400">not set</span>
                    )}
                  </td>
                  <td className="py-3 pr-4 text-zinc-400">
                    {scopeName(secret.serviceId)}
                  </td>
                  <td className="py-3 text-right">
                    <span className="inline-flex gap-2">
                      {revealed?.id === secret.id ? (
                        <Button
                          onClick={() => setRevealed(undefined)}
                          size="sm"
                          variant="ghost"
                        >
                          Hide
                        </Button>
                      ) : (
                        <Button
                          disabled={reveal.isPending || !secret.hasValue}
                          onClick={() => reveal.mutate(secret.id)}
                          size="sm"
                          variant="ghost"
                        >
                          Reveal
                        </Button>
                      )}
                      <Button
                        onClick={() => setSettingValue(secret)}
                        size="sm"
                        variant="ghost"
                      >
                        Set value
                      </Button>
                      <Button
                        onClick={() => setDeleting(secret)}
                        size="sm"
                        variant="danger"
                      >
                        Delete
                      </Button>
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      {secrets.data && secrets.data.items.length === 0 ? (
        <p className="py-3 text-sm text-zinc-500">No secrets yet.</p>
      ) : null}

      {settingValue ? (
        <SetSecretValueDialog
          environmentId={environmentId}
          onClose={() => setSettingValue(undefined)}
          projectId={projectId}
          secret={settingValue}
        />
      ) : null}
      {deleting ? (
        <ConfirmDialog
          busy={remove.isPending}
          confirmLabel="Delete secret"
          danger
          error={remove.error}
          message={`${deleting.name} will no longer be available to deployments.`}
          onCancel={() => setDeleting(undefined)}
          onConfirm={() => remove.mutate(deleting.id)}
          title={`Delete ${deleting.name}?`}
        />
      ) : null}
    </Panel>
  );
}

function SetSecretValueDialog({
  projectId,
  environmentId,
  secret,
  onClose,
}: {
  projectId: string;
  environmentId: string;
  secret: Secret;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [value, setValue] = useState("");

  const save = useMutation({
    mutationFn: () =>
      api.secrets.update(projectId, environmentId, secret.id, { value }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ["secrets", projectId, environmentId],
      });
      onClose();
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    save.mutate();
  }

  return (
    <Modal onClose={onClose} title={`Set value · ${secret.name}`}>
      <form className="grid gap-4" onSubmit={submit}>
        <Input
          aria-label="New secret value"
          autoComplete="off"
          autoFocus
          className="font-mono"
          onChange={(event) => setValue(event.target.value)}
          placeholder="New value"
          required
          type="password"
          value={value}
        />
        {fieldErrorOf(save.error, "value") ? (
          <p className="text-xs text-red-400">
            {fieldErrorOf(save.error, "value")}
          </p>
        ) : null}
        {save.error ? <ErrorNotice error={save.error} /> : null}
        <div className="flex justify-end gap-2">
          <Button onClick={onClose} variant="secondary">
            Cancel
          </Button>
          <Button disabled={save.isPending} type="submit" variant="primary">
            {save.isPending ? "Saving…" : "Save value"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
