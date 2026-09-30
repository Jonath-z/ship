import type { ReactNode } from "react";

export type BadgeTone =
  | "emerald"
  | "amber"
  | "red"
  | "zinc"
  | "sky";

const tones: Record<BadgeTone, string> = {
  emerald: "border-emerald-900/80 bg-emerald-950/40 text-emerald-400",
  amber: "border-amber-900/80 bg-amber-950/40 text-amber-400",
  red: "border-red-900/80 bg-red-950/40 text-red-400",
  zinc: "border-zinc-800 bg-zinc-900/60 text-zinc-400",
  sky: "border-sky-900/80 bg-sky-950/40 text-sky-400",
};

export function Badge({
  tone = "zinc",
  children,
}: {
  tone?: BadgeTone;
  children: ReactNode;
}) {
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium ${tones[tone]}`}
    >
      {children}
    </span>
  );
}
