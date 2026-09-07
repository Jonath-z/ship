/**
 * Typed fetch wrapper and hand-written interfaces for the Ship dashboard.
 *
 * The generated OpenAPI client (@ship/api-client) only covers
 * auth/users/system/audit today. The types below mirror the Go response
 * structs (server/internal/&#42;/routes.go and friends) and are pending OpenAPI
 * spec coverage — once the spec covers these resources, replace them with
 * generated types.
 *
 * Every call goes through the same-origin Next.js proxy at /api. Mutations
 * carry the X-CSRF-Token issued with the session (GET /auth/session).
 */

export interface ApiFieldError {
  field: string;
  code: string;
  message: string;
}

export interface ApiErrorEnvelope {
  error: {
    code: string;
    message: string;
    requestId: string;
    details?: ApiFieldError[];
  };
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string;
  readonly details: ApiFieldError[];

  constructor(
    status: number,
    code: string,
    message: string,
    requestId: string,
    details?: ApiFieldError[],
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
    this.details = details ?? [];
  }

  /** First field-level message for a given form field, if any. */
  fieldError(field: string): string | undefined {
    return this.details.find((detail) => detail.field === field)?.message;
  }
}

/** Field-level message from an unknown error, for inline form errors. */
export function fieldErrorOf(error: unknown, field: string): string | undefined {
  return error instanceof ApiError ? error.fieldError(field) : undefined;
}

// -- Session / CSRF -----------------------------------------------------------

export interface SessionUser {
  id: string;
  email: string;
  role: "owner" | "admin" | "deployer" | "viewer";
}

export interface Session {
  user: SessionUser;
  csrfToken: string;
  expiresAt: string;
}

let csrfToken: string | undefined;

export function primeCsrfToken(token: string) {
  csrfToken = token;
}

async function fetchSession(): Promise<Session> {
  const response = await fetch("/api/auth/session", {
    credentials: "include",
    cache: "no-store",
  });
  if (response.status === 401) {
    redirectToLogin();
    throw new ApiError(401, "unauthenticated", "Session expired", "");
  }
  if (!response.ok) {
    throw new ApiError(
      response.status,
      "session_unavailable",
      "Session is unavailable",
      "",
    );
  }
  const session = (await response.json()) as Session;
  csrfToken = session.csrfToken;
  return session;
}

function redirectToLogin() {
  if (typeof window !== "undefined") {
    window.location.assign("/login?expired=1");
  }
}

// -- Fetch wrapper ------------------------------------------------------------

type Query = Record<string, string | number | boolean | undefined>;

interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
  query?: Query;
}

function buildUrl(path: string, query?: Query): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined && value !== "") search.set(key, String(value));
  }
  const encoded = search.toString();
  return `/api${path}${encoded ? `?${encoded}` : ""}`;
}

async function parseError(response: Response): Promise<ApiError> {
  let envelope: ApiErrorEnvelope | undefined;
  try {
    envelope = (await response.json()) as ApiErrorEnvelope;
  } catch {
    // Non-JSON error body; fall through to a generic error.
  }
  return new ApiError(
    response.status,
    envelope?.error.code ?? "request_failed",
    envelope?.error.message ?? `Request failed (${response.status})`,
    envelope?.error.requestId ?? "",
    envelope?.error.details,
  );
}

export async function apiFetch<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const method = options.method ?? "GET";
  const mutation = method !== "GET";
  const headers: Record<string, string> = { Accept: "application/json" };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (mutation) {
    if (!csrfToken) await fetchSession();
    if (csrfToken) headers["X-CSRF-Token"] = csrfToken;
  }

  const execute = () =>
    fetch(buildUrl(path, options.query), {
      method,
      headers,
      credentials: "include",
      cache: "no-store",
      body:
        options.body !== undefined ? JSON.stringify(options.body) : undefined,
    });

  let response = await execute();
  if (mutation && response.status === 403) {
    // The CSRF token may be stale (session rotation); refresh once and retry.
    await fetchSession();
    if (csrfToken) headers["X-CSRF-Token"] = csrfToken;
    response = await execute();
  }
  if (response.status === 401) {
    redirectToLogin();
    throw await parseError(response);
  }
  if (!response.ok) throw await parseError(response);
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

// -- Shared shapes ------------------------------------------------------------

export interface Page<T> {
  items: T[];
  nextCursor?: string;
}

// -- Projects -----------------------------------------------------------------

export interface Project {
  id: string;
  name: string;
  slug: string;
  createdAt: string;
  updatedAt: string;
}

export interface ProjectDeletionImpact {
  projectId: string;
  slug: string;
  environments: number;
  serverGroups: number;
  services: number;
  accessories: number;
  volumes: number;
  domains: number;
  environmentVariables: number;
  secrets: number;
  dependencies: number;
  configurations: number;
  deployments: number;
  backups: number;
}

// -- Environments -------------------------------------------------------------

export interface Environment {
  id: string;
  projectId: string;
  name: string;
  slug: string;
  createdAt: string;
  updatedAt: string;
}

export interface EnvironmentDeletionImpact {
  environmentId: string;
  slug: string;
  serverGroups: number;
  services: number;
  accessories: number;
  volumes: number;
  domains: number;
  environmentVariables: number;
  secrets: number;
  dependencies: number;
  configurations: number;
  deployments: number;
  backups: number;
}

// -- Services -----------------------------------------------------------------

export interface Service {
  id: string;
  environmentId: string;
  name: string;
  type: string;
  repository?: string;
  branch?: string;
  image?: string;
  port?: number;
  command?: string;
  role: string;
  serverGroupId: string;
  createdAt: string;
  updatedAt: string;
}

export interface ServiceInput {
  name?: string;
  type?: string;
  repository?: string;
  branch?: string;
  image?: string;
  port?: number | null;
  command?: string;
  role?: string;
}

// -- Accessories --------------------------------------------------------------

export interface SuggestedVolume {
  name: string;
  source: string;
  destination: string;
}

export interface Accessory {
  id: string;
  environmentId: string;
  name: string;
  type: string;
  image: string;
  serverId?: string | null;
  serverGroupId?: string | null;
  role?: string;
  port?: number;
  suggestedVolume?: SuggestedVolume;
  suggestedConnectionSecret?: string;
  connectionSecret?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AccessoryInput {
  name?: string;
  type?: string;
  image?: string;
  serverId?: string | null;
  serverGroupId?: string | null;
  port?: number | null;
}

// -- Volumes ------------------------------------------------------------------

export interface Volume {
  id: string;
  environmentId: string;
  serviceId?: string | null;
  accessoryId?: string | null;
  name: string;
  source: string;
  destination: string;
  createdAt: string;
  updatedAt: string;
}

// -- Domains ------------------------------------------------------------------

export interface Domain {
  id: string;
  environmentId: string;
  serviceId: string;
  hostname: string;
  sslEnabled: boolean;
  createdAt: string;
  updatedAt: string;
}

// -- Dependencies -------------------------------------------------------------

export interface Dependency {
  id: string;
  environmentId: string;
  sourceServiceId: string;
  targetServiceId?: string | null;
  targetAccessoryId?: string | null;
  type: string;
  createdAt: string;
}

// -- Environment variables & secrets ------------------------------------------

export interface EnvironmentVariable {
  id: string;
  environmentId: string;
  serviceId?: string | null;
  name: string;
  value: string;
  createdAt: string;
  updatedAt: string;
}

export interface Secret {
  id: string;
  environmentId: string;
  serviceId?: string | null;
  name: string;
  hasValue: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface ImportResult {
  created: number;
  updated: number;
}

// -- SSH keys -----------------------------------------------------------------

export interface SshKey {
  id: string;
  name: string;
  publicKey: string;
  createdAt: string;
}

// -- Servers ------------------------------------------------------------------

export type ServerStatus = "pending" | "connected" | "degraded" | "disconnected";

export interface ServerResources {
  cpuCores?: number;
  memoryBytes?: number;
  diskBytes?: number;
}

export interface Server {
  id: string;
  name: string;
  hostname?: string;
  ipAddress?: string;
  sshUser: string;
  sshPort: number;
  sshKeyId?: string | null;
  architecture?: string;
  os?: string;
  status: ServerStatus;
  resources: ServerResources;
  hostKeySaved: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface ServerInput {
  name?: string;
  hostname?: string;
  ipAddress?: string;
  sshUser?: string;
  sshPort?: number;
  sshKeyId?: string;
}

export interface ServerCheck {
  name: string;
  ok: boolean;
  detail?: string;
}

export interface CheckReport {
  serverId: string;
  status: ServerStatus;
  checks: ServerCheck[];
  ranAt: string;
}

export interface PrepareReport {
  log: string[];
  report: CheckReport;
}

export interface Container {
  name: string;
  image: string;
  status: string;
  state: string;
}

export interface ServerGroup {
  id: string;
  name: string;
  members: Server[];
}

// -- Configuration ------------------------------------------------------------

export interface ConfigDomain {
  hostname: string;
  sslEnabled: boolean;
}

export interface ConfigVolume {
  name: string;
  source: string;
  destination: string;
}

export interface ServiceSpec {
  type: string;
  repository?: string;
  branch?: string;
  image?: string;
  port?: number;
  command?: string;
  role?: string;
  hosts?: string[];
  domains?: ConfigDomain[];
  volumes?: ConfigVolume[];
  env?: Record<string, string>;
  secretRefs?: string[];
  dependsOn?: string[];
}

export interface AccessorySpec {
  type: string;
  image: string;
  role?: string;
  hosts?: string[];
  port?: number;
  volumes?: ConfigVolume[];
}

export interface DesiredState {
  environmentId: string;
  services: Record<string, ServiceSpec>;
  accessories: Record<string, AccessorySpec>;
  roles: Record<string, string[]>;
  env?: Record<string, string>;
  secretRefs?: string[];
  ssh: { user?: string; port?: number };
}

export interface Violation {
  code: string;
  severity: "block" | "warn";
  message: string;
  entityType: string;
  entityName: string;
}

export interface ConfigurationPreview {
  environmentId: string;
  state: DesiredState;
  validation: Violation[];
  rendered: Record<string, string>;
}

export interface ConfigurationVersion {
  version: number;
  state: DesiredState;
  actorUserId?: string;
  changeSummary?: string;
  createdAt: string;
}

export interface FieldDiff {
  field: string;
  from?: string;
  to?: string;
}

export interface EntityDiff {
  kind: string;
  name: string;
  change: "added" | "removed" | "changed" | "unchanged";
  fields?: FieldDiff[];
}

export interface ConfigurationDiff {
  from: number;
  to: number;
  entities: EntityDiff[];
}

// -- Deployments --------------------------------------------------------------

export type DeploymentStatus =
  | "QUEUED"
  | "VALIDATING"
  | "BUILDING"
  | "PUSHING"
  | "DEPLOYING"
  | "VERIFYING"
  | "SUCCESS"
  | "FAILED"
  | "ROLLING_BACK"
  | "ROLLED_BACK";

export const terminalDeploymentStatuses: DeploymentStatus[] = [
  "SUCCESS",
  "FAILED",
  "ROLLED_BACK",
];

export interface Deployment {
  id: string;
  environmentId: string;
  serviceId: string;
  serviceName?: string;
  configurationVersionId?: string;
  sourceDeploymentId?: string | null;
  image?: string;
  status: DeploymentStatus;
  startedAt?: string;
  finishedAt?: string;
  createdAt: string;
}

export interface DeploymentLogEntry {
  sequence: number;
  stream: string;
  message: string;
  at: string;
}

export interface DeploymentStreamEvent {
  sequence: number;
  type: "log" | "status";
  stream?: string;
  message?: string;
  status?: DeploymentStatus;
}

// -- API surface --------------------------------------------------------------

const envBase = (projectId: string, environmentId: string) =>
  `/projects/${projectId}/environments/${environmentId}`;

export const api = {
  session: {
    get: () => fetchSession(),
    logout: () => apiFetch<void>("/auth/logout", { method: "POST" }),
  },

  projects: {
    list: (query?: Query) => apiFetch<Page<Project>>("/projects", { query }),
    get: (id: string) => apiFetch<Project>(`/projects/${id}`),
    create: (body: { name: string; slug: string }) =>
      apiFetch<Project>("/projects", { method: "POST", body }),
    update: (id: string, body: { name?: string; slug?: string }) =>
      apiFetch<Project>(`/projects/${id}`, { method: "PATCH", body }),
    deletionImpact: (id: string) =>
      apiFetch<ProjectDeletionImpact>(`/projects/${id}/deletion-impact`),
    remove: (id: string, confirmSlug: string) =>
      apiFetch<void>(`/projects/${id}`, {
        method: "DELETE",
        body: { confirmSlug },
      }),
  },

  environments: {
    list: (projectId: string, query?: Query) =>
      apiFetch<Page<Environment>>(`/projects/${projectId}/environments`, {
        query,
      }),
    get: (projectId: string, environmentId: string) =>
      apiFetch<Environment>(`/projects/${projectId}/environments/${environmentId}`),
    create: (projectId: string, body: { name: string; slug: string }) =>
      apiFetch<Environment>(`/projects/${projectId}/environments`, {
        method: "POST",
        body,
      }),
    update: (
      projectId: string,
      environmentId: string,
      body: { name?: string; slug?: string },
    ) =>
      apiFetch<Environment>(
        `/projects/${projectId}/environments/${environmentId}`,
        { method: "PATCH", body },
      ),
    clone: (
      projectId: string,
      environmentId: string,
      body: { name: string; slug: string; includeSecrets: boolean },
    ) =>
      apiFetch<Environment>(
        `/projects/${projectId}/environments/${environmentId}/clone`,
        { method: "POST", body },
      ),
    deletionImpact: (projectId: string, environmentId: string) =>
      apiFetch<EnvironmentDeletionImpact>(
        `/projects/${projectId}/environments/${environmentId}/deletion-impact`,
      ),
    remove: (projectId: string, environmentId: string, confirmSlug: string) =>
      apiFetch<void>(`/projects/${projectId}/environments/${environmentId}`, {
        method: "DELETE",
        body: { confirmSlug },
      }),
  },

  services: {
    list: (projectId: string, environmentId: string) =>
      apiFetch<Page<Service>>(`${envBase(projectId, environmentId)}/services`, {
        query: { limit: 100 },
      }),
    get: (projectId: string, environmentId: string, serviceId: string) =>
      apiFetch<Service>(
        `${envBase(projectId, environmentId)}/services/${serviceId}`,
      ),
    create: (projectId: string, environmentId: string, body: ServiceInput) =>
      apiFetch<Service>(`${envBase(projectId, environmentId)}/services`, {
        method: "POST",
        body,
      }),
    update: (
      projectId: string,
      environmentId: string,
      serviceId: string,
      body: ServiceInput,
    ) =>
      apiFetch<Service>(
        `${envBase(projectId, environmentId)}/services/${serviceId}`,
        { method: "PATCH", body },
      ),
    remove: (projectId: string, environmentId: string, serviceId: string) =>
      apiFetch<void>(
        `${envBase(projectId, environmentId)}/services/${serviceId}`,
        { method: "DELETE" },
      ),
  },

  accessories: {
    list: (projectId: string, environmentId: string) =>
      apiFetch<Page<Accessory>>(
        `${envBase(projectId, environmentId)}/accessories`,
        { query: { limit: 100 } },
      ),
    create: (projectId: string, environmentId: string, body: AccessoryInput) =>
      apiFetch<Accessory>(`${envBase(projectId, environmentId)}/accessories`, {
        method: "POST",
        body,
      }),
    update: (
      projectId: string,
      environmentId: string,
      accessoryId: string,
      body: AccessoryInput,
    ) =>
      apiFetch<Accessory>(
        `${envBase(projectId, environmentId)}/accessories/${accessoryId}`,
        { method: "PATCH", body },
      ),
    remove: (projectId: string, environmentId: string, accessoryId: string) =>
      apiFetch<void>(
        `${envBase(projectId, environmentId)}/accessories/${accessoryId}`,
        { method: "DELETE" },
      ),
  },

  volumes: {
    list: (projectId: string, environmentId: string) =>
      apiFetch<Page<Volume>>(`${envBase(projectId, environmentId)}/volumes`, {
        query: { limit: 100 },
      }),
    create: (
      projectId: string,
      environmentId: string,
      body: {
        name: string;
        source: string;
        destination: string;
        serviceId?: string | null;
        accessoryId?: string | null;
      },
    ) =>
      apiFetch<Volume>(`${envBase(projectId, environmentId)}/volumes`, {
        method: "POST",
        body,
      }),
    update: (
      projectId: string,
      environmentId: string,
      volumeId: string,
      body: { name?: string; source?: string; destination?: string },
    ) =>
      apiFetch<Volume>(
        `${envBase(projectId, environmentId)}/volumes/${volumeId}`,
        { method: "PATCH", body },
      ),
    remove: (projectId: string, environmentId: string, volumeId: string) =>
      apiFetch<void>(
        `${envBase(projectId, environmentId)}/volumes/${volumeId}`,
        { method: "DELETE" },
      ),
  },

  domains: {
    list: (projectId: string, environmentId: string) =>
      apiFetch<Page<Domain>>(`${envBase(projectId, environmentId)}/domains`, {
        query: { limit: 100 },
      }),
    create: (
      projectId: string,
      environmentId: string,
      body: { serviceId: string; hostname: string; sslEnabled: boolean },
    ) =>
      apiFetch<Domain>(`${envBase(projectId, environmentId)}/domains`, {
        method: "POST",
        body,
      }),
    update: (
      projectId: string,
      environmentId: string,
      domainId: string,
      body: { hostname?: string; sslEnabled?: boolean },
    ) =>
      apiFetch<Domain>(
        `${envBase(projectId, environmentId)}/domains/${domainId}`,
        { method: "PATCH", body },
      ),
    remove: (projectId: string, environmentId: string, domainId: string) =>
      apiFetch<void>(
        `${envBase(projectId, environmentId)}/domains/${domainId}`,
        { method: "DELETE" },
      ),
  },

  dependencies: {
    list: (projectId: string, environmentId: string) =>
      apiFetch<Page<Dependency>>(
        `${envBase(projectId, environmentId)}/dependencies`,
        { query: { limit: 100 } },
      ),
    create: (
      projectId: string,
      environmentId: string,
      body: {
        sourceServiceId: string;
        targetServiceId?: string | null;
        targetAccessoryId?: string | null;
        type: string;
      },
    ) =>
      apiFetch<Dependency>(
        `${envBase(projectId, environmentId)}/dependencies`,
        { method: "POST", body },
      ),
    remove: (projectId: string, environmentId: string, dependencyId: string) =>
      apiFetch<void>(
        `${envBase(projectId, environmentId)}/dependencies/${dependencyId}`,
        { method: "DELETE" },
      ),
  },

  variables: {
    list: (projectId: string, environmentId: string) =>
      apiFetch<Page<EnvironmentVariable>>(
        `${envBase(projectId, environmentId)}/environment-variables`,
        { query: { limit: 100 } },
      ),
    create: (
      projectId: string,
      environmentId: string,
      body: { name: string; value: string; serviceId?: string | null },
    ) =>
      apiFetch<EnvironmentVariable>(
        `${envBase(projectId, environmentId)}/environment-variables`,
        { method: "POST", body },
      ),
    update: (
      projectId: string,
      environmentId: string,
      variableId: string,
      body: { name?: string; value?: string },
    ) =>
      apiFetch<EnvironmentVariable>(
        `${envBase(projectId, environmentId)}/environment-variables/${variableId}`,
        { method: "PATCH", body },
      ),
    remove: (projectId: string, environmentId: string, variableId: string) =>
      apiFetch<void>(
        `${envBase(projectId, environmentId)}/environment-variables/${variableId}`,
        { method: "DELETE" },
      ),
    import: (
      projectId: string,
      environmentId: string,
      body: { content: string; serviceId?: string | null },
    ) =>
      apiFetch<ImportResult>(
        `${envBase(projectId, environmentId)}/environment-variables/import`,
        { method: "POST", body },
      ),
  },

  secrets: {
    list: (projectId: string, environmentId: string) =>
      apiFetch<Page<Secret>>(`${envBase(projectId, environmentId)}/secrets`, {
        query: { limit: 100 },
      }),
    create: (
      projectId: string,
      environmentId: string,
      body: { name: string; value: string; serviceId?: string | null },
    ) =>
      apiFetch<Secret>(`${envBase(projectId, environmentId)}/secrets`, {
        method: "POST",
        body,
      }),
    update: (
      projectId: string,
      environmentId: string,
      secretId: string,
      body: { name?: string; value?: string },
    ) =>
      apiFetch<Secret>(
        `${envBase(projectId, environmentId)}/secrets/${secretId}`,
        { method: "PATCH", body },
      ),
    remove: (projectId: string, environmentId: string, secretId: string) =>
      apiFetch<void>(
        `${envBase(projectId, environmentId)}/secrets/${secretId}`,
        { method: "DELETE" },
      ),
    reveal: (projectId: string, environmentId: string, secretId: string) =>
      apiFetch<{ value: string }>(
        `${envBase(projectId, environmentId)}/secrets/${secretId}/reveal`,
        { method: "POST" },
      ),
    import: (
      projectId: string,
      environmentId: string,
      body: { content: string; serviceId?: string | null },
    ) =>
      apiFetch<ImportResult>(
        `${envBase(projectId, environmentId)}/secrets/import`,
        { method: "POST", body },
      ),
  },

  sshKeys: {
    list: () => apiFetch<Page<SshKey>>("/ssh-keys"),
    create: (body: { name: string; privateKey?: string }) =>
      apiFetch<SshKey>("/ssh-keys", { method: "POST", body }),
    remove: (keyId: string) =>
      apiFetch<void>(`/ssh-keys/${keyId}`, { method: "DELETE" }),
  },

  servers: {
    list: () => apiFetch<Page<Server>>("/servers"),
    get: (serverId: string) => apiFetch<Server>(`/servers/${serverId}`),
    create: (body: {
      name: string;
      hostname?: string;
      ipAddress?: string;
      sshUser: string;
      sshPort: number;
      sshKeyId: string;
    }) => apiFetch<Server>("/servers", { method: "POST", body }),
    update: (serverId: string, body: ServerInput) =>
      apiFetch<Server>(`/servers/${serverId}`, { method: "PATCH", body }),
    remove: (serverId: string) =>
      apiFetch<void>(`/servers/${serverId}`, { method: "DELETE" }),
    runChecks: (serverId: string) =>
      apiFetch<CheckReport>(`/servers/${serverId}/checks`, { method: "POST" }),
    prepare: (serverId: string) =>
      apiFetch<PrepareReport>(`/servers/${serverId}/prepare`, {
        method: "POST",
      }),
    containers: (serverId: string) =>
      apiFetch<Page<Container>>(`/servers/${serverId}/containers`),
    containerLogs: (serverId: string, containerName: string, lines = 200) =>
      apiFetch<{ log: string }>(
        `/servers/${serverId}/containers/${encodeURIComponent(containerName)}/logs`,
        { query: { lines } },
      ),
  },

  serverGroups: {
    list: (projectId: string, environmentId: string) =>
      apiFetch<Page<ServerGroup>>(
        `${envBase(projectId, environmentId)}/server-groups`,
      ),
    addMember: (
      projectId: string,
      environmentId: string,
      groupId: string,
      serverId: string,
    ) =>
      apiFetch<void>(
        `${envBase(projectId, environmentId)}/server-groups/${groupId}/servers`,
        { method: "POST", body: { serverId } },
      ),
    removeMember: (
      projectId: string,
      environmentId: string,
      groupId: string,
      serverId: string,
    ) =>
      apiFetch<void>(
        `${envBase(projectId, environmentId)}/server-groups/${groupId}/servers/${serverId}`,
        { method: "DELETE" },
      ),
  },

  configuration: {
    preview: (projectId: string, environmentId: string) =>
      apiFetch<ConfigurationPreview>(
        `${envBase(projectId, environmentId)}/configuration/preview`,
      ),
    versions: (projectId: string, environmentId: string, limit = 20) =>
      apiFetch<Page<ConfigurationVersion>>(
        `${envBase(projectId, environmentId)}/configuration/versions`,
        { query: { limit } },
      ),
    version: (projectId: string, environmentId: string, version: number) =>
      apiFetch<ConfigurationVersion>(
        `${envBase(projectId, environmentId)}/configuration/versions/${version}`,
      ),
    snapshot: (projectId: string, environmentId: string, changeSummary: string) =>
      apiFetch<ConfigurationVersion>(
        `${envBase(projectId, environmentId)}/configuration/versions`,
        { method: "POST", body: { changeSummary } },
      ),
    diff: (
      projectId: string,
      environmentId: string,
      version: number,
      against?: number,
    ) =>
      apiFetch<ConfigurationDiff>(
        `${envBase(projectId, environmentId)}/configuration/versions/${version}/diff`,
        { query: { against } },
      ),
    pendingDiff: (projectId: string, environmentId: string) =>
      apiFetch<ConfigurationDiff>(
        `${envBase(projectId, environmentId)}/configuration/pending-diff`,
      ),
  },

  deployments: {
    list: (
      projectId: string,
      environmentId: string,
      query?: {
        serviceId?: string;
        status?: string;
        cursor?: string;
        limit?: number;
      },
    ) =>
      apiFetch<Page<Deployment>>(
        `${envBase(projectId, environmentId)}/deployments`,
        { query },
      ),
    get: (projectId: string, environmentId: string, deploymentId: string) =>
      apiFetch<Deployment>(
        `${envBase(projectId, environmentId)}/deployments/${deploymentId}`,
      ),
    create: (projectId: string, environmentId: string, serviceId: string) =>
      apiFetch<Deployment>(`${envBase(projectId, environmentId)}/deployments`, {
        method: "POST",
        body: { serviceId },
      }),
    rollback: (projectId: string, environmentId: string, deploymentId: string) =>
      apiFetch<Deployment>(
        `${envBase(projectId, environmentId)}/deployments/${deploymentId}/rollback`,
        { method: "POST" },
      ),
    logs: (
      projectId: string,
      environmentId: string,
      deploymentId: string,
      after = 0,
    ) =>
      apiFetch<{ items: DeploymentLogEntry[] }>(
        `${envBase(projectId, environmentId)}/deployments/${deploymentId}/logs`,
        { query: { after } },
      ),
    streamUrl: (
      projectId: string,
      environmentId: string,
      deploymentId: string,
      after?: number,
    ) =>
      buildUrl(
        `${envBase(projectId, environmentId)}/deployments/${deploymentId}/stream`,
        after !== undefined ? { after } : undefined,
      ),
  },
};
