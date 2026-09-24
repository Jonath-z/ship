"use client";

import Link from "next/link";
import { useState } from "react";
import { Field, Input } from "@/components/form";
import { ImagePicker } from "@/features/services/ImagePicker";

export interface ServiceSource {
  repository: string;
  branch: string;
  image: string;
}

type Mode = "image" | "repository";

/**
 * Service source selector: a prebuilt registry image, or a git repository
 * that Ship clones, builds, and pushes as a commit-tagged image to the
 * environment's registry on every deploy. The two are mutually exclusive —
 * the deploy pipeline only builds when a repository is set and the image is
 * empty — so switching modes clears the other source in the emitted value
 * while stashing it locally for an accidental toggle back.
 */
export function SourcePicker({
  projectId,
  environmentId,
  value,
  onChange,
  errors,
}: {
  projectId: string;
  environmentId: string;
  value: ServiceSource;
  onChange: (next: ServiceSource) => void;
  errors?: { repository?: string; branch?: string; image?: string };
}) {
  const [mode, setMode] = useState<Mode>(() =>
    value.repository && !value.image ? "repository" : "image",
  );
  const [stash, setStash] = useState<ServiceSource>(value);

  function switchMode(next: Mode) {
    if (next === mode) return;
    setStash(value);
    setMode(next);
    onChange(
      next === "repository"
        ? { repository: stash.repository, branch: stash.branch, image: "" }
        : { repository: "", branch: "", image: stash.image },
    );
  }

  return (
    <div className="grid gap-4">
      <div className="inline-flex w-fit rounded-lg border border-zinc-700 p-0.5 text-xs">
        {(
          [
            ["image", "Registry image"],
            ["repository", "Git repository"],
          ] as const
        ).map(([key, label]) => (
          <button
            className={`rounded-md px-3 py-1.5 transition-colors ${
              mode === key
                ? "bg-zinc-800 font-medium text-zinc-100"
                : "text-zinc-500 hover:text-zinc-300"
            }`}
            key={key}
            onClick={() => switchMode(key)}
            type="button"
          >
            {label}
          </button>
        ))}
      </div>

      {mode === "image" ? (
        <ImagePicker
          environmentId={environmentId}
          error={errors?.image}
          onChange={(image) => onChange({ repository: "", branch: "", image })}
          projectId={projectId}
          value={value.image}
        />
      ) : (
        <div className="grid grid-cols-[1fr_160px] gap-4">
          <Field
            error={errors?.repository}
            hint="HTTPS clone URL. Every deploy clones this repository, builds its Dockerfile, and pushes a commit-tagged image to your registry."
            label="Repository"
          >
            <Input
              className="font-mono"
              onChange={(event) =>
                onChange({ ...value, repository: event.target.value })
              }
              placeholder="github.com/acme/api"
              value={value.repository}
            />
            <span className="mt-1 block text-xs text-zinc-500">
              Private repository? Save a{" "}
              <span className="font-mono text-zinc-400">GIT_TOKEN</span> secret
              under{" "}
              <Link
                className="underline hover:text-zinc-300"
                href={`/p/${projectId}/e/${environmentId}/variables`}
              >
                Variables
              </Link>
              .
            </span>
          </Field>
          <Field
            error={errors?.branch}
            hint="Defaults to main."
            label="Branch"
          >
            <Input
              className="font-mono"
              onChange={(event) =>
                onChange({ ...value, branch: event.target.value })
              }
              placeholder="main"
              value={value.branch}
            />
          </Field>
        </div>
      )}
    </div>
  );
}
