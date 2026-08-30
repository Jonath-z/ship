import { forwardToShipAPI } from "@/app/api/_lib/forward-to-ship-api";

export const dynamic = "force-dynamic";

export function POST(request: Request) {
  return forwardToShipAPI(request, "/auth/logout");
}
