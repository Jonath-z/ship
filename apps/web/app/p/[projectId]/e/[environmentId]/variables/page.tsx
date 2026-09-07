import { VariablesScreen } from "@/features/variables/VariablesScreen";

export default async function VariablesPage({
  params,
}: {
  params: Promise<{ projectId: string; environmentId: string }>;
}) {
  const { projectId, environmentId } = await params;
  return (
    <VariablesScreen environmentId={environmentId} projectId={projectId} />
  );
}
