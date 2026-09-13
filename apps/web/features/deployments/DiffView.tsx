import { Badge, type BadgeTone } from "@/components/Badge";
import type { ConfigurationDiff } from "@/lib/api";

const changeTones: Record<string, BadgeTone> = {
  added: "emerald",
  removed: "red",
  changed: "amber",
};

/** Entity-grouped configuration diff; secret values never appear here. */
export function DiffView({ diff }: { diff: ConfigurationDiff }) {
  const changed = diff.entities.filter(
    (entity) => entity.change !== "unchanged",
  );
  if (changed.length === 0) {
    return (
      <p className="text-sm text-zinc-500">
        No configuration changes since version {diff.from}.
      </p>
    );
  }
  return (
    <ul className="space-y-2">
      {changed.map((entity) => (
        <li
          className="rounded-lg border border-zinc-800 bg-zinc-950 p-3 text-sm"
          key={`${entity.kind}-${entity.name}`}
        >
          <div className="flex items-center gap-2">
            <Badge tone={changeTones[entity.change] ?? "zinc"}>
              {entity.change}
            </Badge>
            <span className="text-zinc-200">{entity.name}</span>
            <span className="text-xs text-zinc-500">{entity.kind}</span>
          </div>
          {entity.fields && entity.fields.length > 0 ? (
            <ul className="mt-2 space-y-1 font-mono text-xs text-zinc-400">
              {entity.fields.map((field) => (
                <li key={field.field}>
                  {field.field}:{" "}
                  <span className="text-red-300">{field.from ?? "∅"}</span> →{" "}
                  <span className="text-emerald-300">{field.to ?? "∅"}</span>
                </li>
              ))}
            </ul>
          ) : null}
        </li>
      ))}
    </ul>
  );
}
