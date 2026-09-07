"use client";

import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

/** Session query; also primes the CSRF token cache inside lib/api. */
export function useSession() {
  return useQuery({
    queryKey: ["session"],
    queryFn: api.session.get,
    staleTime: 60_000,
  });
}

export function useProjects() {
  return useQuery({
    queryKey: ["projects"],
    queryFn: () => api.projects.list({ limit: 100 }),
  });
}

export function useEnvironments(projectId?: string) {
  return useQuery({
    queryKey: ["environments", projectId],
    queryFn: () => api.environments.list(projectId!, { limit: 100 }),
    enabled: Boolean(projectId),
  });
}

export function useServices(projectId: string, environmentId: string) {
  return useQuery({
    queryKey: ["services", projectId, environmentId],
    queryFn: () => api.services.list(projectId, environmentId),
  });
}

export function useAccessories(projectId: string, environmentId: string) {
  return useQuery({
    queryKey: ["accessories", projectId, environmentId],
    queryFn: () => api.accessories.list(projectId, environmentId),
  });
}

export function useServers() {
  return useQuery({ queryKey: ["servers"], queryFn: api.servers.list });
}

export function useSshKeys() {
  return useQuery({ queryKey: ["ssh-keys"], queryFn: api.sshKeys.list });
}

export function useServerGroups(projectId: string, environmentId: string) {
  return useQuery({
    queryKey: ["server-groups", projectId, environmentId],
    queryFn: () => api.serverGroups.list(projectId, environmentId),
  });
}
