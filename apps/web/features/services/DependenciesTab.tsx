"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Select } from "@/components/form";
import { LoadingState, Panel } from "@/components/panels";
import { api, type Dependency } from "@/lib/api";
import { useAccessories, useServices } from "@/lib/hooks";

/** Startup-order edges from this service to other services or accessories. */
export function DependenciesTab({
  projectId,
  environmentId,
  serviceId,
}: {
  projectId: string;
  environmentId: string;
  serviceId: string;
}) {
  const queryClient = useQueryClient();
  const [target, setTarget] = useState("");

  const services = useServices(projectId, environmentId);
  const accessories = useAccessories(projectId, environmentId);
  const dependencies = useQuery({
    queryKey: ["dependencies", projectId, environmentId],
    queryFn: () => api.dependencies.list(projectId, environmentId),
  });

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: ["dependencies", projectId, environmentId],
    });

  const mine = (dependencies.data?.items ?? []).filter(
    (dependency) => dependency.sourceServiceId === serviceId,
  );

  const create = useMutation({
    mutationFn: () => {
      const [kind, id] = target.split(":", 2);
      return api.dependencies.create(projectId, environmentId, {
        sourceServiceId: serviceId,
        targetServiceId: kind === "service" ? id : null,
        targetAccessoryId: kind === "accessory" ? id : null,
        type: "runtime",
      });
    },
    onSuccess: async () => {
      setTarget("");
      await invalidate();
    },
  });

  const remove = useMutation({
    mutationFn: (dependencyId: string) =>
      api.dependencies.remove(projectId, environmentId, dependencyId),
    onSuccess: invalidate,
  });

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (target) create.mutate();
  }

  function targetLabel(dependency: Dependency): {
    name: string;
    kind: "service" | "database";
  } {
    if (dependency.targetServiceId) {
      const service = services.data?.items.find(
        (candidate) => candidate.id === dependency.targetServiceId,
      );
      return { name: service?.name ?? dependency.targetServiceId, kind: "service" };
    }
    const accessory = accessories.data?.items.find(
      (candidate) => candidate.id === dependency.targetAccessoryId,
    );
    return {
      name: accessory?.name ?? dependency.targetAccessoryId ?? "unknown",
      kind: "database",
    };
  }

  const targetOptions = [
    ...(services.data?.items ?? [])
      .filter((service) => service.id !== serviceId)
      .map((service) => ({
        value: `service:${service.id}`,
        label: `${service.name} (application)`,
      })),
    ...(accessories.data?.items ?? []).map((accessory) => ({
      value: `accessory:${accessory.id}`,
      label: `${accessory.name} (database)`,
    })),
  ];

  return (
    <Panel
      description="This application starts after its dependencies are healthy."
      title="Dependencies"
    >
      <form className="grid gap-3 sm:grid-cols-[1fr_auto]" onSubmit={submit}>
        <Field label="Depends on">
          <Select
            onChange={(event) => setTarget(event.target.value)}
            required
            value={target}
          >
            <option disabled value="">
              Choose an application or database…
            </option>
            {targetOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </Select>
        </Field>
        <div className="pt-6">
          <Button
            disabled={create.isPending || !target}
            type="submit"
            variant="primary"
          >
            {create.isPending ? "Adding…" : "Add dependency"}
          </Button>
        </div>
      </form>
      {create.error ? (
        <div className="mt-3">
          <ErrorNotice error={create.error} />
        </div>
      ) : null}
      {remove.error ? (
        <div className="mt-3">
          <ErrorNotice error={remove.error} />
        </div>
      ) : null}
      {dependencies.error ? (
        <div className="mt-3">
          <ErrorNotice error={dependencies.error} />
        </div>
      ) : null}

      {dependencies.isPending ? (
        <LoadingState label="Loading dependencies…" />
      ) : null}
      <ul className="mt-4 divide-y divide-zinc-800">
        {mine.map((dependency) => {
          const { name, kind } = targetLabel(dependency);
          return (
            <li
              className="flex items-center justify-between gap-3 py-3"
              key={dependency.id}
            >
              <div className="flex items-center gap-3">
                <span className="font-medium text-zinc-200">{name}</span>
                <Badge tone={kind === "database" ? "sky" : "zinc"}>{kind}</Badge>
                <Badge>{dependency.type}</Badge>
              </div>
              <Button
                disabled={remove.isPending}
                onClick={() => remove.mutate(dependency.id)}
                size="sm"
                variant="danger"
              >
                Remove
              </Button>
            </li>
          );
        })}
      </ul>
      {dependencies.data && mine.length === 0 ? (
        <p className="py-3 text-sm text-zinc-500">
          No dependencies — this application starts independently.
        </p>
      ) : null}
    </Panel>
  );
}
