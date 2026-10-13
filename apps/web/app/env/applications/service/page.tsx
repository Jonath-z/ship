"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect } from "react";
import { envRoute } from "@/lib/routes";
import { ServiceDetailScreen } from "@/features/services/ServiceDetailScreen";
import { EnvScopeGate, useEnvScope } from "@/features/shell/env-scope";

function ServiceDetailContent() {
  const router = useRouter();
  const { projectId, environmentId } = useEnvScope();
  const serviceId = useSearchParams().get("service") ?? undefined;

  useEffect(() => {
    if (projectId && environmentId && !serviceId) {
      router.replace(envRoute(projectId, environmentId, "/applications"));
    }
  }, [projectId, environmentId, serviceId, router]);

  if (!projectId || !environmentId || !serviceId) return null;
  return (
    <ServiceDetailScreen
      environmentId={environmentId}
      projectId={projectId}
      serviceId={serviceId}
    />
  );
}

export default function ServiceDetailPage() {
  return (
    <Suspense>
      <EnvScopeGate>
        <ServiceDetailContent />
      </EnvScopeGate>
    </Suspense>
  );
}
