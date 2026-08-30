import { proxyShipRequest } from "@/lib/ship-api";

export const dynamic = "force-dynamic";

export function POST(request: Request) {
  return proxyShipRequest(request, "/auth/logout");
}
