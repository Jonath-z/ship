import type { ReactNode } from "react";
import { AppShell } from "@/features/shell/AppShell";

export default async function EnvironmentLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ projectId: string; environmentId: string }>;
}) {
  const { projectId, environmentId } = await params;
  return (
    <AppShell environmentId={environmentId} projectId={projectId}>
      {children}
    </AppShell>
  );
}
