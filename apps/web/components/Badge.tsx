import type { ReactNode } from "react";

export type BadgeTone =
  | "emerald"
  | "amber"
  | "red"
  | "zinc"
  | "sky";

const tones: Record<BadgeTone, string> = {
  emerald: "border-emerald-900 bg-emerald-950/60 text-emerald-300",
  amber: "border-amber-900 bg-amber-950/60 text-amber-300",
  red: "border-red-900 bg-red-950/60 text-red-300",
  zinc: "border-zinc-700 bg-zinc-900 text-zinc-300",
  sky: "border-sky-900 bg-sky-950/60 text-sky-300",
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
