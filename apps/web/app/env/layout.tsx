"use client";

import { Suspense, type ReactNode } from "react";
import { AppShell } from "@/features/shell/AppShell";
import { useEnvScope } from "@/features/shell/env-scope";

function EnvShell({ children }: { children: ReactNode }) {
  const { projectId, environmentId } = useEnvScope();
  return (
    <AppShell environmentId={environmentId} projectId={projectId}>
      {children}
    </AppShell>
  );
}

/** Environment shell; scope comes from ?project=X&env=Y (static export). */
export default function EnvLayout({ children }: { children: ReactNode }) {
  return (
    <Suspense>
      <EnvShell>{children}</EnvShell>
    </Suspense>
  );
}
