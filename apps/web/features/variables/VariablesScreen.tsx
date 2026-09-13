"use client";

import { PageHeader } from "@/components/panels";
import { ImportPanel } from "@/features/variables/ImportPanel";
import { RegistryPanel } from "@/features/variables/RegistryPanel";
import { SecretsPanel } from "@/features/variables/SecretsPanel";
import { VariablesPanel } from "@/features/variables/VariablesPanel";

/** Environment variables, secrets, and bulk .env import for the environment. */
export function VariablesScreen({
  projectId,
  environmentId,
}: {
  projectId: string;
  environmentId: string;
}) {
  return (
    <main className="mx-auto min-h-screen max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
      <PageHeader
        description="Plain variables are visible in the dashboard; secrets are stored encrypted and every reveal is audited."
        eyebrow="Environment"
        title="Variables & secrets"
      />
      <div className="mt-8 grid gap-6">
        <RegistryPanel environmentId={environmentId} projectId={projectId} />
        <VariablesPanel environmentId={environmentId} projectId={projectId} />
        <SecretsPanel environmentId={environmentId} projectId={projectId} />
        <ImportPanel environmentId={environmentId} projectId={projectId} />
      </div>
    </main>
  );
}
