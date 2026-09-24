"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useState } from "react";
import { Field, Input, Select } from "@/components/form";
import { api, ApiError } from "@/lib/api";

/**
 * Host part of the stored registry server, e.g. "ghcr.io". Any namespace
 * path (registry.digitalocean.com/my-registry) is dropped: browse results
 * already carry namespaced repository names.
 */
function registryPrefix(server?: string): string {
  if (!server) return "";
  return server.replace(/^https?:\/\//, "").replace(/\/.*$/, "");
}

/** Split "[server/]repo:tag" into its parts for preselecting the dropdowns. */
function parseReference(
  reference: string,
  server: string,
): { repository: string; tag: string } {
  let rest = reference;
  if (server && rest.startsWith(`${server}/`)) {
    rest = rest.slice(server.length + 1);
  }
  const colon = rest.lastIndexOf(":");
  if (colon <= 0) return { repository: rest, tag: "" };
  return { repository: rest.slice(0, colon), tag: rest.slice(colon + 1) };
}

/**
 * Image selector backed by the environment's registry credentials: browse
 * repositories and tags instead of typing references. Falls back to manual
 * entry when no registry is configured (public images) or on request.
 */
export function ImagePicker({
  projectId,
  environmentId,
  value,
  onChange,
  error,
}: {
  projectId: string;
  environmentId: string;
  value: string;
  onChange: (image: string) => void;
  error?: string;
}) {
  // Editing an existing reference starts in manual mode so the current value
  // stays visible; switching to browse parses it into the dropdowns.
  const [manual, setManual] = useState(() => Boolean(value));
  const [repository, setRepository] = useState("");
  const [tag, setTag] = useState("");

  const repositories = useQuery({
    queryKey: ["registry-repositories", projectId, environmentId],
    queryFn: () => api.registry.repositories(projectId, environmentId),
    retry: false,
    staleTime: 60_000,
  });
  const tags = useQuery({
    queryKey: ["registry-tags", projectId, environmentId, repository],
    queryFn: () => api.registry.tags(projectId, environmentId, repository),
    enabled: Boolean(repository),
    retry: false,
  });
  const variables = useQuery({
    queryKey: ["variables", projectId, environmentId],
    queryFn: () => api.variables.list(projectId, environmentId),
  });

  const notConfigured =
    repositories.error instanceof ApiError &&
    repositories.error.code === "registry_not_configured";
  const browseFailed = Boolean(repositories.error) && !notConfigured;

  const server = registryPrefix(
    variables.data?.items.find(
      (variable) =>
        variable.name === "KAMAL_REGISTRY_SERVER" && !variable.serviceId,
    )?.value,
  );

  function compose(nextRepository: string, nextTag: string) {
    if (!nextRepository || !nextTag) {
      onChange("");
      return;
    }
    const reference = `${nextRepository}:${nextTag}`;
    onChange(server ? `${server}/${reference}` : reference);
  }

  function startBrowsing() {
    if (value) {
      const parsed = parseReference(value, server);
      setRepository(parsed.repository);
      setTag(parsed.tag);
    }
    setManual(false);
  }

  if (manual || notConfigured || browseFailed) {
    return (
      <Field
        error={error}
        hint={
          notConfigured
            ? undefined
            : "Prebuilt image reference including registry and tag."
        }
        label="Image"
      >
        <Input
          className="font-mono"
          onChange={(event) => onChange(event.target.value)}
          placeholder="ghcr.io/acme/web:latest"
          value={value}
        />
        {notConfigured ? (
          <span className="mt-1 block text-xs text-amber-300">
            No registry credentials —{" "}
            <Link
              className="underline hover:text-amber-200"
              href={`/p/${projectId}/e/${environmentId}/variables`}
            >
              save them under Variables
            </Link>{" "}
            to browse your images here instead of typing references.
          </span>
        ) : null}
        {browseFailed ? (
          <span className="mt-1 block text-xs text-amber-300">
            Could not reach the registry; enter the reference manually.
          </span>
        ) : null}
        {!notConfigured && !browseFailed ? (
          <button
            className="mt-1 text-xs text-emerald-400 hover:text-emerald-300"
            onClick={startBrowsing}
            type="button"
          >
            Browse the registry instead
          </button>
        ) : null}
      </Field>
    );
  }

  return (
    <div className="grid grid-cols-[1fr_140px] gap-4">
      <Field error={error} label="Image repository">
        <Select
          disabled={repositories.isPending}
          onChange={(event) => {
            setRepository(event.target.value);
            setTag("");
            onChange("");
          }}
          value={repository}
        >
          <option disabled value="">
            {repositories.isPending
              ? "Loading repositories…"
              : repositories.data?.items.length
                ? "Choose a repository…"
                : "No repositories found"}
          </option>
          {(repositories.data?.items ?? []).map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </Select>
        <button
          className="mt-1 text-xs text-zinc-500 hover:text-zinc-300"
          onClick={() => setManual(true)}
          type="button"
        >
          Enter an image reference manually
        </button>
      </Field>
      <Field label="Tag">
        <Select
          disabled={!repository || tags.isPending}
          onChange={(event) => {
            setTag(event.target.value);
            compose(repository, event.target.value);
          }}
          value={tag}
        >
          <option disabled value="">
            {!repository
              ? "—"
              : tags.isPending
                ? "Loading…"
                : tags.data?.items.length
                  ? "Tag…"
                  : "No tags"}
          </option>
          {(tags.data?.items ?? []).map((name) => (
            <option key={name} value={name}>
              {name}
            </option>
          ))}
        </Select>
        {tags.error ? (
          <span className="mt-1 block text-xs text-amber-300">
            Tags unavailable.
          </span>
        ) : null}
      </Field>
    </div>
  );
}
