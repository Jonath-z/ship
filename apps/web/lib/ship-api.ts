const apiBaseURL = process.env.SHIP_API_URL ?? "http://localhost:8080";

/**
 * Forwards one explicitly declared Next.js API route to the private Gin API.
 * Only the headers Ship needs cross the internal service boundary.
 */
export async function proxyShipRequest(
  request: Request,
  path: string,
): Promise<Response> {
  const headers = new Headers({ Accept: "application/json" });
  copyHeader(request.headers, headers, "content-type");
  copyHeader(request.headers, headers, "cookie");
  copyHeader(request.headers, headers, "origin");
  copyHeader(request.headers, headers, "x-csrf-token");
  copyHeader(request.headers, headers, "x-request-id");

  if (process.env.SHIP_TRUST_FORWARDED_IP === "true") {
    const clientIP = firstForwardedIP(request.headers.get("x-forwarded-for"));
    if (clientIP) headers.set("X-Ship-Client-IP", clientIP);
  }

  const method = request.method.toUpperCase();
  const body =
    method === "GET" || method === "HEAD" ? undefined : await request.text();

  try {
    const upstream = await fetch(new URL(path, apiBaseURL), {
      method,
      headers,
      body,
      cache: "no-store",
      redirect: "manual",
    });
    const responseHeaders = new Headers({ "Cache-Control": "no-store" });
    for (const name of ["content-type", "retry-after", "x-request-id"]) {
      copyHeader(upstream.headers, responseHeaders, name);
    }
    const setCookie = upstream.headers.get("set-cookie");
    if (setCookie) responseHeaders.append("Set-Cookie", setCookie);
    return new Response(upstream.body, {
      status: upstream.status,
      headers: responseHeaders,
    });
  } catch {
    return Response.json(
      {
        error: {
          code: "api_unavailable",
          message: "The Ship API is unavailable",
          requestId: "",
        },
      },
      { status: 503, headers: { "Cache-Control": "no-store" } },
    );
  }
}

function copyHeader(source: Headers, target: Headers, name: string) {
  const value = source.get(name);
  if (value) target.set(name, value);
}

function firstForwardedIP(value: string | null): string | undefined {
  const candidate = value?.split(",", 1)[0]?.trim();
  if (!candidate) return undefined;
  return /^[0-9a-fA-F:.]+$/.test(candidate) ? candidate : undefined;
}
