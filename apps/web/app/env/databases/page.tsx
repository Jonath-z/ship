"use client";

import { Suspense } from "react";
import { DatabasesScreen } from "@/features/accessories/DatabasesScreen";
import { EnvScopeGate, useEnvScope } from "@/features/shell/env-scope";

function DatabasesContent() {
  const { projectId, environmentId } = useEnvScope();
  if (!projectId || !environmentId) return null;
  return (
    <DatabasesScreen environmentId={environmentId} projectId={projectId} />
  );
}

export default function DatabasesPage() {
  return (
    <Suspense>
      <EnvScopeGate>
        <DatabasesContent />
      </EnvScopeGate>
    </Suspense>
  );
}
