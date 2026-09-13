import { EnvironmentOverview } from "@/features/overview/EnvironmentOverview";

/** Environment overview; the control-plane system view stays at /dashboard. */
export default async function OverviewPage({
  params,
}: {
  params: Promise<{ projectId: string; environmentId: string }>;
}) {
  const { projectId, environmentId } = await params;
  return (
    <EnvironmentOverview environmentId={environmentId} projectId={projectId} />
  );
}
