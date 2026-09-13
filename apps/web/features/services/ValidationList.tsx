import { Badge } from "@/components/Badge";
import type { Violation } from "@/lib/api";

/** Configuration validation results with block/warn severity badges. */
export function ValidationList({ violations }: { violations: Violation[] }) {
  if (violations?.length === 0 || !violations) {
    return (
      <p className="text-sm text-emerald-400">Configuration is valid.</p>
    );
  }
  return (
    <ul className="space-y-2">
      {violations.map((violation, index) => (
        <li
          className="flex items-start gap-3 rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-sm"
          key={`${violation.code}-${violation.entityName}-${index}`}
        >
          <Badge tone={violation.severity === "block" ? "red" : "amber"}>
            {violation.severity}
          </Badge>
          <div>
            <p className="text-zinc-200">{violation.message}</p>
            <p className="mt-0.5 text-xs text-zinc-500">
              {violation.entityType} · {violation.entityName} ·{" "}
              {violation.code}
            </p>
          </div>
        </li>
      ))}
    </ul>
  );
}
