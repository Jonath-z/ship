"use client";

import { Suspense } from "react";
import { LogsScreen } from "@/features/logs/LogsScreen";
import { EnvScopeGate, useEnvScope } from "@/features/shell/env-scope";

function LogsContent() {
  const { projectId, environmentId } = useEnvScope();
  if (!projectId || !environmentId) return null;
  return <LogsScreen environmentId={environmentId} projectId={projectId} />;
}

export default function LogsPage() {
  return (
    <Suspense>
      <EnvScopeGate>
        <LogsContent />
      </EnvScopeGate>
    </Suspense>
  );
}
