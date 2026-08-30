import { createShipClient } from "@ship/api-client";
import type { components } from "@ship/types";

export const dynamic = "force-dynamic";

const ship = createShipClient({
  baseUrl: process.env.SHIP_API_URL ?? "http://localhost:8080",
});

export async function GET() {
  try {
    const { data, error, response } = await ship.GET("/setup");
    return Response.json(
      data ?? error ?? { error: "Unexpected response from the Ship API" },
      {
        status: response.status,
        headers: { "Cache-Control": "no-store" },
      },
    );
  } catch {
    return unavailable();
  }
}

export async function POST(request: Request) {
  try {
    const body =
      (await request.json()) as components["schemas"]["SetupOwnerRequest"];
    const { data, error, response } = await ship.POST("/setup", { body });
    return Response.json(
      data ?? error ?? { error: "Unexpected response from the Ship API" },
      {
        status: response.status,
        headers: { "Cache-Control": "no-store" },
      },
    );
  } catch {
    return unavailable();
  }
}

function unavailable() {
  return Response.json(
    { error: { message: "The Ship API is unavailable" } },
    { status: 503, headers: { "Cache-Control": "no-store" } },
  );
}
