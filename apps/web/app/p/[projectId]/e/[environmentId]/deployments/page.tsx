import { DeploymentsScreen } from "@/features/deployments/DeploymentsScreen";

export default async function DeploymentsPage({
  params,
}: {
  params: Promise<{ projectId: string; environmentId: string }>;
}) {
  const { projectId, environmentId } = await params;
  return (
    <DeploymentsScreen environmentId={environmentId} projectId={projectId} />
  );
}
