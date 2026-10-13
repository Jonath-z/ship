"use client";

import { Suspense } from "react";
import { DeploymentsScreen } from "@/features/deployments/DeploymentsScreen";
import { EnvScopeGate, useEnvScope } from "@/features/shell/env-scope";

function DeploymentsContent() {
  const { projectId, environmentId } = useEnvScope();
  if (!projectId || !environmentId) return null;
  return (
    <DeploymentsScreen environmentId={environmentId} projectId={projectId} />
  );
}

export default function DeploymentsPage() {
  return (
    <Suspense>
      <EnvScopeGate>
        <DeploymentsContent />
      </EnvScopeGate>
    </Suspense>
  );
}
