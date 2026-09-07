import { DeploymentDetailScreen } from "@/features/deployments/DeploymentDetailScreen";

export default async function DeploymentDetailPage({
  params,
}: {
  params: Promise<{
    projectId: string;
    environmentId: string;
    deploymentId: string;
  }>;
}) {
  const { projectId, environmentId, deploymentId } = await params;
  return (
    <DeploymentDetailScreen
      deploymentId={deploymentId}
      environmentId={environmentId}
      projectId={projectId}
    />
  );
}
