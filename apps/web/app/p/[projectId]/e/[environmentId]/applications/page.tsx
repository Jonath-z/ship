import { ServicesScreen } from "@/features/services/ServicesScreen";

export default async function ApplicationsPage({
  params,
}: {
  params: Promise<{ projectId: string; environmentId: string }>;
}) {
  const { projectId, environmentId } = await params;
  return (
    <ServicesScreen environmentId={environmentId} projectId={projectId} />
  );
}
