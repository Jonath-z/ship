import { forwardToShipAPI } from "@/app/api/_lib/forward-to-ship-api";

export const dynamic = "force-dynamic";

// The only API paths the web app proxies to the private Gin API. Anything not
// matched here returns 404 without touching the backend.
const allowedPaths: Array<{ method: string; pattern: RegExp }> = [
  { method: "GET", pattern: /^\/setup$/ },
  { method: "POST", pattern: /^\/setup$/ },
  { method: "GET", pattern: /^\/system$/ },
  { method: "GET", pattern: /^\/audit$/ },
  { method: "GET", pattern: /^\/users$/ },
  { method: "POST", pattern: /^\/users$/ },
  { method: "DELETE", pattern: /^\/users\/[^/]+$/ },
  { method: "PATCH", pattern: /^\/users\/[^/]+\/role$/ },
  { method: "POST", pattern: /^\/auth\/login$/ },
  { method: "POST", pattern: /^\/auth\/logout$/ },
  { method: "POST", pattern: /^\/auth\/password$/ },
  { method: "GET", pattern: /^\/auth\/session$/ },
  // Dashboard v1 resource families (SH E12/E14). Prefix allowlists covering
  // projects (plus nested environments, services, accessories, volumes,
  // domains, environment-variables, secrets, dependencies, server-groups,
  // configuration, deployments), servers (checks/prepare/containers), and
  // SSH keys. The Gin API enforces per-route RBAC behind these prefixes.
  ...["GET", "POST", "PATCH", "DELETE"].flatMap((method) => [
    { method, pattern: /^\/projects(\/.*)?$/ },
    { method, pattern: /^\/servers(\/.*)?$/ },
    { method, pattern: /^\/ssh-keys(\/.*)?$/ },
  ]),
];

async function proxy(
  request: Request,
  context: { params: Promise<{ path: string[] }> },
): Promise<Response> {
  const { path } = await context.params;
  const target = `/${path.map(encodeURIComponent).join("/")}`;
  const method = request.method.toUpperCase();

  const allowed = allowedPaths.some(
    (route) => route.method === method && route.pattern.test(target),
  );
  if (!allowed) {
    return Response.json(
      { error: { code: "not_found", message: "Not found", requestId: "" } },
      { status: 404, headers: { "Cache-Control": "no-store" } },
    );
  }

  const search = new URL(request.url).search;
  return forwardToShipAPI(request, `${target}${search}`);
}

export {
  proxy as GET,
  proxy as POST,
  proxy as PATCH,
  proxy as DELETE,
};
