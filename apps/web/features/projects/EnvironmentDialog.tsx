"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Input } from "@/components/form";
import { Modal } from "@/components/Modal";
import { api, fieldErrorOf, type Environment } from "@/lib/api";
import { slugify } from "@/lib/format";

/** Create a new environment, or clone `source` when provided. */
export function EnvironmentDialog({
  projectId,
  source,
  onClose,
}: {
  projectId: string;
  source?: Environment;
  onClose: () => void;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [name, setName] = useState(source ? `${source.name} copy` : "");
  const [slug, setSlug] = useState(source ? `${source.slug}-copy` : "");
  const [slugTouched, setSlugTouched] = useState(Boolean(source));
  const [includeSecrets, setIncludeSecrets] = useState(false);

  const save = useMutation({
    mutationFn: () =>
      source
        ? api.environments.clone(projectId, source.id, {
            name,
            slug,
            includeSecrets,
          })
        : api.environments.create(projectId, { name, slug }),
    onSuccess: async (environment) => {
      await queryClient.invalidateQueries({
        queryKey: ["environments", projectId],
      });
      onClose();
      router.push(`/p/${projectId}/e/${environment.id}`);
    },
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    save.mutate();
  }

  return (
    <Modal
      onClose={onClose}
      title={source ? `Clone ${source.name}` : "New environment"}
    >
      {source ? (
        <p className="mb-4 text-sm text-zinc-400">
          Copies services, databases, volumes, domains, variables, and
          dependencies into a new environment.
        </p>
      ) : null}
      <form className="grid gap-4" onSubmit={submit}>
        <Field error={fieldErrorOf(save.error, "name")} label="Name">
          <Input
            autoFocus
            onChange={(event) => {
              setName(event.target.value);
              if (!slugTouched) setSlug(slugify(event.target.value));
            }}
            placeholder="Production"
            required
            value={name}
          />
        </Field>
        <Field error={fieldErrorOf(save.error, "slug")} label="Slug">
          <Input
            onChange={(event) => {
              setSlugTouched(true);
              setSlug(event.target.value);
            }}
            placeholder="production"
            required
            value={slug}
          />
        </Field>
        {source ? (
          <label className="flex items-center gap-2 text-sm text-zinc-300">
            <input
              checked={includeSecrets}
              className="h-4 w-4 accent-emerald-400"
              onChange={(event) => setIncludeSecrets(event.target.checked)}
              type="checkbox"
            />
            Copy secret values into the clone
          </label>
        ) : null}
        {save.error ? <ErrorNotice error={save.error} /> : null}
        <div className="flex justify-end gap-2">
          <Button onClick={onClose} variant="secondary">
            Cancel
          </Button>
          <Button disabled={save.isPending} type="submit" variant="primary">
            {save.isPending
              ? "Saving…"
              : source
                ? "Clone environment"
                : "Create environment"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
