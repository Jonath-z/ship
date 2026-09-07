"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input, Textarea } from "@/components/form";
import { Modal } from "@/components/Modal";
import { LoadingState, Panel } from "@/components/panels";
import { api, fieldErrorOf, type SshKey } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import { useSshKeys } from "@/lib/hooks";

/** Copies text to the clipboard and reports a transient "copied" state. */
export function CopyButton({ text, label }: { text: string; label: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      onClick={() => {
        void navigator.clipboard.writeText(text).then(() => {
          setCopied(true);
          window.setTimeout(() => setCopied(false), 1500);
        });
      }}
      size="sm"
      variant="ghost"
    >
      {copied ? "Copied" : label}
    </Button>
  );
}

/** SSH key list with generate/import/delete and public-key copy. */
export function SshKeysPanel() {
  const [dialog, setDialog] = useState<"generate" | "import">();
  const [deleting, setDeleting] = useState<SshKey>();
  const queryClient = useQueryClient();
  const { data, isPending, error } = useSshKeys();

  const remove = useMutation({
    mutationFn: (keyId: string) => api.sshKeys.remove(keyId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["ssh-keys"] });
      setDeleting(undefined);
    },
  });

  return (
    <Panel
      actions={
        <>
          <Button onClick={() => setDialog("import")} size="sm">
            Import key
          </Button>
          <Button onClick={() => setDialog("generate")} size="sm">
            Generate key
          </Button>
        </>
      }
      description="Keys Ship uses to reach your servers. Add the public key to each server's authorized_keys."
      title="SSH keys"
    >
      {error ? <ErrorNotice error={error} /> : null}
      {isPending ? <LoadingState label="Loading SSH keys…" /> : null}
      {data && data.items.length === 0 ? (
        <p className="py-3 text-sm text-zinc-500">
          No keys yet. Generate one, or import an existing private key.
        </p>
      ) : null}
      <ul className="divide-y divide-zinc-800">
        {(data?.items ?? []).map((key) => (
          <li
            className="flex flex-wrap items-center justify-between gap-3 py-3"
            key={key.id}
          >
            <div className="min-w-0">
              <p className="font-medium text-zinc-200">{key.name}</p>
              <p className="mt-0.5 max-w-xl truncate font-mono text-xs text-zinc-500">
                {key.publicKey}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-zinc-600">
                {relativeTime(key.createdAt)}
              </span>
              <CopyButton label="Copy public key" text={key.publicKey} />
              <Button
                onClick={() => setDeleting(key)}
                size="sm"
                variant="danger"
              >
                Delete
              </Button>
            </div>
          </li>
        ))}
      </ul>

      {dialog ? (
        <SshKeyDialog mode={dialog} onClose={() => setDialog(undefined)} />
      ) : null}
      {deleting ? (
        <ConfirmDialog
          busy={remove.isPending}
          confirmLabel="Delete key"
          danger
          error={remove.error}
          message={`Servers registered with "${deleting.name}" will lose SSH access through Ship.`}
          onCancel={() => setDeleting(undefined)}
          onConfirm={() => remove.mutate(deleting.id)}
          title={`Delete ${deleting.name}?`}
        />
      ) : null}
    </Panel>
  );
}

function SshKeyDialog({
  mode,
  onClose,
}: {
  mode: "generate" | "import";
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [privateKey, setPrivateKey] = useState("");
  const [created, setCreated] = useState<SshKey>();

  const create = useMutation({
    mutationFn: () =>
      api.sshKeys.create(
        mode === "import" ? { name, privateKey } : { name },
      ),
    onSuccess: async (key) => {
      await queryClient.invalidateQueries({ queryKey: ["ssh-keys"] });
      if (mode === "generate") setCreated(key);
      else onClose();
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    create.mutate();
  }

  if (created) {
    return (
      <Modal onClose={onClose} title="Key generated">
        <p className="text-sm text-zinc-400">
          Add this public key to <code>~/.ssh/authorized_keys</code> on each
          server before registering it.
        </p>
        <pre className="mt-4 overflow-x-auto whitespace-pre-wrap break-all rounded-lg border border-zinc-800 bg-zinc-950 p-3 font-mono text-xs text-zinc-300">
          {created.publicKey}
        </pre>
        <div className="mt-4 flex justify-end gap-2">
          <CopyButton label="Copy public key" text={created.publicKey} />
          <Button onClick={onClose} variant="primary">
            Done
          </Button>
        </div>
      </Modal>
    );
  }

  return (
    <Modal
      onClose={onClose}
      title={mode === "generate" ? "Generate SSH key" : "Import SSH key"}
    >
      <form className="grid gap-4" onSubmit={submit}>
        <Field error={fieldErrorOf(create.error, "name")} label="Name">
          <Input
            autoFocus
            onChange={(event) => setName(event.target.value)}
            placeholder="deploy-key"
            required
            value={name}
          />
        </Field>
        {mode === "import" ? (
          <Field
            error={fieldErrorOf(create.error, "privateKey")}
            hint="PEM-encoded private key; stored encrypted."
            label="Private key"
          >
            <Textarea
              className="font-mono"
              onChange={(event) => setPrivateKey(event.target.value)}
              placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
              required
              rows={6}
              value={privateKey}
            />
          </Field>
        ) : null}
        {create.error ? <ErrorNotice error={create.error} /> : null}
        <div className="flex justify-end gap-2">
          <Button onClick={onClose} variant="secondary">
            Cancel
          </Button>
          <Button disabled={create.isPending} type="submit" variant="primary">
            {create.isPending
              ? "Saving…"
              : mode === "generate"
                ? "Generate"
                : "Import"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
