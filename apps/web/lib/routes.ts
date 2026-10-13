/**
 * Typed URL builders for the static-export route map. Dynamic path segments
 * became query parameters so every page can be prerendered:
 *
 *   /env?project=X&env=Y                              environment overview
 *   /env/<section>?project=X&env=Y                    environment sections
 *   /env/applications/service?...&service=Z           application detail
 *   /env/deployments/detail?...&deployment=Z          deployment detail
 *   /servers/detail?server=X                          server detail
 */

function envQuery(projectId: string, environmentId: string): string {
  return `project=${encodeURIComponent(projectId)}&env=${encodeURIComponent(environmentId)}`;
}

/**
 * Environment-scoped route. `section` is the path under /env, e.g.
 * "/applications" or "/deployments"; omit it for the overview.
 */
export function envRoute(
  projectId: string,
  environmentId: string,
  section = "",
): string {
  return `/env${section}?${envQuery(projectId, environmentId)}`;
}

export function serviceRoute(
  projectId: string,
  environmentId: string,
  serviceId: string,
): string {
  return `/env/applications/service?${envQuery(projectId, environmentId)}&service=${encodeURIComponent(serviceId)}`;
}

export function deploymentRoute(
  projectId: string,
  environmentId: string,
  deploymentId: string,
): string {
  return `/env/deployments/detail?${envQuery(projectId, environmentId)}&deployment=${encodeURIComponent(deploymentId)}`;
}

export function serverRoute(serverId: string): string {
  return `/servers/detail?server=${encodeURIComponent(serverId)}`;
}
