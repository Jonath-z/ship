import { forwardToShipAPI } from "@/app/api/_lib/forward-to-ship-api";

export const dynamic = "force-dynamic";

export async function PATCH(
  request: Request,
  context: { params: Promise<{ id: string }> },
) {
  const { id } = await context.params;
  return forwardToShipAPI(request, `/users/${encodeURIComponent(id)}/role`);
}
