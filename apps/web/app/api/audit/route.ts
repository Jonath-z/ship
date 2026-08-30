import { forwardToShipAPI } from "@/app/api/_lib/forward-to-ship-api";

export const dynamic = "force-dynamic";

export function GET(request: Request) {
  const url = new URL(request.url);
  return forwardToShipAPI(request, `/audit${url.search}`);
}
