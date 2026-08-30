import { proxyShipRequest } from "@/lib/ship-api";

export const dynamic = "force-dynamic";

export function GET(request: Request) {
  return proxyShipRequest(request, "/setup");
}

export async function POST(request: Request) {
  return proxyShipRequest(request, "/setup");
}
