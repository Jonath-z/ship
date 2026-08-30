import { proxyShipRequest } from "@/lib/ship-api";

export const dynamic = "force-dynamic";

export function GET(request: Request) {
  const url = new URL(request.url);
  return proxyShipRequest(request, `/audit${url.search}`);
}
