"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, type ReactNode } from "react";

/**
 * Environment scope for the static export: /env/* pages carry the selection
 * as ?project=X&env=Y query parameters instead of dynamic path segments.
 */
export function useEnvScope(): {
  projectId?: string;
  environmentId?: string;
} {
  const searchParams = useSearchParams();
  return {
    projectId: searchParams.get("project") ?? undefined,
    environmentId: searchParams.get("env") ?? undefined,
  };
}

/**
 * Gates environment-scoped pages: without a complete ?project=X&env=Y pair
 * the page cannot render, so redirect to the project picker.
 * Must be rendered inside a Suspense boundary (useSearchParams).
 */
export function EnvScopeGate({ children }: { children: ReactNode }) {
  const router = useRouter();
  const { projectId, environmentId } = useEnvScope();
  const missingScope = !projectId || !environmentId;

  useEffect(() => {
    if (missingScope) router.replace("/projects");
  }, [missingScope, router]);

  if (missingScope) return null;
  return <>{children}</>;
}
