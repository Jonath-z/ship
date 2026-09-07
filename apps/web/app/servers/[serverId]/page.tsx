import { ServerDetailScreen } from "@/features/servers/ServerDetailScreen";

export default async function ServerDetailPage({
  params,
}: {
  params: Promise<{ serverId: string }>;
}) {
  const { serverId } = await params;
  return <ServerDetailScreen serverId={serverId} />;
}
