"use client";

import { Select } from "@/components/form";
import { useServices } from "@/lib/hooks";

/** Service scope picker: an entry applies to every service or exactly one. */
export function ScopeSelect({
  projectId,
  environmentId,
  value,
  onChange,
  ariaLabel = "Scope",
}: {
  projectId: string;
  environmentId: string;
  value: string;
  onChange: (serviceId: string) => void;
  ariaLabel?: string;
}) {
  const { data } = useServices(projectId, environmentId);
  return (
    <Select
      aria-label={ariaLabel}
      onChange={(event) => onChange(event.target.value)}
      value={value}
    >
      <option value="">All services</option>
      {(data?.items ?? []).map((service) => (
        <option key={service.id} value={service.id}>
          {service.name}
        </option>
      ))}
    </Select>
  );
}

/** Resolves a serviceId scope to a display name. */
export function useScopeName(projectId: string, environmentId: string) {
  const { data } = useServices(projectId, environmentId);
  return (serviceId?: string | null): string => {
    if (!serviceId) return "All services";
    return (
      data?.items.find((service) => service.id === serviceId)?.name ??
      serviceId
    );
  };
}
