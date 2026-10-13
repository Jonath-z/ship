"use client";

import { Suspense } from "react";
import { EnvironmentOverview } from "@/features/overview/EnvironmentOverview";
import { EnvScopeGate, useEnvScope } from "@/features/shell/env-scope";

/** Environment overview; the control-plane system view stays at /dashboard. */
function OverviewContent() {
  const { projectId, environmentId } = useEnvScope();
  if (!projectId || !environmentId) return null;
  return (
    <EnvironmentOverview environmentId={environmentId} projectId={projectId} />
  );
}

export default function OverviewPage() {
  return (
    <Suspense>
      <EnvScopeGate>
        <OverviewContent />
      </EnvScopeGate>
    </Suspense>
  );
}
