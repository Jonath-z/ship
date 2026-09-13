"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { Button } from "@/components/Button";
import { ErrorNotice } from "@/components/ErrorNotice";
import { EmptyState, LoadingState, Panel } from "@/components/panels";
import { api } from "@/lib/api";
import { DeploymentTable } from "@/features/deployments/DeploymentTable";

/** Deployment history filtered to a single application. */
export function ServiceDeploymentsTab({
  projectId,
  environmentId,
  serviceId,
  onDeploy,
}: {
  projectId: string;
  environmentId: string;
  serviceId: string;
  onDeploy: () => void;
}) {
  const deployments = useInfiniteQuery({
    queryKey: ["deployments", projectId, environmentId, serviceId],
    queryFn: ({ pageParam }) =>
      api.deployments.list(projectId, environmentId, {
        serviceId,
        cursor: pageParam,
        limit: 25,
      }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.nextCursor,
  });

  const items = deployments.data?.pages.flatMap((page) => page.items) ?? [];

  return (
    <Panel
      description="Deployments of this application, newest first."
      title="Deployments"
    >
      {deployments.error ? <ErrorNotice error={deployments.error} /> : null}
      {deployments.isPending ? (
        <LoadingState label="Loading deployments…" />
      ) : null}
      {deployments.data && items.length === 0 ? (
        <EmptyState
          action={
            <Button onClick={onDeploy} variant="primary">
              Deploy this application
            </Button>
          }
          description="This application has not been deployed yet."
          title="No deployments"
        />
      ) : null}
      {items.length > 0 ? (
        <DeploymentTable
          deployments={items}
          detailBase={`/p/${projectId}/e/${environmentId}/deployments`}
          showService={false}
        />
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
  );
}
