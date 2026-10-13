"use client";

import { Suspense } from "react";
import { EnvScopeGate, useEnvScope } from "@/features/shell/env-scope";
import { VariablesScreen } from "@/features/variables/VariablesScreen";

function VariablesContent() {
  const { projectId, environmentId } = useEnvScope();
  if (!projectId || !environmentId) return null;
  return (
    <VariablesScreen environmentId={environmentId} projectId={projectId} />
  );
}

export default function VariablesPage() {
  return (
    <Suspense>
      <EnvScopeGate>
        <VariablesContent />
      </EnvScopeGate>
    </Suspense>
  );
}
