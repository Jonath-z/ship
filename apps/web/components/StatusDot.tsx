const colors: Record<string, string> = {
  connected: "bg-emerald-400",
  degraded: "bg-amber-400",
  disconnected: "bg-red-400",
  pending: "bg-zinc-500",
};

/** Small colored dot for server connection states. */
export function StatusDot({ status }: { status: string }) {
  return (
    <span className="inline-flex items-center gap-2">
      <span
        aria-hidden
        className={`h-2 w-2 rounded-full ${colors[status] ?? "bg-zinc-500"}`}
      />
      <span className="capitalize text-zinc-300">{status}</span>
    </span>
  );
}
