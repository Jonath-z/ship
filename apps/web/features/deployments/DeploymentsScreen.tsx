"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useState } from "react";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { Select } from "@/components/form";
import {
  EmptyState,
  LoadingState,
  PageHeader,
  Panel,
} from "@/components/panels";
import { api, type DeploymentStatus } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import { useServices } from "@/lib/hooks";
import { DeployModal } from "@/features/deployments/DeployModal";
import { StatusBadge } from "@/features/deployments/StatusBadge";

const statusOptions: DeploymentStatus[] = [
  "QUEUED",
  "VALIDATING",
  "BUILDING",
  "PUSHING",
  "DEPLOYING",
  "VERIFYING",
  "SUCCESS",
  "FAILED",
  "ROLLING_BACK",
  "ROLLED_BACK",
];

/** Deployment history with filters, cursor paging, and the deploy flow. */
export function DeploymentsScreen({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  const [serviceFilter, setServiceFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [deploying, setDeploying] = useState(false);

  const services = useServices(projectId, environmentId);

  const deployments = useInfiniteQuery({
    queryKey: [
      "deployments",
      projectId,
      environmentId,
      serviceFilter,
      statusFilter,
    ],
    queryFn: ({ pageParam }) =>
      api.deployments.list(projectId, environmentId, {
        serviceId: serviceFilter || undefined,
        status: statusFilter || undefined,
        cursor: pageParam,
        limit: 25,
      }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.nextCursor,
  });

  const items = deployments.data?.pages.flatMap((page) => page.items) ?? [];
  const base = `/p/${projectId}/e/${environmentId}/deployments`;

  function serviceName(serviceId: string, fallback?: string): string {
    return (
      fallback ??
      services.data?.items.find((service) => service.id === serviceId)?.name ??
      serviceId
    );
  }

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        actions={
          <Button onClick={() => setDeploying(true)} variant="primary">
            Deploy
          </Button>
        }
        description="Every deployment in this environment, newest first."
        eyebrow="Environment"
        title="Deployments"
      />

      <div className="mt-8">
        <Panel
          actions={
            <>
              <Select
                aria-label="Filter by service"
                className="w-44"
                onChange={(event) => setServiceFilter(event.target.value)}
                value={serviceFilter}
              >
                <option value="">All services</option>
                {(services.data?.items ?? []).map((service) => (
                  <option key={service.id} value={service.id}>
                    {service.name}
                  </option>
                ))}
              </Select>
              <Select
                aria-label="Filter by status"
                className="w-40"
                onChange={(event) => setStatusFilter(event.target.value)}
                value={statusFilter}
              >
                <option value="">All statuses</option>
                {statusOptions.map((status) => (
                  <option key={status} value={status}>
                    {status}
                  </option>
                ))}
              </Select>
            </>
          }
        >
          {deployments.error ? <ErrorNotice error={deployments.error} /> : null}
          {deployments.isPending ? (
            <LoadingState label="Loading deployments…" />
          ) : null}
          {deployments.data && items.length === 0 ? (
            <EmptyState
              action={
                <Button onClick={() => setDeploying(true)} variant="primary">
                  Deploy something
                </Button>
              }
              description="No deployments match the current filters."
              title="No deployments"
            />
          ) : null}
          {items.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="text-zinc-500">
                  <tr>
                    <th className="pb-3 pr-4">Status</th>
                    <th className="pb-3 pr-4">Service</th>
                    <th className="pb-3 pr-4">Image</th>
                    <th className="pb-3">Created</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-800">
                  {items.map((deployment) => (
                    <tr key={deployment.id}>
                      <td className="py-3 pr-4">
                        <Link href={`${base}/${deployment.id}`}>
                          <StatusBadge status={deployment.status} />
                        </Link>
                      </td>
                      <td className="py-3 pr-4">
                        <Link
                          className="font-medium text-zinc-200 hover:text-emerald-300"
                          href={`${base}/${deployment.id}`}
                        >
                          {serviceName(
                            deployment.serviceId,
                            deployment.serviceName,
                          )}
                        </Link>
                      </td>
                      <td className="py-3 pr-4 font-mono text-xs text-zinc-400">
                        {deployment.image ?? "—"}
                      </td>
                      <td className="py-3 text-zinc-500">
                        {relativeTime(deployment.createdAt)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
          {deployments.hasNextPage ? (
            <div className="mt-4 flex justify-center">
              <Button
                disabled={deployments.isFetchingNextPage}
                onClick={() => void deployments.fetchNextPage()}
              >
                {deployments.isFetchingNextPage ? "Loading…" : "Load more"}
              </Button>
            </div>
          ) : null}
        </Panel>
      </div>

      {deploying ? (
        <DeployModal
          environmentId={environmentId}
          initialServiceId={serviceFilter || undefined}
          onClose={() => setDeploying(false)}
          projectId={projectId}
        />
      ) : null}
    </main>
  );
}
