import { LogsScreen } from "@/features/logs/LogsScreen";

export default async function LogsPage({
  params,
}: {
  params: Promise<{ projectId: string; environmentId: string }>;
}) {
  const { projectId, environmentId } = await params;
  return <LogsScreen environmentId={environmentId} projectId={projectId} />;
}
