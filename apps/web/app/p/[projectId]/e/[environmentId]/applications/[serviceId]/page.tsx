import { ServiceDetailScreen } from "@/features/services/ServiceDetailScreen";

export default async function ServiceDetailPage({
  params,
}: {
  params: Promise<{
    projectId: string;
    environmentId: string;
    serviceId: string;
  }>;
}) {
  const { projectId, environmentId, serviceId } = await params;
  return (
    <ServiceDetailScreen
      environmentId={environmentId}
      projectId={projectId}
      serviceId={serviceId}
    />
  );
}
