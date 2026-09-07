"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Badge, type BadgeTone } from "@/components/Badge";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Field, Select } from "@/components/form";
import { Modal } from "@/components/Modal";
import { LoadingState } from "@/components/panels";
import { api, type ConfigurationDiff } from "@/lib/api";
import { useServices } from "@/lib/hooks";
import { ValidationList } from "@/features/services/ValidationList";

const changeTones: Record<string, BadgeTone> = {
  added: "emerald",
  removed: "red",
  changed: "amber",
};

/**
 * Deploy flow: pick a service, review validations and the diff against the
 * latest configuration version, then queue the deployment.
 */
export function DeployModal({
  projectId,
  environmentId,
  initialServiceId,
  onClose,
}: {
  projectId: string;
  environmentId: string;
  initialServiceId?: string;
  onClose: () => void;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [serviceId, setServiceId] = useState(initialServiceId ?? "");

  const services = useServices(projectId, environmentId);

  const preview = useQuery({
    queryKey: ["configuration-preview", projectId, environmentId],
    queryFn: () => api.configuration.preview(projectId, environmentId),
  });

  // What this deploy will change: current state vs the latest snapshot.
  const diff = useQuery({
    queryKey: ["configuration-pending-diff", projectId, environmentId],
    queryFn: () => api.configuration.pendingDiff(projectId, environmentId),
  });

  const deploy = useMutation({
    mutationFn: () => api.deployments.create(projectId, environmentId, serviceId),
    onSuccess: async (deployment) => {
      await queryClient.invalidateQueries({
        queryKey: ["deployments", projectId, environmentId],
      });
      onClose();
      router.push(
        `/p/${projectId}/e/${environmentId}/deployments/${deployment.id}`,
      );
    },
  });

  const blocked = (preview.data?.validation ?? []).some(
    (violation) => violation.severity === "block",
  );

  return (
    <Modal onClose={onClose} title="Deploy" wide>
      <div className="grid gap-5">
        <Field label="Service">
          <Select
            onChange={(event) => setServiceId(event.target.value)}
            value={serviceId}
          >
            <option disabled value="">
              Choose a service…
            </option>
            {(services.data?.items ?? []).map((service) => (
              <option key={service.id} value={service.id}>
                {service.name}
              </option>
            ))}
          </Select>
        </Field>

        <section>
          <h3 className="mb-2 text-sm font-medium text-zinc-300">Validation</h3>
          {preview.isPending ? (
            <LoadingState label="Validating configuration…" />
          ) : null}
          {preview.error ? <ErrorNotice error={preview.error} /> : null}
          {preview.data ? (
            <ValidationList violations={preview.data.validation} />
          ) : null}
        </section>

        <section>
          <h3 className="mb-2 text-sm font-medium text-zinc-300">
            What this deploy will change
          </h3>
          {diff.isPending ? <LoadingState label="Computing diff…" /> : null}
          {diff.data && diff.data.from === 0 ? (
            <p className="mb-2 text-sm text-zinc-500">
              No configuration snapshot yet — this deploys the full current
              configuration.
            </p>
          ) : null}
          {diff.error ? <ErrorNotice error={diff.error} /> : null}
          {diff.data ? <DiffView diff={diff.data} /> : null}
        </section>

        {deploy.error ? <ErrorNotice error={deploy.error} /> : null}
        {blocked ? (
          <p className="text-sm text-red-300">
            Deployment is blocked until the validation errors above are
            resolved.
          </p>
        ) : null}

        <div className="flex justify-end gap-2">
          <Button onClick={onClose} variant="secondary">
            Cancel
          </Button>
          <Button
            disabled={!serviceId || blocked || deploy.isPending || preview.isPending}
            onClick={() => deploy.mutate()}
            variant="primary"
          >
            {deploy.isPending ? "Queuing…" : "Deploy"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function DiffView({ diff }: { diff: ConfigurationDiff }) {
  const changed = diff.entities.filter(
    (entity) => entity.change !== "unchanged",
  );
  if (changed.length === 0) {
    return (
      <p className="text-sm text-zinc-500">
        No configuration changes since version {diff.from}.
      </p>
    );
  }
  return (
    <ul className="space-y-2">
      {changed.map((entity) => (
        <li
          className="rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-sm"
          key={`${entity.kind}-${entity.name}`}
        >
          <div className="flex items-center gap-2">
            <Badge tone={changeTones[entity.change] ?? "zinc"}>
              {entity.change}
            </Badge>
            <span className="text-zinc-200">{entity.name}</span>
            <span className="text-xs text-zinc-500">{entity.kind}</span>
          </div>
          {entity.fields && entity.fields.length > 0 ? (
            <ul className="mt-2 space-y-1 font-mono text-xs text-zinc-400">
              {entity.fields.map((field) => (
                <li key={field.field}>
                  {field.field}:{" "}
                  <span className="text-red-300">{field.from ?? "∅"}</span> →{" "}
                  <span className="text-emerald-300">{field.to ?? "∅"}</span>
                </li>
              ))}
            </ul>
          ) : null}
        </li>
      ))}
    </ul>
  );
}
