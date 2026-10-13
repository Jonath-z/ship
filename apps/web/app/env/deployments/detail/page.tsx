"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect } from "react";
import { envRoute } from "@/lib/routes";
import { DeploymentDetailScreen } from "@/features/deployments/DeploymentDetailScreen";
import { EnvScopeGate, useEnvScope } from "@/features/shell/env-scope";

function DeploymentDetailContent() {
  const router = useRouter();
  const { projectId, environmentId } = useEnvScope();
  const deploymentId = useSearchParams().get("deployment") ?? undefined;

  useEffect(() => {
    if (projectId && environmentId && !deploymentId) {
      router.replace(envRoute(projectId, environmentId, "/deployments"));
    }
  }, [projectId, environmentId, deploymentId, router]);

  if (!projectId || !environmentId || !deploymentId) return null;
  return (
    <DeploymentDetailScreen
      deploymentId={deploymentId}
      environmentId={environmentId}
      projectId={projectId}
    />
  );
}

export default function DeploymentDetailPage() {
  return (
    <Suspense>
      <EnvScopeGate>
        <DeploymentDetailContent />
      </EnvScopeGate>
    </Suspense>
  );
}
