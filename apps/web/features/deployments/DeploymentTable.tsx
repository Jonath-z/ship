import Link from "next/link";
import type { Deployment } from "@/lib/api";
import { relativeTime } from "@/lib/format";
import { StatusBadge } from "@/features/deployments/StatusBadge";

/**
 * Compact deployment history table. `deploymentHref` builds the detail link
 * for a deployment id; `resolveServiceName` covers records created before
 * the API started denormalizing serviceName.
 */
export function DeploymentTable({
  deployments,
  deploymentHref,
  resolveServiceName,
  showService = true,
}: {
  deployments: Deployment[];
  deploymentHref: (deploymentId: string) => string;
  resolveServiceName?: (serviceId: string) => string | undefined;
  showService?: boolean;
}) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead className="text-zinc-500">
          <tr>
            <th className="pb-3 pr-4">Status</th>
            {showService ? <th className="pb-3 pr-4">Service</th> : null}
            <th className="pb-3 pr-4">Image</th>
            <th className="pb-3">Created</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-zinc-800">
          {deployments.map((deployment) => (
            <tr key={deployment.id}>
              <td className="py-3 pr-4">
                <Link href={deploymentHref(deployment.id)}>
                  <StatusBadge status={deployment.status} />
                </Link>
              </td>
              {showService ? (
                <td className="py-3 pr-4">
                  <Link
                    className="font-medium text-zinc-200 hover:underline"
                    href={deploymentHref(deployment.id)}
                  >
                    {deployment.serviceName ??
                      resolveServiceName?.(deployment.serviceId) ??
                      deployment.serviceId}
                  </Link>
                </td>
              ) : null}
              <td className="py-3 pr-4 font-mono text-xs text-zinc-400">
                {deployment.image ?? "—"}
              </td>
              <td className="py-3 text-zinc-500">
                {relativeTime(deployment.createdAt)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
