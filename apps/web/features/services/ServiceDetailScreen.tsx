"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Badge } from "@/components/Badge";
import { Button } from "@/components/Button";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { ErrorNotice } from "@/components/ErrorNotice";
import { LoadingState, PageHeader } from "@/components/panels";
import { Tabs } from "@/components/Tabs";
import { api } from "@/lib/api";
import { DeployModal } from "@/features/deployments/DeployModal";
import { ConfigTab } from "@/features/services/ConfigTab";
import { DependenciesTab } from "@/features/services/DependenciesTab";
import { DomainsTab } from "@/features/services/DomainsTab";
import { ServiceDeploymentsTab } from "@/features/services/ServiceDeploymentsTab";
import { ServiceLogsTab } from "@/features/services/ServiceLogsTab";
import { ServiceOverviewTab } from "@/features/services/ServiceOverviewTab";
import { serviceSource } from "@/features/services/ServicesScreen";
import { VolumesTab } from "@/features/services/VolumesTab";

type TabId =
  | "overview"
  | "domains"
  | "volumes"
  | "dependencies"
  | "config"
  | "deployments"
  | "logs";

const tabs: Array<{ id: TabId; label: string }> = [
  { id: "overview", label: "Overview" },
  { id: "domains", label: "Domains" },
  { id: "volumes", label: "Volumes" },
  { id: "dependencies", label: "Dependencies" },
  { id: "config", label: "Config" },
  { id: "deployments", label: "Deployments" },
  { id: "logs", label: "Logs" },
];

/** Single application: settings, domains, volumes, dependencies, config, deployments, logs. */
export function ServiceDetailScreen({
  projectId,
  environmentId,
  serviceId,
}: {
  projectId: string;
  environmentId: string;
  serviceId: string;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<TabId>("overview");
  const [deploying, setDeploying] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const service = useQuery({
    queryKey: ["service", projectId, environmentId, serviceId],
    queryFn: () => api.services.get(projectId, environmentId, serviceId),
  });

  const remove = useMutation({
    mutationFn: () => api.services.remove(projectId, environmentId, serviceId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ["services", projectId, environmentId],
      });
      router.push(`/p/${projectId}/e/${environmentId}/applications`);
    },
  });

  if (service.isPending) {
    return (
      <main className="mx-auto max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
        <LoadingState label="Loading application…" />
      </main>
    );
  }
  if (service.error || !service.data) {
    return (
      <main className="mx-auto max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
        <ErrorNotice error={service.error ?? new Error("Service not found")} />
      </main>
    );
  }
  const data = service.data;

  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        actions={
          <>
            <Button onClick={() => setDeleting(true)} variant="danger">
              Delete
            </Button>
            <Button onClick={() => setDeploying(true)} variant="primary">
              Deploy
            </Button>
          </>
        }
        description={serviceSource(data)}
        eyebrow="Application"
        title={data.name}
      />
      <div className="mt-3 flex items-center gap-2">
        <Badge>{data.type}</Badge>
        <Badge tone="sky">{data.role}</Badge>
        {data.port ? <Badge>port {data.port}</Badge> : null}
      </div>

      <div className="mt-8">
        <Tabs active={tab} onChange={setTab} tabs={tabs} />
        <div className="mt-6">
          {tab === "overview" ? (
            <ServiceOverviewTab
              environmentId={environmentId}
              projectId={projectId}
              service={data}
            />
          ) : null}
          {tab === "domains" ? (
            <DomainsTab
              environmentId={environmentId}
              projectId={projectId}
              serviceId={serviceId}
            />
          ) : null}
          {tab === "volumes" ? (
            <VolumesTab
              environmentId={environmentId}
              owner={{ serviceId }}
              projectId={projectId}
            />
          ) : null}
          {tab === "dependencies" ? (
            <DependenciesTab
              environmentId={environmentId}
              projectId={projectId}
              serviceId={serviceId}
            />
          ) : null}
          {tab === "config" ? (
            <ConfigTab environmentId={environmentId} projectId={projectId} />
          ) : null}
          {tab === "deployments" ? (
            <ServiceDeploymentsTab
              environmentId={environmentId}
              onDeploy={() => setDeploying(true)}
              projectId={projectId}
              serviceId={serviceId}
            />
          ) : null}
          {tab === "logs" ? (
            <ServiceLogsTab
              environmentId={environmentId}
              projectId={projectId}
              service={data}
            />
          ) : null}
        </div>
      </div>

      {deploying ? (
        <DeployModal
          environmentId={environmentId}
          initialServiceId={serviceId}
          onClose={() => setDeploying(false)}
          projectId={projectId}
        />
      ) : null}
      {deleting ? (
        <ConfirmDialog
          busy={remove.isPending}
          confirmLabel="Delete application"
          danger
          error={remove.error}
          message={`Deletes ${data.name} along with its domains, volumes, and dependency edges.`}
          onCancel={() => setDeleting(false)}
          onConfirm={() => remove.mutate()}
          title={`Delete ${data.name}?`}
        />
      ) : null}
    </main>
  );
}
