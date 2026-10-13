"use client";

import { Suspense } from "react";
import { ServicesScreen } from "@/features/services/ServicesScreen";
import { EnvScopeGate, useEnvScope } from "@/features/shell/env-scope";

function ApplicationsContent() {
  const { projectId, environmentId } = useEnvScope();
  if (!projectId || !environmentId) return null;
  return <ServicesScreen environmentId={environmentId} projectId={projectId} />;
}

export default function ApplicationsPage() {
  return (
    <Suspense>
      <EnvScopeGate>
        <ApplicationsContent />
      </EnvScopeGate>
    </Suspense>
  );
}
