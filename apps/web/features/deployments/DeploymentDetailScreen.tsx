"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useCallback, useState } from "react";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorNotice } from "@/components/ErrorNotice";
import { LoadingState, PageHeader } from "@/components/panels";
import {
  api,
  terminalDeploymentStatuses,
  type DeploymentStatus,
} from "@/lib/api";
import { formatDuration, relativeTime } from "@/lib/format";
import { LogTerminal } from "@/features/deployments/LogTerminal";
import { StatusBadge } from "@/features/deployments/StatusBadge";

const phases: DeploymentStatus[] = [
  "QUEUED",
  "VALIDATING",
  "BUILDING",
  "PUSHING",
  "DEPLOYING",
  "VERIFYING",
  "SUCCESS",
];

/** One deployment: phase timeline, live log stream, and rollback. */
export function DeploymentDetailScreen({
  projectId,
  environmentId,
  deploymentId,
}: {
  projectId: string;
  environmentId: string;
  deploymentId: string;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [rollingBack, setRollingBack] = useState(false);

  const deployment = useQuery({
    queryKey: ["deployment", projectId, environmentId, deploymentId],
    queryFn: () => api.deployments.get(projectId, environmentId, deploymentId),
    refetchInterval: (query) =>
      query.state.data &&
      terminalDeploymentStatuses.includes(query.state.data.status)
        ? false
        : 3000,
  });

  const rollback = useMutation({
    mutationFn: () =>
      api.deployments.rollback(projectId, environmentId, deploymentId),
    onSuccess: async (created) => {
      await queryClient.invalidateQueries({
        queryKey: ["deployments", projectId, environmentId],
      });
      setRollingBack(false);
      router.push(
        `/p/${projectId}/e/${environmentId}/deployments/${created.id}`,
      );
    },
  });

  const onStreamStatus = useCallback(() => {
    void queryClient.invalidateQueries({
      queryKey: ["deployment", projectId, environmentId, deploymentId],
    });
  }, [queryClient, projectId, environmentId, deploymentId]);

  if (deployment.isPending) {
    return (
      <main className="mx-auto max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
        <LoadingState label="Loading deployment…" />
      </main>
    );
  }
  if (deployment.error || !deployment.data) {
    return (
      <main className="mx-auto max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
        <ErrorNotice
          error={deployment.error ?? new Error("Deployment not found")}
        />
      </main>
    );
  }
  const data = deployment.data;

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        actions={
          data.status === "SUCCESS" ? (
            <Button onClick={() => setRollingBack(true)} variant="danger">
              Rollback
            </Button>
          ) : undefined
        }
        description={`${data.serviceName ?? data.serviceId}${data.image ? ` · ${data.image}` : ""} · started ${relativeTime(data.startedAt ?? data.createdAt)}${
          data.finishedAt
            ? ` · took ${formatDuration(data.startedAt ?? data.createdAt, data.finishedAt)}`
            : ""
        }`}
        eyebrow="Deployment"
        title={`Deployment ${data.id.slice(0, 8)}`}
      />

      <div className="mt-6 flex flex-wrap items-center gap-4">
        <StatusBadge status={data.status} />
        <PhaseTimeline status={data.status} />
      </div>

      <div className="mt-8">
        <LogTerminal
          deploymentId={deploymentId}
          environmentId={environmentId}
          onStatus={onStreamStatus}
          projectId={projectId}
        />
      </div>

      {rollingBack ? (
        <ConfirmDialog
          busy={rollback.isPending}
          confirmLabel="Rollback"
          danger
          error={rollback.error}
          message="Re-deploys the previously successful version of this service. A new deployment is created to track the rollback."
          onCancel={() => setRollingBack(false)}
          onConfirm={() => rollback.mutate()}
          title="Rollback this deployment?"
        />
      ) : null}
    </main>
  );
}

function PhaseTimeline({ status }: { status: DeploymentStatus }) {
  const failed = status === "FAILED";
  const rolling = status === "ROLLING_BACK" || status === "ROLLED_BACK";
  const activeIndex = phases.indexOf(status);
  // FAILED/ROLLING_BACK happen mid-flow; treat every standard phase before the
  // terminal marker as reached so the trail stays visible.
  const reached = (index: number) => {
    if (status === "SUCCESS") return true;
    if (activeIndex === -1) return index < phases.length - 1;
    return index <= activeIndex;
  };

  return (
    <ol className="flex flex-wrap items-center gap-1 text-xs">
      {phases.map((phase, index) => (
        <li className="flex items-center gap-1" key={phase}>
          {index > 0 ? <span className="h-px w-4 bg-zinc-700" /> : null}
          <span
            className={`rounded-full px-2 py-0.5 ${
              phase === status
                ? "bg-emerald-950/60 font-medium text-emerald-300 ring-1 ring-emerald-800"
                : reached(index)
                  ? "text-zinc-300"
                  : "text-zinc-600"
            }`}
          >
            {phase}
          </span>
        </li>
      ))}
      {failed || rolling ? (
        <li className="flex items-center gap-1">
          <span className="h-px w-4 bg-zinc-700" />
          <span
            className={`rounded-full px-2 py-0.5 font-medium ${
              failed
                ? "bg-red-950/60 text-red-300 ring-1 ring-red-900"
                : "bg-amber-950/60 text-amber-300 ring-1 ring-amber-900"
            }`}
          >
            {status}
          </span>
        </li>
      ) : null}
    </ol>
  );
}
