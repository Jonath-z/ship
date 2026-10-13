"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect } from "react";
import { ServerDetailScreen } from "@/features/servers/ServerDetailScreen";

function ServerDetailContent() {
  const router = useRouter();
  const serverId = useSearchParams().get("server") ?? undefined;

  useEffect(() => {
    if (!serverId) router.replace("/servers");
  }, [serverId, router]);

  if (!serverId) return null;
  return <ServerDetailScreen serverId={serverId} />;
}

export default function ServerDetailPage() {
  return (
    <Suspense>
      <ServerDetailContent />
    </Suspense>
  );
}
