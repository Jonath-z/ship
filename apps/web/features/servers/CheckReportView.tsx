import { StatusDot } from "@/components/StatusDot";
import type { CheckReport } from "@/lib/api";
import { formatDateTime } from "@/lib/format";

/** Per-check pass/fail list for a server check report. */
export function CheckReportView({ report }: { report: CheckReport }) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950 p-4">
      <div className="flex items-center justify-between text-sm">
        <StatusDot status={report.status} />
        <span className="text-xs text-zinc-500">
          ran {formatDateTime(report.ranAt)}
        </span>
      </div>
      <ul className="mt-3 space-y-2">
        {report.checks.map((check) => (
          <li className="flex items-start gap-2 text-sm" key={check.name}>
            <span
              aria-hidden
              className={`mt-0.5 ${check.ok ? "text-emerald-400" : "text-red-400"}`}
            >
              {check.ok ? "✓" : "✗"}
            </span>
            <div>
              <span className="text-zinc-200">{check.name}</span>
              {check.detail ? (
                <p className="text-xs text-zinc-500">{check.detail}</p>
              ) : null}
            </div>
          </li>
        ))}
        {report.checks.length === 0 ? (
          <li className="text-sm text-zinc-500">No checks reported.</li>
        ) : null}
      </ul>
    </div>
  );
}
