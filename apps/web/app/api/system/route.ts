import { createShipClient } from "@ship/api-client";

export const dynamic = "force-dynamic";

const ship = createShipClient({
  baseUrl: process.env.SHIP_API_URL ?? "http://localhost:8080",
});

export async function GET() {
  try {
    const { data, error, response } = await ship.GET("/system");
    return Response.json(
      data ?? error ?? { error: "Unexpected response from the Ship API" },
      {
        status: response.status,
        headers: { "Cache-Control": "no-store" },
      },
    );
  } catch {
    return Response.json(
      { error: "The Ship API is unavailable" },
      { status: 503, headers: { "Cache-Control": "no-store" } },
    );
  }
}
