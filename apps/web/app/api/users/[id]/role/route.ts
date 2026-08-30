import { proxyShipRequest } from "@/lib/ship-api";

export const dynamic = "force-dynamic";

export async function PATCH(
  request: Request,
  context: { params: Promise<{ id: string }> },
) {
  const { id } = await context.params;
  return proxyShipRequest(request, `/users/${encodeURIComponent(id)}/role`);
}
