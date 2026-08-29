/**
 * Typed HTTP client for the Ship API (spec §47).
 *
 * This package is transport only. It contains no Kamal logic, no SSH logic, and
 * no infrastructure execution of any kind:
 *
 *   Next.js -> @ship/api-client -> HTTP -> Go API
 *
 * One resource module per domain area: projects.ts, environments.ts,
 * servers.ts, services.ts, deployments.ts, configuration.ts, logs.ts,
 * metrics.ts. Generated types live in src/generated/.
 */
import createClient from "openapi-fetch";
import type { paths } from "./generated/schema";

export interface ShipClientOptions {
  baseUrl: string;
  fetch?: typeof globalThis.fetch;
}

export function createShipClient(options: ShipClientOptions) {
  return createClient<paths>({
    baseUrl: options.baseUrl.replace(/\/$/, ""),
    fetch: options.fetch,
  });
}
