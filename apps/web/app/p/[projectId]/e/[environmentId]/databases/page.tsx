import { DatabasesScreen } from "@/features/accessories/DatabasesScreen";

export default async function DatabasesPage({
  params,
}: {
  params: Promise<{ projectId: string; environmentId: string }>;
}) {
  const { projectId, environmentId } = await params;
  return (
    <DatabasesScreen environmentId={environmentId} projectId={projectId} />
  );
}
