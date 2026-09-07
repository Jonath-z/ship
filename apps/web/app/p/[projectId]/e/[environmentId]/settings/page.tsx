import { EnvironmentSettingsScreen } from "@/features/environments/EnvironmentSettingsScreen";

export default async function EnvironmentSettingsPage({
  params,
}: {
  params: Promise<{ projectId: string; environmentId: string }>;
}) {
  const { projectId, environmentId } = await params;
  return (
    <EnvironmentSettingsScreen
      environmentId={environmentId}
      projectId={projectId}
    />
  );
}
