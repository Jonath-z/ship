"use client";

import { Suspense } from "react";
import { EnvironmentSettingsScreen } from "@/features/environments/EnvironmentSettingsScreen";
import { EnvScopeGate, useEnvScope } from "@/features/shell/env-scope";

function EnvironmentSettingsContent() {
  const { projectId, environmentId } = useEnvScope();
  if (!projectId || !environmentId) return null;
  return (
    <EnvironmentSettingsScreen
      environmentId={environmentId}
      projectId={projectId}
    />
  );
}

export default function EnvironmentSettingsPage() {
  return (
    <Suspense>
      <EnvScopeGate>
        <EnvironmentSettingsContent />
      </EnvScopeGate>
    </Suspense>
  );
}
